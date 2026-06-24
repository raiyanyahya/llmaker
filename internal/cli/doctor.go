package cli

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/engine"
	"github.com/raiyanyahya/llmaker/internal/ui"
)

func newDoctorCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "doctor",
		Short:   "Check your environment for running LLM instances",
		GroupID: groupAdvanced,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.Context(), app)
		},
	}
}

// check is one diagnostic line.
type check struct {
	name   string
	level  ui.Level
	detail string
}

func runDoctor(ctx context.Context, app *App) error {
	io := app.IO
	t := io.Theme
	host := app.Host

	io.Println(t.Heading("Environment"))
	io.Println(t.KeyValues([][2]string{
		{"OS / Arch", host.OS + "/" + host.Arch},
		{"CPU cores", strings.TrimSpace(itoa(host.CPUs))},
		{"Total RAM", ramString(host)},
		{"Default limits", ramDefaultString(host)},
	}))
	io.Println()

	var checks []check

	// Docker daemon.
	checks = append(checks, dockerCheck(ctx, app))

	// GPU.
	checks = append(checks, gpuCheck())

	// macOS + Docker caveat (plan §7).
	if host.IsMac() {
		checks = append(checks, check{
			name:   "Apple Silicon",
			level:  ui.LevelWarn,
			detail: "Docker can't pass through the Apple GPU; containers run CPU-only. Native Metal mode is planned.",
		})
	}

	io.Println(t.Heading("Checks"))
	worst := ui.LevelOK
	for _, c := range checks {
		io.Printf("  %s %s  %s\n", t.Dot(c.level), padName(c.name), t.Muted.Render(c.detail))
		if c.level > worst && c.level != ui.LevelMuted {
			worst = c.level
		}
	}
	io.Println()

	switch {
	case worst >= ui.LevelError:
		io.Println(t.FailLine("Some checks failed — `llmaker up` may not work until they're resolved."))
	case worst == ui.LevelWarn:
		io.Println(t.WarnLine("Ready, with caveats noted above."))
	default:
		io.Println(t.SuccessLine("All systems go. Try `llmaker up`."))
	}
	return nil
}

func dockerCheck(ctx context.Context, app *App) check {
	if app.NewRuntime == nil {
		return check{"Docker", ui.LevelError, "no runtime configured"}
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rt, err := app.NewRuntime(cctx)
	if err != nil {
		return check{"Docker", ui.LevelError, "cannot connect: " + err.Error()}
	}
	defer rt.Close()
	if err := rt.Ping(cctx); err != nil {
		return check{"Docker", ui.LevelError, "daemon not responding: " + err.Error()}
	}
	return check{"Docker", ui.LevelOK, "daemon reachable"}
}

func gpuCheck() check {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return check{"GPU", ui.LevelMuted, "no NVIDIA GPU detected (CPU inference)"}
	}
	out, err := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader").Output()
	if err != nil {
		return check{"GPU", ui.LevelWarn, "nvidia-smi present but failed to query"}
	}
	names := strings.Fields(strings.ReplaceAll(strings.TrimSpace(string(out)), "\n", ", "))
	return check{"GPU", ui.LevelOK, "NVIDIA detected: " + strings.Join(names, " ")}
}

func ramString(h engine.HostInfo) string {
	if h.MemoryBytes <= 0 {
		return "unknown"
	}
	return ui.HumanBytes(h.MemoryBytes)
}

func ramDefaultString(h engine.HostInfo) string {
	return engine.FormatSize(h.DefaultMemoryBytes()) + " memory · " +
		strings.TrimSpace(ftoa(h.DefaultCPUs())) + " cpus"
}

func padName(s string) string {
	const w = 16
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
