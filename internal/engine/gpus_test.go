package engine

import (
	"strings"
	"testing"
)

func fixed(n int) func() int { return func() int { return n } }

func TestParseGPURequest(t *testing.T) {
	cases := []struct {
		in      string
		want    GPURequest
		wantErr bool
	}{
		{in: "", want: GPURequest{}},
		{in: "all", want: GPURequest{All: true}},
		{in: "ALL", want: GPURequest{All: true}},
		{in: "2", want: GPURequest{Count: 2}},
		{in: " 1 ", want: GPURequest{Count: 1}},
		{in: "01", want: GPURequest{Count: 1}}, // numeric = count, normalized
		{in: "0,1", want: GPURequest{IDs: []string{"0", "1"}}},
		{in: "0, 1", want: GPURequest{IDs: []string{"0", "1"}}},
		{in: "0,01", want: GPURequest{IDs: []string{"0", "1"}}}, // indexes normalize
		{in: "1,01", wantErr: true},                             // "01" ≡ "1": duplicate device
		// UUIDs keep their case — NVIDIA tooling matching isn't guaranteed
		// case-insensitive, and labels should match nvidia-smi output.
		{in: "GPU-8f0c1a2b", want: GPURequest{IDs: []string{"GPU-8f0c1a2b"}}},
		{in: "MIG-abc", want: GPURequest{IDs: []string{"MIG-abc"}}},
		{in: "0,GPU-abc", want: GPURequest{IDs: []string{"0", "GPU-abc"}}},
		{in: "0", wantErr: true}, // a count must be ≥ 1 (0 is not "GPU 0")
		{in: "-1", wantErr: true},
		{in: "0,-1", wantErr: true}, // negative index is never a device
		{in: "1.5", wantErr: true},  // junk must fail at parse time,
		{in: "1e2", wantErr: true},  // not as an opaque NVIDIA error
		{in: "0x2", wantErr: true},  // at container start
		{in: "true", wantErr: true}, // e.g. unquoted `gpus: true` in YAML
		{in: "null", wantErr: true},
		{in: "0,0", wantErr: true}, // duplicate id
		{in: "0,,1", wantErr: true},
	}
	for _, c := range cases {
		got, err := ParseGPURequest(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseGPURequest(%q): expected error, got %+v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseGPURequest(%q): %v", c.in, err)
			continue
		}
		if got.All != c.want.All || got.Count != c.want.Count || strings.Join(got.IDs, ",") != strings.Join(c.want.IDs, ",") {
			t.Errorf("ParseGPURequest(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestGPUAllocatorCountsAndConflicts(t *testing.T) {
	// 4-GPU host; an existing instance holds 0,1.
	existing := []Instance{{Name: "held", GPUs: "0,1"}}
	a := NewGPUAllocator(fixed(4), existing)

	// A counted request gets the lowest free indexes.
	ids, err := a.Allocate("one", GPURequest{Count: 1})
	if err != nil || strings.Join(ids, ",") != "2" {
		t.Fatalf("count 1: ids=%v err=%v, want [2]", ids, err)
	}

	// Explicit ids conflict with existing claims.
	if _, err := a.Allocate("bad", GPURequest{IDs: []string{"0"}}); err == nil {
		t.Fatal("expected conflict on GPU 0 (held by another instance)")
	}
	// Out-of-range ids are rejected.
	if _, err := a.Allocate("oob", GPURequest{IDs: []string{"9"}}); err == nil {
		t.Fatal("expected out-of-range error for GPU 9 on a 4-GPU host")
	}
	// The last free GPU can be taken explicitly…
	if ids, err := a.Allocate("last", GPURequest{IDs: []string{"3"}}); err != nil || strings.Join(ids, ",") != "3" {
		t.Fatalf("explicit 3: ids=%v err=%v", ids, err)
	}
	// …after which counted requests fail with a clear shortfall.
	if _, err := a.Allocate("late", GPURequest{Count: 1}); err == nil || !strings.Contains(err.Error(), "0 of 4 are free") {
		t.Fatalf("expected shortfall error, got %v", err)
	}
}

func TestGPUAllocatorAllIsShareable(t *testing.T) {
	// Legacy behavior preserved: multiple all-GPU instances coexist (engines
	// like Ollama timeshare a GPU) — so a legacy stack with several
	// `gpu: true` members still gang-admits.
	a := NewGPUAllocator(fixed(2), []Instance{{Name: "old", GPUs: "all"}})
	if _, err := a.Allocate("second", GPURequest{All: true}); err != nil {
		t.Fatalf("all+all must coexist (legacy sharing), got %v", err)
	}
	// But partitions can't coexist with an all-holder: their exclusivity
	// promise would be violated by a container that sees every device.
	_, err := a.Allocate("part", GPURequest{Count: 1})
	if err == nil || !strings.Contains(err.Error(), `"old"`) {
		t.Fatalf("expected all-claimed error naming a holder, got %v", err)
	}
	// And "all" is refused while any partition exists.
	b := NewGPUAllocator(fixed(2), []Instance{{Name: "held", GPUs: "1"}})
	if _, err := b.Allocate("greedy", GPURequest{All: true}); err == nil {
		t.Fatal("expected 'all' to be refused while GPU 1 is reserved")
	}
}

func TestGPUAllocatorInventoryIsLazy(t *testing.T) {
	// "all" grants never need the host inventory — the probe (an nvidia-smi
	// subprocess in production) must not run for them.
	probed := false
	a := NewGPUAllocator(func() int { probed = true; return 0 }, nil)
	if _, err := a.Allocate("x", GPURequest{All: true}); err != nil {
		t.Fatalf("all on unknown-inventory host: %v", err)
	}
	if probed {
		t.Fatal("inventory probed for an all request")
	}
	// Counted/id requests do probe — and fail cleanly with no GPUs.
	if _, err := a.Allocate("y", GPURequest{All: true}); err != nil {
		t.Fatalf("second all grant refused: %v", err)
	}
	b := NewGPUAllocator(fixed(0), nil)
	if _, err := b.Allocate("x", GPURequest{Count: 1}); err == nil {
		t.Fatal("expected counted request to fail with no GPUs detected")
	}
	if _, err := b.Allocate("x", GPURequest{IDs: []string{"0"}}); err == nil {
		t.Fatal("expected id request to fail with no GPUs detected")
	}
}
