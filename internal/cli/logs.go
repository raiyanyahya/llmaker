package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

func newLogsCmd(app *App) *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:     "logs <name>",
		Short:   "Stream an instance's container logs",
		GroupID: groupFleet,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd.Context(), app, args[0], follow)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	return cmd
}

func runLogs(ctx context.Context, app *App, name string, follow bool) error {
	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	in, err := app.mustGet(ctx, rt, name)
	if err != nil {
		return err
	}

	rc, err := rt.Logs(ctx, in.Name, follow)
	if err != nil {
		return err
	}
	defer rc.Close()

	// Copy until EOF or context cancellation (Ctrl-C ends a follow).
	_, err = io.Copy(app.IO.Out, rc)
	if err != nil && ctx.Err() != nil {
		return nil // cancelled by the user; not an error
	}
	return err
}
