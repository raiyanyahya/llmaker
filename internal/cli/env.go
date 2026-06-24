package cli

import (
	"strconv"

	"github.com/raiyanyahya/llmaker/internal/backend"
	"github.com/raiyanyahya/llmaker/internal/engine"
)

// facadeEnv returns the environment a container needs for its facade to identify
// itself and bind correctly. User-supplied values in spec.Env always win, so an
// explicit override is never clobbered.
func facadeEnv(spec engine.Spec) map[string]string {
	env := map[string]string{}
	for k, v := range spec.Env {
		env[k] = v
	}
	setDefault(env, "LLMAKER_BACKEND", string(spec.Backend))
	setDefault(env, "LLMAKER_NAME", spec.Name)
	setDefault(env, "LLMAKER_DEFAULT_MODEL", spec.Model)
	setDefault(env, "FACADE_PORT", strconv.Itoa(backend.FacadePort))
	return env
}

func setDefault(m map[string]string, key, val string) {
	if _, ok := m[key]; !ok && val != "" {
		m[key] = val
	}
}
