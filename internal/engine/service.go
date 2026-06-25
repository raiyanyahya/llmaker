package engine

import (
	"fmt"
	"time"
)

// ServiceSpec is the desired state for a single infrastructure service (a
// vector DB, cache, embeddings server, …). It is the service-world analogue of
// Spec: the CLI builds it from the catalog + flags (or stack.yaml) and hands it
// to a Runtime. Unlike an LLM instance, a service has no facade contract — it is
// a plain container with one or more ports, optional volumes, and its own image
// health.
type ServiceSpec struct {
	Name     string
	Service  string // catalog kind, e.g. "qdrant"
	Category string
	Image    string
	Ports    []PortBinding
	Host     string // host bind address; defaults to 127.0.0.1
	Env      map[string]string
	Volumes  []VolumeBinding
	Memory   int64 // bytes; 0 means "no limit"
	CPUs     float64
	Stack    string // named stack this service belongs to (set by `apply`)
}

// PortBinding maps a container port to a host port.
type PortBinding struct {
	Host      int
	Container int
	Name      string // human label, e.g. "http"
	Primary   bool   // the main port users connect to
}

// VolumeBinding mounts a named Docker volume at a path inside the container.
type VolumeBinding struct {
	Name string // full Docker volume name
	Path string // mount path inside the container
}

// Service is the observed state of a managed infrastructure service, the
// service-world analogue of Instance.
type Service struct {
	ID       string
	Name     string
	Kind     string // catalog kind, e.g. "qdrant"
	Category string
	Image    string
	Host     string
	Ports    []PortBinding
	State    State
	Health   Health
	Created  time.Time
	Stack    string // the named stack this service belongs to (empty if standalone)
}

// PrimaryPort returns the host port users primarily connect to (the binding
// marked primary, else the first), or 0 if the service exposes none.
func (s Service) PrimaryPort() int {
	for _, p := range s.Ports {
		if p.Primary {
			return p.Host
		}
	}
	if len(s.Ports) > 0 {
		return s.Ports[0].Host
	}
	return 0
}

// URL is the address of the service's primary port as reachable from this host.
func (s Service) URL() string {
	host := s.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", host, s.PrimaryPort())
}

// Endpoint is how other containers reach this service over the llmaker network:
// the service name plus its primary container port (e.g. "qdrant:6333").
func (s Service) Endpoint() string {
	for _, p := range s.Ports {
		if p.Primary {
			return fmt.Sprintf("%s:%d", s.Name, p.Container)
		}
	}
	if len(s.Ports) > 0 {
		return fmt.Sprintf("%s:%d", s.Name, s.Ports[0].Container)
	}
	return s.Name
}

// IsRunning reports whether the service container is up.
func (s Service) IsRunning() bool { return s.State == StateRunning }

// Uptime returns how long the service has existed.
func (s Service) Uptime() time.Duration {
	if s.Created.IsZero() {
		return 0
	}
	return time.Since(s.Created)
}

// ServiceVolumeName returns the Docker volume name for a service's data volume.
func ServiceVolumeName(instance, suffix string) string {
	return fmt.Sprintf("llmaker-%s-%s", instance, suffix)
}
