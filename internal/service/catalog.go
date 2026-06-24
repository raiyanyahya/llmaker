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
	CategoryObservability Category = "observability"
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
		Description: "In-memory store for chat memory, sessions, and semantic cache.",
		Ports:       []Port{{Container: 6379, Name: "redis", Primary: true}},
		Volumes:     []Volume{{Suffix: "data", Path: "/data"}},
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
		},
		Notes: "Needs a Postgres: run `llmaker service add pgvector` first (the default DATABASE_URL points at it over the llmaker network).",
	},
}

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
	case "qdrant", "chroma", "weaviate", "redis", "embeddings", "pgvector", "langfuse":
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

// Names returns the sorted list of service identifiers.
func Names() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
