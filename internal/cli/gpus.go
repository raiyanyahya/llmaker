package cli

import (
	"os/exec"
	"strings"

	"github.com/raiyanyahya/llmaker/internal/engine"
)

// detectGPUCount probes the host's NVIDIA GPU inventory via nvidia-smi.
// Returns 0 when the tool is missing or reports nothing — GPU requests are
// then rejected with a clear error instead of failing inside Docker.
func detectGPUCount() int {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return 0
	}
	out, err := exec.Command("nvidia-smi", "--query-gpu=index", "--format=csv,noheader").Output()
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// gpuCount returns the host GPU inventory, using the injected prober in tests.
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
func resolveSpecGPUs(alloc *engine.GPUAllocator, spec *engine.Spec) error {
	req, err := engine.ParseGPURequest(spec.GPUs)
	if err != nil {
		return err
	}
	if req.IsZero() && spec.GPU {
		req = engine.GPURequest{All: true}
	}
	ids, all, err := alloc.Allocate(spec.Name, req)
	if err != nil {
		return err
	}
	spec.GPUIDs = ids
	spec.GPU = all
	return nil
}

// admitGPUs is apply's gang-admission pass: it resolves the GPU requests of
// every to-be-created instance in the stack against one allocator seeded with
// the existing fleet's claims. All grants succeed or the whole stack is
// rejected before any provisioning. Instances that already exist are skipped —
// they keep the reservation on their labels. The host is only probed when the
// stack actually asks for GPUs.
func admitGPUs(app *App, specs []engine.Spec, byName map[string]engine.Instance, existing []engine.Instance) error {
	var alloc *engine.GPUAllocator
	for i := range specs {
		spec := &specs[i]
		if _, ok := byName[spec.Name]; ok {
			continue
		}
		if spec.GPUs == "" && !spec.GPU {
			continue
		}
		if alloc == nil {
			alloc = engine.NewGPUAllocator(app.gpuCount(), existing)
		}
		if err := resolveSpecGPUs(alloc, spec); err != nil {
			return err
		}
	}
	return nil
}
