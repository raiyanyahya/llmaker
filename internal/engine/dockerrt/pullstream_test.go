package dockerrt

import (
	"strings"
	"testing"
)

func TestDecodePullStream(t *testing.T) {
	stream := `{"status":"Pulling from raiyanyahya/llmaker-ollama","id":"latest"}
{"status":"Downloading","id":"abc123","progress":"[====>   ] 50MB/100MB"}
{"status":"Download complete","id":"abc123"}
{"status":"Status: Downloaded newer image"}`

	var lines []string
	err := decodePullStream(strings.NewReader(stream), func(s string) {
		lines = append(lines, s)
	})
	if err != nil {
		t.Fatalf("decodePullStream: %v", err)
	}
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[1], "50MB/100MB") {
		t.Errorf("progress line = %q", lines[1])
	}
}

func TestDecodePullStreamError(t *testing.T) {
	stream := `{"status":"Pulling"}
{"error":"manifest unknown"}`
	err := decodePullStream(strings.NewReader(stream), nil)
	if err == nil || !strings.Contains(err.Error(), "manifest unknown") {
		t.Fatalf("expected manifest error, got %v", err)
	}
}
