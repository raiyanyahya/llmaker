package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/raiyanyahya/llmaker/internal/engine"
)

// provision brings a single spec from nothing to a healthy, model-loaded
// instance: pull image → create → start → wait healthy → preload model. It is
// shared by `up` (one instance) and `apply` (a declared fleet). It returns the
// instance's base URL so the caller can print endpoints.
func provision(ctx context.Context, app *App, rt engine.Runtime, spec engine.Spec, healthTimeout time.Duration, pullOnBoot bool) (string, error) {
	io := app.IO
	t := io.Theme
	baseURL := (engine.Instance{Host: spec.Host, Port: spec.Port}).URL()

	// Ensure the container always carries the env its facade needs to self-identify.
	spec.Env = facadeEnv(spec)

	// A public bind with no API key exposes the facade — including model
	// pull/delete and host metrics — to anyone on the network. Warn loudly.
	// Trimmed to mirror the facade, which strips the key and disables auth
	// when it is blank.
	if !isLoopbackHost(spec.Host) && strings.TrimSpace(spec.Env["API_KEY"]) == "" {
		warnPublicNoKey(app, spec.Name, spec.Host)
	}

	// Pull the prebuilt image, streaming progress when the runtime supports it.
	if puller, ok := rt.(engine.ImagePuller); ok {
		sp := t.NewSpinner(io.Out, "Pulling image "+spec.Image)
		sp.Start()
		perr := puller.PullImage(ctx, spec.Image, func(line string) {
			if line != "" {
				sp.SetLabel("Pulling image — " + line)
			}
		})
		if perr != nil {
			sp.Stop(t.FailLine("Pulling image — " + perr.Error()))
			return baseURL, perr
		}
		sp.Stop(t.SuccessLine("Image ready"))
	}

	if _, err := stepf(app, "Creating "+spec.Name, func() (engine.Instance, error) {
		return rt.Create(ctx, spec)
	}); err != nil {
		return baseURL, err
	}

	if err := app.step("Starting "+spec.Name, func() error {
		return rt.Start(ctx, spec.Name)
	}); err != nil {
		// Don't leave a half-created instance lying around.
		_ = rt.Remove(ctx, spec.Name, true)
		return baseURL, err
	}

	if err := app.step("Waiting for "+spec.Name+" facade", func() error {
		return pollHealth(ctx, app, baseURL, healthTimeout)
	}); err != nil {
		return baseURL, fmt.Errorf("%w\n  Inspect logs with `llmaker logs %s`", err, spec.Name)
	}

	if pullOnBoot && spec.Model != "" {
		if err := pullModel(ctx, app, baseURL, spec.Model); err != nil {
			io.Println(t.WarnLine(fmt.Sprintf("Model preload failed; instance is up. Retry: `llmaker pull %s --on %s`", spec.Model, spec.Name)))
		} else {
			_ = app.Facade.SetDefault(ctx, baseURL, spec.Model)
		}
	}
	return baseURL, nil
}

// warnPublicNoKey prints the loud public-bind/no-API-key warning. provision
// emits it at creation; start/restart re-warn via warnIfPublicNoKey, since the
// exposure recurs on every boot, not just the first.
func warnPublicNoKey(app *App, name, host string) {
	t := app.IO.Theme
	app.IO.Println(t.WarnLine(fmt.Sprintf(
		"%s binds to %s with no API key: the facade (model pull/delete, /metrics) is reachable by anyone on the network.",
		name, host)))
	app.IO.Println(t.Muted.Render("  Require a bearer token with --api-key (or `env: {API_KEY: …}` in a fleet file), or bind to 127.0.0.1."))
}

// warnIfPublicNoKey re-warns when an existing instance is known — via the auth
// label stamped at creation — to bind publicly with no API key. Instances
// created before the label existed are skipped rather than false-alarmed.
func warnIfPublicNoKey(app *App, in engine.Instance) {
	if in.Auth != engine.AuthNone || isLoopbackHost(in.Host) {
		return
	}
	warnPublicNoKey(app, in.Name, in.Host)
}
