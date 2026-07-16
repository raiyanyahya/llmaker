package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// GPURequest is a parsed GPU reservation request:
//
//	"all"      → every host GPU (the legacy `--gpu` / `gpu: true` behavior;
//	             shareable between all-GPU instances)
//	"2"        → any 2 free GPUs, chosen by the allocator (exclusive)
//	"0,3"      → exactly these device indexes/UUIDs, exclusively
//	""         → no GPU
type GPURequest struct {
	All   bool
	Count int
	IDs   []string
}

// IsZero reports whether nothing was requested.
func (r GPURequest) IsZero() bool { return !r.All && r.Count == 0 && len(r.IDs) == 0 }

// String renders the request back to its canonical spec form.
func (r GPURequest) String() string {
	switch {
	case r.All:
		return "all"
	case r.Count > 0:
		return strconv.Itoa(r.Count)
	default:
		return strings.Join(r.IDs, ",")
	}
}

// ParseGPURequest parses the user-facing request syntax shared by the --gpus
// flag and the `gpus:` stack-file field. Device tokens are validated here so
// junk ("true", "1.5", "-1") fails at parse time with a clear message instead
// of surfacing later as an opaque NVIDIA runtime error at container start.
func ParseGPURequest(s string) (GPURequest, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return GPURequest{}, nil
	}
	if strings.EqualFold(s, "all") {
		return GPURequest{All: true}, nil
	}
	if !strings.Contains(s, ",") {
		if n, err := strconv.Atoi(s); err == nil {
			if n <= 0 {
				return GPURequest{}, fmt.Errorf("invalid GPU count %q (must be ≥ 1, or use device ids like \"0,1\")", s)
			}
			return GPURequest{Count: n}, nil
		}
	}
	// A comma list (or a single non-numeric token, e.g. a GPU UUID) names
	// explicit devices.
	var ids []string
	seen := map[string]bool{}
	for _, tok := range strings.Split(s, ",") {
		id, err := canonicalGPUID(strings.TrimSpace(tok))
		if err != nil {
			return GPURequest{}, err
		}
		if seen[id] {
			return GPURequest{}, fmt.Errorf("invalid GPU request %q: device %s named twice", s, id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return GPURequest{IDs: ids}, nil
}

// canonicalGPUID validates one device token: a non-negative index (normalized,
// so "01" and "1" can't double-book one device) or an NVIDIA GPU/MIG UUID
// (case preserved — UUID matching in NVIDIA tooling is not guaranteed to be
// case-insensitive, and the stored label should match nvidia-smi output).
func canonicalGPUID(tok string) (string, error) {
	if tok == "" {
		return "", fmt.Errorf("invalid GPU request: empty device id")
	}
	if isDigits(tok) {
		n, err := strconv.Atoi(tok)
		if err != nil { // only overflow can fail here
			return "", fmt.Errorf("invalid GPU device index %q", tok)
		}
		return strconv.Itoa(n), nil
	}
	up := strings.ToUpper(tok)
	if strings.HasPrefix(up, "GPU-") || strings.HasPrefix(up, "MIG-") {
		return tok, nil
	}
	return "", fmt.Errorf("invalid GPU device %q (use an index like 0, or a UUID like GPU-…)", tok)
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// GPUAllocator hands out GPU reservations against a host inventory, accounting
// for what existing instances already claim (via their llmaker.gpus labels).
// Counted/id reservations are exclusive partitions; "all" grants are shareable
// with each other (the legacy `--gpu` behavior — engines like Ollama timeshare
// a GPU) but never coexist with partitions, whose exclusivity they would
// violate. One allocator is threaded through a whole stack admission so that
// either every member gets its GPUs or the stack is rejected as a unit.
//
// The inventory probe is lazy: "all" grants never need it, so a plain --gpu on
// a GPU-less dev box keeps its legacy pass-through behavior with no probe.
type GPUAllocator struct {
	totalFn    func() int
	total      *int
	claimed    map[string]bool // device ids reserved by existing instances
	allHolders []string        // instances holding ALL GPUs (shareable set)
}

// NewGPUAllocator builds an allocator over the host inventory reported by
// total() (probed lazily), pre-claiming the reservations recorded on existing
// instances. Instances created before GPU labels existed are invisible here —
// they reserved all GPUs implicitly and cannot be accounted for.
func NewGPUAllocator(total func() int, existing []Instance) *GPUAllocator {
	a := &GPUAllocator{totalFn: total, claimed: map[string]bool{}}
	for _, in := range existing {
		switch in.GPUs {
		case "":
		case "all":
			a.allHolders = append(a.allHolders, in.Name)
		default:
			for _, id := range strings.Split(in.GPUs, ",") {
				a.claimed[id] = true
			}
		}
	}
	return a
}

func (a *GPUAllocator) hostGPUs() int {
	if a.total == nil {
		n := 0
		if a.totalFn != nil {
			n = a.totalFn()
		}
		a.total = &n
	}
	return *a.total
}

// Allocate resolves one request to concrete device ids ("all" grants resolve
// to nil ids — the runtime maps them to every device), marking claims so
// subsequent calls see them as taken. `name` is the requesting instance, used
// for error messages.
func (a *GPUAllocator) Allocate(name string, req GPURequest) ([]string, error) {
	if req.IsZero() {
		return nil, nil
	}
	if req.All {
		if len(a.claimed) > 0 {
			return nil, fmt.Errorf("instance %q: cannot reserve all GPUs — %s already reserved by other instances",
				name, joinSorted(a.claimed))
		}
		a.allHolders = append(a.allHolders, name)
		return nil, nil
	}
	if len(a.allHolders) > 0 {
		return nil, fmt.Errorf("instance %q: all GPUs are reserved by %q", name, a.allHolders[0])
	}
	if a.hostGPUs() <= 0 {
		return nil, fmt.Errorf("instance %q: no NVIDIA GPUs detected on this host (is nvidia-smi installed?)", name)
	}
	if len(req.IDs) > 0 {
		for _, id := range req.IDs {
			if a.claimed[id] {
				return nil, fmt.Errorf("instance %q: GPU %s is already reserved by another instance", name, id)
			}
			if n, err := strconv.Atoi(id); err == nil && n >= a.hostGPUs() {
				return nil, fmt.Errorf("instance %q: GPU %s does not exist (host has %d: 0-%d)", name, id, a.hostGPUs(), a.hostGPUs()-1)
			}
		}
		for _, id := range req.IDs {
			a.claimed[id] = true
		}
		return req.IDs, nil
	}
	// Counted request: hand out the lowest-numbered free indexes.
	var free []string
	for i := 0; i < a.hostGPUs() && len(free) < req.Count; i++ {
		if id := strconv.Itoa(i); !a.claimed[id] {
			free = append(free, id)
		}
	}
	if len(free) < req.Count {
		return nil, fmt.Errorf("instance %q: needs %d GPU(s) but only %d of %d are free",
			name, req.Count, len(free), a.hostGPUs())
	}
	for _, id := range free {
		a.claimed[id] = true
	}
	return free, nil
}

func joinSorted(set map[string]bool) string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
