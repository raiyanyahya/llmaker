package engine

import (
	"testing"

	"github.com/raiyanyahya/llmaker/internal/backend"
)

func TestSpecLabelsRoundTrip(t *testing.T) {
	spec := Spec{
		Name:    "brave-llama",
		Backend: backend.Ollama,
		Model:   "llama3:8b",
		Host:    "127.0.0.1",
		Runtime: RuntimeContainer,
	}
	labels := SpecLabels(spec, "ghcr.io/raiyanyahya/llmaker-ollama:latest", 11500)

	if labels[LabelManagedBy] != ManagedByValue {
		t.Errorf("missing managed-by label: %v", labels)
	}

	inst := InstanceFromLabels("abc123", StateRunning, labels)
	if inst.ID != "abc123" {
		t.Errorf("ID = %q", inst.ID)
	}
	if inst.Name != "brave-llama" {
		t.Errorf("Name = %q", inst.Name)
	}
	if inst.Backend != backend.Ollama {
		t.Errorf("Backend = %q", inst.Backend)
	}
	if inst.Model != "llama3:8b" {
		t.Errorf("Model = %q", inst.Model)
	}
	if inst.Port != 11500 {
		t.Errorf("Port = %d", inst.Port)
	}
	if inst.Image != "ghcr.io/raiyanyahya/llmaker-ollama:latest" {
		t.Errorf("Image = %q", inst.Image)
	}
	if inst.Host != "127.0.0.1" {
		t.Errorf("Host = %q", inst.Host)
	}
	if inst.State != StateRunning {
		t.Errorf("State = %q", inst.State)
	}
	if inst.Runtime != RuntimeContainer {
		t.Errorf("Runtime = %q", inst.Runtime)
	}
	if inst.Created.IsZero() {
		t.Errorf("Created should be parsed from labels")
	}
}

func TestSpecLabelsAuth(t *testing.T) {
	// The auth label mirrors the facade's semantics: a blank/whitespace key is
	// no key at all. It lets lifecycle commands re-warn about public keyless
	// binds without inspecting container env.
	keyed := SpecLabels(Spec{Name: "a", Env: map[string]string{"API_KEY": "s3cret"}}, "img", 1)
	if keyed[LabelAuth] != AuthKey {
		t.Errorf("keyed LabelAuth = %q, want %q", keyed[LabelAuth], AuthKey)
	}
	blank := SpecLabels(Spec{Name: "a", Env: map[string]string{"API_KEY": "  "}}, "img", 1)
	if blank[LabelAuth] != AuthNone {
		t.Errorf("whitespace-key LabelAuth = %q, want %q", blank[LabelAuth], AuthNone)
	}
	none := SpecLabels(Spec{Name: "a"}, "img", 1)
	if none[LabelAuth] != AuthNone {
		t.Errorf("no-env LabelAuth = %q, want %q", none[LabelAuth], AuthNone)
	}

	if in := InstanceFromLabels("id", StateRunning, keyed); in.Auth != AuthKey {
		t.Errorf("round-tripped Auth = %q, want %q", in.Auth, AuthKey)
	}
	// Pre-label containers read as unknown ("") — never as AuthNone.
	if in := InstanceFromLabels("id", StateRunning, map[string]string{}); in.Auth != "" {
		t.Errorf("pre-label Auth = %q, want empty", in.Auth)
	}
}

func TestInstanceFromLabelsDegradesGracefully(t *testing.T) {
	// A container with only the managed-by label (e.g. created by an older
	// version) should still surface, not vanish or panic.
	labels := map[string]string{LabelManagedBy: ManagedByValue}
	inst := InstanceFromLabels("id", StateExited, labels)
	if inst.ID != "id" {
		t.Errorf("ID = %q", inst.ID)
	}
	if inst.Runtime != RuntimeContainer {
		t.Errorf("Runtime should default to container, got %q", inst.Runtime)
	}
	if inst.Port != 0 {
		t.Errorf("Port should be 0 when unparseable, got %d", inst.Port)
	}
	if inst.Health != HealthUnknown {
		t.Errorf("Health should default to unknown, got %q", inst.Health)
	}
}

func TestSpecLabelsDefaultsHostAndRuntime(t *testing.T) {
	labels := SpecLabels(Spec{Name: "x", Backend: backend.Ollama}, "img", 8080)
	if labels[LabelHost] != "127.0.0.1" {
		t.Errorf("default host = %q, want 127.0.0.1", labels[LabelHost])
	}
	if labels[LabelRuntime] != string(RuntimeContainer) {
		t.Errorf("default runtime = %q, want container", labels[LabelRuntime])
	}
}
