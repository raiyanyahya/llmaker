package engine

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// HostInfo is a best-effort snapshot of the machine llmaker is running on. It
// drives the sane defaults the plan calls for ("derived from host RAM/cores")
// and the `llmaker doctor` report.
type HostInfo struct {
	OS          string
	Arch        string
	CPUs        int
	MemoryBytes int64 // 0 if it could not be determined
}

// Host inspects the local machine.
func Host() HostInfo {
	return HostInfo{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		CPUs:        runtime.NumCPU(),
		MemoryBytes: hostMemoryBytes(),
	}
}

// DefaultCPUs picks a conservative CPU quota: all host cores, capped at 4, so a
// single instance never monopolizes a big machine by default.
func (h HostInfo) DefaultCPUs() float64 {
	c := h.CPUs
	switch {
	case c <= 0:
		return 4
	case c > 4:
		return 4
	default:
		return float64(c)
	}
}

// DefaultMemoryBytes picks a memory limit of ~75% of host RAM, capped at 8 GiB,
// with a 1 GiB floor. Falls back to 8 GiB when host RAM is unknown.
func (h HostInfo) DefaultMemoryBytes() int64 {
	if h.MemoryBytes <= 0 {
		return 8 * GiB
	}
	limit := h.MemoryBytes * 3 / 4
	if limit > 8*GiB {
		limit = 8 * GiB
	}
	if limit < 1*GiB {
		limit = 1 * GiB
	}
	return limit
}

// IsMac reports whether we're on macOS, where Docker cannot pass through the
// Apple GPU and `--native` mode should be recommended (plan §7).
func (h HostInfo) IsMac() bool { return h.OS == "darwin" }

// hostMemoryBytes reads total RAM from /proc/meminfo on Linux. On other
// platforms (or on failure) it returns 0, and callers fall back to a default.
func hostMemoryBytes() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * KiB // meminfo reports kibibytes
	}
	return 0
}
