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
			if !cur.IsRunning() {
				if err := app.step("Starting "+spec.Name, func() error { return rt.Start(ctx, spec.Name) }); err != nil {
					errs = append(errs, err)
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
		// When the file names itself, prune is scoped to that stack so applying
		// one stack never deletes another's resources. An unnamed file prunes the
		// whole managed fleet (the original, documented behavior).
		stackName := cfg.StackName()
		inStack := func(resourceStack string) bool { return stackName == "" || resourceStack == stackName }

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
			}
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
