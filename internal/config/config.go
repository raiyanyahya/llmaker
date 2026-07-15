// Package config parses the declarative stack file (llm.yaml) used by
// `llmaker apply`. It is compose-like but LLM-aware: LLM instances plus the
// services around them (vector DBs, caches, …) with shared defaults, validated
// and lowered into engine.Spec / engine.ServiceSpec values.
package config

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/raiyanyahya/llmaker/internal/backend"
	"github.com/raiyanyahya/llmaker/internal/engine"
	"github.com/raiyanyahya/llmaker/internal/service"
)

// File is the root of an llm.yaml document.
type File struct {
	Version string `yaml:"version"`
	Name    string `yaml:"name"` // optional stack name; scopes `apply --prune`
	// Network is an optional boundary: every container in the file joins this
	// private group network instead of the shared llmaker one, so the stack's
	// members resolve each other by name while nothing outside the group can
	// reach them. Two files declaring the same network share one boundary.
	// Per-instance/per-service `network:` overrides this default.
	Network   string     `yaml:"network"`
	Defaults  Defaults   `yaml:"defaults"`
	Instances []Instance `yaml:"instances"`
	Services  []Service  `yaml:"services"`
}

// StackName returns the normalized stack name (the file's top-level `name`),
// used to label resources and scope `apply --prune`. Empty when unnamed.
func (f *File) StackName() string {
	return engine.NormalizeName(f.Name)
}

// validStack returns the normalized stack name, erroring if it's set but not a
// valid identifier (it becomes a Docker label value and a prune selector).
func (f *File) validStack() (string, error) {
	name := f.StackName()
	if name != "" && !engine.ValidName(name) {
		return "", fmt.Errorf("invalid stack name %q (use lowercase letters, digits, - or _)", f.Name)
	}
	return name, nil
}

// resolveNetwork normalizes and validates a group-network name, falling back to
// the file-level `network:`. Empty means the shared llmaker network (it becomes
// part of a Docker network name and a label value, hence the identifier rules).
func (f *File) resolveNetwork(override string) (string, error) {
	raw := firstNonEmpty(override, f.Network)
	name := engine.NormalizeName(raw)
	if name != "" && !engine.ValidName(name) {
		return "", fmt.Errorf("invalid network name %q (use lowercase letters, digits, - or _)", raw)
	}
	return name, nil
}

// Service is one declared infrastructure service (resolved against the catalog).
type Service struct {
	Name    string            `yaml:"name"`
	Use     string            `yaml:"use"` // catalog kind (qdrant, redis, …)
	Port    int               `yaml:"port"`
	Host    string            `yaml:"host"`
	Image   string            `yaml:"image"`
	Memory  string            `yaml:"memory"`
	CPUs    string            `yaml:"cpus"`
	Env     map[string]string `yaml:"env"`
	Network string            `yaml:"network"` // overrides the file-level network
}

// GPUSpec accepts the `gpus:` YAML scalar — a count (2), "all", or device ids
// ("0,1") — keeping its literal form for engine.ParseGPURequest.
type GPUSpec string

// UnmarshalYAML accepts any scalar (so `gpus: 2` needs no quotes).
func (g *GPUSpec) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("gpus must be a scalar: \"all\", a count, or device ids like \"0,1\"")
	}
	*g = GPUSpec(value.Value)
	return nil
}

// Defaults are applied to any instance that omits a field.
type Defaults struct {
	Backend string  `yaml:"backend"`
	Model   string  `yaml:"model"`
	Memory  string  `yaml:"memory"`
	CPUs    string  `yaml:"cpus"`
	GPU     *bool   `yaml:"gpu"`
	GPUs    GPUSpec `yaml:"gpus"` // "all", a count, or device ids
	Host    string  `yaml:"host"`
}

// Instance is one declared server.
type Instance struct {
	Name    string            `yaml:"name"`
	Backend string            `yaml:"backend"`
	Model   string            `yaml:"model"`
	Memory  string            `yaml:"memory"`
	CPUs    string            `yaml:"cpus"`
	GPU     *bool             `yaml:"gpu"`
	GPUs    GPUSpec           `yaml:"gpus"` // "all", a count, or device ids
	Port    int               `yaml:"port"`
	Host    string            `yaml:"host"`
	Image   string            `yaml:"image"`
	Env     map[string]string `yaml:"env"`
	Network string            `yaml:"network"` // overrides the file-level network
}

// resolveGPUs picks an instance's effective GPU request. The instance's own
// gpu/gpus settings win over defaults as a unit (so `gpu: false` on an
// instance disables an inherited default); within one level `gpu:` and `gpus:`
// are mutually exclusive. Returns the raw request for engine.ParseGPURequest
// plus the legacy all-GPUs bool.
func resolveGPUs(in Instance, d Defaults) (request string, all bool, err error) {
	pick := func(gpus GPUSpec, gpu *bool) (string, bool, error) {
		if gpus != "" && gpu != nil {
			return "", false, fmt.Errorf("use either gpu or gpus, not both")
		}
		if gpus != "" {
			return string(gpus), false, nil
		}
		return "", gpu != nil && *gpu, nil
	}
	if in.GPUs != "" || in.GPU != nil {
		return pick(in.GPUs, in.GPU)
	}
	return pick(d.GPUs, d.GPU)
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
	// A stack may declare only services (e.g. a vector DB + cache with no LLM
	// yet), so an empty instances list is allowed; apply guards the
	// everything-empty case.
	stack, err := f.validStack()
	if err != nil {
		return nil, err
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

		netName, err := f.resolveNetwork(in.Network)
		if err != nil {
			return nil, fmt.Errorf("instance %q: %w", name, err)
		}

		gpuReq, gpuAll, err := resolveGPUs(in, f.Defaults)
		if err != nil {
			return nil, fmt.Errorf("instance %q: %w", name, err)
		}
		// Validate the request syntax here so a typo fails at parse time; the
		// resolution to concrete devices happens in apply's admission pass.
		if _, perr := engine.ParseGPURequest(gpuReq); perr != nil {
			return nil, fmt.Errorf("instance %q: %w", name, perr)
		}

		spec := engine.Spec{
			Name:    name,
			Backend: b.Kind,
			Model:   model,
			Image:   firstNonEmpty(in.Image, b.Image),
			Port:    in.Port,
			Host:    firstNonEmpty(in.Host, f.Defaults.Host, "127.0.0.1"),
			GPU:     gpuAll,
			GPUs:    gpuReq,
			Env:     in.Env,
			Runtime: engine.RuntimeContainer,
			Stack:   stack,
			Network: netName,
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

// ToServiceSpecs validates the document's services and lowers them into
// engine.ServiceSpec values, resolving each against the catalog. Primary host
// ports left unset (0) are auto-allocated later by apply, so the file stays
// portable.
func (f *File) ToServiceSpecs() ([]engine.ServiceSpec, error) {
	stack, err := f.validStack()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	specs := make([]engine.ServiceSpec, 0, len(f.Services))

	for i, s := range f.Services {
		kind := firstNonEmpty(s.Use, s.Name)
		cat, err := service.Get(kind)
		if err != nil {
			return nil, fmt.Errorf("service #%d: %w", i+1, err)
		}
		name := engine.NormalizeName(firstNonEmpty(s.Name, cat.Kind))
		if !engine.ValidName(name) {
			return nil, fmt.Errorf("service %q: invalid name (use lowercase letters, digits, - or _)", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate service name %q", name)
		}
		seen[name] = true

		ports := make([]engine.PortBinding, 0, len(cat.Ports))
		for _, p := range cat.Ports {
			host := 0
			if p.Primary && s.Port > 0 {
				host = s.Port
			}
			ports = append(ports, engine.PortBinding{
				Host: host, Container: p.Container, Name: p.Name, Primary: p.Primary,
			})
		}
		volumes := make([]engine.VolumeBinding, 0, len(cat.Volumes))
		for _, v := range cat.Volumes {
			volumes = append(volumes, engine.VolumeBinding{
				Name: engine.ServiceVolumeName(name, v.Suffix), Path: v.Path,
			})
		}

		netName, err := f.resolveNetwork(s.Network)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", name, err)
		}

		spec := engine.ServiceSpec{
			Name:     name,
			Service:  cat.Kind,
			Category: string(cat.Category),
			Image:    firstNonEmpty(s.Image, cat.Image),
			Ports:    ports,
			Host:     firstNonEmpty(s.Host, f.Defaults.Host, "127.0.0.1"),
			Env:      mergeServiceEnv(cat.Env, s.Env),
			Volumes:  volumes,
			Stack:    stack,
			Network:  netName,
		}

		if s.Memory != "" {
			bytes, err := engine.ParseSize(s.Memory)
			if err != nil {
				return nil, fmt.Errorf("service %q: memory: %w", name, err)
			}
			spec.Memory = bytes
		}
		if s.CPUs != "" {
			cpus, err := parseFloat(s.CPUs)
			if err != nil {
				return nil, fmt.Errorf("service %q: cpus: %w", name, err)
			}
			spec.CPUs = cpus
		}

		specs = append(specs, spec)
	}

	// Order by startup tier (data stores before apps before the agent), then by
	// name, so apply brings dependencies up first and stays deterministic.
	sort.Slice(specs, func(i, j int) bool {
		ti, tj := service.TierOf(specs[i].Category), service.TierOf(specs[j].Category)
		if ti != tj {
			return ti < tj
		}
		return specs[i].Name < specs[j].Name
	})
	return specs, nil
}

// ServiceNames returns the declared service names (normalized).
func (f *File) ServiceNames() []string {
	out := make([]string, 0, len(f.Services))
	for _, s := range f.Services {
		out = append(out, engine.NormalizeName(firstNonEmpty(s.Name, s.Use)))
	}
	return out
}

func mergeServiceEnv(base, over map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
