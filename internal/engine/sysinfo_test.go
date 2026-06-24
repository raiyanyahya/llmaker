package engine

import "testing"

func TestDefaultCPUs(t *testing.T) {
	cases := []struct {
		cpus int
		want float64
	}{
		{0, 4},
		{1, 1},
		{2, 2},
		{4, 4},
		{16, 4},
	}
	for _, c := range cases {
		h := HostInfo{CPUs: c.cpus}
		if got := h.DefaultCPUs(); got != c.want {
			t.Errorf("DefaultCPUs(cpus=%d) = %v, want %v", c.cpus, got, c.want)
		}
	}
}

func TestDefaultMemoryBytes(t *testing.T) {
	// Unknown host RAM -> 8 GiB fallback.
	if got := (HostInfo{MemoryBytes: 0}).DefaultMemoryBytes(); got != 8*GiB {
		t.Errorf("unknown RAM default = %d, want %d", got, int64(8*GiB))
	}
	// Large host -> capped at 8 GiB.
	if got := (HostInfo{MemoryBytes: 64 * GiB}).DefaultMemoryBytes(); got != 8*GiB {
		t.Errorf("large RAM default = %d, want cap %d", got, int64(8*GiB))
	}
	// Small host -> ~75% with a 1 GiB floor.
	if got := (HostInfo{MemoryBytes: 4 * GiB}).DefaultMemoryBytes(); got != 3*GiB {
		t.Errorf("4GiB host default = %d, want %d", got, int64(3*GiB))
	}
	if got := (HostInfo{MemoryBytes: 512 * MiB}).DefaultMemoryBytes(); got != 1*GiB {
		t.Errorf("tiny host default = %d, want 1 GiB floor", got)
	}
}

func TestHostPopulatesBasics(t *testing.T) {
	h := Host()
	if h.OS == "" || h.Arch == "" {
		t.Errorf("Host() should populate OS/Arch, got %+v", h)
	}
	if h.CPUs < 1 {
		t.Errorf("Host() CPUs = %d, want >= 1", h.CPUs)
	}
}
