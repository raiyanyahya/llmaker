package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/engine"
	"github.com/raiyanyahya/llmaker/internal/facade"
	"github.com/raiyanyahya/llmaker/internal/ui"
)

func newStatusCmd(app *App) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:     "status <name>",
		Short:   "Show detailed status for an instance",
		GroupID: groupFleet,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd.Context(), app, args[0], asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output the raw status as JSON")
	return cmd
}

func runStatus(ctx context.Context, app *App, name string, asJSON bool) error {
	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	in, err := app.mustGet(ctx, rt, name)
	if err != nil {
		return err
	}

	io := app.IO
	t := io.Theme

	if !in.IsRunning() {
		if asJSON {
			return writeJSON(io.Out, map[string]any{"name": in.Name, "state": in.State, "running": false})
		}
		io.Println(t.Heading(in.Name) + "  " + t.Badge(string(in.State), stateLevel(in.State)))
		io.Println(t.Muted.Render("Instance is not running. Start it with ") + t.Accent.Render("llmaker start "+in.Name))
		return nil
	}

	st, err := app.Facade.Status(ctx, in.URL())
	if err != nil {
		if asJSON {
			return writeJSON(io.Out, map[string]any{"name": in.Name, "state": in.State, "error": err.Error()})
		}
		io.Println(t.Heading(in.Name) + "  " + t.Badge("starting", ui.LevelWarn))
		io.Println(t.Muted.Render("Facade not responding yet: ") + err.Error())
		return nil
	}

	if asJSON {
		return writeJSON(io.Out, st)
	}
	renderStatus(io, in, st)
	return nil
}

func renderStatus(io *ui.IOStreams, in engine.Instance, st *facade.Status) {
	t := io.Theme

	io.Println(t.Heading(in.Name) + "  " + t.Dot(ui.LevelOK) + " " + t.Success.Render("healthy"))
	io.Println()

	uptime := time.Duration(st.Instance.UptimeSeconds) * time.Second
	io.Println(t.Card("Instance", [][2]string{
		{"backend", string(in.Backend)},
		{"default model", orDash(st.Models.Default)},
		{"version", orDash(st.Instance.Version)},
		{"uptime", ui.HumanDuration(uptime)},
		{"endpoint", in.URL() + "/v1"},
		{"web UI", in.URL()},
	}, false))
	io.Println()

	// System gauges.
	io.Println(t.Heading("System"))
	io.Println("  " + t.Gauge("CPU", st.System.CPUPercent/100, 24))
	memFrac := -1.0
	if st.System.MemoryTotal > 0 {
		memFrac = float64(st.System.MemoryUsed) / float64(st.System.MemoryTotal)
	}
	io.Println("  " + t.Gauge("RAM", memFrac, 24) + "  " +
		t.Muted.Render(fmt.Sprintf("%s / %s", ui.HumanBytes(st.System.MemoryUsed), ui.HumanBytes(st.System.MemoryTotal))))
	for i, g := range st.System.GPUs {
		label := fmt.Sprintf("GPU%d", i)
		io.Println("  " + t.Gauge(label, g.Utilization/100, 24) + "  " +
			t.Muted.Render(fmt.Sprintf("%s · VRAM %s / %s", ui.Truncate(g.Name, 20), ui.HumanBytes(g.MemoryUsed), ui.HumanBytes(g.MemoryTotal))))
	}
	if len(st.System.GPUs) == 0 {
		io.Println("  " + t.Muted.Render("GPU    none detected (CPU inference)"))
	}
	io.Println()

	// Running models.
	io.Println(t.Heading("Loaded models"))
	if len(st.Models.Running) == 0 {
		io.Println("  " + t.Muted.Render("none loaded (will load on first request)"))
	} else {
		tbl := t.NewTable("MODEL", "SIZE", "VRAM")
		for _, m := range st.Models.Running {
			tbl.Row(modelLabel(m.Name, st.Models.Default, t), ui.HumanBytes(m.Size), ui.HumanBytes(m.VRAM))
		}
		io.Println(indent(tbl.Render()))
	}
	io.Println()

	// Installed models.
	io.Println(t.Heading("Installed models"))
	if len(st.Models.Installed) == 0 {
		io.Println("  " + t.Muted.Render("none — pull one with `llmaker pull <model> --on "+in.Name+"`"))
	} else {
		tbl := t.NewTable("MODEL", "SIZE")
		for _, m := range st.Models.Installed {
			tbl.Row(modelLabel(m.Name, st.Models.Default, t), ui.HumanBytes(m.Size))
		}
		io.Println(indent(tbl.Render()))
	}
}

// modelLabel marks the default model with a star.
func modelLabel(name, def string, t *ui.Theme) string {
	if name == def && def != "" {
		return t.Warning.Render("★ ") + name
	}
	return "  " + name
}

func indent(s string) string {
	out := "  "
	for _, r := range s {
		out += string(r)
		if r == '\n' {
			out += "  "
		}
	}
	return out
}

func writeJSON(w interface{ Write([]byte) (int, error) }, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
