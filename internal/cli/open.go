package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newOpenCmd(app *App) *cobra.Command {
	var printOnly bool
	cmd := &cobra.Command{
		Use:     "open <name>",
		Short:   "Open an instance's web UI in the browser",
		GroupID: groupFleet,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOpen(cmd.Context(), app, args[0], printOnly)
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the URL instead of opening a browser")
	return cmd
}

func runOpen(ctx context.Context, app *App, name string, printOnly bool) error {
	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	in, err := app.mustGet(ctx, rt, name)
	if err != nil {
		return err
	}
	url := in.URL()

	if printOnly {
		app.IO.Println(url)
		return nil
	}

	open := app.OpenURL
	if open == nil {
		open = openBrowser
	}
	if err := open(url); err != nil {
		// Fall back to just printing the URL — still useful.
		app.IO.Println(app.IO.Theme.WarnLine(err.Error()))
		app.IO.Println(url)
		return nil
	}
	app.IO.Println(app.IO.Theme.InfoLine("Opening " + app.IO.Theme.Accent.Render(url)))
	return nil
}
