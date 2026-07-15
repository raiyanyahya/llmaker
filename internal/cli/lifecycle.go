package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/engine"
)

func newStopCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "stop <name>...",
		Short:   "Stop running instances",
		GroupID: groupLifecycle,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLifecycle(cmd.Context(), app, args, "Stopping", func(ctx context.Context, rt engine.Runtime, in engine.Instance) error {
				return rt.Stop(ctx, in.Name, engine.DefaultStopTimeout)
			})
		},
	}
}

func newStartCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "start <name>...",
		Short:   "Start stopped instances",
		GroupID: groupLifecycle,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLifecycle(cmd.Context(), app, args, "Starting", func(ctx context.Context, rt engine.Runtime, in engine.Instance) error {
				if err := rt.Start(ctx, in.Name); err != nil {
					return err
				}
				warnIfPublicNoKey(app, in)
				return nil
			})
		},
	}
}

func newRestartCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "restart <name>...",
		Short:   "Restart instances (stop then start)",
		GroupID: groupLifecycle,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLifecycle(cmd.Context(), app, args, "Restarting", func(ctx context.Context, rt engine.Runtime, in engine.Instance) error {
				if in.IsRunning() {
					if err := rt.Stop(ctx, in.Name, engine.DefaultStopTimeout); err != nil {
						return err
					}
				}
				if err := rt.Start(ctx, in.Name); err != nil {
					return err
				}
				warnIfPublicNoKey(app, in)
				return nil
			})
		},
	}
}

func newRmCmd(app *App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "rm <name>...",
		Aliases: []string{"remove"},
		Short:   "Remove instances and their model volumes",
		GroupID: groupLifecycle,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRm(cmd.Context(), app, args, force)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "remove even if running (stops first)")
	return cmd
}

// lifecycleFn performs one verb against one instance.
type lifecycleFn func(ctx context.Context, rt engine.Runtime, in engine.Instance) error

// runLifecycle applies fn to each named instance, continuing past individual
// failures and returning an aggregate error so a partial failure is still
// reported with a non-zero exit.
func runLifecycle(ctx context.Context, app *App, names []string, verb string, fn lifecycleFn) error {
	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	var errs []error
	for _, name := range names {
		in, err := app.mustGet(ctx, rt, name)
		if err != nil {
			app.IO.Println(app.IO.Theme.FailLine(err.Error()))
			errs = append(errs, err)
			continue
		}
		if err := app.step(fmt.Sprintf("%s %s", verb, in.Name), func() error {
			return fn(ctx, rt, in)
		}); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrs(errs)
}

func runRm(ctx context.Context, app *App, names []string, force bool) error {
	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	var errs []error
	for _, name := range names {
		in, err := app.mustGet(ctx, rt, name)
		if err != nil {
			app.IO.Println(app.IO.Theme.FailLine(err.Error()))
			errs = append(errs, err)
			continue
		}
		if in.IsRunning() && !force {
			msg := fmt.Errorf("instance %q is running; stop it first or pass --force", in.Name)
			app.IO.Println(app.IO.Theme.FailLine(msg.Error()))
			errs = append(errs, msg)
			continue
		}
		if err := app.step(fmt.Sprintf("Removing %s", in.Name), func() error {
			return rt.Remove(ctx, in.Name, force)
		}); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrs(errs)
}

func joinErrs(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("%d operations failed: %w", len(errs), errors.Join(errs...))
}
