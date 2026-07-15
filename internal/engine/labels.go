package engine

import (
	"strconv"
	"strings"
	"time"

	"github.com/raiyanyahya/llmaker/internal/backend"
)

// llmaker tracks its fleet purely through Docker labels rather than a local
// state file, so `llmaker ls` can never drift out of sync with reality: the
// container set *is* the source of truth. These are the label keys.
const (
	LabelManagedBy = "managed-by"
	ManagedByValue = "llmaker"

	LabelName    = "llmaker.name"
	LabelBackend = "llmaker.backend"
	LabelModel   = "llmaker.model"
	LabelImage   = "llmaker.image"
	LabelPort    = "llmaker.port"
	LabelHost    = "llmaker.host"
	LabelRuntime = "llmaker.runtime"
	LabelCreated = "llmaker.created"
	// LabelStack groups resources created together from a named stack file, so
	// `apply --prune` can be scoped to that stack instead of the whole fleet.
	LabelStack = "llmaker.stack"
	// LabelAuth records whether the instance was created with an API key
	// (AuthKey) or without (AuthNone), so lifecycle commands can re-warn about
	// a public keyless bind on every boot without inspecting container env.
	// Absent on instances created before the label existed.
	LabelAuth = "llmaker.auth"
	// LabelNetwork is the logical group network the container joined (absent =
	// the shared llmaker network), so `ls` and network GC can see the boundary
	// without inspecting Docker's network attachments.
	LabelNetwork = "llmaker.network"
	// LabelGPUs records the instance's GPU reservation — "all" or a comma list
	// of device ids — so the allocator can account existing claims from labels
	// alone. Absent when the instance reserved no GPUs (or predates the label).
	LabelGPUs = "llmaker.gpus"

	// LabelType distinguishes an LLM instance from an infrastructure service.
	// It is absent on instances created before services existed, which is why
	// a missing value reads as TypeInstance.
	LabelType = "llmaker.type"
	// LabelService is the catalog kind for a service (e.g. "qdrant").
	LabelService = "llmaker.service"
	// LabelCategory is the catalog category for a service (e.g. "vector-db").
	LabelCategory = "llmaker.category"
	// LabelPorts encodes a service's port bindings (instances use LabelPort).
	LabelPorts = "llmaker.ports"
)

// Managed object types, stored in LabelType.
const (
	TypeInstance = "instance"
	TypeService  = "service"
)

// Values stored in LabelAuth.
const (
	AuthKey  = "key"
	AuthNone = "none"
)

// SpecLabels builds the label set stamped onto an instance at creation time.
// image and port are passed explicitly because they may be resolved (image
// override, auto-allocated port) after the Spec is built.
func SpecLabels(s Spec, image string, port int) map[string]string {
	rt := s.Runtime
	if rt == "" {
		rt = RuntimeContainer
	}
	host := s.Host
	if host == "" {
		host = "127.0.0.1"
	}
	auth := AuthNone
	if strings.TrimSpace(s.Env["API_KEY"]) != "" {
		auth = AuthKey
	}
	m := map[string]string{
		LabelManagedBy: ManagedByValue,
		LabelType:      TypeInstance,
		LabelName:      s.Name,
		LabelBackend:   string(s.Backend),
		LabelModel:     s.Model,
		LabelImage:     image,
		LabelPort:      strconv.Itoa(port),
		LabelHost:      host,
		LabelRuntime:   string(rt),
		LabelAuth:      auth,
		LabelCreated:   time.Now().UTC().Format(time.RFC3339),
	}
	if s.Stack != "" {
		m[LabelStack] = s.Stack
	}
	if s.Network != "" {
		m[LabelNetwork] = s.Network
	}
	if len(s.GPUIDs) > 0 {
		m[LabelGPUs] = strings.Join(s.GPUIDs, ",")
	} else if s.GPU {
		m[LabelGPUs] = "all"
	}
	return m
}

// ServiceLabels builds the label set stamped onto a service container. Port
// bindings are encoded into a single label so ListServices can reconstruct them
// without inspecting the container's port map.
func ServiceLabels(s ServiceSpec) map[string]string {
	host := s.Host
	if host == "" {
		host = "127.0.0.1"
	}
	m := map[string]string{
		LabelManagedBy: ManagedByValue,
		LabelType:      TypeService,
		LabelName:      s.Name,
		LabelService:   s.Service,
		LabelCategory:  s.Category,
		LabelImage:     s.Image,
		LabelHost:      host,
		LabelPorts:     encodePorts(s.Ports),
		LabelCreated:   time.Now().UTC().Format(time.RFC3339),
	}
	if s.Stack != "" {
		m[LabelStack] = s.Stack
	}
	if s.Network != "" {
		m[LabelNetwork] = s.Network
	}
	return m
}

// ManagedFilter is the label selector that matches only llmaker's own objects.
func ManagedFilter() (key, value string) {
	return LabelManagedBy, ManagedByValue
}

// TypeOf reports whether a labeled container is an instance or a service,
// defaulting to TypeInstance when the label is absent (pre-services containers).
func TypeOf(labels map[string]string) string {
	if t := labels[LabelType]; t != "" {
		return t
	}
	return TypeInstance
}

// InstanceFromLabels reconstructs an Instance from the labels Docker reports
// back, plus the runtime-observed id/state. Missing or malformed labels degrade
// gracefully rather than erroring, so a partially-labeled container still shows
// up in `llmaker ls` instead of vanishing.
func InstanceFromLabels(id string, state State, labels map[string]string) Instance {
	inst := Instance{
		ID:      id,
		Name:    labels[LabelName],
		Backend: backend.Kind(labels[LabelBackend]),
		Model:   labels[LabelModel],
		Image:   labels[LabelImage],
		Host:    labels[LabelHost],
		State:   state,
		Health:  HealthUnknown,
		Runtime: RuntimeKind(labels[LabelRuntime]),
		Stack:   labels[LabelStack],
		Auth:    labels[LabelAuth],
		Network: labels[LabelNetwork],
		GPUs:    labels[LabelGPUs],
	}
	if inst.Runtime == "" {
		inst.Runtime = RuntimeContainer
	}
	if p, err := strconv.Atoi(labels[LabelPort]); err == nil {
		inst.Port = p
	}
	if ts, err := time.Parse(time.RFC3339, labels[LabelCreated]); err == nil {
		inst.Created = ts
	}
	return inst
}

// ServiceFromLabels reconstructs a Service from its container labels plus the
// runtime-observed id/state. Like InstanceFromLabels it degrades gracefully.
func ServiceFromLabels(id string, state State, labels map[string]string) Service {
	svc := Service{
		ID:       id,
		Name:     labels[LabelName],
		Kind:     labels[LabelService],
		Category: labels[LabelCategory],
		Image:    labels[LabelImage],
		Host:     labels[LabelHost],
		Ports:    decodePorts(labels[LabelPorts]),
		State:    state,
		Health:   HealthUnknown,
		Stack:    labels[LabelStack],
		Network:  labels[LabelNetwork],
	}
	if ts, err := time.Parse(time.RFC3339, labels[LabelCreated]); err == nil {
		svc.Created = ts
	}
	return svc
}

// Port bindings are encoded as "host:container:name:primary" tuples joined by
// ";", e.g. "11500:6333:http:1;11501:6334:grpc:0". This keeps a service's full
// port topology in a single label so it round-trips without container inspect.
func encodePorts(ports []PortBinding) string {
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		primary := "0"
		if p.Primary {
			primary = "1"
		}
		parts = append(parts, strings.Join([]string{
			strconv.Itoa(p.Host), strconv.Itoa(p.Container), p.Name, primary,
		}, ":"))
	}
	return strings.Join(parts, ";")
}

func decodePorts(s string) []PortBinding {
	if s == "" {
		return nil
	}
	var out []PortBinding
	for _, tok := range strings.Split(s, ";") {
		f := strings.Split(tok, ":")
		if len(f) < 4 {
			continue
		}
		host, err1 := strconv.Atoi(f[0])
		cont, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, PortBinding{
			Host: host, Container: cont, Name: f[2], Primary: f[3] == "1",
		})
	}
	return out
}
