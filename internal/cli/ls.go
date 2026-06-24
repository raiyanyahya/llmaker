package cli

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/ui"
)

func newLsCmd(app *App) *cobra.Command {
	var asJSON bool
	var quiet bool
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list", "ps"},
		Short:   "List the instance fleet",
		GroupID: groupFleet,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLs(cmd.Context(), app, lsOptions{json: asJSON, quiet: quiet})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "only print instance names")
	return cmd
}

type lsOptions struct {
	json  bool
	quiet bool
}

// instanceJSON is the stable, documented JSON shape for `ls --json`.
type instanceJSON struct {
	Name    string `json:"name"`
	Backend string `json:"backend"`
	Model   string `json:"model"`
	State   string `json:"state"`
	Health  string `json:"health"`
	Port    int    `json:"port"`
	URL     string `json:"url"`
	Image   string `json:"image"`
	Runtime string `json:"runtime"`
}

func runLs(ctx context.Context, app *App, opts lsOptions) error {
	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	ins, err := app.listFleet(ctx, rt, !opts.quiet)
	if err != nil {
		return err
	}

	io := app.IO
	t := io.Theme

	if opts.json {
		out := make([]instanceJSON, 0, len(ins))
		for _, in := range ins {
			out = append(out, instanceJSON{
				Name: in.Name, Backend: string(in.Backend), Model: in.Model,
				State: string(in.State), Health: healthLabel(in.Health),
				Port: in.Port, URL: in.URL(), Image: in.Image, Runtime: string(in.Runtime),
			})
		}
		enc := json.NewEncoder(io.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	if opts.quiet {
		for _, in := range ins {
			io.Println(in.Name)
		}
		return nil
	}

	if len(ins) == 0 {
		io.Println(t.Muted.Render("No instances yet. Start one with ") + t.Accent.Render("llmaker up") + t.Muted.Render("."))
		return nil
	}

	tbl := t.NewTable("", "NAME", "BACKEND", "MODEL", "STATE", "HEALTH", "PORT", "URL", "UPTIME")
	for _, in := range ins {
		dot := t.Dot(healthLevel(in.Health))
		tbl.Row(
			dot,
			in.Name,
			string(in.Backend),
			ui.Truncate(in.Model, 28),
			t.Badge(string(in.State), stateLevel(in.State)),
			healthLabel(in.Health),
			itoa(in.Port),
			in.URL(),
			ui.HumanDuration(in.Uptime()),
		)
	}
	io.Println(tbl.Render())
	return nil
}

func itoa(i int) string {
	if i == 0 {
		return "-"
	}
	return strconv.Itoa(i)
}
