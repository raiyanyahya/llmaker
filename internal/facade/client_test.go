package facade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","ready":true}`))
	}))
	defer srv.Close()

	c := NewClient()
	h, err := c.Health(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !h.Ready || h.Status != "ok" {
		t.Fatalf("Health = %+v", h)
	}
}

func TestHealthNotReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"starting"}`))
	}))
	defer srv.Close()

	if _, err := NewClient().Health(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error for 503")
	}
}

func TestStatusAndAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want Bearer secret", got)
		}
		_, _ = w.Write([]byte(`{
			"instance":{"name":"brave-llama","backend":"ollama","version":"0.1.0","uptime_seconds":42,"default_model":"llama3:8b"},
			"system":{"cpu_percent":12.5,"memory_used":100,"memory_total":200,"memory_percent":50,"gpus":[{"name":"RTX","utilization":30,"memory_used":1,"memory_total":2}]},
			"models":{"default":"llama3:8b","running":[{"name":"llama3:8b","size":10,"vram":5}],"installed":[{"name":"llama3:8b","size":10}]},
			"metrics":{"tokens_per_second":12.3,"requests_total":7}
		}`))
	}))
	defer srv.Close()

	c := NewClient(WithAPIKey("secret"))
	st, err := c.Status(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Instance.Name != "brave-llama" || st.Instance.Backend != "ollama" {
		t.Errorf("instance = %+v", st.Instance)
	}
	if st.System.CPUPercent != 12.5 || len(st.System.GPUs) != 1 {
		t.Errorf("system = %+v", st.System)
	}
	if st.Models.Default != "llama3:8b" || len(st.Models.Running) != 1 {
		t.Errorf("models = %+v", st.Models)
	}
	if st.Metrics.TokensPerSecond != 12.3 {
		t.Errorf("metrics = %+v", st.Metrics)
	}
}

func TestPullStreamsProgressAndSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		lines := []string{
			`{"status":"pulling manifest"}`,
			`{"status":"downloading","completed":50,"total":100}`,
			`{"status":"downloading","completed":100,"total":100}`,
			`{"status":"success"}`,
		}
		for _, l := range lines {
			_, _ = w.Write([]byte(l + "\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	var events []PullEvent
	err := NewClient().Pull(context.Background(), srv.URL, "llama3:8b", func(e PullEvent) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d: %+v", len(events), events)
	}
	if f := events[1].Fraction(); f != 0.5 {
		t.Errorf("second event fraction = %v, want 0.5", f)
	}
	if !events[3].Done() {
		t.Errorf("last event should be Done()")
	}
}

func TestPullReportsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"pulling manifest"}` + "\n" + `{"status":"error","error":"no such model"}` + "\n"))
	}))
	defer srv.Close()

	err := NewClient().Pull(context.Background(), srv.URL, "bogus", nil)
	if err == nil || !strings.Contains(err.Error(), "no such model") {
		t.Fatalf("expected error mentioning model, got %v", err)
	}
}

func TestChatStreamsDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":" world"}}]}`,
			`data: [DONE]`,
		}
		for _, c := range chunks {
			_, _ = w.Write([]byte(c + "\n\n"))
		}
	}))
	defer srv.Close()

	var sb strings.Builder
	err := NewClient().Chat(context.Background(), srv.URL, ChatRequest{
		Model:    "llama3:8b",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, func(s string) { sb.WriteString(s) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if sb.String() != "Hello world" {
		t.Fatalf("assembled content = %q, want %q", sb.String(), "Hello world")
	}
}

func TestSetDefaultAndDelete(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient()
	if err := c.SetDefault(context.Background(), srv.URL, "llama3:8b"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	if err := c.Delete(context.Background(), srv.URL, "llama3:8b"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(calls) != 2 || calls[0] != "/api/models/default" || calls[1] != "/api/models/delete" {
		t.Fatalf("unexpected calls: %v", calls)
	}
}

func TestErrorBodyExtraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"bad model name"}`))
	}))
	defer srv.Close()

	_, err := NewClient().ListModels(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "bad model name") {
		t.Fatalf("expected detail in error, got %v", err)
	}
}
