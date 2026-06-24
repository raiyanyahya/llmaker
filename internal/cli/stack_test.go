package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raiyanyahya/llmaker/internal/config"
)

func TestStackInitWritesValidTemplates(t *testing.T) {
	for _, name := range stackTemplateNames() {
		t.Run(name, func(t *testing.T) {
			app, _, _, out := testApp(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "stack.yaml")
			cmd := newStackInitCmd(app)
			cmd.SetArgs([]string{name, "-o", path})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("stack init %s: %v", name, err)
			}
			if !strings.Contains(out.String(), "Wrote") {
				t.Errorf("missing confirmation for %s", name)
			}
			// The scaffolded file must parse and lower like any stack.yaml.
			f, err := config.Load(path)
			if err != nil {
				t.Fatalf("%s: load: %v", name, err)
			}
			specs, err := f.ToSpecs()
			if err != nil {
				t.Fatalf("%s: ToSpecs: %v", name, err)
			}
			svcs, err := f.ToServiceSpecs()
			if err != nil {
				t.Fatalf("%s: ToServiceSpecs: %v", name, err)
			}
			if len(specs) == 0 || len(svcs) == 0 {
				t.Errorf("%s: expected instances and services, got %d/%d", name, len(specs), len(svcs))
			}
			// Every template includes the agent.
			hasAgent := false
			for _, s := range svcs {
				if s.Service == "agent" {
					hasAgent = true
				}
			}
			if !hasAgent {
				t.Errorf("%s: template missing the agent service", name)
			}
		})
	}
}

func TestStackInitRefusesOverwrite(t *testing.T) {
	app, _, _, _ := testApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.yaml")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newStackInitCmd(app)
	cmd.SetArgs([]string{"rag", "-o", path})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected refusal to overwrite without --force")
	}
	// --force overwrites.
	cmd = newStackInitCmd(app)
	cmd.SetArgs([]string{"rag", "-o", path, "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--force should overwrite: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "existing") {
		t.Error("file was not overwritten")
	}
}

func TestStackInitUnknownTemplate(t *testing.T) {
	app, _, _, _ := testApp(t)
	cmd := newStackInitCmd(app)
	cmd.SetArgs([]string{"nope", "-o", filepath.Join(t.TempDir(), "s.yaml")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for unknown template")
	}
}
