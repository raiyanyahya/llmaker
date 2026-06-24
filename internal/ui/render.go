package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Level is a semantic severity used by badges and gauges, decoupling ui from
// any domain enum.
type Level int

const (
	LevelMuted Level = iota
	LevelOK
	LevelWarn
	LevelError
	LevelInfo
)

func (t *Theme) styleFor(l Level) lipgloss.Style {
	switch l {
	case LevelOK:
		return t.Success
	case LevelWarn:
		return t.Warning
	case LevelError:
		return t.Error
	case LevelInfo:
		return t.Accent
	default:
		return t.Muted
	}
}

// Dot returns a colored status dot for a level.
func (t *Theme) Dot(l Level) string {
	return t.styleFor(l).Render("●")
}

// Badge renders a small status word in the level's color.
func (t *Theme) Badge(text string, l Level) string {
	return t.styleFor(l).Render(text)
}

// Heading renders a section heading with a leading accent bar.
func (t *Theme) Heading(text string) string {
	if !t.Enabled {
		return text
	}
	return t.Accent.Render("▌ ") + t.Title.Render(text)
}

// KeyValues renders aligned "key  value" lines.
func (t *Theme) KeyValues(rows [][2]string) string {
	width := 0
	for _, r := range rows {
		if len(r[0]) > width {
			width = len(r[0])
		}
	}
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		key := t.Key.Render(fmt.Sprintf("%-*s", width, r[0]))
		b.WriteString(key + "  " + t.Value.Render(r[1]))
	}
	return b.String()
}

// Card renders a titled rounded box around key/value rows. The success flag
// tints the border green (used by `llmaker up`'s summary).
func (t *Theme) Card(title string, rows [][2]string, ok bool) string {
	body := t.KeyValues(rows)
	if !t.Enabled {
		// Plain, pipe-friendly rendering: a header line then key: value lines.
		var b strings.Builder
		b.WriteString(title + "\n")
		for _, r := range rows {
			fmt.Fprintf(&b, "  %s: %s\n", r[0], r[1])
		}
		return strings.TrimRight(b.String(), "\n")
	}
	border := t.CardBorder
	if ok {
		border = border.BorderForeground(colorSuccess)
	}
	heading := t.Bold.Render(title)
	content := heading + "\n" + body
	return border.Render(content)
}

// Gauge renders a labeled progress/utilization bar colored by load. fraction is
// clamped to [0,1]; pass a negative fraction for an indeterminate "—" bar.
func (t *Theme) Gauge(label string, fraction float64, width int) string {
	if width < 4 {
		width = 4
	}
	pctText := "  ? "
	if fraction >= 0 {
		pctText = fmt.Sprintf("%3.0f%%", fraction*100)
	}

	if !t.Enabled {
		filled := 0
		if fraction > 0 {
			filled = int(fraction * float64(width))
		}
		bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
		return fmt.Sprintf("%-6s [%s] %s", label, bar, pctText)
	}

	level := LevelOK
	switch {
	case fraction >= 0.9:
		level = LevelError
	case fraction >= 0.7:
		level = LevelWarn
	}

	filled := 0
	if fraction > 0 {
		filled = int(fraction*float64(width) + 0.5)
		if filled > width {
			filled = width
		}
	}
	fillStyle := t.styleFor(level)
	bar := fillStyle.Render(strings.Repeat("█", filled)) +
		t.Muted.Render(strings.Repeat("░", width-filled))
	return fmt.Sprintf("%s %s %s",
		t.Key.Render(fmt.Sprintf("%-6s", label)),
		bar,
		t.Value.Render(pctText),
	)
}

// Success/Warn/Fail prefix helpers for one-line status messages.
func (t *Theme) SuccessLine(msg string) string { return t.Success.Render("✓ ") + msg }
func (t *Theme) WarnLine(msg string) string    { return t.Warning.Render("! ") + msg }
func (t *Theme) FailLine(msg string) string    { return t.Error.Render("✗ ") + msg }
func (t *Theme) InfoLine(msg string) string    { return t.Accent.Render("➜ ") + msg }
