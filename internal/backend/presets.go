package backend

import (
	"sort"
	"strings"
)

// Preset is a one-word shortcut that resolves to an obvious backend + model, so
// the most common instances start with zero flags and no wizard:
//
//	llmaker up chat     # general chat
//	llmaker up code     # a coding model
//
// Resource limits are intentionally not pinned here — they fall back to
// host-derived defaults so a preset behaves sensibly on any machine.
type Preset struct {
	Name        string
	Backend     Kind
	Model       string
	Description string
}

// presetRegistry holds the built-in quick-start presets. They all use Ollama —
// the zero-config default backend — so a preset never needs a model file
// mounted ahead of time.
var presetRegistry = []Preset{
	{Name: "chat", Backend: Ollama, Model: "llama3:8b", Description: "General-purpose chat (the default)."},
	{Name: "code", Backend: Ollama, Model: "qwen2.5-coder:7b", Description: "Code generation and completion."},
	{Name: "small", Backend: Ollama, Model: "llama3.2:1b", Description: "Tiny and fast — good for CPU or low RAM."},
	{Name: "embed", Backend: Ollama, Model: "nomic-embed-text", Description: "Text embeddings for search / RAG."},
	{Name: "vision", Backend: Ollama, Model: "llava:7b", Description: "Multimodal — images and text."},
}

// Presets returns the built-in presets, sorted by name for stable help output.
func Presets() []Preset {
	out := make([]Preset, len(presetRegistry))
	copy(out, presetRegistry)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// GetPreset resolves a preset by name (case-insensitive). The boolean reports
// whether it was found, mirroring map-lookup ergonomics.
func GetPreset(name string) (Preset, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, p := range presetRegistry {
		if p.Name == key {
			return p, true
		}
	}
	return Preset{}, false
}

// PresetNames returns the sorted preset identifiers, handy for shell completion
// and "unknown preset" error messages.
func PresetNames() []string {
	names := make([]string, 0, len(presetRegistry))
	for _, p := range presetRegistry {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}
