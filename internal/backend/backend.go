// Package backend describes the inference engines llmaker can run and the
// prebuilt, parameterized images that wrap each one with the normalized
// control-plane facade.
//
// Adding a backend is intentionally cheap: register a Backend here and ship a
// facade adapter. Nothing in the CLI, the web UI, or a user's application code
// needs to change, because every backend is reached through the same facade
// contract (see the Python facade in ./facade).
package backend

import (
	"fmt"
	"sort"
	"strings"
)

// Kind identifies an inference engine.
type Kind string

const (
	// Ollama is the default backend: easiest to run, great model library.
	Ollama Kind = "ollama"
	// LlamaCPP offers maximum control over GGUF models and quantization.
	LlamaCPP Kind = "llamacpp"
)

// FacadePort is the fixed port the control-plane facade listens on *inside*
// every container. The host-facing port is mapped to this one, so the CLI only
// ever has to reason about a single internal port regardless of backend.
const FacadePort = 8080

// Backend is the static description of a supported engine.
type Backend struct {
	Kind         Kind
	DisplayName  string
	Image        string
	DefaultModel string
	Description  string
	// Native reports whether this backend can also run as a host-native
	// process (used for Apple Silicon Metal acceleration, see §7 of the plan).
	Native bool
}

// imageRepo is the GHCR namespace for the prebuilt images.
const imageRepo = "ghcr.io/raiyanyahya"

var registry = map[Kind]Backend{
	Ollama: {
		Kind:         Ollama,
		DisplayName:  "Ollama",
		Image:        imageRepo + "/llmaker-ollama:latest",
		DefaultModel: "llama3:8b",
		Description:  "Easiest path. Rich model library, simple pull/run semantics.",
		Native:       true,
	},
	LlamaCPP: {
		Kind:         LlamaCPP,
		DisplayName:  "llama.cpp",
		Image:        imageRepo + "/llmaker-llamacpp:latest",
		DefaultModel: "qwen2.5:7b-instruct-q4_K_M",
		Description:  "Maximum control: GGUF, quantization, fine-grained perf flags.",
		Native:       true,
	},
}

// Default returns the backend used when none is specified.
func Default() Backend { return registry[Ollama] }

// Get resolves a backend by name, accepting a few friendly aliases.
func Get(name string) (Backend, error) {
	key := Kind(strings.ToLower(strings.TrimSpace(name)))
	switch key {
	case "llama.cpp", "llama-cpp", "llamacpp", "llama_cpp":
		key = LlamaCPP
	case "":
		return Default(), nil
	}
	b, ok := registry[key]
	if !ok {
		return Backend{}, fmt.Errorf("unknown backend %q (supported: %s)", name, strings.Join(Names(), ", "))
	}
	return b, nil
}

// All returns every registered backend, sorted by name for stable output.
func All() []Backend {
	out := make([]Backend, 0, len(registry))
	for _, b := range registry {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// Names returns the sorted list of backend identifiers.
func Names() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, string(k))
	}
	sort.Strings(names)
	return names
}
