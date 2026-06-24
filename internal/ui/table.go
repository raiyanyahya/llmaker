package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Table is a lightweight, ANSI-width-aware columnar renderer. Cells may already
// contain styling (e.g. a colored health dot); column widths are measured with
// lipgloss.Width so alignment stays correct regardless of escape codes. The
// borderless layout reads cleanly in a terminal and pipes nicely.
type Table struct {
	theme   *Theme
	headers []string
	rows    [][]string
}

// NewTable starts a table with the given column headers.
func (t *Theme) NewTable(headers ...string) *Table {
	return &Table{theme: t, headers: headers}
}

// Row appends a row. Extra cells beyond the header count are ignored; missing
// cells render blank.
func (tb *Table) Row(cells ...string) *Table {
	tb.rows = append(tb.rows, cells)
	return tb
}

// Len reports how many data rows have been added.
func (tb *Table) Len() int { return len(tb.rows) }

// Render produces the full table string (no trailing newline).
func (tb *Table) Render() string {
	cols := len(tb.headers)
	widths := make([]int, cols)
	for c, h := range tb.headers {
		widths[c] = lipgloss.Width(h)
	}
	for _, row := range tb.rows {
		for c := 0; c < cols && c < len(row); c++ {
			if w := lipgloss.Width(row[c]); w > widths[c] {
				widths[c] = w
			}
		}
	}

	var b strings.Builder

	// Header.
	headerCells := make([]string, cols)
	for c, h := range tb.headers {
		padded := pad(h, widths[c])
		if tb.theme.Enabled {
			padded = tb.theme.HeaderRow.Render(padded)
		}
		headerCells[c] = padded
	}
	b.WriteString(strings.TrimRight(strings.Join(headerCells, "  "), " "))
	b.WriteByte('\n')

	// Rows.
	for i, row := range tb.rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		cells := make([]string, cols)
		for c := 0; c < cols; c++ {
			val := ""
			if c < len(row) {
				val = row[c]
			}
			cells[c] = pad(val, widths[c])
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, "  "), " "))
	}
	return b.String()
}

// pad right-pads s with spaces to display width w (ANSI-aware).
func pad(s string, w int) string {
	gap := w - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}
