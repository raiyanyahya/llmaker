package cli

import (
	"context"
	"fmt"
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
