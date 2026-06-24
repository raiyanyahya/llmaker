package engine

import "testing"

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"1024", 1024},
		{"1b", 1},
		{"8g", 8 * GiB},
		{"8G", 8 * GiB},
		{"8gb", 8 * GiB},
		{"8gib", 8 * GiB},
		{"512m", 512 * MiB},
		{"1.5g", GiB + 512*MiB},
		{"  2g  ", 2 * GiB},
		{"4k", 4 * KiB},
		{"1t", TiB},
	}
	for _, c := range cases {
		got, err := ParseSize(c.in)
		if err != nil {
			t.Errorf("ParseSize(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseSizeErrors(t *testing.T) {
	for _, in := range []string{"", "g", "abc", "-1g", "8xb", "1.2.3g"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) expected error, got nil", in)
		}
	}
}

func TestFormatSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{2 * KiB, "2.0 KiB"},
		{3 * MiB, "3.0 MiB"},
		{8 * GiB, "8.0 GiB"},
		{2 * TiB, "2.0 TiB"},
	}
	for _, c := range cases {
		if got := FormatSize(c.in); got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ParseSize and FormatSize should be approximately inverse for whole units.
func TestSizeRoundTrip(t *testing.T) {
	for _, s := range []string{"8g", "512m", "4k", "2t"} {
		bytes, err := ParseSize(s)
		if err != nil {
			t.Fatalf("ParseSize(%q): %v", s, err)
		}
		if got, err := ParseSize(FormatSize(bytes)); err != nil || got != bytes {
			t.Errorf("round trip of %q: got %d (%v), want %d", s, got, err, bytes)
		}
	}
}
