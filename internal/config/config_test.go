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

func TestShippedExamplesAreValid(t *testing.T) {
	for _, path := range []string{"../../examples/llm.yaml", "../../examples/stack.yaml"} {
		f, err := Load(path)
		if err != nil {
			t.Fatalf("%s: load: %v", path, err)
		}
		if _, err := f.ToSpecs(); err != nil {
			t.Fatalf("%s: ToSpecs: %v", path, err)
		}
		if _, err := f.ToServiceSpecs(); err != nil {
			t.Fatalf("%s: ToServiceSpecs: %v", path, err)
		}
	}
}

func TestStackServiceLowering(t *testing.T) {
	f, err := Parse([]byte(`
instances:
  - { name: chat, model: llama3:8b }
services:
  - use: qdrant
  - { name: cache, use: redis }
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	svcs, err := f.ToServiceSpecs()
	if err != nil {
		t.Fatalf("ToServiceSpecs: %v", err)
	}
	if len(svcs) != 2 {
		t.Fatalf("want 2 services, got %d", len(svcs))
	}
	// Sorted by name: cache, qdrant.
	if svcs[0].Name != "cache" || svcs[0].Service != "redis" {
		t.Errorf("svc[0] = %+v", svcs[0])
	}
	if svcs[1].Name != "qdrant" || len(svcs[1].Ports) != 2 {
		t.Errorf("svc[1] = %+v (want qdrant w/ 2 ports)", svcs[1])
	}
	if len(svcs[1].Volumes) != 1 || svcs[1].Volumes[0].Name != "llmaker-qdrant-storage" {
		t.Errorf("qdrant volume wrong: %+v", svcs[1].Volumes)
	}
}

func TestServicesOrderedByStartupTier(t *testing.T) {
	// Declared out of dependency order; ToServiceSpecs must return them so data
	// stores come first, then apps (langfuse needs pgvector), then the agent.
	f, err := Parse([]byte(`
services:
  - use: agent
  - use: langfuse
  - use: pgvector
  - use: qdrant
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	specs, err := f.ToServiceSpecs()
	if err != nil {
		t.Fatalf("ToServiceSpecs: %v", err)
	}
	order := make(map[string]int, len(specs))
	for i, s := range specs {
		order[s.Service] = i
	}
	if !(order["pgvector"] < order["langfuse"]) {
		t.Errorf("pgvector (%d) must come before langfuse (%d)", order["pgvector"], order["langfuse"])
	}
	if !(order["qdrant"] < order["agent"]) || !(order["langfuse"] < order["agent"]) {
		t.Errorf("agent must come last; order=%v", order)
	}
}

func TestEmptyFileYieldsNoSpecs(t *testing.T) {
	// A stack may declare only services, so an empty instances list is no longer
	// an error here; apply is what rejects an entirely empty stack.
	f, _ := Parse([]byte(`version: "1"`))
	specs, err := f.ToSpecs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("expected no specs, got %d", len(specs))
	}
	svc, err := f.ToServiceSpecs()
	if err != nil || len(svc) != 0 {
		t.Fatalf("expected no service specs, got %d (err=%v)", len(svc), err)
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
