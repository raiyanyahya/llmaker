package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raiyanyahya/llmaker/internal/engine"
)

func TestRunServiceAddProvisions(t *testing.T) {
	app, rt, _, out := testApp(t)
	// noWait avoids the real TCP readiness probe against the fake.
	opts := serviceAddOptions{noWait: true, host: "127.0.0.1", timeout: 5 * time.Second}
	if err := runServiceAdd(context.Background(), app, "qdrant", opts); err != nil {
		t.Fatalf("runServiceAdd: %v", err)
	}

	svc, err := rt.GetService(context.Background(), "qdrant")
	if err != nil {
		t.Fatalf("service not created: %v", err)
	}
	if svc.Kind != "qdrant" || string(svc.State) != "running" {
		t.Fatalf("unexpected service: %+v", svc)
	}
	if svc.PrimaryPort() == 0 {
		t.Fatal("primary port not allocated")
	}
	// Both catalog ports (http + grpc) should be bound to distinct host ports.
	if len(svc.Ports) != 2 || svc.Ports[0].Host == svc.Ports[1].Host {
		t.Fatalf("expected 2 distinct host ports, got %+v", svc.Ports)
	}
	if !strings.Contains(out.String(), "Service ready") {
		t.Errorf("missing ready card; got:\n%s", out.String())
	}
}

func TestServiceAddRejectsDuplicateName(t *testing.T) {
	app, _, _, _ := testApp(t)
	ctx := context.Background()
	opts := serviceAddOptions{noWait: true, timeout: time.Second}
	if err := runServiceAdd(ctx, app, "redis", opts); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := runServiceAdd(ctx, app, "redis", opts); err == nil {
		t.Fatal("expected duplicate-name error")
	}
}

func TestServiceAddCustomNameAndEnv(t *testing.T) {
	app, rt, _, _ := testApp(t)
	opts := serviceAddOptions{
		name: "store", noWait: true, timeout: time.Second,
		env: map[string]string{"POSTGRES_PASSWORD": "secret"},
	}
	if err := runServiceAdd(context.Background(), app, "pgvector", opts); err != nil {
		t.Fatalf("runServiceAdd: %v", err)
	}
	svc, err := rt.GetService(context.Background(), "store")
	if err != nil {
		t.Fatalf("service 'store' not found: %v", err)
	}
	if svc.Kind != "pgvector" {
		t.Errorf("kind = %q, want pgvector", svc.Kind)
	}
}

func TestServiceLsJSON(t *testing.T) {
	app, rt, _, out := testApp(t)
	rt.SeedService(engine.Service{
		Name: "qdrant", Kind: "qdrant", Category: "vector-db",
		Host: "127.0.0.1", State: engine.StateRunning,
		Ports: []engine.PortBinding{{Host: 11500, Container: 6333, Name: "http", Primary: true}},
	})
	cmd := newServiceLsCmd(app)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("service ls --json: %v", err)
	}
	var got []serviceJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("parse json: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].Name != "qdrant" || got[0].Endpoint != "qdrant:6333" {
		t.Fatalf("unexpected json: %+v", got)
	}
}

func TestServiceLifecycleStopStartRemove(t *testing.T) {
	app, rt, _, _ := testApp(t)
	rt.SeedService(engine.Service{Name: "redis", Kind: "redis", State: engine.StateRunning})
	ctx := context.Background()

	if err := newServiceStopCmd(app).RunE(newServiceStopCmd(app), []string{"redis"}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if s, _ := rt.GetService(ctx, "redis"); s.State != engine.StateExited {
		t.Fatalf("expected exited, got %s", s.State)
	}
	if err := newServiceStartCmd(app).RunE(newServiceStartCmd(app), []string{"redis"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if s, _ := rt.GetService(ctx, "redis"); s.State != engine.StateRunning {
		t.Fatalf("expected running, got %s", s.State)
	}
	// rm needs --force on a running service.
	rmForce := newServiceRmCmd(app)
	rmForce.SetArgs([]string{"--force", "redis"})
	if err := rmForce.Execute(); err != nil {
		t.Fatalf("rm --force: %v", err)
	}
	if _, err := rt.GetService(ctx, "redis"); err == nil {
		t.Fatal("service still present after rm")
	}
}

func TestServicesShownInLs(t *testing.T) {
	app, rt, _, out := testApp(t)
	seedRunning(rt, "chat")
	rt.SeedService(engine.Service{
		Name: "qdrant", Kind: "qdrant", Category: "vector-db", State: engine.StateRunning,
		Ports: []engine.PortBinding{{Host: 11600, Container: 6333, Primary: true}},
	})
	if err := runLs(context.Background(), app, lsOptions{}); err != nil {
		t.Fatalf("runLs: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "chat") || !strings.Contains(s, "Services") || !strings.Contains(s, "qdrant") {
		t.Fatalf("ls did not show instances + services:\n%s", s)
	}
}

func TestApplyBringsUpServicesAndInstances(t *testing.T) {
	app, rt, _, _ := testApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.yaml")
	if err := writeFile(path, `
defaults: { backend: ollama }
instances:
  - { name: chat, model: llama3:8b }
services:
  - { name: vectors, use: qdrant }
  - { use: redis }
`); err != nil {
		t.Fatal(err)
	}
	if err := runApply(context.Background(), app, applyOptions{file: path, noPull: true}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	ctx := context.Background()
	if _, err := rt.Get(ctx, "chat"); err != nil {
		t.Errorf("instance chat missing: %v", err)
	}
	if _, err := rt.GetService(ctx, "vectors"); err != nil {
		t.Errorf("service vectors missing: %v", err)
	}
	if _, err := rt.GetService(ctx, "redis"); err != nil {
		t.Errorf("service redis (defaulted name) missing: %v", err)
	}
}

func TestApplyPrunesServices(t *testing.T) {
	app, rt, _, _ := testApp(t)
	rt.SeedService(engine.Service{Name: "stale", Kind: "redis", State: engine.StateRunning})
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.yaml")
	if err := writeFile(path, `
services:
  - { use: qdrant }
`); err != nil {
		t.Fatal(err)
	}
	if err := runApply(context.Background(), app, applyOptions{file: path, prune: true, noPull: true}); err != nil {
		t.Fatalf("apply --prune: %v", err)
	}
	if _, err := rt.GetService(context.Background(), "stale"); err == nil {
		t.Fatal("stale service should have been pruned")
	}
}
