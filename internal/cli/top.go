package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/tui"
)

func newTopCmd(app *App) *cobra.Command {
	var interval time.Duration
	cmd := &cobra.Command{
		Use:     "top",
		Short:   "Live dashboard of load across the fleet",
		GroupID: groupFleet,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTop(cmd.Context(), app, interval)
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "refresh interval")
	return cmd
}

func runTop(ctx context.Context, app *App, interval time.Duration) error {
	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// The dashboard is an interactive full-screen TUI. When output isn't a
	// terminal (piped, CI), fall back to a one-shot fleet snapshot instead.
	if !app.IO.IsInteractive() {
		return runLs(ctx, app, lsOptions{})
	}
	if interval < 250*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	return tui.Run(ctx, rt, app.Facade, app.IO.Theme, interval)
}
