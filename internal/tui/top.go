// Package tui implements llmaker's live, animated fleet dashboard (`llmaker
// top`) — "htop for your local LLM fleet". It's a Bubble Tea program that polls
// each instance's /api/status on an interval and renders animated load gauges.
package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/raiyanyahya/llmaker/internal/engine"
	"github.com/raiyanyahya/llmaker/internal/facade"
	"github.com/raiyanyahya/llmaker/internal/ui"
)

// Run launches the dashboard and blocks until the user quits.
func Run(ctx context.Context, rt engine.Runtime, fc facade.Client, theme *ui.Theme, interval time.Duration) error {
	m := Model{rt: rt, fc: fc, theme: theme, interval: interval, width: 80}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}

// Model is the Bubble Tea state for the dashboard.
type Model struct {
	rt       engine.Runtime
	fc       facade.Client
	theme    *ui.Theme
	interval time.Duration

	width, height int
	views         []instanceView
	err           error
	updated       time.Time
	loading       bool
}

type instanceView struct {
	inst   engine.Instance
	status *facade.Status
	err    error
}

type tickMsg time.Time
type dataMsg struct {
	views []instanceView
	err   error
}

// Init kicks off the first fetch and the refresh ticker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), tick(m.interval))
}

// Update handles keys, resize, ticks and incoming data.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			return m, m.fetchCmd()
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		return m, tea.Batch(m.fetchCmd(), tick(m.interval))
	case dataMsg:
		m.views, m.err, m.updated, m.loading = msg.views, msg.err, time.Now(), false
	}
	return m, nil
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// fetchCmd lists the fleet and fetches each running instance's status in
// parallel, off the UI goroutine.
func (m Model) fetchCmd() tea.Cmd {
	rt, fc := m.rt, m.fc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()

		ins, err := rt.List(ctx)
		if err != nil {
			return dataMsg{err: err}
		}
		sort.Slice(ins, func(i, j int) bool { return ins[i].Name < ins[j].Name })

		views := make([]instanceView, len(ins))
		var wg sync.WaitGroup
		for i := range ins {
			views[i].inst = ins[i]
			if !ins[i].IsRunning() {
				continue
			}
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				st, err := fc.Status(ctx, ins[i].URL())
				if err != nil {
					views[i].err = err
					return
				}
				views[i].status = st
			}(i)
		}
		wg.Wait()
		return dataMsg{views: views}
	}
}

// View renders the whole dashboard.
func (m Model) View() string {
	t := m.theme
	var b strings.Builder

	title := t.Logo.Render("llmaker top")
	b.WriteString(title + "  " + t.Muted.Render(fmt.Sprintf("%d instances", len(m.views))))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(t.FailLine("error: " + m.err.Error()))
		b.WriteString("\n\n" + m.footer())
		return b.String()
	}
	if len(m.views) == 0 {
		b.WriteString(t.Muted.Render("No instances. Start one with `llmaker up`."))
		b.WriteString("\n\n" + m.footer())
		return b.String()
	}

	for _, v := range m.views {
		b.WriteString(m.renderInstance(v))
		b.WriteString("\n")
	}
	b.WriteString("\n" + m.footer())
	return b.String()
}

func (m Model) renderInstance(v instanceView) string {
	t := m.theme
	var b strings.Builder

	// Header: dot + name + model + uptime.
	level := ui.LevelMuted
	state := string(v.inst.State)
	switch {
	case v.status != nil:
		level = ui.LevelOK
		state = "healthy"
	case v.inst.IsRunning():
		level = ui.LevelWarn
		state = "starting"
	case v.inst.State == engine.StateExited:
		level = ui.LevelError
	}

	header := fmt.Sprintf("%s %s  %s  %s",
		t.Dot(level),
		t.Value.Render(v.inst.Name),
		t.Muted.Render(string(v.inst.Backend)),
		t.Badge(state, level),
	)
	b.WriteString(header)
	b.WriteString("\n")

	if v.status == nil {
		detail := "  " + t.Muted.Render("(no live metrics)")
		if v.err != nil {
			detail = "  " + t.Muted.Render(ui.Truncate(v.err.Error(), m.barWidth()+30))
		}
		b.WriteString(detail + "\n")
		return b.String()
	}

	st := v.status
	w := m.barWidth()
	b.WriteString("  " + t.Gauge("CPU", st.System.CPUPercent/100, w) + "\n")

	memFrac := -1.0
	if st.System.MemoryTotal > 0 {
		memFrac = float64(st.System.MemoryUsed) / float64(st.System.MemoryTotal)
	}
	b.WriteString("  " + t.Gauge("RAM", memFrac, w) + "  " +
		t.Muted.Render(fmt.Sprintf("%s/%s", ui.HumanBytes(st.System.MemoryUsed), ui.HumanBytes(st.System.MemoryTotal))) + "\n")

	for i, g := range st.System.GPUs {
		b.WriteString("  " + t.Gauge(fmt.Sprintf("GPU%d", i), g.Utilization/100, w) + "  " +
			t.Muted.Render(fmt.Sprintf("VRAM %s/%s", ui.HumanBytes(g.MemoryUsed), ui.HumanBytes(g.MemoryTotal))) + "\n")
	}

	// Loaded model + tokens/sec.
	loaded := "none"
	if len(st.Models.Running) > 0 {
		loaded = st.Models.Running[0].Name
	} else if st.Models.Default != "" {
		loaded = st.Models.Default + " (idle)"
	}
	tps := ""
	if st.Metrics.TokensPerSecond > 0 {
		tps = fmt.Sprintf(" · %.1f tok/s", st.Metrics.TokensPerSecond)
	}
	b.WriteString("  " + t.Key.Render("model  ") + t.Value.Render(ui.Truncate(loaded, 32)) + t.Muted.Render(tps) + "\n")
	return b.String()
}

func (m Model) barWidth() int {
	w := m.width - 24
	if w < 10 {
		w = 10
	}
	if w > 40 {
		w = 40
	}
	return w
}

func (m Model) footer() string {
	t := m.theme
	ago := "just now"
	if !m.updated.IsZero() {
		ago = ui.HumanDuration(time.Since(m.updated)) + " ago"
	}
	return t.Muted.Render(fmt.Sprintf("↻ every %s · updated %s · ", m.interval.Round(time.Second), ago)) +
		t.Key.Render("r") + t.Muted.Render(" refresh  ") +
		t.Key.Render("q") + t.Muted.Render(" quit")
}
