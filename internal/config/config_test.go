package config

import (
	"testing"

	"github.com/raiyanyahya/llmaker/internal/backend"
	"github.com/raiyanyahya/llmaker/internal/engine"
)

func TestParseAndLower(t *testing.T) {
	doc := []byte(`
version: "1"
defaults:
  backend: ollama
  memory: 8g
  cpus: "4"
instances:
  - name: chat
    model: llama3:8b
    gpu: true
    port: 11500
    env:
      KEEP_ALIVE: 10m
  - name: embed
    model: nomic-embed-text
    memory: 2g
`)
	f, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	specs, err := f.ToSpecs()
	if err != nil {
		t.Fatalf("ToSpecs: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}

	// Sorted by name: chat, embed.
	chat := specs[0]
	if chat.Name != "chat" {
		t.Fatalf("first spec = %q, want chat", chat.Name)
	}
	if chat.Backend != backend.Ollama {
		t.Errorf("backend = %q", chat.Backend)
	}
	if chat.Model != "llama3:8b" {
		t.Errorf("model = %q", chat.Model)
	}
	if chat.Memory != 8*engine.GiB {
		t.Errorf("memory = %d, want 8 GiB", chat.Memory)
	}
	if chat.CPUs != 4 {
		t.Errorf("cpus = %v, want 4", chat.CPUs)
	}
	if !chat.GPU {
		t.Errorf("gpu should be true")
	}
	if chat.Port != 11500 {
		t.Errorf("port = %d", chat.Port)
	}
	if chat.Env["KEEP_ALIVE"] != "10m" {
		t.Errorf("env = %v", chat.Env)
	}

	embed := specs[1]
	if embed.Memory != 2*engine.GiB {
		t.Errorf("embed memory = %d, want 2 GiB (override of default)", embed.Memory)
	}
	if embed.CPUs != 4 {
		t.Errorf("embed cpus = %v, want inherited 4", embed.CPUs)
	}
	if embed.GPU {
		t.Errorf("embed gpu should default false")
	}
}

func TestDefaultsBackendFallsBackToImageDefaultModel(t *testing.T) {
	f, err := Parse([]byte(`
instances:
  - name: a
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	specs, err := f.ToSpecs()
	if err != nil {
		t.Fatalf("ToSpecs: %v", err)
	}
	if specs[0].Backend != backend.Ollama {
		t.Errorf("backend = %q, want default ollama", specs[0].Backend)
	}
	if specs[0].Model != backend.Default().DefaultModel {
		t.Errorf("model = %q, want backend default", specs[0].Model)
	}
	if specs[0].Host != "127.0.0.1" {
		t.Errorf("host = %q, want default 127.0.0.1", specs[0].Host)
	}
}

func TestRejectsUnknownFields(t *testing.T) {
	_, err := Parse([]byte(`
instances:
  - name: a
    bogus: true
`))
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestRejectsDuplicateNames(t *testing.T) {
	f, _ := Parse([]byte(`
instances:
  - name: dup
  - name: dup
`))
	if _, err := f.ToSpecs(); err == nil {
		t.Fatal("expected duplicate-name error")
	}
}

func TestRejectsInvalidName(t *testing.T) {
	f, _ := Parse([]byte(`
instances:
  - name: "Bad Name"
`))
	if _, err := f.ToSpecs(); err == nil {
		t.Fatal("expected invalid-name error")
	}
}

func TestRejectsEmptyFleet(t *testing.T) {
	f, _ := Parse([]byte(`version: "1"`))
	if _, err := f.ToSpecs(); err == nil {
		t.Fatal("expected error for empty fleet")
	}
}

func TestRejectsBadMemory(t *testing.T) {
	f, _ := Parse([]byte(`
instances:
  - name: a
    memory: "not-a-size"
`))
	if _, err := f.ToSpecs(); err == nil {
		t.Fatal("expected error for bad memory")
	}
}
