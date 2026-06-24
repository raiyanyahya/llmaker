package cli

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/backend"
	"github.com/raiyanyahya/llmaker/internal/engine"
)

func newUpCmd(app *App) *cobra.Command {
	var opts upOptions
	var forceWizard bool

	cmd := &cobra.Command{
		Use:     "up",
		Short:   "Create and start an LLM instance",
		GroupID: groupLifecycle,
		Args:    cobra.NoArgs,
		Example: "  llmaker up --backend ollama --model llama3:8b --memory 8g --cpus 4 --gpu\n  llmaker up   # interactive wizard",
		RunE: func(cmd *cobra.Command, args []string) error {
			useWizard := forceWizard || (!coreFlagsChanged(cmd) && app.IO.IsInteractive())
			return runUp(cmd.Context(), app, opts, useWizard)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.name, "name", "", "instance name (default: a generated friendly name)")
	f.StringVar(&opts.backendName, "backend", "ollama", "inference backend (ollama, llamacpp)")
	f.StringVar(&opts.model, "model", "", "model to preload (default: the backend's default)")
	f.StringVar(&opts.memory, "memory", "", "memory limit, e.g. 8g (default: derived from host)")
	f.Float64Var(&opts.cpus, "cpus", 0, "CPU quota (default: derived from host)")
	f.BoolVar(&opts.gpu, "gpu", false, "reserve host GPU(s) (NVIDIA Container Toolkit)")
	f.IntVar(&opts.port, "port", 0, "host port for the facade (default: auto-assigned)")
	f.StringVar(&opts.host, "host", "127.0.0.1", "host address to bind the facade to")
	f.StringVar(&opts.image, "image", "", "override the backend image (advanced)")
	f.StringVar(&opts.apiKey, "api-key", "", "require this bearer token on the facade")
	f.StringVar(&opts.keepAlive, "keep-alive", "", "how long to keep models in (V)RAM, e.g. 10m")
	f.StringVar(&opts.cors, "cors", "", "allowed CORS origins (default: *)")
	f.BoolVar(&opts.noPull, "no-pull", false, "don't preload the model after boot")
	f.DurationVar(&opts.healthTimeout, "timeout", 90*time.Second, "how long to wait for the facade to become healthy")
	f.BoolVar(&forceWizard, "wizard", false, "force the interactive wizard")

	return cmd
}

type upOptions struct {
	name          string
	backendName   string
	model         string
	memory        string
	cpus          float64
	gpu           bool
	port          int
	host          string
	image         string
	apiKey        string
	keepAlive     string
	cors          string
	noPull        bool
	healthTimeout time.Duration
}

// coreFlagsChanged reports whether the user set any instance-shaping flag, which
// is how `up` decides between running the wizard and honoring explicit flags.
func coreFlagsChanged(cmd *cobra.Command) bool {
	for _, n := range []string{"name", "backend", "model", "memory", "cpus", "gpu", "port", "host", "image", "api-key", "keep-alive", "cors"} {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}

func runUp(ctx context.Context, app *App, opts upOptions, useWizard bool) error {
	if useWizard {
		if err := runWizard(app, &opts); err != nil {
			return err
		}
	}

	b, err := backend.Get(opts.backendName)
	if err != nil {
		return err
	}
	image := firstNonEmpty(opts.image, b.Image)
	model := firstNonEmpty(opts.model, b.DefaultModel)

	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	existing, err := rt.List(ctx)
	if err != nil {
		return err
	}

	// Resolve name.
	name, err := resolveName(opts.name, existing)
	if err != nil {
		return err
	}

	// Resolve port.
	port, err := resolvePort(opts.port, existing)
	if err != nil {
		return err
	}

	host := firstNonEmpty(opts.host, "127.0.0.1")

	// Resolve resources.
	mem, err := resolveMemory(opts.memory, app.Host)
	if err != nil {
		return err
	}
	cpus := opts.cpus
	if cpus <= 0 {
		cpus = app.Host.DefaultCPUs()
	}

	spec := engine.Spec{
		Name:    name,
		Backend: b.Kind,
		Model:   model,
		Image:   image,
		Memory:  mem,
		CPUs:    cpus,
		GPU:     opts.gpu,
		Port:    port,
		Host:    host,
		Runtime: engine.RuntimeContainer,
		Env:     upEnv(opts),
	}

	io := app.IO
	t := io.Theme

	// Honest hardware warning (plan §7): containers on macOS get no Metal.
	if app.Host.IsMac() && opts.gpu {
		io.Println(t.WarnLine("Docker on macOS can't pass through the Apple GPU; this container will run CPU-only."))
		io.Println(t.Muted.Render("  Native Metal mode (`--native`) is planned; see `llmaker doctor`."))
	}

	io.Println(t.InfoLine(fmt.Sprintf("Starting %s (%s · %s)", t.Value.Render(name), b.DisplayName, model)))

	baseURL, err := provision(ctx, app, rt, spec, opts.healthTimeout, !opts.noPull)
	if err != nil {
		return err
	}

	printReadyCard(app, name, b, model, baseURL, port)
	return nil
}

// upEnv collects the user-facing env knobs; provision() injects the LLMAKER_*
// identity vars on top of these.
func upEnv(opts upOptions) map[string]string {
	env := map[string]string{}
	if opts.apiKey != "" {
		env["API_KEY"] = opts.apiKey
	}
	if opts.cors != "" {
		env["CORS_ORIGINS"] = opts.cors
	}
	if opts.keepAlive != "" {
		env["KEEP_ALIVE"] = opts.keepAlive
	}
	return env
}

func printReadyCard(app *App, name string, b backend.Backend, model, baseURL string, port int) {
	io := app.IO
	t := io.Theme
	io.Println()
	io.Println(t.Card("✓ Instance ready", [][2]string{
		{"name", name},
		{"backend", b.DisplayName},
		{"model", model},
		{"endpoint", baseURL + "/v1"},
		{"web UI", baseURL},
		{"port", strconv.Itoa(port)},
	}, true))
	io.Println()
	io.Println(t.Muted.Render("Next:"))
	io.Println("  " + t.Accent.Render("llmaker chat "+name) + t.Muted.Render("   # quick test in the terminal"))
	io.Println("  " + t.Accent.Render("llmaker open "+name) + t.Muted.Render("   # open the web UI"))
	io.Println("  " + t.Accent.Render("llmaker top") + t.Muted.Render("            # live fleet dashboard"))
}
