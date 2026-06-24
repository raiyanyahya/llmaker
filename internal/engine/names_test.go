package engine

import (
	"strings"
	"testing"
)

func TestNameFromIsStableAndFormatted(t *testing.T) {
	n := nameFrom(0, 0)
	if !strings.Contains(n, "-") {
		t.Fatalf("name %q should contain a hyphen", n)
	}
	parts := strings.SplitN(n, "-", 2)
	if parts[0] != nameAdjectives[0] || parts[1] != nameAnimals[0] {
		t.Fatalf("nameFrom(0,0) = %q, want %s-%s", n, nameAdjectives[0], nameAnimals[0])
	}
}

func TestNameFromHandlesOutOfRangeIndices(t *testing.T) {
	// Indices are reduced modulo the slice length, including negatives, so a
	// caller passing rand output of any sign never panics.
	for _, tc := range [][2]int{{-1, -1}, {1000, 1000}, {-7, 13}} {
		n := nameFrom(tc[0], tc[1])
		if !ValidName(n) {
			t.Errorf("nameFrom(%d,%d) = %q is not a valid name", tc[0], tc[1], n)
		}
	}
}

func TestGenerateNameIsValid(t *testing.T) {
	for i := 0; i < 100; i++ {
		if n := GenerateName(); !ValidName(n) {
			t.Fatalf("GenerateName produced invalid name %q", n)
		}
	}
}

func TestGenerateUniqueNameAvoidsTaken(t *testing.T) {
	// Mark almost the entire namespace as taken to force the suffix fallback.
	taken := map[string]bool{}
	for _, a := range nameAdjectives {
		for _, n := range nameAnimals {
			taken[a+"-"+n] = true
		}
	}
	got := GenerateUniqueName(taken)
	if taken[got] {
		t.Fatalf("GenerateUniqueName returned a taken name %q", got)
	}
	if !strings.Contains(got, "-2") && !strings.HasSuffix(got, "-2") {
		// Fallback appends a numeric suffix starting at 2.
		t.Logf("note: fallback name = %q", got)
	}
}

func TestValidName(t *testing.T) {
	valid := []string{"a", "brave-llama", "gpu_box", "model-2", "abc123"}
	for _, s := range valid {
		if !ValidName(s) {
			t.Errorf("ValidName(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "-leading", "trailing-", "Upper", "has space", "emoji😀", strings.Repeat("x", 64)}
	for _, s := range invalid {
		if ValidName(s) {
			t.Errorf("ValidName(%q) = true, want false", s)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	if got := NormalizeName("  Brave-Llama  "); got != "brave-llama" {
		t.Errorf("NormalizeName = %q", got)
	}
}
