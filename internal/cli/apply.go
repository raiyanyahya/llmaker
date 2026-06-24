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

	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	existing, err := rt.List(ctx)
	if err != nil {
		return err
	}
	byName := make(map[string]engine.Instance, len(existing))
	for _, in := range existing {
		byName[in.Name] = in
	}
	used := engine.UsedPorts(existing)

	io := app.IO
	t := io.Theme

	desired := make(map[string]bool, len(specs))
	var created, unchanged int
	var errs []error

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
		for _, in := range existing {
			if desired[in.Name] {
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
	}

	io.Println()
	summary := fmt.Sprintf("Applied %s: %d created, %d unchanged", opts.file, created, unchanged)
	if opts.prune {
		summary += fmt.Sprintf(", %d pruned", pruned)
	}
	io.Println(t.SuccessLine(summary))
	return joinErrs(errs)
}
