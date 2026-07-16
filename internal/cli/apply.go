package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/config"
	"github.com/raiyanyahya/llmaker/internal/engine"
)

const applyHealthTimeout = 90 * time.Second

func newApplyCmd(app *App) *cobra.Command {
	var file string
	var prune bool
	var noPull bool
	cmd := &cobra.Command{
		Use:     "apply",
		Short:   "Reconcile the fleet to a declarative llm.yaml",
		GroupID: groupFleet,
		Args:    cobra.NoArgs,
		Long: `Bring the running fleet in line with a declarative file (compose-like, but
LLM-aware). Missing instances are created; existing ones are left in place.
Pass --prune to also remove managed instances that aren't in the file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(cmd.Context(), app, applyOptions{file: file, prune: prune, noPull: noPull})
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "llm.yaml", "path to the fleet file")
	cmd.Flags().BoolVar(&prune, "prune", false, "remove managed instances not present in the file")
	cmd.Flags().BoolVar(&noPull, "no-pull", false, "don't preload models")
	return cmd
}

type applyOptions struct {
	file   string
	prune  bool
	noPull bool
}

func runApply(ctx context.Context, app *App, opts applyOptions) error {
	cfg, err := config.Load(opts.file)
	if err != nil {
		return fmt.Errorf("read %s: %w", opts.file, err)
	}
	specs, err := cfg.ToSpecs()
	if err != nil {
		return err
	}
	serviceSpecs, err := cfg.ToServiceSpecs()
	if err != nil {
		return err
	}
	if len(specs) == 0 && len(serviceSpecs) == 0 {
		return fmt.Errorf("%s declares no instances or services", opts.file)
	}

	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	existing, err := rt.List(ctx)
	if err != nil {
		return err
	}
	existingServices, err := rt.ListServices(ctx)
	if err != nil {
		return err
	}
	byName := make(map[string]engine.Instance, len(existing))
	for _, in := range existing {
		byName[in.Name] = in
	}
	used := usedPorts(existing, existingServices)

	// When the file names itself, prune (and GPU admission below) is scoped to
	// that stack so applying one stack never touches another's resources. An
	// unnamed file covers the whole managed fleet (the original, documented
	// behavior).
	stackName := cfg.StackName()
	inStack := func(resourceStack string) bool { return stackName == "" || resourceStack == stackName }

	// Gang admission for GPUs: resolve every new member's request against one
	// shared allocator BEFORE anything is provisioned, so a stack whose GPU
	// demand doesn't fit is rejected as a unit instead of coming up partially.
	// Existing instances keep the reservations recorded on their labels —
	// except ones --prune is about to remove, whose devices this run frees
	// (otherwise renaming a GPU-holding instance could never converge).
	var doomed map[string]bool
	if opts.prune {
		declared := make(map[string]bool, len(specs))
		for _, s := range specs {
			declared[s.Name] = true
		}
		doomed = map[string]bool{}
		for _, in := range existing {
			if !declared[in.Name] && inStack(in.Stack) {
				doomed[in.Name] = true
			}
		}
	}
	if err := admitGPUs(app, specs, existing, doomed); err != nil {
		return fmt.Errorf("GPU admission for %s failed (nothing was created): %w", opts.file, err)
	}

	io := app.IO
	t := io.Theme
	var errs []error

	// Services come up first: an LLM app declared in the same file may connect
	// to a vector DB or cache at boot, and they reach each other by name on the
	// shared network.
	desiredSvc, svcCreated, svcUnchanged := applyServices(ctx, app, rt, serviceSpecs, existingServices, used, &errs)

	desired := make(map[string]bool, len(specs))
	var created, unchanged int

	for i := range specs {
		spec := &specs[i]
		desired[spec.Name] = true

		if cur, ok := byName[spec.Name]; ok {
			unchanged++
			warnSpecDrift(app, spec.Name, cur.Network, spec.Network, cur.GPUs, spec.GPU || spec.GPUs != "")
			if !cur.IsRunning() {
				if err := app.step("Starting "+spec.Name, func() error { return rt.Start(ctx, spec.Name) }); err != nil {
					errs = append(errs, err)
				} else {
					warnIfPublicNoKey(app, cur)
				}
			} else {
				io.Println(t.Muted.Render("• " + spec.Name + " already running"))
			}
			continue
		}

		if spec.Port == 0 {
			p, perr := engine.AllocatePort(used)
			if perr != nil {
				errs = append(errs, perr)
				continue
			}
			spec.Port = p
		}
		used[spec.Port] = true

		io.Println(t.Heading("Creating " + spec.Name))
		if _, perr := provision(ctx, app, rt, *spec, applyHealthTimeout, !opts.noPull); perr != nil {
			errs = append(errs, perr)
			continue
		}
		created++
	}

	var pruned int
	if opts.prune {
		sawGroup := false
		for _, in := range existing {
			if desired[in.Name] || !inStack(in.Stack) {
				continue
			}
			if err := app.step("Removing "+in.Name+" (pruned)", func() error {
				return rt.Remove(ctx, in.Name, true)
			}); err != nil {
				errs = append(errs, err)
			} else {
				pruned++
				sawGroup = sawGroup || in.Network != ""
			}
		}
		for _, svc := range existingServices {
			if desiredSvc[svc.Name] || !inStack(svc.Stack) {
				continue
			}
			if err := app.step("Removing service "+svc.Name+" (pruned)", func() error {
				return rt.Remove(ctx, svc.Name, true)
			}); err != nil {
				errs = append(errs, err)
			} else {
				pruned++
				sawGroup = sawGroup || svc.Network != ""
			}
		}
		// Only sweep when a grouped resource was actually removed — the
		// common shared-network prune has nothing to collect.
		if sawGroup {
			gcNetworks(ctx, app, rt)
		}
	}

	io.Println()
	summary := fmt.Sprintf("Applied %s: %d created, %d unchanged",
		opts.file, created+svcCreated, unchanged+svcUnchanged)
	if opts.prune {
		summary += fmt.Sprintf(", %d pruned", pruned)
	}
	io.Println(t.SuccessLine(summary))
	return joinErrs(errs)
}

// warnSpecDrift surfaces declared settings apply cannot reconcile on an
// existing container: networks and GPU reservations are fixed at creation, so
// a drifted member keeps its current values until it is removed and
// re-applied. Staying silent would let a declared boundary quietly not exist.
func warnSpecDrift(app *App, name, curNet, wantNet, curGPUs string, wantsGPU bool) {
	t := app.IO.Theme
	display := func(n string) string {
		if n == "" {
			return "the shared network"
		}
		return "network " + n
	}
	if curNet != wantNet {
		app.IO.Println(t.WarnLine(fmt.Sprintf(
			"%s already exists on %s but the file declares %s — apply doesn't move existing containers; rm it and re-apply to change its network.",
			name, display(curNet), display(wantNet))))
	}
	if wantsGPU && curGPUs == "" {
		app.IO.Println(t.WarnLine(fmt.Sprintf(
			"%s already exists without a GPU reservation — the declared gpu/gpus setting isn't applied to existing containers; rm it and re-apply.",
			name)))
	} else if !wantsGPU && curGPUs != "" {
		app.IO.Println(t.WarnLine(fmt.Sprintf(
			"%s still holds GPU reservation %q that the file no longer declares — rm it and re-apply to release the devices.",
			name, curGPUs)))
	}
}

// applyServices reconciles the declared services: existing ones are started if
// stopped, new ones get host ports allocated and are provisioned. It returns the
// desired-name set (for pruning) and create/unchanged counts.
func applyServices(ctx context.Context, app *App, rt engine.Runtime, specs []engine.ServiceSpec, existing []engine.Service, used map[int]bool, errs *[]error) (map[string]bool, int, int) {
	io := app.IO
	t := io.Theme
	byName := make(map[string]engine.Service, len(existing))
	for _, s := range existing {
		byName[s.Name] = s
	}

	desired := make(map[string]bool, len(specs))
	var created, unchanged int

	for i := range specs {
		spec := &specs[i]
		desired[spec.Name] = true

		if cur, ok := byName[spec.Name]; ok {
			unchanged++
			warnSpecDrift(app, "service "+spec.Name, cur.Network, spec.Network, "", false)
			if !cur.IsRunning() {
				if err := app.step("Starting service "+spec.Name, func() error { return rt.Start(ctx, spec.Name) }); err != nil {
					*errs = append(*errs, err)
				}
			} else {
				io.Println(t.Muted.Render("• service " + spec.Name + " already running"))
			}
			continue
		}

		if err := allocateBindings(spec.Ports, used); err != nil {
			*errs = append(*errs, fmt.Errorf("service %q: %w", spec.Name, err))
			continue
		}

		io.Println(t.Heading("Creating service " + spec.Name))
		if err := provisionService(ctx, app, rt, *spec, serviceHealthTimeout, true); err != nil {
			*errs = append(*errs, err)
			continue
		}
		created++
	}
	return desired, created, unchanged
}

// allocateBindings fills any port binding left without a host port (Host == 0),
// reserving each in used so nothing collides.
func allocateBindings(ports []engine.PortBinding, used map[int]bool) error {
	for i := range ports {
		if ports[i].Host > 0 {
			if used[ports[i].Host] || !engine.PortAvailable(ports[i].Host) {
				return fmt.Errorf("port %d is not available", ports[i].Host)
			}
		} else {
			p, err := engine.AllocatePort(used)
			if err != nil {
				return err
			}
			ports[i].Host = p
		}
		used[ports[i].Host] = true
	}
	return nil
}
