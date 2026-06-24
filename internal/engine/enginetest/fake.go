// Package enginetest provides an in-memory engine.Runtime so CLI command logic
// can be exercised without Docker. It models just enough lifecycle behavior to
// drive the commands: create/start/stop/remove and label-derived listing.
package enginetest

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/raiyanyahya/llmaker/internal/engine"
)

// Fake is an in-memory Runtime.
type Fake struct {
	mu        sync.Mutex
	instances map[string]*engine.Instance
	logs      map[string]string

	// PingErr, when set, makes Ping fail (simulating a down daemon).
	PingErr error
	// CreateErr, when set, makes Create fail.
	CreateErr error
	// nextID generates deterministic-ish ids.
	nextID int
}

// New returns an empty fake runtime.
func New() *Fake {
	return &Fake{
		instances: map[string]*engine.Instance{},
		logs:      map[string]string{},
	}
}

// Seed inserts a pre-existing instance (e.g. to test ls/stop on a running set).
func (f *Fake) Seed(in engine.Instance) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := in
	f.instances[in.Name] = &cp
}

// SetLogs sets the canned log output for an instance.
func (f *Fake) SetLogs(name, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs[name] = content
}

func (f *Fake) Ping(ctx context.Context) error { return f.PingErr }

func (f *Fake) Create(ctx context.Context, spec engine.Spec) (engine.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CreateErr != nil {
		return engine.Instance{}, f.CreateErr
	}
	if _, ok := f.instances[spec.Name]; ok {
		return engine.Instance{}, engine.ErrAlreadyExists
	}
	f.nextID++
	host := spec.Host
	if host == "" {
		host = "127.0.0.1"
	}
	in := &engine.Instance{
		ID:      strings.Repeat("0", 8) + itoa(f.nextID),
		Name:    spec.Name,
		Backend: spec.Backend,
		Model:   spec.Model,
		Image:   spec.Image,
		Port:    spec.Port,
		Host:    host,
		State:   engine.StateCreated,
		Health:  engine.HealthUnknown,
		Created: time.Now(),
		Runtime: engine.RuntimeContainer,
	}
	f.instances[spec.Name] = in
	return *in, nil
}

func (f *Fake) Start(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	in, ok := f.instances[name]
	if !ok {
		return engine.ErrNotFound
	}
	in.State = engine.StateRunning
	return nil
}

func (f *Fake) Stop(ctx context.Context, name string, timeout time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	in, ok := f.instances[name]
	if !ok {
		return engine.ErrNotFound
	}
	in.State = engine.StateExited
	return nil
}

func (f *Fake) Remove(ctx context.Context, name string, force bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	in, ok := f.instances[name]
	if !ok {
		return engine.ErrNotFound
	}
	if in.State == engine.StateRunning && !force {
		return engine.ErrAlreadyExists // reuse as "running, won't remove"
	}
	delete(f.instances, name)
	return nil
}

func (f *Fake) List(ctx context.Context) ([]engine.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]engine.Instance, 0, len(f.instances))
	for _, in := range f.instances {
		out = append(out, *in)
	}
	return out, nil
}

func (f *Fake) Get(ctx context.Context, name string) (engine.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	in, ok := f.instances[name]
	if !ok {
		return engine.Instance{}, engine.ErrNotFound
	}
	return *in, nil
}

func (f *Fake) Logs(ctx context.Context, name string, follow bool) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.instances[name]; !ok {
		return nil, engine.ErrNotFound
	}
	return io.NopCloser(strings.NewReader(f.logs[name])), nil
}

func (f *Fake) Close() error { return nil }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}

var _ engine.Runtime = (*Fake)(nil)
