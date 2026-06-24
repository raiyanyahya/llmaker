package ui

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
)

// clearLine is the ANSI sequence to return to column 0 and erase the line, used
// to redraw spinners/bars in place on a TTY.
const clearLine = "\r\x1b[K"

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner animates a single status line while a blocking operation runs. On a
// non-interactive stream it degrades to a "label…" line plus a final result
// line, so logs stay readable.
type Spinner struct {
	out     io.Writer
	theme   *Theme
	mu      sync.Mutex
	label   string
	active  bool
	stop    chan struct{}
	stopped chan struct{}
}

// NewSpinner creates a spinner writing to out.
func (t *Theme) NewSpinner(out io.Writer, label string) *Spinner {
	return &Spinner{out: out, theme: t, label: label}
}

// Start begins animating (or prints a single line when non-interactive).
func (s *Spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return
	}
	s.active = true

	if !s.interactive() {
		fmt.Fprintf(s.out, "%s…\n", s.label)
		return
	}
	s.stop = make(chan struct{})
	s.stopped = make(chan struct{})
	go s.run()
}

func (s *Spinner) run() {
	defer close(s.stopped)
	ticker := time.NewTicker(90 * time.Millisecond)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			label := s.label
			s.mu.Unlock()
			frame := s.theme.Accent.Render(spinnerFrames[i%len(spinnerFrames)])
			fmt.Fprintf(s.out, "%s%s %s", clearLine, frame, label)
			i++
		}
	}
}

// SetLabel updates the animated label.
func (s *Spinner) SetLabel(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

// Stop halts the spinner and prints finalLine (already styled by the caller).
func (s *Spinner) Stop(finalLine string) {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	interactive := s.interactive()
	stop := s.stop
	stopped := s.stopped
	s.mu.Unlock()

	if interactive {
		close(stop)
		<-stopped
		fmt.Fprint(s.out, clearLine)
	}
	if finalLine != "" {
		fmt.Fprintln(s.out, finalLine)
	}
}

func (s *Spinner) interactive() bool {
	return s.theme.Enabled && IsTerminal(s.out)
}

// ProgressBar renders a single in-place download/progress bar. On a TTY it draws
// a Bubbles gradient bar; otherwise it prints sparse percentage lines.
type ProgressBar struct {
	out     io.Writer
	theme   *Theme
	model   progress.Model
	label   string
	lastPct int
	drew    bool
}

// NewProgressBar creates a bar writing to out.
func (t *Theme) NewProgressBar(out io.Writer, label string) *ProgressBar {
	m := progress.New(progress.WithDefaultGradient(), progress.WithWidth(28), progress.WithoutPercentage())
	return &ProgressBar{out: out, theme: t, model: m, label: label, lastPct: -1}
}

// Update redraws the bar. fraction is clamped to [0,1]; a negative fraction
// renders an indeterminate state. suffix is appended (e.g. "1.2 GB / 4.7 GB").
func (p *ProgressBar) Update(fraction float64, suffix string) {
	if fraction > 1 {
		fraction = 1
	}
	if !p.interactive() {
		p.updatePlain(fraction, suffix)
		return
	}
	p.drew = true
	bar := p.model.ViewAs(clamp01(fraction))
	pct := "  ? "
	if fraction >= 0 {
		pct = fmt.Sprintf("%3.0f%%", fraction*100)
	}
	fmt.Fprintf(p.out, "%s%s %s %s %s",
		clearLine,
		p.theme.Key.Render(p.label),
		bar,
		p.theme.Value.Render(pct),
		p.theme.Muted.Render(suffix),
	)
}

func (p *ProgressBar) updatePlain(fraction float64, suffix string) {
	pct := -1
	if fraction >= 0 {
		pct = int(fraction * 100)
	}
	// Only emit when crossing a 10% boundary to avoid flooding logs.
	if pct/10 == p.lastPct/10 && pct >= 0 {
		return
	}
	p.lastPct = pct
	if pct < 0 {
		fmt.Fprintf(p.out, "%s: %s\n", p.label, suffix)
		return
	}
	fmt.Fprintf(p.out, "%s: %d%% %s\n", p.label, pct, suffix)
}

// Finish clears the bar (on a TTY) and prints finalLine.
func (p *ProgressBar) Finish(finalLine string) {
	if p.interactive() && p.drew {
		fmt.Fprint(p.out, clearLine)
	}
	if finalLine != "" {
		fmt.Fprintln(p.out, finalLine)
	}
}

func (p *ProgressBar) interactive() bool {
	return p.theme.Enabled && IsTerminal(p.out)
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
