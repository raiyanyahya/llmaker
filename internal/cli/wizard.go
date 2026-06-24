package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/raiyanyahya/llmaker/internal/backend"
	"github.com/raiyanyahya/llmaker/internal/engine"
)

// runWizard collects instance settings interactively (the `llmaker up` with no
// flags experience), pre-filling sane, host-derived defaults. It mutates opts in
// place. It is only invoked on an interactive TTY.
func runWizard(app *App, opts *upOptions) error {
	backendChoice := firstNonEmpty(opts.backendName, "ollama")
	b, _ := backend.Get(backendChoice)
	model := firstNonEmpty(opts.model, b.DefaultModel)
	memory := firstNonEmpty(opts.memory, defaultMemoryString(app.Host))
	cpus := strconv.Itoa(int(app.Host.DefaultCPUs()))
	gpu := opts.gpu
	name := opts.name

	backendOptions := make([]huh.Option[string], 0, len(backend.All()))
	for _, bk := range backend.All() {
		label := bk.DisplayName
		if bk.Kind == backend.Ollama {
			label += " (default)"
		}
		backendOptions = append(backendOptions, huh.NewOption(label, string(bk.Kind)))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Backend").
				Description("Inference engine to run").
				Options(backendOptions...).
				Value(&backendChoice),
			huh.NewInput().
				Title("Model").
				Description("Model to preload on first boot").
				Value(&model).
				Validate(notEmpty("model")),
			huh.NewInput().
				Title("Name").
				Description("Leave blank for a generated name").
				Value(&name).
				Validate(optionalName),
			huh.NewInput().
				Title("Memory").
				Description("Container memory limit (e.g. 8g)").
				Value(&memory).
				Validate(validSize),
			huh.NewInput().
				Title("CPUs").
				Description("CPU quota").
				Value(&cpus).
				Validate(validCPUs),
			huh.NewConfirm().
				Title("Reserve GPU?").
				Description("Requires NVIDIA Container Toolkit").
				Value(&gpu),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return fmt.Errorf("wizard cancelled: %w", err)
	}

	opts.backendName = backendChoice
	opts.model = model
	opts.name = strings.TrimSpace(name)
	opts.memory = memory
	if c, err := strconv.ParseFloat(strings.TrimSpace(cpus), 64); err == nil {
		opts.cpus = c
	}
	opts.gpu = gpu
	return nil
}

func defaultMemoryString(h engine.HostInfo) string {
	g := h.DefaultMemoryBytes() / engine.GiB
	if g < 1 {
		g = 1
	}
	return fmt.Sprintf("%dg", g)
}

func notEmpty(field string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
}

func optionalName(s string) error {
	s = engine.NormalizeName(s)
	if s == "" {
		return nil
	}
	if !engine.ValidName(s) {
		return fmt.Errorf("use lowercase letters, digits, - or _")
	}
	return nil
}

func validSize(s string) error {
	_, err := engine.ParseSize(s)
	return err
}

func validCPUs(s string) error {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return fmt.Errorf("must be a number")
	}
	if v <= 0 {
		return fmt.Errorf("must be greater than 0")
	}
	return nil
}
