// Package config parses the declarative fleet file (llm.yaml) used by
// `llmaker apply`. It is compose-like but LLM-aware: a list of instances with
// shared defaults, validated and lowered into engine.Spec values.
package config

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/raiyanyahya/llmaker/internal/backend"
	"github.com/raiyanyahya/llmaker/internal/engine"
)

// File is the root of an llm.yaml document.
type File struct {
	Version   string     `yaml:"version"`
	Defaults  Defaults   `yaml:"defaults"`
	Instances []Instance `yaml:"instances"`
}

// Defaults are applied to any instance that omits a field.
type Defaults struct {
	Backend string `yaml:"backend"`
	Model   string `yaml:"model"`
	Memory  string `yaml:"memory"`
	CPUs    string `yaml:"cpus"`
	GPU     *bool  `yaml:"gpu"`
	Host    string `yaml:"host"`
}

// Instance is one declared server.
type Instance struct {
	Name    string            `yaml:"name"`
	Backend string            `yaml:"backend"`
	Model   string            `yaml:"model"`
	Memory  string            `yaml:"memory"`
	CPUs    string            `yaml:"cpus"`
	GPU     *bool             `yaml:"gpu"`
	Port    int               `yaml:"port"`
	Host    string            `yaml:"host"`
	Image   string            `yaml:"image"`
	Env     map[string]string `yaml:"env"`
}

// Load reads and parses an llm.yaml file from disk.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse parses llm.yaml bytes. Strict decoding rejects unknown keys so typos
// surface immediately instead of being silently ignored.
func Parse(data []byte) (*File, error) {
	var f File
	dec := yaml.NewDecoder(byteReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("parse llm.yaml: %w", err)
	}
	return &f, nil
}

// ToSpecs validates the document and lowers it into engine.Spec values, applying
// defaults and resolving backends. Ports left at 0 are auto-allocated later by
// the apply command (so the file can omit ports and stay portable).
func (f *File) ToSpecs() ([]engine.Spec, error) {
	if len(f.Instances) == 0 {
		return nil, fmt.Errorf("no instances declared")
	}
	seen := map[string]bool{}
	specs := make([]engine.Spec, 0, len(f.Instances))

	for i, in := range f.Instances {
		name := engine.NormalizeName(firstNonEmpty(in.Name, ""))
		if name == "" {
			return nil, fmt.Errorf("instance #%d: name is required", i+1)
		}
		if !engine.ValidName(name) {
			return nil, fmt.Errorf("instance %q: invalid name (use lowercase letters, digits, - or _)", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate instance name %q", name)
		}
		seen[name] = true

		bname := firstNonEmpty(in.Backend, f.Defaults.Backend)
		b, err := backend.Get(bname)
		if err != nil {
			return nil, fmt.Errorf("instance %q: %w", name, err)
		}

		model := firstNonEmpty(in.Model, f.Defaults.Model, b.DefaultModel)

		spec := engine.Spec{
			Name:    name,
			Backend: b.Kind,
			Model:   model,
			Image:   firstNonEmpty(in.Image, b.Image),
			Port:    in.Port,
			Host:    firstNonEmpty(in.Host, f.Defaults.Host, "127.0.0.1"),
			GPU:     resolveBool(in.GPU, f.Defaults.GPU, false),
			Env:     in.Env,
			Runtime: engine.RuntimeContainer,
		}

		memStr := firstNonEmpty(in.Memory, f.Defaults.Memory)
		if memStr != "" {
			bytes, err := engine.ParseSize(memStr)
			if err != nil {
				return nil, fmt.Errorf("instance %q: memory: %w", name, err)
			}
			spec.Memory = bytes
		}

		cpuStr := firstNonEmpty(in.CPUs, f.Defaults.CPUs)
		if cpuStr != "" {
			cpus, err := parseFloat(cpuStr)
			if err != nil {
				return nil, fmt.Errorf("instance %q: cpus: %w", name, err)
			}
			spec.CPUs = cpus
		}

		specs = append(specs, spec)
	}

	// Stable order so apply output and reconciliation are deterministic.
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs, nil
}

// Names returns the declared instance names (normalized).
func (f *File) Names() []string {
	out := make([]string, 0, len(f.Instances))
	for _, in := range f.Instances {
		out = append(out, engine.NormalizeName(in.Name))
	}
	return out
}
