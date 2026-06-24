// Command llmaker is the CLI for spinning up isolated, self-hosted LLM API
// servers as containers and managing the whole fleet from your terminal.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
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

// versionInfo resolves build metadata. It prefers the values stamped in via
// -ldflags (what `make build` and releases do) and otherwise falls back to the
// data the Go toolchain embeds itself — so `go install …@v1.2.3` reports its
// module version and a `go build` from a checkout reports the VCS commit/time,
// instead of a bare "dev".
func versionInfo() cli.VersionInfo {
	v := cli.VersionInfo{Version: version, Commit: commit, Date: date}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v
	}
	// Module version, e.g. "v1.2.3" or a pseudo-version from `go install`.
	// "(devel)" means an unstamped local build, which carries no useful version.
	if (v.Version == "" || v.Version == "dev") &&
		info.Main.Version != "" && info.Main.Version != "(devel)" {
		v.Version = info.Main.Version
	}
	var rev, when string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			when = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if v.Commit == "" && rev != "" {
		if len(rev) > 7 { // match the Makefile's short-commit style
			rev = rev[:7]
		}
		if dirty {
			rev += "-dirty"
		}
		v.Commit = rev
	}
	if v.Date == "" && when != "" {
		v.Date = when
	}
	return v
}

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
		Version: versionInfo(),
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
