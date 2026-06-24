# llmaker — build, test, and image tooling.

BINARY      := llmaker
PKG         := github.com/raiyanyahya/llmaker
BIN_DIR     := bin
REGISTRY    ?= ghcr.io/raiyanyahya

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: all
all: tidy fmt vet test build ## Tidy, format, vet, test and build

## --- Go ---

.PHONY: build
build: ## Build the llmaker binary into ./bin
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) ./cmd/llmaker
	@echo "built $(BIN_DIR)/$(BINARY) ($(VERSION))"

.PHONY: install
install: ## go install the binary into GOBIN
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/llmaker

.PHONY: test
test: ## Run Go tests
	go test ./...

.PHONY: cover
cover: ## Run Go tests with a coverage summary
	go test -cover ./...

.PHONY: race
race: ## Run Go tests with the race detector
	go test -race ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format Go code
	gofmt -w $(GOFILES)

.PHONY: fmt-check
fmt-check: ## Fail if Go code isn't gofmt-clean
	@out=$$(gofmt -l $(GOFILES)); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

.PHONY: tidy
tidy: ## Tidy go.mod
	go mod tidy

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)

## --- Facade (Python) ---

.PHONY: facade-setup
facade-setup: ## Create a venv and install the facade with dev deps
	cd facade && python3 -m venv .venv && . .venv/bin/activate && pip install -e ".[dev]"

.PHONY: facade-test
facade-test: ## Run the facade test suite
	cd facade && . .venv/bin/activate && python -m pytest -q

.PHONY: facade-run
facade-run: ## Run the facade locally (needs a backend on localhost)
	cd facade && . .venv/bin/activate && python -m app

## --- Agent (Python / LangGraph) ---

.PHONY: agent-setup
agent-setup: ## Create a venv and install the RAG agent with dev deps
	cd agent && python3 -m venv .venv && . .venv/bin/activate && pip install -e ".[dev]"

.PHONY: agent-test
agent-test: ## Run the agent test suite
	cd agent && . .venv/bin/activate && python -m pytest -q

## --- Images ---

.PHONY: image-ollama
image-ollama: ## Build the Ollama backend image (GPU-capable)
	docker build -f images/ollama/Dockerfile -t $(REGISTRY)/llmaker-ollama:latest .

.PHONY: image-ollama-cpu
image-ollama-cpu: ## Build the slim CPU-only Ollama image
	docker build -f images/ollama/Dockerfile.cpu -t $(REGISTRY)/llmaker-ollama:cpu .

.PHONY: image-llamacpp
image-llamacpp: ## Build the llama.cpp backend image
	docker build -f images/llamacpp/Dockerfile -t $(REGISTRY)/llmaker-llamacpp:latest .

.PHONY: image-agent
image-agent: ## Build the LangGraph RAG agent image
	docker build -f images/agent/Dockerfile -t $(REGISTRY)/llmaker-agent:latest .

.PHONY: images
images: image-ollama image-llamacpp image-agent ## Build all backend + agent images

## --- Meta ---

.PHONY: check
check: fmt-check vet test ## CI-style checks (no rebuild)

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
