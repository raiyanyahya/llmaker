package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// plainTheme builds a styling-disabled theme writing to a buffer, mirroring how
// output looks when piped or NO_COLOR is set.
func plainTheme() (*Theme, *bytes.Buffer) {
	var buf bytes.Buffer
	return NewTheme(&buf, false), &buf
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1000, "1.0 kB"},
		{1500, "1.5 kB"},
		{4_700_000_000, "4.7 GB"},
	}
	for _, c := range cases {
		if got := HumanBytes(c.in); got != c.want {
			t.Errorf("HumanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "-"},
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h30m"},
		{2 * time.Hour, "2h"},
		{49 * time.Hour, "2d1h"},
		{48 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := HumanDuration(c.in); got != c.want {
			t.Errorf("HumanDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("hello", 10); got != "hello" {
		t.Errorf("no truncate = %q", got)
	}
	if got := Truncate("hello world", 5); got != "hell…" {
		t.Errorf("truncate = %q, want hell…", got)
	}
	if got := Truncate("hello", 0); got != "" {
		t.Errorf("zero max = %q", got)
	}
}

func TestPlainTableHasNoANSI(t *testing.T) {
	th, _ := plainTheme()
	out := th.NewTable("NAME", "BACKEND", "PORT").
		Row("brave-llama", "ollama", "11500").
		Row("calm-otter", "llamacpp", "11501").
		Render()
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("plain table should contain no ANSI escapes:\n%q", out)
	}
	for _, want := range []string{"NAME", "brave-llama", "llamacpp", "11501"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	// Columns should align: header line and rows have the same number of lines.
	if lines := strings.Count(out, "\n"); lines != 2 {
		t.Errorf("expected 3 lines (header + 2 rows), got %d:\n%s", lines+1, out)
	}
}

func TestPlainGauge(t *testing.T) {
	th, _ := plainTheme()
	g := th.Gauge("CPU", 0.5, 10)
	if strings.Contains(g, "\x1b[") {
		t.Fatalf("plain gauge has ANSI: %q", g)
	}
	if !strings.Contains(g, "50%") {
		t.Errorf("gauge missing percentage: %q", g)
	}
	if !strings.Contains(g, "#####") {
		t.Errorf("gauge missing fill: %q", g)
	}
	// Indeterminate gauge.
	if !strings.Contains(th.Gauge("GPU", -1, 10), "?") {
		t.Errorf("indeterminate gauge should show ?")
	}
}

func TestPlainCard(t *testing.T) {
	th, _ := plainTheme()
	card := th.Card("Instance ready", [][2]string{
		{"name", "brave-llama"},
		{"port", "11500"},
	}, true)
	if strings.Contains(card, "\x1b[") || strings.Contains(card, "╭") {
		t.Fatalf("plain card should be borderless ASCII:\n%q", card)
	}
	if !strings.Contains(card, "brave-llama") || !strings.Contains(card, "Instance ready") {
		t.Errorf("card missing content:\n%s", card)
	}
}

func TestColorEnabledHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	if ColorEnabled(&buf) {
		t.Error("NO_COLOR should disable color")
	}
}

func TestNonTTYBufferDisablesColor(t *testing.T) {
	var buf bytes.Buffer
	if ColorEnabled(&buf) {
		t.Error("a non-*os.File writer is not a terminal; color should be off")
	}
}

func TestSpinnerNonInteractive(t *testing.T) {
	th, buf := plainTheme()
	sp := th.NewSpinner(buf, "Pulling image")
	sp.Start()
	sp.Stop(th.SuccessLine("done"))
	out := buf.String()
	if !strings.Contains(out, "Pulling image") || !strings.Contains(out, "done") {
		t.Errorf("spinner output = %q", out)
	}
	if strings.Contains(out, "\r") {
		t.Errorf("non-interactive spinner should not emit carriage returns: %q", out)
	}
}

func TestProgressBarNonInteractive(t *testing.T) {
	th, buf := plainTheme()
	bar := th.NewProgressBar(buf, "model")
	bar.Update(0.0, "0/100")
	bar.Update(0.5, "50/100")
	bar.Update(1.0, "100/100")
	bar.Finish(th.SuccessLine("pulled"))
	out := buf.String()
	if strings.Contains(out, "\r") {
		t.Errorf("non-interactive bar should not emit carriage returns: %q", out)
	}
	if !strings.Contains(out, "50%") || !strings.Contains(out, "pulled") {
		t.Errorf("bar output = %q", out)
	}
}
