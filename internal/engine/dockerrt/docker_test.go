package dockerrt

import (
	"testing"

	"github.com/raiyanyahya/llmaker/internal/engine"
)

func TestResourcesGPUPartition(t *testing.T) {
	// Resolved device ids become an exclusive DeviceIDs request…
	res := resources(engine.Spec{GPUIDs: []string{"0", "2"}})
	if len(res.DeviceRequests) != 1 {
		t.Fatalf("DeviceRequests = %v, want exactly one", res.DeviceRequests)
	}
	dr := res.DeviceRequests[0]
	if dr.Count != 0 || len(dr.DeviceIDs) != 2 || dr.DeviceIDs[0] != "0" || dr.DeviceIDs[1] != "2" {
		t.Errorf("partition request = %+v, want DeviceIDs [0 2]", dr)
	}
	// …the legacy bool still reserves everything…
	all := resources(engine.Spec{GPU: true})
	if len(all.DeviceRequests) != 1 || all.DeviceRequests[0].Count != -1 {
		t.Errorf("all request = %+v, want Count -1", all.DeviceRequests)
	}
	// …and an unresolved raw request is ignored (must be resolved upstream).
	raw := resources(engine.Spec{GPUs: "2"})
	if len(raw.DeviceRequests) != 0 {
		t.Errorf("unresolved request must not reach Docker, got %+v", raw.DeviceRequests)
	}
}

func TestNetworkFor(t *testing.T) {
	if got := networkFor(""); got != NetworkName {
		t.Errorf(`networkFor("") = %q, want the shared %q`, got, NetworkName)
	}
	if got := networkFor("rag"); got != NetworkName+"-rag" {
		t.Errorf(`networkFor("rag") = %q, want %q`, got, NetworkName+"-rag")
	}
}
