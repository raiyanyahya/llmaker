package engine

import (
	"strings"
	"testing"
)

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
		{in: "0,1", want: GPURequest{IDs: []string{"0", "1"}}},
		{in: "gpu-uuid-abc", want: GPURequest{IDs: []string{"gpu-uuid-abc"}}},
		{in: "0", wantErr: true}, // a count must be ≥ 1 (0 is not "GPU 0")
		{in: "-1", wantErr: true},
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
	a := NewGPUAllocator(4, existing)

	// A counted request gets the lowest free indexes.
	ids, all, err := a.Allocate("one", GPURequest{Count: 1})
	if err != nil || all || strings.Join(ids, ",") != "2" {
		t.Fatalf("count 1: ids=%v all=%v err=%v, want [2]", ids, all, err)
	}

	// Explicit ids conflict with existing claims.
	if _, _, err := a.Allocate("bad", GPURequest{IDs: []string{"0"}}); err == nil {
		t.Fatal("expected conflict on GPU 0 (held by another instance)")
	}
	// Out-of-range ids are rejected.
	if _, _, err := a.Allocate("oob", GPURequest{IDs: []string{"9"}}); err == nil {
		t.Fatal("expected out-of-range error for GPU 9 on a 4-GPU host")
	}
	// The last free GPU can be taken explicitly…
	if ids, _, err := a.Allocate("last", GPURequest{IDs: []string{"3"}}); err != nil || strings.Join(ids, ",") != "3" {
		t.Fatalf("explicit 3: ids=%v err=%v", ids, err)
	}
	// …after which counted requests fail with a clear shortfall.
	if _, _, err := a.Allocate("late", GPURequest{Count: 1}); err == nil || !strings.Contains(err.Error(), "0 of 4 are free") {
		t.Fatalf("expected shortfall error, got %v", err)
	}
}

func TestGPUAllocatorAllSemantics(t *testing.T) {
	// "all" is refused while anything is claimed…
	a := NewGPUAllocator(2, []Instance{{Name: "held", GPUs: "1"}})
	if _, _, err := a.Allocate("greedy", GPURequest{All: true}); err == nil {
		t.Fatal("expected 'all' to be refused while GPU 1 is reserved")
	}
	// …and once granted, blocks everything else, naming the holder.
	b := NewGPUAllocator(2, nil)
	if _, all, err := b.Allocate("greedy", GPURequest{All: true}); err != nil || !all {
		t.Fatalf("all grant failed: all=%v err=%v", all, err)
	}
	_, _, err := b.Allocate("late", GPURequest{Count: 1})
	if err == nil || !strings.Contains(err.Error(), `"greedy"`) {
		t.Fatalf("expected all-claimed error naming the holder, got %v", err)
	}
	// An existing all-GPUs instance (label "all") blocks new requests too.
	c := NewGPUAllocator(2, []Instance{{Name: "old", GPUs: "all"}})
	if _, _, err := c.Allocate("new", GPURequest{Count: 1}); err == nil {
		t.Fatal("expected existing all-GPUs reservation to block a counted request")
	}
}

func TestGPUAllocatorNoGPUsDetected(t *testing.T) {
	a := NewGPUAllocator(0, nil)
	if _, _, err := a.Allocate("x", GPURequest{Count: 1}); err == nil {
		t.Fatal("expected counted request to fail with no GPUs detected")
	}
	if _, _, err := a.Allocate("x", GPURequest{IDs: []string{"0"}}); err == nil {
		t.Fatal("expected id request to fail with no GPUs detected")
	}
	// "all" keeps the legacy pass-through behavior (Docker decides), so a
	// GPU-less dev box behaves exactly as before this feature.
	if _, all, err := a.Allocate("x", GPURequest{All: true}); err != nil || !all {
		t.Fatalf("all on 0-GPU host: all=%v err=%v, want grant", all, err)
	}
}
