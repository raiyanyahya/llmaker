package engine

import (
	"fmt"
	"time"

	"github.com/raiyanyahya/llmaker/internal/backend"
)

// Health is the readiness of an instance's facade, as observed by the CLI.
type Health string

const (
	HealthUnknown   Health = "unknown"
	HealthStarting  Health = "starting"
	HealthHealthy   Health = "healthy"
	HealthUnhealthy Health = "unhealthy"
)

// State is the lifecycle state of the underlying container/process.
type State string

const (
	StateCreated State = "created"
	StateRunning State = "running"
	StateExited  State = "exited"
	StatePaused  State = "paused"
	StateUnknown State = "unknown"
)

// Instance is the observed state of a managed LLM server.
type Instance struct {
	ID      string
	Name    string
	Backend backend.Kind
	Model   string
	Image   string
	Port    int
	Host    string
	State   State
	Health  Health
	Created time.Time
	Runtime RuntimeKind
	Stack   string // the named stack this resource belongs to (empty if standalone)
}

// URL is the base address of the instance's facade as reachable from this host.
func (i Instance) URL() string {
	host := i.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", host, i.Port)
}

// IsRunning reports whether the container/process is up (regardless of facade
// readiness).
func (i Instance) IsRunning() bool { return i.State == StateRunning }

// Uptime returns how long the instance has existed. It is a coarse proxy when
// the runtime cannot report a precise start time.
func (i Instance) Uptime() time.Duration {
	if i.Created.IsZero() {
		return 0
	}
	return time.Since(i.Created)
}
