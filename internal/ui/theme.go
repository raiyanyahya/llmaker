// Package ui centralizes llmaker's terminal look and feel: a single Lip Gloss
// theme, fleet tables, status gauges, spinners and progress bars. Everything
// here is TTY- and NO_COLOR-aware — when output is piped or color is disabled,
// styling collapses to clean plain text so llmaker stays script-friendly.
package ui

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// Palette — a small, deliberate set so the whole CLI feels like one product.
const (
	colorAccent  = lipgloss.Color("#8B5CF6") // violet — the llmaker brand
	colorAccent2 = lipgloss.Color("#22D3EE") // cyan — secondary accent
	colorSuccess = lipgloss.Color("#22C55E")
	colorWarning = lipgloss.Color("#F59E0B")
	colorError   = lipgloss.Color("#EF4444")
	colorMuted   = lipgloss.Color("#9CA3AF")
	colorText    = lipgloss.Color("#E5E7EB")
)

// Theme holds every style llmaker renders with. Build one per output stream.
type Theme struct {
	Enabled  bool
	renderer *lipgloss.Renderer

	Logo     lipgloss.Style
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Muted    lipgloss.Style
	Accent   lipgloss.Style
	Bold     lipgloss.Style
	Success  lipgloss.Style
	Warning  lipgloss.Style
	Error    lipgloss.Style
	Key      lipgloss.Style
	Value    lipgloss.Style

	CardBorder lipgloss.Style
	HeaderRow  lipgloss.Style
}

// NewTheme builds a theme for out. When enabled is false the renderer is forced
// to an ASCII profile, so styled strings come out as plain text.
func NewTheme(out io.Writer, enabled bool) *Theme {
	r := lipgloss.NewRenderer(out)
	if !enabled {
		r.SetColorProfile(termenv.Ascii)
	}
	t := &Theme{Enabled: enabled, renderer: r}
	s := r.NewStyle

	t.Logo = s().Bold(true).Foreground(colorAccent)
	t.Title = s().Bold(true).Foreground(colorText)
	t.Subtitle = s().Foreground(colorMuted)
	t.Muted = s().Foreground(colorMuted)
	t.Accent = s().Foreground(colorAccent)
	t.Bold = s().Bold(true)
	t.Success = s().Foreground(colorSuccess)
	t.Warning = s().Foreground(colorWarning)
	t.Error = s().Foreground(colorError).Bold(true)
	t.Key = s().Foreground(colorMuted)
	t.Value = s().Foreground(colorText).Bold(true)

	t.CardBorder = s().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(0, 1)
	t.HeaderRow = s().Bold(true).Foreground(colorAccent2)

	return t
}

// Renderer exposes the underlying Lip Gloss renderer for components (Bubble Tea
// models, tables) that need to build their own styles on the same stream.
func (t *Theme) Renderer() *lipgloss.Renderer { return t.renderer }

// NewStyle returns a fresh style bound to this theme's renderer/profile.
func (t *Theme) NewStyle() lipgloss.Style { return t.renderer.NewStyle() }

// Color returns c when styling is enabled, or an empty color (no-op) otherwise.
// Useful for components that set colors imperatively.
func (t *Theme) Color(c lipgloss.Color) lipgloss.Color {
	if !t.Enabled {
		return lipgloss.Color("")
	}
	return c
}

// --- color/TTY detection ---

// ColorEnabled decides whether to emit ANSI styling for w. It honors the de
// facto NO_COLOR standard and an llmaker-specific override, and otherwise
// enables color only for real terminals.
func ColorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("LLMAKER_NO_COLOR") != "" {
		return false
	}
	if os.Getenv("LLMAKER_FORCE_COLOR") != "" {
		return true
	}
	return IsTerminal(w)
}

// IsTerminal reports whether w is an interactive terminal.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
