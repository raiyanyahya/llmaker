package backend

import "testing"

func TestGetDefault(t *testing.T) {
	b, err := Get("")
	if err != nil {
		t.Fatalf("Get(\"\"): %v", err)
	}
	if b.Kind != Ollama {
		t.Errorf("default backend = %q, want ollama", b.Kind)
	}
	if Default().Kind != Ollama {
		t.Errorf("Default() = %q, want ollama", Default().Kind)
	}
}

func TestGetAliases(t *testing.T) {
	for _, name := range []string{"llamacpp", "llama.cpp", "llama-cpp", "LLAMA_CPP", " LlamaCpp "} {
		b, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		if b.Kind != LlamaCPP {
			t.Errorf("Get(%q) = %q, want llamacpp", name, b.Kind)
		}
	}
}

func TestGetUnknown(t *testing.T) {
	if _, err := Get("vllm"); err == nil {
		t.Error("Get(\"vllm\") should error until the adapter exists")
	}
}

func TestAllBackendsAreComplete(t *testing.T) {
	all := All()
	if len(all) < 2 {
		t.Fatalf("expected at least 2 backends, got %d", len(all))
	}
	for _, b := range all {
		if b.Image == "" {
			t.Errorf("backend %q has no image", b.Kind)
		}
		if b.DefaultModel == "" {
			t.Errorf("backend %q has no default model", b.Kind)
		}
		if b.DisplayName == "" {
			t.Errorf("backend %q has no display name", b.Kind)
		}
	}
}

func TestNamesSorted(t *testing.T) {
	names := Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("Names() not sorted: %v", names)
		}
	}
}
