package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/raiyanyahya/llmaker/internal/engine"
)

// nvidiaGPUs probes the host's NVIDIA inventory via nvidia-smi, returning one
// name per device. A missing binary surfaces as exec.ErrNotFound (via err);
// any error means "no usable inventory". One probe serves both doctor's report
// and the GPU allocator so the two can never disagree about the same host.
//
// Note the probe runs on the CLI host: with DOCKER_HOST pointing at a remote
// daemon, counted/id GPU requests can't see the remote inventory.
func nvidiaGPUs() ([]string, error) {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil, err
	}
	out, err := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader").Output()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func detectGPUCount() int {
	names, err := nvidiaGPUs()
	if err != nil {
		return 0
	}
	return len(names)
}

// gpuCount returns the host GPU inventory, using the injected prober in tests.
// Callers pass it (not its result) to the allocator, which probes lazily —
// "all" requests never need the count.
func (a *App) gpuCount() int {
	if a.GPUCount != nil {
		return a.GPUCount()
	}
	return detectGPUCount()
}

// resolveSpecGPUs resolves a spec's raw GPU request (spec.GPUs / legacy
// spec.GPU) into a concrete reservation using the shared allocator: GPUIDs for
// a partition, GPU=true for an all-GPUs grant. Threading one allocator through
// several specs gives gang admission — each grant is visible to the next.
// This is the choke point every entry surface funnels through, so the
// gpu-vs-gpus conflict is enforced here too (surfaces may add friendlier
// checks earlier, but none can bypass this one).
func resolveSpecGPUs(alloc *engine.GPUAllocator, spec *engine.Spec) error {
	if spec.GPUs != "" && spec.GPU {
		return fmt.Errorf("instance %q: use either gpu or gpus, not both", spec.Name)
	}
	req, err := engine.ParseGPURequest(spec.GPUs)
	if err != nil {
		return fmt.Errorf("instance %q: %w", spec.Name, err)
	}
	if req.IsZero() && spec.GPU {
		req = engine.GPURequest{All: true}
	}
	ids, err := alloc.Allocate(spec.Name, req)
	if err != nil {
		return err
	}
	spec.GPUIDs = ids
	spec.GPU = req.All
	return nil
}

// admitGPUs is apply's gang-admission pass: it resolves the GPU requests of
// every to-be-created instance in the stack against one allocator seeded with
// the existing fleet's claims. All grants succeed or the whole stack is
// rejected before any provisioning. Instances that already exist are skipped —
// they keep the reservation on their labels. Names in `doomed` (instances the
// same run will prune) don't count as claim holders: their devices are freed
// by the prune, and the moment of overlap while the new container is created
// before the old one is removed is harmless — Docker doesn't enforce GPU
// exclusivity. The host is probed only when the stack actually asks for GPUs.
func admitGPUs(app *App, specs []engine.Spec, existing []engine.Instance, doomed map[string]bool) error {
	current := make(map[string]bool, len(existing))
	for _, in := range existing {
		current[in.Name] = true
	}
	var alloc *engine.GPUAllocator
	for i := range specs {
		spec := &specs[i]
		if current[spec.Name] {
			continue
		}
		if spec.GPUs == "" && !spec.GPU {
			continue
		}
		if alloc == nil {
			seed := existing
			if len(doomed) > 0 {
				seed = make([]engine.Instance, 0, len(existing))
				for _, in := range existing {
					if !doomed[in.Name] {
						seed = append(seed, in)
					}
				}
			}
			alloc = engine.NewGPUAllocator(app.gpuCount, seed)
		}
		if err := resolveSpecGPUs(alloc, spec); err != nil {
			return err
		}
	}
	return nil
}
