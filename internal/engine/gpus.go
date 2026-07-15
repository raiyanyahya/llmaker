package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// GPURequest is a parsed GPU reservation request:
//
//	"all"      → every host GPU (the legacy `--gpu` / `gpu: true` behavior)
//	"2"        → any 2 free GPUs, chosen by the allocator
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
// flag and the `gpus:` stack-file field.
func ParseGPURequest(s string) (GPURequest, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return GPURequest{}, nil
	}
	if s == "all" {
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
		tok = strings.TrimSpace(tok)
		if tok == "" || seen[tok] {
			return GPURequest{}, fmt.Errorf("invalid GPU request %q (use \"all\", a count, or unique device ids like \"0,1\")", s)
		}
		seen[tok] = true
		ids = append(ids, tok)
	}
	return GPURequest{IDs: ids}, nil
}

// GPUAllocator hands out exclusive GPU reservations against a host inventory,
// accounting for what existing instances already claim (via their llmaker.gpus
// labels). One allocator is threaded through a whole stack admission so that
// either every member gets its GPUs or the stack is rejected as a unit.
type GPUAllocator struct {
	total      int             // detected host GPUs; 0 = none/unknown
	claimed    map[string]bool // device ids reserved by existing instances
	allClaimed string          // name of an instance holding ALL GPUs ("" if none)
}

// NewGPUAllocator builds an allocator over `total` host GPUs, pre-claiming the
// reservations recorded on existing instances. Instances created before GPU
// labels existed are invisible here — they reserved all GPUs implicitly and
// cannot be accounted for.
func NewGPUAllocator(total int, existing []Instance) *GPUAllocator {
	a := &GPUAllocator{total: total, claimed: map[string]bool{}}
	for _, in := range existing {
		switch in.GPUs {
		case "":
		case "all":
			a.allClaimed = in.Name
		default:
			for _, id := range strings.Split(in.GPUs, ",") {
				a.claimed[id] = true
			}
		}
	}
	return a
}

// Allocate resolves one request to concrete device ids (or an all-GPUs grant),
// marking them claimed so subsequent calls see them as taken. `name` is the
// requesting instance, used only for error messages.
func (a *GPUAllocator) Allocate(name string, req GPURequest) (ids []string, all bool, err error) {
	if req.IsZero() {
		return nil, false, nil
	}
	if a.allClaimed != "" {
		return nil, false, fmt.Errorf("instance %q: all GPUs are reserved by %q", name, a.allClaimed)
	}
	if req.All {
		if len(a.claimed) > 0 {
			return nil, false, fmt.Errorf("instance %q: cannot reserve all GPUs — %s already reserved by other instances",
				name, joinSorted(a.claimed))
		}
		a.allClaimed = name
		return nil, true, nil
	}
	if a.total <= 0 {
		return nil, false, fmt.Errorf("instance %q: no NVIDIA GPUs detected on this host (is nvidia-smi installed?)", name)
	}
	if len(req.IDs) > 0 {
		for _, id := range req.IDs {
			if a.claimed[id] {
				return nil, false, fmt.Errorf("instance %q: GPU %s is already reserved by another instance", name, id)
			}
			if n, err := strconv.Atoi(id); err == nil && n >= a.total {
				return nil, false, fmt.Errorf("instance %q: GPU %s does not exist (host has %d: 0-%d)", name, id, a.total, a.total-1)
			}
		}
		for _, id := range req.IDs {
			a.claimed[id] = true
		}
		return req.IDs, false, nil
	}
	// Counted request: hand out the lowest-numbered free indexes.
	var free []string
	for i := 0; i < a.total && len(free) < req.Count; i++ {
		if id := strconv.Itoa(i); !a.claimed[id] {
			free = append(free, id)
		}
	}
	if len(free) < req.Count {
		return nil, false, fmt.Errorf("instance %q: needs %d GPU(s) but only %d of %d are free",
			name, req.Count, len(free), a.total)
	}
	for _, id := range free {
		a.claimed[id] = true
	}
	return free, false, nil
}

func joinSorted(set map[string]bool) string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
