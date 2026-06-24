package engine

import (
	"strconv"
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
	return map[string]string{
		LabelManagedBy: ManagedByValue,
		LabelName:      s.Name,
		LabelBackend:   string(s.Backend),
		LabelModel:     s.Model,
		LabelImage:     image,
		LabelPort:      strconv.Itoa(port),
		LabelHost:      host,
		LabelRuntime:   string(rt),
		LabelCreated:   time.Now().UTC().Format(time.RFC3339),
	}
}

// ManagedFilter is the label selector that matches only llmaker's own objects.
func ManagedFilter() (key, value string) {
	return LabelManagedBy, ManagedByValue
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
