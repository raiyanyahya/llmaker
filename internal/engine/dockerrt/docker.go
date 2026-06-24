// Package dockerrt is the production engine.Runtime: a thin, opinionated wrapper
// over the Docker Go SDK. It is the ONLY package that imports the Docker SDK, so
// the rest of llmaker stays decoupled from Docker and easy to test. The CLI's
// fleet is tracked entirely through container labels — there is no local state
// file to drift out of sync.
package dockerrt

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	"github.com/raiyanyahya/llmaker/internal/backend"
	"github.com/raiyanyahya/llmaker/internal/engine"
)

// Runtime implements engine.Runtime (and engine.ImagePuller) using Docker.
type Runtime struct {
	cli *client.Client
}

// New connects to Docker using the standard environment (DOCKER_HOST, etc.) and
// negotiates the API version so llmaker works across daemon versions.
func New() (*Runtime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("connect to Docker: %w", err)
	}
	return &Runtime{cli: cli}, nil
}

// Ping verifies the daemon is reachable.
func (r *Runtime) Ping(ctx context.Context) error {
	_, err := r.cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("Docker daemon not responding: %w", err)
	}
	return nil
}

// Close releases the Docker client.
func (r *Runtime) Close() error { return r.cli.Close() }

// Create provisions the model volume and the container, stamped with llmaker's
// labels, resource limits, port mapping and (optionally) GPU reservation.
func (r *Runtime) Create(ctx context.Context, spec engine.Spec) (engine.Instance, error) {
	cname := engine.ContainerName(spec.Name)
	labels := engine.SpecLabels(spec, spec.Image, spec.Port)

	// Persisted model cache volume.
	volName := engine.VolumeName(spec.Name)
	if _, err := r.cli.VolumeCreate(ctx, volume.CreateOptions{Name: volName, Labels: labels}); err != nil {
		return engine.Instance{}, fmt.Errorf("create volume: %w", err)
	}

	facadePort := nat.Port(fmt.Sprintf("%d/tcp", backend.FacadePort))
	hostIP := spec.Host
	if hostIP == "" {
		hostIP = "127.0.0.1"
	}

	cfg := &container.Config{
		Image:        spec.Image,
		Env:          envSlice(spec.Env),
		Labels:       labels,
		ExposedPorts: nat.PortSet{facadePort: struct{}{}},
	}

	hostCfg := &container.HostConfig{
		PortBindings:  nat.PortMap{facadePort: []nat.PortBinding{{HostIP: hostIP, HostPort: strconv.Itoa(spec.Port)}}},
		Binds:         []string{volName + ":" + modelMountPath(spec.Backend)},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
		Resources:     resources(spec),
	}

	created, err := r.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, cname)
	if err != nil {
		return engine.Instance{}, fmt.Errorf("create container: %w", err)
	}

	inst := engine.InstanceFromLabels(created.ID, engine.StateCreated, labels)
	return inst, nil
}

// Start starts a previously created instance.
func (r *Runtime) Start(ctx context.Context, name string) error {
	if err := r.cli.ContainerStart(ctx, engine.ContainerName(name), container.StartOptions{}); err != nil {
		return mapErr(name, err)
	}
	return nil
}

// Stop gracefully stops an instance.
func (r *Runtime) Stop(ctx context.Context, name string, timeout time.Duration) error {
	secs := int(timeout.Seconds())
	if err := r.cli.ContainerStop(ctx, engine.ContainerName(name), container.StopOptions{Timeout: &secs}); err != nil {
		return mapErr(name, err)
	}
	return nil
}

// Remove deletes the container and its model volume.
func (r *Runtime) Remove(ctx context.Context, name string, force bool) error {
	cname := engine.ContainerName(name)
	if err := r.cli.ContainerRemove(ctx, cname, container.RemoveOptions{Force: force, RemoveVolumes: false}); err != nil {
		return mapErr(name, err)
	}
	// Named volume isn't removed with the container; clean it up best-effort.
	_ = r.cli.VolumeRemove(ctx, engine.VolumeName(name), true)
	return nil
}

// List returns every llmaker-managed instance (running or not).
func (r *Runtime) List(ctx context.Context) ([]engine.Instance, error) {
	key, val := engine.ManagedFilter()
	f := filters.NewArgs(filters.Arg("label", key+"="+val))
	summaries, err := r.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	out := make([]engine.Instance, 0, len(summaries))
	for _, s := range summaries {
		inst := engine.InstanceFromLabels(s.ID, mapState(s.State), s.Labels)
		if inst.Created.IsZero() && s.Created > 0 {
			inst.Created = time.Unix(s.Created, 0)
		}
		out = append(out, inst)
	}
	return out, nil
}

// Get inspects a single managed instance.
func (r *Runtime) Get(ctx context.Context, name string) (engine.Instance, error) {
	info, err := r.cli.ContainerInspect(ctx, engine.ContainerName(name))
	if err != nil {
		if client.IsErrNotFound(err) {
			return engine.Instance{}, engine.ErrNotFound
		}
		return engine.Instance{}, err
	}
	state := engine.StateUnknown
	if info.State != nil {
		state = mapState(info.State.Status)
	}
	inst := engine.InstanceFromLabels(info.ID, state, info.Config.Labels)
	if created, perr := time.Parse(time.RFC3339Nano, info.Created); perr == nil && inst.Created.IsZero() {
		inst.Created = created
	}
	return inst, nil
}

// Logs streams an instance's combined, demultiplexed stdout/stderr.
func (r *Runtime) Logs(ctx context.Context, name string, follow bool) (io.ReadCloser, error) {
	raw, err := r.cli.ContainerLogs(ctx, engine.ContainerName(name), container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: follow, Tail: "all",
	})
	if err != nil {
		return nil, mapErr(name, err)
	}
	// Docker multiplexes stdout/stderr on one stream; demux into a clean pipe.
	pr, pw := io.Pipe()
	go func() {
		_, copyErr := stdcopy.StdCopy(pw, pw, raw)
		_ = raw.Close()
		_ = pw.CloseWithError(copyErr)
	}()
	return readClposer{Reader: pr, closer: func() error { return raw.Close() }}, nil
}

// PullImage implements a "pull if missing" policy: an image already present
// locally is used as-is, so locally-built or custom `--image` builds work
// without a registry; otherwise it's pulled with streamed progress.
func (r *Runtime) PullImage(ctx context.Context, ref string, onEvent func(string)) error {
	if r.imageExists(ctx, ref) {
		if onEvent != nil {
			onEvent("using local image")
		}
		return nil
	}
	rc, err := r.cli.ImagePull(ctx, ref, imagetypes.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", ref, err)
	}
	defer rc.Close()
	return decodePullStream(rc, onEvent)
}

// imageExists reports whether an image reference is present in the local daemon.
func (r *Runtime) imageExists(ctx context.Context, ref string) bool {
	_, _, err := r.cli.ImageInspectWithRaw(ctx, ref)
	return err == nil
}

// --- helpers ---

func resources(spec engine.Spec) container.Resources {
	res := container.Resources{}
	if spec.Memory > 0 {
		res.Memory = spec.Memory
	}
	if spec.CPUs > 0 {
		res.NanoCPUs = int64(spec.CPUs * 1e9)
	}
	if spec.GPU {
		res.DeviceRequests = []container.DeviceRequest{{
			Driver:       "nvidia",
			Count:        -1, // all GPUs
			Capabilities: [][]string{{"gpu"}},
		}}
	}
	return res
}

func envSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func modelMountPath(b backend.Kind) string {
	if b == backend.LlamaCPP {
		return "/models"
	}
	return "/root/.ollama"
}

// mapState normalizes Docker's state strings to engine.State.
func mapState(s string) engine.State {
	switch s {
	case "running":
		return engine.StateRunning
	case "exited", "dead", "removing":
		return engine.StateExited
	case "created":
		return engine.StateCreated
	case "paused":
		return engine.StatePaused
	default:
		return engine.StateUnknown
	}
}

func mapErr(name string, err error) error {
	if client.IsErrNotFound(err) {
		return engine.ErrNotFound
	}
	return fmt.Errorf("%s: %w", name, err)
}

// readClposer adapts a reader plus a close func into an io.ReadCloser.
type readClposer struct {
	io.Reader
	closer func() error
}

func (rc readClposer) Close() error { return rc.closer() }

var (
	_ engine.Runtime     = (*Runtime)(nil)
	_ engine.ImagePuller = (*Runtime)(nil)
)
