// Package service describes the infrastructure services llmaker can run
// alongside LLM instances — the rest of a modern LLM stack: vector databases,
// caches, dedicated embedding servers, and observability. Each entry is a
// curated, parameterized container definition, the same way internal/backend
// describes inference engines.
//
// Adding a service is intentionally cheap: register a Service here. The CLI,
// the fleet view, and declarative stacks pick it up automatically, and every
// service joins the shared llmaker network so instances and services can reach
// each other by name (e.g. an app talks to "qdrant:6333" and "chat:8080").
package service

import (
	"fmt"
	"sort"
	"strings"
)

// Category groups catalog entries for help output and discovery.
type Category string

const (
	CategoryVectorDB      Category = "vector-db"
	CategoryCache         Category = "cache"
	CategoryEmbeddings    Category = "embeddings"
	CategorySearch        Category = "search"
	CategorySpeech        Category = "speech"
	CategoryObservability Category = "observability"
	CategoryUI            Category = "ui"
	CategoryAgent         Category = "agent"
)

// Port is a port a service listens on inside its container.
type Port struct {
	Container int    // port inside the container
	Name      string // human label, e.g. "http", "grpc"
	Primary   bool   // the main port users hit; gets the first friendly host port
}

// Volume is a named data volume a service needs to persist state across
// restarts. The full Docker volume name is derived from the instance name.
type Volume struct {
	Suffix string // volume-name suffix (llmaker-<name>-<suffix>)
	Path   string // mount path inside the container
}

// Service is the static catalog description of a supported infrastructure
// service.
type Service struct {
	Kind        string
	DisplayName string
	Category    Category
	Image       string
	Description string
	Ports       []Port
	Volumes     []Volume
	Env         map[string]string
	// Notes is optional guidance surfaced in `service catalog` (e.g. a
	// dependency on another service).
	Notes string
}

// PrimaryPort returns the port users primarily connect to.
func (s Service) PrimaryPort() Port {
	for _, p := range s.Ports {
		if p.Primary {
			return p
		}
	}
	if len(s.Ports) > 0 {
		return s.Ports[0]
	}
	return Port{}
}

var registry = map[string]Service{
	"qdrant": {
		Kind:        "qdrant",
		DisplayName: "Qdrant",
		Category:    CategoryVectorDB,
		Image:       "qdrant/qdrant:latest",
		Description: "High-performance vector database. The default for RAG.",
		Ports: []Port{
			{Container: 6333, Name: "http", Primary: true},
			{Container: 6334, Name: "grpc"},
		},
		Volumes: []Volume{{Suffix: "storage", Path: "/qdrant/storage"}},
	},
	"chroma": {
		Kind:        "chroma",
		DisplayName: "Chroma",
		Category:    CategoryVectorDB,
		Image:       "chromadb/chroma:latest",
		Description: "Lightweight, developer-friendly embedding database.",
		Ports:       []Port{{Container: 8000, Name: "http", Primary: true}},
		Volumes:     []Volume{{Suffix: "data", Path: "/data"}},
		Env: map[string]string{
			"IS_PERSISTENT":        "TRUE",
			"PERSIST_DIRECTORY":    "/data",
			"ANONYMIZED_TELEMETRY": "FALSE",
		},
	},
	"pgvector": {
		Kind:        "pgvector",
		DisplayName: "Postgres + pgvector",
		Category:    CategoryVectorDB,
		Image:       "pgvector/pgvector:pg16",
		Description: "Postgres with the pgvector extension — SQL plus vectors.",
		Ports:       []Port{{Container: 5432, Name: "postgres", Primary: true}},
		Volumes:     []Volume{{Suffix: "data", Path: "/var/lib/postgresql/data"}},
		Env: map[string]string{
			"POSTGRES_USER":     "llmaker",
			"POSTGRES_PASSWORD": "llmaker",
			"POSTGRES_DB":       "llmaker",
		},
	},
	"weaviate": {
		Kind:        "weaviate",
		DisplayName: "Weaviate",
		Category:    CategoryVectorDB,
		Image:       "cr.weaviate.io/semitechnologies/weaviate:1.27.0",
		Description: "Vector database with hybrid search and built-in modules.",
		Ports: []Port{
			{Container: 8080, Name: "http", Primary: true},
			{Container: 50051, Name: "grpc"},
		},
		Volumes: []Volume{{Suffix: "data", Path: "/var/lib/weaviate"}},
		Env: map[string]string{
			"PERSISTENCE_DATA_PATH":                   "/var/lib/weaviate",
			"QUERY_DEFAULTS_LIMIT":                    "25",
			"AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED": "true",
			"DEFAULT_VECTORIZER_MODULE":               "none",
			"CLUSTER_HOSTNAME":                        "node1",
		},
	},
	"redis": {
		Kind:        "redis",
		DisplayName: "Redis",
		Category:    CategoryCache,
		Image:       "redis:7-alpine",
		Description: "In-memory store — powers the agent's per-session conversation memory.",
		Ports:       []Port{{Container: 6379, Name: "redis", Primary: true}},
		Volumes:     []Volume{{Suffix: "data", Path: "/data"}},
		Notes:       "Set REDIS_URL=redis://redis:6379 on the agent to persist per-session chat history (send a session_id). The `chatbot` stack wires this automatically.",
	},
	"embeddings": {
		Kind:        "embeddings",
		DisplayName: "Text Embeddings Inference",
		Category:    CategoryEmbeddings,
		Image:       "ghcr.io/huggingface/text-embeddings-inference:cpu-latest",
		Description: "HuggingFace TEI — a fast, dedicated embeddings endpoint.",
		Ports:       []Port{{Container: 80, Name: "http", Primary: true}},
		Volumes:     []Volume{{Suffix: "data", Path: "/data"}},
		Env:         map[string]string{"MODEL_ID": "BAAI/bge-small-en-v1.5"},
		Notes:       "Set MODEL_ID with --env to pick the embedding model.",
	},
	"searxng": {
		Kind:        "searxng",
		DisplayName: "SearXNG",
		Category:    CategorySearch,
		Image:       "searxng/searxng:latest",
		Description: "Self-hosted metasearch — powers the agent's web_search tool, no paid API.",
		Ports:       []Port{{Container: 8080, Name: "http", Primary: true}},
		Volumes:     []Volume{{Suffix: "config", Path: "/etc/searxng"}},
		Env: map[string]string{
			"SEARXNG_BASE_URL": "http://searxng:8080/",
			// Dev default — rotate before exposing beyond localhost.
			"SEARXNG_SECRET": "llmaker-dev-change-me",
		},
		Notes: "Powers the agent's web_search tool: set SEARCH_URL=http://searxng:8080 on the " +
			"agent (the `research` stack does this). Enable JSON results once — in the mounted " +
			"/etc/searxng/settings.yml set `search.formats: [html, json]`, then restart searxng.",
	},
	"whisper": {
		Kind:        "whisper",
		DisplayName: "Whisper (faster-whisper)",
		Category:    CategorySpeech,
		Image:       "fedirz/faster-whisper-server:latest-cpu",
		Description: "Speech-to-text — self-hosted, OpenAI-compatible /v1/audio/transcriptions.",
		Ports:       []Port{{Container: 8000, Name: "http", Primary: true}},
		Volumes:     []Volume{{Suffix: "cache", Path: "/root/.cache/huggingface"}},
		Notes: "Point WHISPER_URL=http://whisper:8000 at the agent to enable /api/transcribe, or " +
			"call /v1/audio/transcriptions directly. For NVIDIA GPUs use the `latest-cuda` tag with --gpu.",
	},
	"langfuse": {
		Kind:        "langfuse",
		DisplayName: "Langfuse",
		Category:    CategoryObservability,
		Image:       "langfuse/langfuse:2",
		Description: "Tracing, prompt analytics, and evals for your LLM stack.",
		Ports:       []Port{{Container: 3000, Name: "http", Primary: true}},
		Env: map[string]string{
			"DATABASE_URL":      "postgresql://llmaker:llmaker@pgvector:5432/llmaker",
			"NEXTAUTH_URL":      "http://localhost:3000",
			"NEXTAUTH_SECRET":   "llmaker-dev-secret-change-me",
			"SALT":              "llmaker-dev-salt-change-me",
			"TELEMETRY_ENABLED": "false",
			// Headless bootstrap: create a project + fixed dev API keys on first
			// boot so the RAG agent can trace with no manual setup. Rotate these
			// before exposing Langfuse beyond localhost.
			"LANGFUSE_INIT_ORG_ID":             "llmaker",
			"LANGFUSE_INIT_ORG_NAME":           "llmaker",
			"LANGFUSE_INIT_PROJECT_ID":         "llmaker",
			"LANGFUSE_INIT_PROJECT_NAME":       "llmaker",
			"LANGFUSE_INIT_PROJECT_PUBLIC_KEY": "pk-lf-llmaker",
			"LANGFUSE_INIT_PROJECT_SECRET_KEY": "sk-lf-llmaker",
			"LANGFUSE_INIT_USER_EMAIL":         "admin@llmaker.local",
			"LANGFUSE_INIT_USER_NAME":          "llmaker",
			"LANGFUSE_INIT_USER_PASSWORD":      "llmaker-dev",
		},
		Notes: "Needs a Postgres: run `llmaker service add pgvector` first (the default DATABASE_URL points at it over the llmaker network). Boots with fixed dev API keys pk-lf-llmaker / sk-lf-llmaker; sign in as admin@llmaker.local / llmaker-dev.",
	},
	"open-webui": {
		Kind:        "open-webui",
		DisplayName: "Open WebUI",
		Category:    CategoryUI,
		Image:       "ghcr.io/open-webui/open-webui:main",
		Description: "ChatGPT-style web UI for your models — chats, prompts, RAG, multi-user.",
		Ports:       []Port{{Container: 8080, Name: "http", Primary: true}},
		Volumes:     []Volume{{Suffix: "data", Path: "/app/backend/data"}},
		Env: map[string]string{
			"OPENAI_API_BASE_URL": "http://chat:8080/v1",
			"OPENAI_API_KEY":      "not-needed",
			// Frictionless on trusted localhost; turn on before exposing it.
			"WEBUI_AUTH": "False",
		},
		Notes: "Talks to the in-network model named \"chat\" (http://chat:8080/v1) — the " +
			"`assistant` stack wires this for you (or point OPENAI_API_BASE_URL at any " +
			"instance's facade). WEBUI_AUTH=False suits trusted localhost; set it True with a " +
			"WEBUI_SECRET_KEY before exposing the UI beyond your machine.",
	},
	"agent": {
		Kind:        "agent",
		DisplayName: "RAG agent (LangGraph)",
		Category:    CategoryAgent,
		Image:       imageRepo + "/llmaker-agent:latest",
		Description: "LangGraph RAG: ingest docs, answer grounded questions. Ties the stack together.",
		Ports:       []Port{{Container: 8800, Name: "http", Primary: true}},
		Env: map[string]string{
			"LLM_BASE_URL":   "http://chat:8080/v1",
			"LLM_MODEL":      "llama3:8b",
			"EMBEDDINGS_URL": "http://embeddings:80",
			"QDRANT_URL":     "http://qdrant:6333",
			"COLLECTION":     "llmaker",
		},
		Notes: "Wires to your LLM + qdrant + embeddings by in-network name. The easy path is `llmaker stack init rag`. Build the image with `make image-agent` (or --image a local tag).",
	},
}

// imageRepo is the GHCR namespace for llmaker's own images (the agent). Catalog
// entries for third-party services use their upstream images directly.
const imageRepo = "ghcr.io/raiyanyahya"

// Get resolves a service by name, accepting a few friendly aliases.
func Get(name string) (Service, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	switch key {
	case "postgres", "postgresql", "pg":
		key = "pgvector"
	case "valkey":
		key = "redis"
	case "tei", "embed", "embedding":
		key = "embeddings"
	case "searx", "search", "websearch":
		key = "searxng"
	case "stt", "asr", "speech-to-text", "transcribe":
		key = "whisper"
	case "openwebui", "open-webui", "webui", "ui", "chat-ui", "chatui":
		key = "open-webui"
	case "qdrant", "chroma", "weaviate", "redis", "embeddings", "pgvector", "langfuse", "searxng", "whisper":
		// canonical
	}
	s, ok := registry[key]
	if !ok {
		return Service{}, fmt.Errorf("unknown service %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	return s, nil
}

// All returns every catalog entry, sorted by category then name for stable
// help output.
func All() []Service {
	out := make([]Service, 0, len(registry))
	for _, s := range registry {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// Tier is the startup ordering for a category: lower tiers come up first. This
// is a lightweight dependency model — data stores before the apps that use
// them, and the agent (which talks to everything) last — so `apply` can bring a
// stack up in an order that works (e.g. pgvector before Langfuse).
func Tier(c Category) int {
	switch c {
	case CategoryVectorDB, CategoryCache:
		return 0
	case CategoryEmbeddings, CategorySearch, CategorySpeech, CategoryObservability:
		return 1
	case CategoryUI, CategoryAgent:
		return 2
	default:
		return 1
	}
}

// TierOf returns the startup tier for a category name (as stored on a spec).
func TierOf(category string) int {
	return Tier(Category(category))
}

// Names returns the sorted list of service identifiers.
func Names() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
