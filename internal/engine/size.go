package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// Size unit constants (binary, matching Docker's interpretation of `--memory`).
const (
	KiB = 1024
	MiB = 1024 * KiB
	GiB = 1024 * MiB
	TiB = 1024 * GiB
)

// ParseSize converts a human memory string ("8g", "512m", "1.5gb", "2048")
// into bytes. A bare number is interpreted as bytes. Units are binary
// (1g == 1024m), matching Docker's `--memory` semantics so what the user types
// is exactly what Docker enforces.
func ParseSize(s string) (int64, error) {
	orig := s
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}

	// Split the numeric prefix from the unit suffix.
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	numPart, unit := s[:i], strings.TrimSpace(s[i:])
	if numPart == "" {
		return 0, fmt.Errorf("invalid size %q: missing number", orig)
	}

	num, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", orig, err)
	}
	if num < 0 {
		return 0, fmt.Errorf("invalid size %q: negative", orig)
	}

	var mult float64
	switch unit {
	case "", "b":
		mult = 1
	case "k", "kb", "kib":
		mult = KiB
	case "m", "mb", "mib":
		mult = MiB
	case "g", "gb", "gib":
		mult = GiB
	case "t", "tb", "tib":
		mult = TiB
	default:
		return 0, fmt.Errorf("invalid size %q: unknown unit %q", orig, unit)
	}
	return int64(num * mult), nil
}

// FormatSize renders a byte count as a compact human string ("7.8 GiB").
func FormatSize(bytes int64) string {
	switch {
	case bytes <= 0:
		return "0 B"
	case bytes >= TiB:
		return fmt.Sprintf("%.1f TiB", float64(bytes)/TiB)
	case bytes >= GiB:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/GiB)
	case bytes >= MiB:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/MiB)
	case bytes >= KiB:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/KiB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
