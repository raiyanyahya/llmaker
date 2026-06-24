// Package engine models an llmaker instance and the runtime that brings it to
// life. The Runtime interface is the single seam between llmaker's orchestration
// logic and Docker: production code wires a Docker-backed implementation
// (internal/engine/dockerrt), while tests use an in-memory fake
// (internal/engine/enginetest). Nothing in this package imports the Docker SDK,
// which keeps the domain logic fast to compile and trivial to test.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/raiyanyahya/llmaker/internal/backend"
)

// ErrNotFound is returned by a Runtime when no managed instance matches a name.
var ErrNotFound = errors.New("instance not found")

// ErrAlreadyExists is returned when creating an instance whose name is taken.
var ErrAlreadyExists = errors.New("instance already exists")

// RuntimeKind distinguishes a containerized instance from a host-native process.
type RuntimeKind string

const (
	// RuntimeContainer runs the backend + facade inside a Docker container.
	RuntimeContainer RuntimeKind = "container"
	// RuntimeNative runs a host-native backend process (e.g. Metal on macOS),
	// still wrapped by the same facade contract.
	RuntimeNative RuntimeKind = "native"
)

// Spec is the desired state for a single instance. The CLI builds it from flags
// (or the wizard / llm.yaml) and hands it to a Runtime.
type Spec struct {
	Name    string
	Backend backend.Kind
	Model   string
	Image   string
	Memory  int64 // bytes; 0 means "no limit"
	CPUs    float64
	GPU     bool
	Port    int    // host port; 0 means "auto-allocate"
	Host    string // host bind address; defaults to 127.0.0.1
	Env     map[string]string
	Runtime RuntimeKind
}

// Runtime is the orchestration backend. Implementations must be safe to call
// with a cancelable context and should map their own primitives onto these
// logical, name-addressed operations.
type Runtime interface {
	// Create provisions (but does not necessarily start) an instance.
	Create(ctx context.Context, spec Spec) (Instance, error)
	// Start brings a created/stopped instance up.
	Start(ctx context.Context, name string) error
	// Stop gracefully stops a running instance, killing it after timeout.
	Stop(ctx context.Context, name string, timeout time.Duration) error
	// Remove deletes an instance (and its volumes). force stops it first.
	Remove(ctx context.Context, name string, force bool) error
	// List returns every llmaker-managed instance.
	List(ctx context.Context) ([]Instance, error)
	// Get returns a single managed instance by name.
	Get(ctx context.Context, name string) (Instance, error)
	// Logs streams an instance's combined stdout/stderr.
	Logs(ctx context.Context, name string, follow bool) (io.ReadCloser, error)
	// Ping verifies the runtime backend is reachable (e.g. Docker daemon up).
	Ping(ctx context.Context) error
	// Close releases any resources held by the runtime client.
	Close() error
}

// ImagePuller is an optional capability: a Runtime that can report image-pull
// progress implements it so `llmaker up` can render a live status line.
type ImagePuller interface {
	// PullImage pulls ref, invoking onEvent for human-readable progress lines.
	PullImage(ctx context.Context, ref string, onEvent func(string)) error
}

// DefaultStopTimeout is how long to wait for graceful shutdown before killing.
const DefaultStopTimeout = 10 * time.Second

// ContainerName returns the Docker object name for a logical instance name.
// Centralizing the prefix keeps the user-facing name clean while avoiding
// collisions with unmanaged containers.
func ContainerName(instance string) string {
	return "llmaker-" + instance
}

// VolumeName returns the model-cache volume name for an instance.
func VolumeName(instance string) string {
	return fmt.Sprintf("llmaker-%s-models", instance)
}
