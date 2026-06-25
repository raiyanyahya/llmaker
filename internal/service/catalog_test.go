package service

import "testing"

func TestGetResolvesAliases(t *testing.T) {
	cases := map[string]string{
		"qdrant":     "qdrant",
		"Qdrant":     "qdrant",
		"  redis  ":  "redis",
		"valkey":     "redis",
		"postgres":   "pgvector",
		"postgresql": "pgvector",
		"pg":         "pgvector",
		"tei":        "embeddings",
		"embedding":  "embeddings",
		"openwebui":  "open-webui",
		"webui":      "open-webui",
		"ui":         "open-webui",
	}
	for in, want := range cases {
		got, err := Get(in)
		if err != nil {
			t.Fatalf("Get(%q): %v", in, err)
		}
		if got.Kind != want {
			t.Errorf("Get(%q).Kind = %q, want %q", in, got.Kind, want)
		}
	}
}

func TestGetUnknown(t *testing.T) {
	if _, err := Get("not-a-service"); err == nil {
		t.Fatal("expected error for unknown service")
	}
}

func TestEveryEntryIsWellFormed(t *testing.T) {
	for _, s := range All() {
		if s.Kind == "" || s.Image == "" || s.Category == "" {
			t.Errorf("%q missing core fields", s.Kind)
		}
		if len(s.Ports) == 0 {
			t.Errorf("%q declares no ports", s.Kind)
		}
		// Exactly one primary port, and PrimaryPort returns it.
		primaries := 0
		for _, p := range s.Ports {
			if p.Primary {
				primaries++
			}
		}
		if primaries != 1 {
			t.Errorf("%q has %d primary ports, want 1", s.Kind, primaries)
		}
		if !s.PrimaryPort().Primary {
			t.Errorf("%q PrimaryPort is not marked primary", s.Kind)
		}
	}
}

func TestNamesSorted(t *testing.T) {
	names := Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("Names not sorted: %v", names)
		}
	}
}
