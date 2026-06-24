// Package facade is the Go client for the per-instance control-plane facade
// (implemented in Python/FastAPI under ./facade). Every backend — Ollama,
// llama.cpp, future engines — is reached through this one normalized contract,
// so the CLI never special-cases an engine.
//
// The CLI deliberately speaks plain HTTP here (JSON for control, NDJSON for
// pull progress, SSE for chat) and never opens a WebSocket: the facade's
// WebSocket endpoints exist for the browser UI, while the terminal gets
// everything it needs by polling /api/status and streaming the pull/chat
// responses. That keeps the Go binary dependency-light and robust.
package facade

// Health is the response of GET /api/health.
type Health struct {
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
}

// Status is the aggregate snapshot from GET /api/status that powers both the
// web UI and `llmaker top`.
type Status struct {
	Instance InstanceInfo `json:"instance"`
	System   SystemInfo   `json:"system"`
	Models   ModelsInfo   `json:"models"`
	Metrics  MetricsInfo  `json:"metrics"`
}

// InstanceInfo identifies the instance and its lifetime.
type InstanceInfo struct {
	Name          string  `json:"name"`
	Backend       string  `json:"backend"`
	Version       string  `json:"version"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	DefaultModel  string  `json:"default_model"`
}

// SystemInfo carries live host/accelerator load.
type SystemInfo struct {
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryUsed    int64     `json:"memory_used"`
	MemoryTotal   int64     `json:"memory_total"`
	MemoryPercent float64   `json:"memory_percent"`
	GPUs          []GPUInfo `json:"gpus"`
}

// GPUInfo is one accelerator's utilization and VRAM.
type GPUInfo struct {
	Name        string  `json:"name"`
	Utilization float64 `json:"utilization"`
	MemoryUsed  int64   `json:"memory_used"`
	MemoryTotal int64   `json:"memory_total"`
}

// ModelsInfo splits models into currently-loaded vs installed-on-disk.
type ModelsInfo struct {
	Default   string           `json:"default"`
	Running   []RunningModel   `json:"running"`
	Installed []InstalledModel `json:"installed"`
}

// RunningModel is a model loaded in (V)RAM.
type RunningModel struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	VRAM int64  `json:"vram"`
}

// InstalledModel is a model present on disk.
type InstalledModel struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// ModelList is the response of GET /api/models.
type ModelList struct {
	Default string           `json:"default"`
	Models  []InstalledModel `json:"models"`
}

// MetricsInfo carries lightweight serving metrics.
type MetricsInfo struct {
	TokensPerSecond float64 `json:"tokens_per_second"`
	RequestsTotal   int64   `json:"requests_total"`
}

// PullEvent is one NDJSON line from POST /api/models/pull.
type PullEvent struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Completed int64  `json:"completed"`
	Total     int64  `json:"total"`
	Error     string `json:"error"`
}

// Done reports whether this event marks successful completion.
func (e PullEvent) Done() bool { return e.Status == "success" }

// Failed reports whether this event marks a failed pull.
func (e PullEvent) Failed() bool { return e.Status == "error" || e.Error != "" }

// Fraction returns download progress in [0,1], or -1 when total is unknown.
func (e PullEvent) Fraction() float64 {
	if e.Total <= 0 {
		return -1
	}
	f := float64(e.Completed) / float64(e.Total)
	if f > 1 {
		return 1
	}
	if f < 0 {
		return 0
	}
	return f
}

// ChatMessage is one OpenAI-style message.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the subset of the OpenAI chat schema the CLI sends.
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}
