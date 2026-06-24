package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/facade"
	"github.com/raiyanyahya/llmaker/internal/ui"
)

func newPullCmd(app *App) *cobra.Command {
	var on string
	var setDefault bool
	cmd := &cobra.Command{
		Use:     "pull <model>",
		Short:   "Download a model into an instance",
		GroupID: groupModels,
		Args:    cobra.ExactArgs(1),
		Example: "  llmaker pull llama3:70b --on brave-llama\n  llmaker pull qwen2.5:7b --default",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd.Context(), app, pullOptions{model: args[0], on: on, setDefault: setDefault})
		},
	}
	cmd.Flags().StringVar(&on, "on", "", "target instance (defaults to the only running one)")
	cmd.Flags().BoolVar(&setDefault, "default", false, "set as the instance default after pulling")
	return cmd
}

type pullOptions struct {
	model      string
	on         string
	setDefault bool
}

func runPull(ctx context.Context, app *App, opts pullOptions) error {
	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	in, err := app.resolveTarget(ctx, rt, opts.on)
	if err != nil {
		return err
	}
	if err := requireRunning(in); err != nil {
		return err
	}

	io := app.IO
	t := io.Theme
	io.Println(t.InfoLine(fmt.Sprintf("Pulling %s into %s", t.Value.Render(opts.model), t.Accent.Render(in.Name))))

	if err := pullModel(ctx, app, in.URL(), opts.model); err != nil {
		return err
	}

	if opts.setDefault {
		if err := app.step("Setting as default model", func() error {
			return app.Facade.SetDefault(ctx, in.URL(), opts.model)
		}); err != nil {
			return err
		}
	}
	return nil
}

// pullModel streams a model download, rendering a live progress bar that the
// CLI drives from the facade's NDJSON progress events.
func pullModel(ctx context.Context, app *App, baseURL, model string) error {
	t := app.IO.Theme
	bar := t.NewProgressBar(app.IO.Out, ui.Truncate(model, 24))

	err := app.Facade.Pull(ctx, baseURL, model, func(ev facade.PullEvent) {
		bar.Update(ev.Fraction(), pullSuffix(ev))
	})
	if err != nil {
		bar.Finish(t.FailLine("pull failed: " + err.Error()))
		return err
	}
	bar.Finish(t.SuccessLine("Pulled " + t.Value.Render(model)))
	return nil
}

func pullSuffix(ev facade.PullEvent) string {
	if ev.Total > 0 {
		return fmt.Sprintf("%s / %s", ui.HumanBytes(ev.Completed), ui.HumanBytes(ev.Total))
	}
	if ev.Status != "" {
		return ev.Status
	}
	return ""
}
