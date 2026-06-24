// Command llmaker is the CLI for spinning up isolated, self-hosted LLM API
// servers as containers and managing the whole fleet from your terminal.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/raiyanyahya/llmaker/internal/cli"
	"github.com/raiyanyahya/llmaker/internal/engine"
	"github.com/raiyanyahya/llmaker/internal/engine/dockerrt"
	"github.com/raiyanyahya/llmaker/internal/facade"
	"github.com/raiyanyahya/llmaker/internal/ui"
)

// Build metadata, injected via -ldflags at release time.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	// Cancel on the usual termination signals so long-running commands (pull,
	// chat, logs -f) shut down cleanly. The TUI handles Ctrl-C itself in raw
	// mode, so this won't fight it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	io := ui.Standard()

	app := &cli.App{
		IO:      io,
		Facade:  facade.NewClient(facade.WithAPIKey(os.Getenv("LLMAKER_API_KEY"))),
		Host:    engine.Host(),
		Version: cli.VersionInfo{Version: version, Commit: commit, Date: date},
		NewRuntime: func(context.Context) (engine.Runtime, error) {
			return dockerrt.New()
		},
	}

	root := cli.NewRootCmd(app)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(io.Err, io.Theme.FailLine(err.Error()))
		os.Exit(1)
	}
}
