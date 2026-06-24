package engine

import (
	"testing"
	"time"
)

func TestInstanceURL(t *testing.T) {
	cases := []struct {
		host string
		port int
		want string
	}{
		{"127.0.0.1", 11500, "http://127.0.0.1:11500"},
		{"", 8080, "http://localhost:8080"},
		{"0.0.0.0", 9000, "http://localhost:9000"},
		{"::", 9000, "http://localhost:9000"},
		{"example.com", 80, "http://example.com:80"},
	}
	for _, c := range cases {
		in := Instance{Host: c.host, Port: c.port}
		if got := in.URL(); got != c.want {
			t.Errorf("URL(host=%q, port=%d) = %q, want %q", c.host, c.port, got, c.want)
		}
	}
}

func TestInstanceIsRunning(t *testing.T) {
	if !(Instance{State: StateRunning}).IsRunning() {
		t.Error("running instance should report IsRunning")
	}
	if (Instance{State: StateExited}).IsRunning() {
		t.Error("exited instance should not report IsRunning")
	}
}

func TestInstanceUptime(t *testing.T) {
	if (Instance{}).Uptime() != 0 {
		t.Error("zero Created should yield zero uptime")
	}
	in := Instance{Created: time.Now().Add(-time.Minute)}
	if in.Uptime() < 50*time.Second {
		t.Errorf("uptime = %v, want ~1m", in.Uptime())
	}
}

func TestContainerAndVolumeNames(t *testing.T) {
	if got := ContainerName("brave-llama"); got != "llmaker-brave-llama" {
		t.Errorf("ContainerName = %q", got)
	}
	if got := VolumeName("brave-llama"); got != "llmaker-brave-llama-models" {
		t.Errorf("VolumeName = %q", got)
	}
}
