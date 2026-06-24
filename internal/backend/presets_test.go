package backend

import "testing"

func TestGetPreset(t *testing.T) {
	p, ok := GetPreset("chat")
	if !ok {
		t.Fatal("chat preset should exist")
	}
	if p.Model == "" || p.Backend == "" {
		t.Errorf("preset incomplete: %+v", p)
	}
	if _, ok := GetPreset(" CHAT "); !ok {
		t.Error("preset lookup should be case-insensitive and trimmed")
	}
	if _, ok := GetPreset("nope"); ok {
		t.Error("unknown preset should not resolve")
	}
}

func TestPresetsAreComplete(t *testing.T) {
	ps := Presets()
	if len(ps) == 0 {
		t.Fatal("expected built-in presets")
	}
	for _, p := range ps {
		if p.Name == "" || p.Description == "" {
			t.Errorf("preset missing name/description: %+v", p)
		}
		// Every preset must resolve to a real backend.
		if _, err := Get(string(p.Backend)); err != nil {
			t.Errorf("preset %q has invalid backend %q", p.Name, p.Backend)
		}
	}
	for i := 1; i < len(ps); i++ {
		if ps[i-1].Name > ps[i].Name {
			t.Fatalf("Presets() not sorted: %v", ps)
		}
	}
}

func TestPresetNamesSorted(t *testing.T) {
	names := PresetNames()
	if len(names) == 0 {
		t.Fatal("expected preset names")
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("PresetNames() not sorted: %v", names)
		}
	}
}
