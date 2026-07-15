package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/raiyanyahya/llmaker/internal/backend"
	"github.com/raiyanyahya/llmaker/internal/engine"
	"github.com/raiyanyahya/llmaker/internal/engine/enginetest"
	"github.com/raiyanyahya/llmaker/internal/facade"
	"github.com/raiyanyahya/llmaker/internal/facade/facadetest"
	"github.com/raiyanyahya/llmaker/internal/ui"
)

// testApp wires an App with in-memory fakes and a captured output buffer.
func testApp(t *testing.T) (*App, *enginetest.Fake, *facadetest.Fake, *bytes.Buffer) {
	t.Helper()
	rt := enginetest.New()
	fc := &facadetest.Fake{}
	var out bytes.Buffer
	theme := ui.NewTheme(&out, false) // plain output, no TTY
	io := &ui.IOStreams{Out: &out, Err: &out, In: strings.NewReader(""), Theme: theme}
	app := &App{
		IO:      io,
		Facade:  fc,
		Host:    engine.HostInfo{OS: "linux", Arch: "amd64", CPUs: 4, MemoryBytes: 8 * engine.GiB},
		Version: VersionInfo{Version: "test"},
		NewRuntime: func(context.Context) (engine.Runtime, error) {
			return rt, nil
		},
		OpenURL: func(string) error { return nil },
		// Skip real TCP readiness probes against the fake runtime.
		ServiceReady: func(context.Context, string, int, time.Duration) error { return nil },
	}
	return app, rt, fc, &out
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func seedRunning(rt *enginetest.Fake, name string) {
	rt.Seed(engine.Instance{
		Name: name, Backend: backend.Ollama, Model: "llama3:8b",
		Port: 11500, Host: "127.0.0.1", State: engine.StateRunning,
		Created: time.Now().Add(-time.Minute),
	})
}

func TestRunUpProvisionsInstance(t *testing.T) {
	app, rt, fc, out := testApp(t)
	opts := upOptions{
		name: "alpha", backendName: "ollama", model: "llama3:8b",
		healthTimeout: 5 * time.Second,
	}
	if err := runUp(context.Background(), app, opts, false); err != nil {
		t.Fatalf("runUp: %v", err)
	}

	in, err := rt.Get(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("instance not created: %v", err)
	}
	if !in.IsRunning() {
		t.Errorf("instance should be running, got %q", in.State)
	}
	if in.Port < engine.PortRangeStart || in.Port > engine.PortRangeEnd {
		t.Errorf("port %d not auto-allocated in range", in.Port)
	}
	if fc.CallCount("Pull:llama3:8b") != 1 {
		t.Errorf("expected model pull, calls=%v", fc.Calls)
	}
	if fc.CallCount("SetDefault:llama3:8b") != 1 {
		t.Errorf("expected SetDefault, calls=%v", fc.Calls)
	}
	if s := out.String(); !strings.Contains(s, "alpha") || !strings.Contains(s, "Instance ready") {
		t.Errorf("missing ready card:\n%s", s)
	}
}

func TestRunUpDefaultsModelFromBackend(t *testing.T) {
	app, rt, _, _ := testApp(t)
	if err := runUp(context.Background(), app, upOptions{name: "b", backendName: "ollama", healthTimeout: time.Second}, false); err != nil {
		t.Fatalf("runUp: %v", err)
	}
	in, _ := rt.Get(context.Background(), "b")
	if in.Model != backend.Default().DefaultModel {
		t.Errorf("model = %q, want backend default", in.Model)
	}
}

func TestUpCommandPresetResolves(t *testing.T) {
	app, rt, fc, _ := testApp(t)
	want, ok := backend.GetPreset("code")
	if !ok {
		t.Fatal("code preset should exist")
	}

	cmd := newUpCmd(app)
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"code", "--name", "c", "--timeout", "1s"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("up code: %v", err)
	}

	in, err := rt.Get(context.Background(), "c")
	if err != nil {
		t.Fatalf("instance not created: %v", err)
	}
	if in.Model != want.Model {
		t.Errorf("model = %q, want preset model %q", in.Model, want.Model)
	}
	if in.Backend != want.Backend {
		t.Errorf("backend = %q, want %q", in.Backend, want.Backend)
	}
	if fc.CallCount("Pull:"+want.Model) != 1 {
		t.Errorf("expected preset model pull, calls=%v", fc.Calls)
	}
}

func TestUpCommandPresetFlagOverride(t *testing.T) {
	app, rt, _, _ := testApp(t)
	cmd := newUpCmd(app)
	cmd.SilenceUsage = true
	// An explicit --model must win over the preset's model.
	cmd.SetArgs([]string{"chat", "--name", "c", "--model", "custom:1b", "--timeout", "1s"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("up chat --model: %v", err)
	}
	in, _ := rt.Get(context.Background(), "c")
	if in.Model != "custom:1b" {
		t.Errorf("model = %q, want explicit override custom:1b", in.Model)
	}
}

func TestUpCommandUnknownPreset(t *testing.T) {
	app, _, _, _ := testApp(t)
	cmd := newUpCmd(app)
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"bogus"})
	err := cmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unknown preset") {
		t.Fatalf("expected unknown-preset error, got %v", err)
	}
}

func TestRunUpRejectsDuplicateName(t *testing.T) {
	app, rt, _, _ := testApp(t)
	seedRunning(rt, "dup")
	err := runUp(context.Background(), app, upOptions{name: "dup", backendName: "ollama", healthTimeout: time.Second}, false)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate-name error, got %v", err)
	}
}

func TestFacadeEnv(t *testing.T) {
	env := facadeEnv(engine.Spec{
		Name: "a", Backend: backend.Ollama, Model: "m",
		Env: map[string]string{"API_KEY": "x", "KEEP_ALIVE": "10m"},
	})
	checks := map[string]string{
		"LLMAKER_BACKEND":       "ollama",
		"LLMAKER_NAME":          "a",
		"LLMAKER_DEFAULT_MODEL": "m",
		"FACADE_PORT":           "8080",
		"API_KEY":               "x",
		"KEEP_ALIVE":            "10m",
	}
	for k, want := range checks {
		if env[k] != want {
			t.Errorf("env[%q] = %q, want %q", k, env[k], want)
		}
	}

	// An explicit user value must win over the injected default.
	env2 := facadeEnv(engine.Spec{Name: "a", Backend: backend.Ollama, Env: map[string]string{"LLMAKER_NAME": "custom"}})
	if env2["LLMAKER_NAME"] != "custom" {
		t.Errorf("user override should win, got %q", env2["LLMAKER_NAME"])
	}
}

func TestIsLoopbackHost(t *testing.T) {
	cases := map[string]bool{
		"":            true, // engine defaults empty host to 127.0.0.1
		"localhost":   true,
		"LOCALHOST":   true,
		" 127.0.0.1 ": true,
		"127.0.0.1":   true,
		"127.9.9.9":   true, // all of 127/8 is loopback
		"::1":         true,
		"[::1]":       true,
		"::1%lo":      true, // zone suffix
		"0.0.0.0":     false,
		"::":          false,
		"192.168.1.5": false,
		"10.0.0.1":    false,
		"example.com": false, // unresolvable spellings warn (safe direction)
	}
	for in, want := range cases {
		if got := isLoopbackHost(in); got != want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRunLsTableAndJSON(t *testing.T) {
	app, rt, _, out := testApp(t)
	seedRunning(rt, "alpha")
	seedRunning(rt, "beta")

	if err := runLs(context.Background(), app, lsOptions{}); err != nil {
		t.Fatalf("runLs: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "alpha") || !strings.Contains(s, "beta") || !strings.Contains(s, "NAME") {
		t.Errorf("table missing content:\n%s", s)
	}

	out.Reset()
	if err := runLs(context.Background(), app, lsOptions{json: true}); err != nil {
		t.Fatalf("runLs json: %v", err)
	}
	var rows []instanceJSON
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Health != string(engine.HealthHealthy) {
		t.Errorf("expected healthy (fake facade answers), got %q", rows[0].Health)
	}
}

func TestRunLsQuiet(t *testing.T) {
	app, rt, _, out := testApp(t)
	seedRunning(rt, "alpha")
	if err := runLs(context.Background(), app, lsOptions{quiet: true}); err != nil {
		t.Fatalf("runLs: %v", err)
	}
	if strings.TrimSpace(out.String()) != "alpha" {
		t.Errorf("quiet output = %q", out.String())
	}
}

func TestLifecycleStopStartRemove(t *testing.T) {
	app, rt, _, _ := testApp(t)
	seedRunning(rt, "x")

	if err := runLifecycle(context.Background(), app, []string{"x"}, "Stopping", func(ctx context.Context, rt engine.Runtime, in engine.Instance) error {
		return rt.Stop(ctx, in.Name, engine.DefaultStopTimeout)
	}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if in, _ := rt.Get(context.Background(), "x"); in.State != engine.StateExited {
		t.Errorf("state after stop = %q", in.State)
	}

	if err := runLifecycle(context.Background(), app, []string{"x"}, "Starting", func(ctx context.Context, rt engine.Runtime, in engine.Instance) error {
		return rt.Start(ctx, in.Name)
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if in, _ := rt.Get(context.Background(), "x"); in.State != engine.StateRunning {
		t.Errorf("state after start = %q", in.State)
	}
}

func TestRestartInstance(t *testing.T) {
	app, rt, _, _ := testApp(t)
	seedRunning(rt, "x")
	cmd := newRestartCmd(app)
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"x"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if in, _ := rt.Get(context.Background(), "x"); in.State != engine.StateRunning {
		t.Errorf("state after restart = %q, want running", in.State)
	}
}

func TestRmRunningRequiresForce(t *testing.T) {
	app, rt, _, _ := testApp(t)
	seedRunning(rt, "x")

	// Without force: refused, instance still present.
	if err := runRm(context.Background(), app, []string{"x"}, false); err == nil {
		t.Fatal("expected error removing a running instance without --force")
	}
	if _, err := rt.Get(context.Background(), "x"); err != nil {
		t.Fatal("instance should still exist after refused rm")
	}

	// With force: removed.
	if err := runRm(context.Background(), app, []string{"x"}, true); err != nil {
		t.Fatalf("force rm: %v", err)
	}
	if _, err := rt.Get(context.Background(), "x"); err == nil {
		t.Fatal("instance should be gone after force rm")
	}
}

func TestRunPull(t *testing.T) {
	app, rt, fc, out := testApp(t)
	seedRunning(rt, "a")
	fc.PullEvents = []facade.PullEvent{
		{Status: "downloading", Completed: 50, Total: 100},
		{Status: "success"},
	}
	if err := runPull(context.Background(), app, pullOptions{model: "qwen2.5:7b", on: "a", setDefault: true}); err != nil {
		t.Fatalf("runPull: %v", err)
	}
	if fc.CallCount("Pull:qwen2.5:7b") != 1 || fc.CallCount("SetDefault:qwen2.5:7b") != 1 {
		t.Errorf("unexpected calls: %v", fc.Calls)
	}
	if !strings.Contains(out.String(), "Pulled") {
		t.Errorf("missing success line:\n%s", out.String())
	}
}

func TestRunPullAmbiguousTarget(t *testing.T) {
	app, rt, _, _ := testApp(t)
	seedRunning(rt, "a")
	seedRunning(rt, "b")
	err := runPull(context.Background(), app, pullOptions{model: "m"})
	if err == nil || !strings.Contains(err.Error(), "multiple instances") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestRunPullNoInstances(t *testing.T) {
	app, _, _, _ := testApp(t)
	err := runPull(context.Background(), app, pullOptions{model: "m"})
	if err == nil {
		t.Fatal("expected error when no instances exist")
	}
}

func TestRunChatOnce(t *testing.T) {
	app, rt, fc, out := testApp(t)
	seedRunning(rt, "a")
	fc.ChatDeltas = []string{"Hello", " world"}
	err := runChat(context.Background(), app, chatOptions{name: "a", model: "llama3:8b", message: "hi"})
	if err != nil {
		t.Fatalf("runChat: %v", err)
	}
	if !strings.Contains(out.String(), "Hello world") {
		t.Errorf("chat output = %q", out.String())
	}
}

func TestRunStatusRendersGauges(t *testing.T) {
	app, rt, fc, out := testApp(t)
	seedRunning(rt, "a")
	fc.StatusResp = &facade.Status{
		Instance: facade.InstanceInfo{Name: "a", Backend: "ollama", UptimeSeconds: 120, DefaultModel: "llama3:8b"},
		System:   facade.SystemInfo{CPUPercent: 42, MemoryUsed: 1 << 30, MemoryTotal: 2 << 30},
		Models:   facade.ModelsInfo{Default: "llama3:8b", Installed: []facade.InstalledModel{{Name: "llama3:8b", Size: 4 << 30}}},
	}
	if err := runStatus(context.Background(), app, "a", false); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	s := out.String()
	for _, want := range []string{"CPU", "RAM", "llama3:8b", "Installed models"} {
		if !strings.Contains(s, want) {
			t.Errorf("status missing %q:\n%s", want, s)
		}
	}
}

func TestRunOpenInvokesLauncher(t *testing.T) {
	app, rt, _, _ := testApp(t)
	seedRunning(rt, "a")
	var opened string
	app.OpenURL = func(u string) error { opened = u; return nil }
	if err := runOpen(context.Background(), app, "a", false); err != nil {
		t.Fatalf("runOpen: %v", err)
	}
	if opened != "http://127.0.0.1:11500" {
		t.Errorf("opened %q", opened)
	}
}

func TestRunOpenPrintOnly(t *testing.T) {
	app, rt, _, out := testApp(t)
	seedRunning(rt, "a")
	called := false
	app.OpenURL = func(string) error { called = true; return nil }
	if err := runOpen(context.Background(), app, "a", true); err != nil {
		t.Fatalf("runOpen: %v", err)
	}
	if called {
		t.Error("browser should not be opened with --print")
	}
	if strings.TrimSpace(out.String()) != "http://127.0.0.1:11500" {
		t.Errorf("print output = %q", out.String())
	}
}

func TestRunDoctor(t *testing.T) {
	app, _, _, out := testApp(t)
	if err := runDoctor(context.Background(), app); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}
	s := out.String()
	for _, want := range []string{"Environment", "Docker", "Checks"} {
		if !strings.Contains(s, want) {
			t.Errorf("doctor missing %q:\n%s", want, s)
		}
	}
}

func TestRunApplyCreatesFleet(t *testing.T) {
	app, rt, _, out := testApp(t)
	dir := t.TempDir()
	path := dir + "/llm.yaml"
	if err := writeFile(path, `
version: "1"
defaults:
  backend: ollama
instances:
  - name: chat
    model: llama3:8b
  - name: embed
    model: nomic-embed-text
`); err != nil {
		t.Fatal(err)
	}
	if err := runApply(context.Background(), app, applyOptions{file: path}); err != nil {
		t.Fatalf("runApply: %v", err)
	}
	for _, name := range []string{"chat", "embed"} {
		in, err := rt.Get(context.Background(), name)
		if err != nil {
			t.Errorf("instance %q not created", name)
			continue
		}
		if !in.IsRunning() {
			t.Errorf("instance %q not running", name)
		}
	}
	if !strings.Contains(out.String(), "2 created") {
		t.Errorf("apply summary missing:\n%s", out.String())
	}
}

func TestRunApplyPrunes(t *testing.T) {
	app, rt, _, _ := testApp(t)
	seedRunning(rt, "stale")
	dir := t.TempDir()
	path := dir + "/llm.yaml"
	_ = writeFile(path, "instances:\n  - name: keep\n")

	if err := runApply(context.Background(), app, applyOptions{file: path, prune: true}); err != nil {
		t.Fatalf("runApply: %v", err)
	}
	if _, err := rt.Get(context.Background(), "stale"); err == nil {
		t.Error("stale instance should have been pruned")
	}
	if _, err := rt.Get(context.Background(), "keep"); err != nil {
		t.Error("declared instance should exist")
	}
}

func TestRunApplyPruneScopedToStack(t *testing.T) {
	app, rt, _, _ := testApp(t)
	// Resources belonging to a *different* named stack must survive a scoped prune.
	rt.Seed(engine.Instance{Name: "other", Backend: backend.Ollama, State: engine.StateRunning, Stack: "otherstack"})
	// And an unrelated standalone instance (no stack) must also be left alone.
	rt.Seed(engine.Instance{Name: "solo", Backend: backend.Ollama, State: engine.StateRunning})

	dir := t.TempDir()
	path := dir + "/stack.yaml"
	if err := writeFile(path, "name: mystack\ninstances:\n  - name: keep\n    model: llama3:8b\n"); err != nil {
		t.Fatal(err)
	}
	if err := runApply(context.Background(), app, applyOptions{file: path, prune: true}); err != nil {
		t.Fatalf("runApply: %v", err)
	}

	if _, err := rt.Get(context.Background(), "keep"); err != nil {
		t.Errorf("declared instance should exist: %v", err)
	}
	if _, err := rt.Get(context.Background(), "other"); err != nil {
		t.Error("a different stack's instance must NOT be pruned")
	}
	if _, err := rt.Get(context.Background(), "solo"); err != nil {
		t.Error("an unrelated standalone instance must NOT be pruned by a named stack")
	}
}

func TestResolveName(t *testing.T) {
	existing := []engine.Instance{{Name: "taken"}}
	if _, err := resolveName("taken", existing); err == nil {
		t.Error("expected error for taken name")
	}
	if _, err := resolveName("Bad Name", existing); err == nil {
		t.Error("expected error for invalid name")
	}
	n, err := resolveName("", existing)
	if err != nil || !engine.ValidName(n) {
		t.Errorf("generated name invalid: %q (%v)", n, err)
	}
}

func TestResolvePort(t *testing.T) {
	existing := []engine.Instance{{Name: "a", Port: 11500}}
	if _, err := resolvePort(11500, existing); err == nil {
		t.Error("expected error for port used by another instance")
	}
	p, err := resolvePort(0, existing)
	if err != nil {
		t.Fatalf("auto port: %v", err)
	}
	if p == 11500 {
		t.Errorf("auto port collided with used port")
	}
}

func TestRunBuildPrintsDockerfile(t *testing.T) {
	app, _, _, out := testApp(t)
	if err := runBuild(context.Background(), app, buildOptions{backendName: "ollama", printOnly: true}); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "FROM ollama/ollama:latest") {
		t.Errorf("dockerfile missing base image:\n%s", s)
	}
	if !strings.Contains(s, "LLMAKER_BACKEND=ollama") {
		t.Errorf("dockerfile missing backend env:\n%s", s)
	}
}

func TestRunBuildWritesContext(t *testing.T) {
	app, _, _, _ := testApp(t)
	dir := t.TempDir() + "/ctx"
	if err := runBuild(context.Background(), app, buildOptions{backendName: "llamacpp", output: dir}); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	for _, name := range []string{"Dockerfile", "entrypoint.sh", "README.md"} {
		if _, err := os.Stat(dir + "/" + name); err != nil {
			t.Errorf("expected generated file %q: %v", name, err)
		}
	}
}

func TestRunLogs(t *testing.T) {
	app, rt, _, out := testApp(t)
	seedRunning(rt, "a")
	rt.SetLogs("a", "hello from container\nline two\n")
	if err := runLogs(context.Background(), app, "a", false); err != nil {
		t.Fatalf("runLogs: %v", err)
	}
	if !strings.Contains(out.String(), "hello from container") {
		t.Errorf("logs output = %q", out.String())
	}
}

func TestRunTopFallsBackToLsWhenNonInteractive(t *testing.T) {
	app, rt, _, out := testApp(t)
	seedRunning(rt, "a")
	// testApp's IO writes to a buffer, so IsInteractive() is false → snapshot.
	if err := runTop(context.Background(), app, 0); err != nil {
		t.Fatalf("runTop: %v", err)
	}
	if !strings.Contains(out.String(), "a") {
		t.Errorf("top fallback should list the fleet:\n%s", out.String())
	}
}

func TestRunStatusNotRunning(t *testing.T) {
	app, rt, _, out := testApp(t)
	rt.Seed(engine.Instance{Name: "stopped", Backend: backend.Ollama, State: engine.StateExited})
	if err := runStatus(context.Background(), app, "stopped", false); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	if !strings.Contains(out.String(), "not running") {
		t.Errorf("expected not-running message:\n%s", out.String())
	}
}

func TestMustGetUnknown(t *testing.T) {
	app, rt, _, _ := testApp(t)
	_, err := app.mustGet(context.Background(), rt, "ghost")
	if err == nil || !strings.Contains(err.Error(), "no instance named") {
		t.Fatalf("expected friendly not-found error, got %v", err)
	}
}

func TestNewRootCmdHasAllCommands(t *testing.T) {
	app, _, _, _ := testApp(t)
	root := NewRootCmd(app)
	want := []string{"up", "ls", "service", "stack", "status", "top", "pull", "chat", "open", "logs", "stop", "start", "restart", "rm", "apply", "doctor", "build", "version"}
	have := map[string]bool{}
	for _, c := range root.Commands() {
		have[c.Name()] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("root command missing subcommand %q", w)
		}
	}
}
