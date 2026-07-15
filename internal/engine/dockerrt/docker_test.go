package dockerrt

import "testing"

func TestNetworkFor(t *testing.T) {
	if got := networkFor(""); got != NetworkName {
		t.Errorf(`networkFor("") = %q, want the shared %q`, got, NetworkName)
	}
	if got := networkFor("rag"); got != NetworkName+"-rag" {
		t.Errorf(`networkFor("rag") = %q, want %q`, got, NetworkName+"-rag")
	}
}
