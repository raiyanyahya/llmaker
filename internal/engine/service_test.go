package engine

import "testing"

func TestServiceLabelRoundTrip(t *testing.T) {
	spec := ServiceSpec{
		Name:     "qdrant",
		Service:  "qdrant",
		Category: "vector-db",
		Image:    "qdrant/qdrant:latest",
		Host:     "127.0.0.1",
		Ports: []PortBinding{
			{Host: 11500, Container: 6333, Name: "http", Primary: true},
			{Host: 11501, Container: 6334, Name: "grpc"},
		},
	}
	labels := ServiceLabels(spec)
	if TypeOf(labels) != TypeService {
		t.Fatalf("TypeOf = %q, want service", TypeOf(labels))
	}

	svc := ServiceFromLabels("abc123", StateRunning, labels)
	if svc.Name != "qdrant" || svc.Kind != "qdrant" || svc.Category != "vector-db" {
		t.Fatalf("unexpected service fields: %+v", svc)
	}
	if len(svc.Ports) != 2 {
		t.Fatalf("got %d ports, want 2", len(svc.Ports))
	}
	if svc.PrimaryPort() != 11500 {
		t.Errorf("PrimaryPort = %d, want 11500", svc.PrimaryPort())
	}
	if got := svc.Endpoint(); got != "qdrant:6333" {
		t.Errorf("Endpoint = %q, want qdrant:6333", got)
	}
	p := svc.Ports[1]
	if p.Host != 11501 || p.Container != 6334 || p.Name != "grpc" || p.Primary {
		t.Errorf("secondary port round-trip wrong: %+v", p)
	}
}

func TestTypeOfDefaultsToInstance(t *testing.T) {
	// Containers created before services existed carry no type label.
	if TypeOf(map[string]string{}) != TypeInstance {
		t.Fatal("missing type label should read as instance")
	}
}

func TestInstanceLabelsAreTypedInstance(t *testing.T) {
	labels := SpecLabels(Spec{Name: "a", Host: "127.0.0.1"}, "img", 11500)
	if TypeOf(labels) != TypeInstance {
		t.Fatalf("instance labels TypeOf = %q", TypeOf(labels))
	}
}
