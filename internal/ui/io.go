package ui

import (
	"fmt"
	"io"
	"os"
)

// IOStreams bundles the streams a command writes to, so output is injectable and
// testable (a test passes bytes.Buffers; production passes the OS streams).
type IOStreams struct {
	Out   io.Writer
	Err   io.Writer
	In    io.Reader
	Theme *Theme
}

// Standard returns IOStreams wired to the process std streams, with a theme
// whose styling is auto-detected from stdout.
func Standard() *IOStreams {
	out := io.Writer(os.Stdout)
	return &IOStreams{
		Out:   out,
		Err:   os.Stderr,
		In:    os.Stdin,
		Theme: NewTheme(out, ColorEnabled(out)),
	}
}

// Printf writes formatted text to Out.
func (s *IOStreams) Printf(format string, a ...any) {
	fmt.Fprintf(s.Out, format, a...)
}

// Println writes a line to Out.
func (s *IOStreams) Println(a ...any) {
	fmt.Fprintln(s.Out, a...)
}

// Errorf writes formatted text to Err.
func (s *IOStreams) Errorf(format string, a ...any) {
	fmt.Fprintf(s.Err, format, a...)
}

// IsInteractive reports whether Out is a real terminal (controls wizards,
// spinners and animations).
func (s *IOStreams) IsInteractive() bool {
	return s.Theme != nil && s.Theme.Enabled && IsTerminal(s.Out)
}
