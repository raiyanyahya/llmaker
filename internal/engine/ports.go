package engine

import (
	"fmt"
	"net"
)

// llmaker auto-assigns a free host port per instance so several servers never
// collide on a single well-known port. We prefer a friendly, predictable range
// (so URLs are memorable) and fall back to an OS-chosen ephemeral port if the
// whole range is taken.
const (
	PortRangeStart = 11500
	PortRangeEnd   = 11999
)

// AllocatePort returns a host port that is both free on the OS and not already
// claimed by another managed instance. used should contain the ports of the
// current fleet (from labels) so we don't hand out one that's reserved but not
// yet bound.
func AllocatePort(used map[int]bool) (int, error) {
	for p := PortRangeStart; p <= PortRangeEnd; p++ {
		if used[p] {
			continue
		}
		if PortAvailable(p) {
			return p, nil
		}
	}
	// Range exhausted: ask the OS for any free port.
	p, err := FreePort()
	if err != nil {
		return 0, fmt.Errorf("no free port available: %w", err)
	}
	if used[p] {
		return 0, fmt.Errorf("no free port available")
	}
	return p, nil
}

// PortAvailable reports whether a TCP port can currently be bound on localhost.
func PortAvailable(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// FreePort asks the operating system for an unused ephemeral TCP port.
func FreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// UsedPorts collects the ports currently claimed by a set of instances.
func UsedPorts(instances []Instance) map[int]bool {
	used := make(map[int]bool, len(instances))
	for _, in := range instances {
		if in.Port > 0 {
			used[in.Port] = true
		}
	}
	return used
}
