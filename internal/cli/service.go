package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/raiyanyahya/llmaker/internal/engine"
	"github.com/raiyanyahya/llmaker/internal/service"
	"github.com/raiyanyahya/llmaker/internal/ui"
)

const serviceHealthTimeout = 120 * time.Second

func newServiceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service",
		Aliases: []string{"svc", "services"},
		Short:   "Run stack services (vector DBs, cache, embeddings, observability)",
		GroupID: groupFleet,
		Long: "Run the infrastructure around your LLMs — vector databases, caches,\n" +
			"embedding servers, and observability — as managed containers on a shared\n" +
			"network, so an app can reach both `qdrant:6333` and `chat:8080` by name.",
	}
	cmd.AddCommand(
		newServiceCatalogCmd(app),
		newServiceAddCmd(app),
		newServiceLsCmd(app),
		newServiceRmCmd(app),
		newServiceStopCmd(app),
		newServiceStartCmd(app),
	)
	return cmd
}

// --- catalog ---

func newServiceCatalogCmd(app *App) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "List the services llmaker can run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			io := app.IO
			t := io.Theme
			if asJSON {
				enc := json.NewEncoder(io.Out)
				enc.SetIndent("", "  ")
				return enc.Encode(service.All())
			}
			tbl := t.NewTable("SERVICE", "CATEGORY", "IMAGE", "DESCRIPTION")
			for _, s := range service.All() {
				tbl.Row(s.Kind, string(s.Category), s.Image, ui.Truncate(s.Description, 44))
			}
			io.Println(tbl.Render())
			io.Println()
			io.Println(t.Muted.Render("Start one with ") + t.Accent.Render("llmaker service add <service>"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

// --- add ---

type serviceAddOptions struct {
	name    string
	port    int
	host    string
	image   string
	memory  string
	cpus    float64
	env     map[string]string
	noWait  bool
	timeout time.Duration
}

func newServiceAddCmd(app *App) *cobra.Command {
	var opts serviceAddOptions
	cmd := &cobra.Command{
		Use:       "add <service> [name]",
		Short:     "Create and start a service from the catalog",
		Args:      cobra.RangeArgs(1, 2),
		ValidArgs: service.Names(),
		Example: "  llmaker service add qdrant                 # default name = qdrant\n" +
			"  llmaker service add pgvector store         # custom name\n" +
			"  llmaker service add embeddings --env MODEL_ID=BAAI/bge-large-en-v1.5",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				opts.name = args[1]
			}
			return runServiceAdd(cmd.Context(), app, args[0], opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.name, "name", "", "service name (default: the catalog kind)")
	f.IntVar(&opts.port, "port", 0, "host port for the primary port (default: auto-assigned)")
	f.StringVar(&opts.host, "host", "127.0.0.1", "host address to bind to")
	f.StringVar(&opts.image, "image", "", "override the service image (advanced)")
	f.StringVar(&opts.memory, "memory", "", "memory limit, e.g. 2g")
	f.Float64Var(&opts.cpus, "cpus", 0, "CPU quota")
	f.StringToStringVar(&opts.env, "env", nil, "extra env vars (repeatable), e.g. --env KEY=VALUE")
	f.BoolVar(&opts.noWait, "no-wait", false, "don't wait for the service to accept connections")
	f.DurationVar(&opts.timeout, "timeout", serviceHealthTimeout, "how long to wait for readiness")
	return cmd
}

func runServiceAdd(ctx context.Context, app *App, kind string, opts serviceAddOptions) error {
	cat, err := service.Get(kind)
	if err != nil {
		return err
	}

	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	instances, err := rt.List(ctx)
	if err != nil {
		return err
	}
	services, err := rt.ListServices(ctx)
	if err != nil {
		return err
	}

	name := engine.NormalizeName(firstNonEmpty(opts.name, cat.Kind))
	if !engine.ValidName(name) {
		return fmt.Errorf("invalid name %q (use lowercase letters, digits, - or _)", name)
	}
	if nameTaken(name, instances, services) {
		return fmt.Errorf("a service or instance named %q already exists (pass a different name)", name)
	}

	host := firstNonEmpty(opts.host, "127.0.0.1")
	used := usedPorts(instances, services)
	ports, err := allocateServicePorts(cat, opts.port, used)
	if err != nil {
		return err
	}

	mem, err := resolveMemory(opts.memory, app.Host)
	if err != nil {
		return err
	}

	spec := engine.ServiceSpec{
		Name:     name,
		Service:  cat.Kind,
		Category: string(cat.Category),
		Image:    firstNonEmpty(opts.image, cat.Image),
		Ports:    ports,
		Host:     host,
		Env:      mergeEnv(cat.Env, opts.env),
		Volumes:  serviceVolumes(name, cat),
		Memory:   mem,
		CPUs:     opts.cpus,
	}

	io := app.IO
	t := io.Theme
	io.Println(t.InfoLine(fmt.Sprintf("Starting service %s (%s)", t.Value.Render(name), cat.DisplayName)))
	if cat.Notes != "" {
		io.Println(t.Muted.Render("  note: " + cat.Notes))
	}

	if err := provisionService(ctx, app, rt, spec, opts.timeout, !opts.noWait); err != nil {
		return err
	}

	printServiceCard(app, name, cat, spec)
	return nil
}

// --- ls ---

func newServiceLsCmd(app *App) *cobra.Command {
	var asJSON, quiet bool
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list", "ps"},
		Short:   "List running services",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, cleanup, err := app.runtime(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			svcs, err := app.listServices(cmd.Context(), rt, !quiet)
			if err != nil {
				return err
			}
			io := app.IO
			t := io.Theme
			if asJSON {
				enc := json.NewEncoder(io.Out)
				enc.SetIndent("", "  ")
				return enc.Encode(servicesJSON(svcs))
			}
			if quiet {
				for _, s := range svcs {
					io.Println(s.Name)
				}
				return nil
			}
			if len(svcs) == 0 {
				io.Println(t.Muted.Render("No services yet. Start one with ") + t.Accent.Render("llmaker service add qdrant") + t.Muted.Render("."))
				return nil
			}
			io.Println(renderServiceTable(t, svcs))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "only print service names")
	return cmd
}

// --- rm / stop / start ---

func newServiceRmCmd(app *App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "rm <name>...",
		Short: "Remove services (and their data volumes)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return forEachService(cmd.Context(), app, args, "Removing", func(ctx context.Context, rt engine.Runtime, name string) error {
				return rt.Remove(ctx, name, force)
			})
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "remove even if running")
	return cmd
}

func newServiceStopCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>...",
		Short: "Stop running services",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return forEachService(cmd.Context(), app, args, "Stopping", func(ctx context.Context, rt engine.Runtime, name string) error {
				return rt.Stop(ctx, name, engine.DefaultStopTimeout)
			})
		},
	}
}

func newServiceStartCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>...",
		Short: "Start stopped services",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return forEachService(cmd.Context(), app, args, "Starting", func(ctx context.Context, rt engine.Runtime, name string) error {
				return rt.Start(ctx, name)
			})
		},
	}
}

// forEachService applies a lifecycle action to each named service, resolving the
// name to a real service first so a typo gives a friendly error.
func forEachService(ctx context.Context, app *App, names []string, verb string, action func(context.Context, engine.Runtime, string) error) error {
	rt, cleanup, err := app.runtime(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	var errs []error
	for _, raw := range names {
		name := engine.NormalizeName(raw)
		if _, gerr := rt.GetService(ctx, name); gerr != nil {
			errs = append(errs, fmt.Errorf("no service named %q (try `llmaker service ls`)", name))
			continue
		}
		if aerr := app.step(verb+" "+name, func() error { return action(ctx, rt, name) }); aerr != nil {
			errs = append(errs, aerr)
		}
	}
	return joinErrs(errs)
}

// --- provisioning ---

// provisionService brings a service from nothing to running: pull image →
// create → start → (optionally) wait until its primary port accepts
// connections. Shared by `service add` and `apply`.
func provisionService(ctx context.Context, app *App, rt engine.Runtime, spec engine.ServiceSpec, timeout time.Duration, wait bool) error {
	io := app.IO
	t := io.Theme

	if puller, ok := rt.(engine.ImagePuller); ok {
		sp := t.NewSpinner(io.Out, "Pulling image "+spec.Image)
		sp.Start()
		perr := puller.PullImage(ctx, spec.Image, func(line string) {
			if line != "" {
				sp.SetLabel("Pulling image — " + line)
			}
		})
		if perr != nil {
			sp.Stop(t.FailLine("Pulling image — " + perr.Error()))
			return perr
		}
		sp.Stop(t.SuccessLine("Image ready"))
	}

	if _, err := stepf(app, "Creating "+spec.Name, func() (engine.Service, error) {
		return rt.CreateService(ctx, spec)
	}); err != nil {
		return err
	}

	if err := app.step("Starting "+spec.Name, func() error {
		return rt.Start(ctx, spec.Name)
	}); err != nil {
		_ = rt.Remove(ctx, spec.Name, true)
		return err
	}

	if wait {
		primary := primaryHostPort(spec.Ports)
		if err := app.step("Waiting for "+spec.Name, func() error {
			return app.waitServiceReady(ctx, spec.Host, primary, timeout)
		}); err != nil {
			return fmt.Errorf("%w\n  Inspect logs with `llmaker logs %s`", err, spec.Name)
		}
	}
	return nil
}

// waitPortReady polls until a TCP connection to host:port succeeds or the
// timeout elapses — a backend-agnostic readiness check for services that don't
// share the facade's /api/health contract.
func waitPortReady(ctx context.Context, host string, port int, timeout time.Duration) error {
	if port == 0 {
		return nil
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service did not start accepting connections within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// --- helpers ---

// listServices returns managed services sorted by name, optionally probing each
// running service's primary port so `service ls` can color readiness.
func (a *App) listServices(ctx context.Context, rt engine.Runtime, withHealth bool) ([]engine.Service, error) {
	svcs, err := rt.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(svcs, func(i, j int) bool { return svcs[i].Name < svcs[j].Name })
	if withHealth {
		a.enrichServiceHealth(ctx, svcs)
	}
	return svcs, nil
}

// enrichServiceHealth probes each running service's primary port concurrently,
// annotating Service.Health in place. A single quick dial decides ready vs.
// still-starting (it does not loop, so ls/top stay snappy).
func (a *App) enrichServiceHealth(ctx context.Context, svcs []engine.Service) {
	var wg sync.WaitGroup
	for i := range svcs {
		if !svcs[i].IsRunning() {
			svcs[i].Health = engine.HealthUnhealthy
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if a.waitServiceReady(ctx, svcs[i].Host, svcs[i].PrimaryPort(), time.Second) == nil {
				svcs[i].Health = engine.HealthHealthy
			} else {
				svcs[i].Health = engine.HealthStarting
			}
		}(i)
	}
	wg.Wait()
}

func renderServiceTable(t *ui.Theme, svcs []engine.Service) string {
	tbl := t.NewTable("", "NAME", "SERVICE", "CATEGORY", "STATE", "HEALTH", "ENDPOINT", "URL", "UPTIME")
	for _, s := range svcs {
		tbl.Row(
			t.Dot(healthLevel(s.Health)),
			s.Name,
			s.Kind,
			s.Category,
			t.Badge(string(s.State), stateLevel(s.State)),
			healthLabel(s.Health),
			s.Endpoint(),
			s.URL(),
			ui.HumanDuration(s.Uptime()),
		)
	}
	return tbl.Render()
}

// serviceJSON is the stable JSON shape for `service ls --json`.
type serviceJSON struct {
	Name     string `json:"name"`
	Service  string `json:"service"`
	Category string `json:"category"`
	State    string `json:"state"`
	Health   string `json:"health"`
	Endpoint string `json:"endpoint"`
	URL      string `json:"url"`
	Image    string `json:"image"`
}

func servicesJSON(svcs []engine.Service) []serviceJSON {
	out := make([]serviceJSON, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, serviceJSON{
			Name: s.Name, Service: s.Kind, Category: s.Category,
			State: string(s.State), Health: healthLabel(s.Health),
			Endpoint: s.Endpoint(), URL: s.URL(), Image: s.Image,
		})
	}
	return out
}

func printServiceCard(app *App, name string, cat service.Service, spec engine.ServiceSpec) {
	io := app.IO
	t := io.Theme
	primary := primaryHostPort(spec.Ports)
	url := (engine.Service{Host: spec.Host, Ports: spec.Ports}).URL()
	endpoint := (engine.Service{Name: name, Ports: spec.Ports}).Endpoint()
	io.Println()
	io.Println(t.Card("✓ Service ready", [][2]string{
		{"name", name},
		{"service", cat.DisplayName},
		{"url", url},
		{"in-network", endpoint},
		{"port", strconv.Itoa(primary)},
	}, true))
	io.Println()
	io.Println(t.Muted.Render("Reachable from other llmaker containers as ") + t.Accent.Render(endpoint) + t.Muted.Render("."))
}

// allocateServicePorts assigns a host port to each of the catalog's ports,
// honoring an explicit --port for the primary one. Primary is allocated first so
// it lands on the lowest free port (friendliest URL).
func allocateServicePorts(cat service.Service, primaryOverride int, used map[int]bool) ([]engine.PortBinding, error) {
	ordered := append([]service.Port(nil), cat.Ports...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Primary && !ordered[j].Primary })

	out := make([]engine.PortBinding, 0, len(ordered))
	for _, p := range ordered {
		var host int
		if p.Primary && primaryOverride > 0 {
			if used[primaryOverride] || !engine.PortAvailable(primaryOverride) {
				return nil, fmt.Errorf("port %d is not available", primaryOverride)
			}
			host = primaryOverride
		} else {
			h, err := engine.AllocatePort(used)
			if err != nil {
				return nil, err
			}
			host = h
		}
		used[host] = true
		out = append(out, engine.PortBinding{
			Host: host, Container: p.Container, Name: p.Name, Primary: p.Primary,
		})
	}
	return out, nil
}

func serviceVolumes(name string, cat service.Service) []engine.VolumeBinding {
	out := make([]engine.VolumeBinding, 0, len(cat.Volumes))
	for _, v := range cat.Volumes {
		out = append(out, engine.VolumeBinding{
			Name: engine.ServiceVolumeName(name, v.Suffix),
			Path: v.Path,
		})
	}
	return out
}

func primaryHostPort(ports []engine.PortBinding) int {
	for _, p := range ports {
		if p.Primary {
			return p.Host
		}
	}
	if len(ports) > 0 {
		return ports[0].Host
	}
	return 0
}

// usedPorts collects every host port currently claimed by instances and
// services, so a new allocation never collides.
func usedPorts(instances []engine.Instance, services []engine.Service) map[int]bool {
	used := engine.UsedPorts(instances)
	for _, s := range services {
		for _, p := range s.Ports {
			if p.Host > 0 {
				used[p.Host] = true
			}
		}
	}
	return used
}

func nameTaken(name string, instances []engine.Instance, services []engine.Service) bool {
	for _, in := range instances {
		if in.Name == name {
			return true
		}
	}
	for _, s := range services {
		if s.Name == name {
			return true
		}
	}
	return false
}

func mergeEnv(base, over map[string]string) map[string]string {
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
