// Package cli builds llmaker's Cobra command tree. Commands depend only on
// small interfaces (engine.Runtime, facade.Client) carried on *App, so every
// command's logic can be unit-tested with in-memory fakes and no Docker.
package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/engine"
	"github.com/raiyanyahya/llmaker/internal/facade"
	"github.com/raiyanyahya/llmaker/internal/ui"
)

// VersionInfo is stamped in at build time via -ldflags.
type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

// String renders a one-line version.
func (v VersionInfo) String() string {
	ver := v.Version
	if ver == "" {
		ver = "dev"
	}
	if v.Commit != "" {
		return fmt.Sprintf("%s (%s)", ver, v.Commit)
	}
	return ver
}

// App carries the dependencies every command shares.
type App struct {
	IO      *ui.IOStreams
	Facade  facade.Client
	Host    engine.HostInfo
	Version VersionInfo

	// NewRuntime lazily constructs the orchestration backend. It is a factory
	// (not a value) so commands that don't need Docker — version, doctor,
	// help — never fail just because the daemon is down, and so tests can
	// inject an in-memory fake.
	NewRuntime func(context.Context) (engine.Runtime, error)

	// OpenURL launches a browser; injectable for testing `llmaker open`.
	OpenURL func(string) error

	// ServiceReady probes whether a service is accepting connections on
	// host:port. It is injectable so tests can skip real network waits; when
	// nil it defaults to a TCP dial (waitPortReady).
	ServiceReady func(ctx context.Context, host string, port int, timeout time.Duration) error

	// forceNoColor is toggled by the global --no-color flag.
	forceNoColor bool
}

// waitServiceReady blocks until a service's primary port accepts connections,
// using the injected prober when present (tests) and a real TCP dial otherwise.
func (a *App) waitServiceReady(ctx context.Context, host string, port int, timeout time.Duration) error {
	if a.ServiceReady != nil {
		return a.ServiceReady(ctx, host, port, timeout)
	}
	return waitPortReady(ctx, host, port, timeout)
}

// Command groups keep `llmaker --help` organized.
const (
	groupLifecycle = "lifecycle"
	groupFleet     = "fleet"
	groupModels    = "models"
	groupAdvanced  = "advanced"
)

// NewRootCmd assembles the full command tree.
func NewRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "llmaker",
		Short:         "Spin up isolated, self-hosted LLM API servers from your terminal",
		Long:          rootLong(app),
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       app.Version.String(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if app.forceNoColor {
				app.IO.Theme = ui.NewTheme(app.IO.Out, false)
			}
			return nil
		},
	}

	root.PersistentFlags().BoolVar(&app.forceNoColor, "no-color", false, "disable colored output")
	root.SetVersionTemplate("llmaker {{.Version}}\n")

	root.AddGroup(
		&cobra.Group{ID: groupLifecycle, Title: "Lifecycle:"},
		&cobra.Group{ID: groupFleet, Title: "Fleet & observability:"},
		&cobra.Group{ID: groupModels, Title: "Models:"},
		&cobra.Group{ID: groupAdvanced, Title: "Advanced:"},
	)

	root.AddCommand(
		newUpCmd(app),
		newLsCmd(app),
		newServiceCmd(app),
		newStackCmd(app),
		newStatusCmd(app),
		newTopCmd(app),
		newPullCmd(app),
		newChatCmd(app),
		newOpenCmd(app),
		newLogsCmd(app),
		newStopCmd(app),
		newStartCmd(app),
		newRmCmd(app),
		newApplyCmd(app),
		newDoctorCmd(app),
		newBuildCmd(app),
		newVersionCmd(app),
	)
	return root
}

func rootLong(app *App) string {
	t := app.IO.Theme
	logo := t.Logo.Render("llmaker")
	return fmt.Sprintf(`%s — one CLI to run self-hosted LLM servers.

Each instance is an isolated container exposing a stable OpenAI-compatible API,
health/status endpoints, and its own web UI. Pick a backend (Ollama, llama.cpp),
set resource limits, and manage the whole fleet from your terminal.

Quick start:
  llmaker up chat            start an instance from a preset (zero flags)
  llmaker up                 start an instance (interactive wizard)
  llmaker ls                 list the fleet
  llmaker top                live dashboard across all instances
  llmaker chat <name>        chat with an instance to sanity-check it`, logo)
}

// runtime constructs the orchestration backend and returns it with a cleanup
// func. A failure is wrapped with actionable guidance.
func (a *App) runtime(ctx context.Context) (engine.Runtime, func(), error) {
	if a.NewRuntime == nil {
		return nil, nil, errors.New("no runtime configured")
	}
	rt, err := a.NewRuntime(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("%w\n\nRun `llmaker doctor` to diagnose your environment", err)
	}
	return rt, func() { _ = rt.Close() }, nil
}

// mustGet resolves an instance by name with a friendly not-found message.
func (a *App) mustGet(ctx context.Context, rt engine.Runtime, name string) (engine.Instance, error) {
	name = engine.NormalizeName(name)
	in, err := rt.Get(ctx, name)
	if errors.Is(err, engine.ErrNotFound) {
		return engine.Instance{}, fmt.Errorf("no instance named %q (try `llmaker ls`)", name)
	}
	return in, err
}

// requireRunning returns a friendly error if the instance isn't up.
func requireRunning(in engine.Instance) error {
	if !in.IsRunning() {
		return fmt.Errorf("instance %q is not running (start it with `llmaker start %s`)", in.Name, in.Name)
	}
	return nil
}
