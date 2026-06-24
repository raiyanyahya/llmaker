package engine

import (
	"fmt"
	"net"
	"testing"
)

func TestAllocatePortSkipsUsed(t *testing.T) {
	used := map[int]bool{PortRangeStart: true, PortRangeStart + 1: true}
	p, err := AllocatePort(used)
	if err != nil {
		t.Fatalf("AllocatePort: %v", err)
	}
	if used[p] {
		t.Fatalf("AllocatePort returned a used port %d", p)
	}
	if p < PortRangeStart || p > PortRangeEnd {
		// Fallback to ephemeral is allowed only when the range is exhausted,
		// which is not the case here.
		t.Fatalf("AllocatePort returned %d outside friendly range", p)
	}
}

func TestAllocatePortSkipsBoundPort(t *testing.T) {
	// Bind the first port in the range so it appears unavailable at the OS level.
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", PortRangeStart))
	if err != nil {
		t.Skipf("could not bind %d to set up test: %v", PortRangeStart, err)
	}
	defer l.Close()

	p, err := AllocatePort(nil)
	if err != nil {
		t.Fatalf("AllocatePort: %v", err)
	}
	if p == PortRangeStart {
		t.Fatalf("AllocatePort returned a bound port %d", p)
	}
}

func TestFreePortReturnsUsablePort(t *testing.T) {
	p, err := FreePort()
	if err != nil {
		t.Fatalf("FreePort: %v", err)
	}
	if p <= 0 || p > 65535 {
		t.Fatalf("FreePort returned invalid port %d", p)
	}
	// The returned port should be immediately bindable.
	if !PortAvailable(p) {
		t.Fatalf("FreePort returned %d but it is not available", p)
	}
}

func TestUsedPorts(t *testing.T) {
	instances := []Instance{
		{Name: "a", Port: 11500},
		{Name: "b", Port: 11501},
		{Name: "c", Port: 0}, // not yet assigned
	}
	used := UsedPorts(instances)
	if !used[11500] || !used[11501] {
		t.Fatalf("expected 11500 and 11501 used, got %v", used)
	}
	if used[0] {
		t.Fatalf("port 0 should not be considered used")
	}
}
