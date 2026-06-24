package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/raiyanyahya/llmaker/internal/engine"
)

// resolveName validates a user-supplied name or generates a unique friendly one.
func resolveName(given string, existing []engine.Instance) (string, error) {
	taken := make(map[string]bool, len(existing))
	for _, in := range existing {
		taken[in.Name] = true
	}
	if given != "" {
		name := engine.NormalizeName(given)
		if !engine.ValidName(name) {
			return "", fmt.Errorf("invalid name %q: use lowercase letters, digits, - or _ (max 63 chars)", given)
		}
		if taken[name] {
			return "", fmt.Errorf("an instance named %q already exists", name)
		}
		return name, nil
	}
	return engine.GenerateUniqueName(taken), nil
}

// resolvePort honors an explicit port (verifying it's free) or auto-allocates.
func resolvePort(given int, existing []engine.Instance) (int, error) {
	used := engine.UsedPorts(existing)
	if given > 0 {
		if used[given] {
			return 0, fmt.Errorf("port %d is already used by another instance", given)
		}
		if !engine.PortAvailable(given) {
			return 0, fmt.Errorf("port %d is not available on this host", given)
		}
		return given, nil
	}
	return engine.AllocatePort(used)
}

// resolveMemory parses an explicit memory string or derives a host-based default.
func resolveMemory(s string, host engine.HostInfo) (int64, error) {
	if strings.TrimSpace(s) == "" {
		return host.DefaultMemoryBytes(), nil
	}
	return engine.ParseSize(s)
}

// pollHealth blocks until the facade reports healthy or the timeout elapses.
func pollHealth(ctx context.Context, app *App, baseURL string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		if _, err := app.Facade.Health(ctx, baseURL); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out after %s waiting for the facade to become healthy", timeout)
		case <-ticker.C:
		}
	}
}

// firstNonEmpty returns the first non-blank string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
