<div align="center">

# llmaker

### Self-host the modern LLM stack.

**llmaker is an open-source platform for running the complete modern LLM stack on
your own infrastructure** — large language models, vector databases, embeddings,
caching, observability, and a built-in retrieval & agent layer — provisioned,
networked, and production-shaped from a single command.

Build private retrieval-augmented chatbots, FAQ assistants, and recommendation
engines locally. No third-party API keys. No data leaving your machine.

<br/>

[![CI](https://github.com/raiyanyahya/llmaker/actions/workflows/ci.yml/badge.svg)](https://github.com/raiyanyahya/llmaker/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/raiyanyahya/llmaker)](https://goreportcard.com/report/github.com/raiyanyahya/llmaker)
[![Self-hosted](https://img.shields.io/badge/100%25-self--hosted-success)](#why-self-host-your-llm-stack)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/status-alpha-orange.svg)](#roadmap)

[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Python](https://img.shields.io/badge/Python-3.10%2B-3776AB?logo=python&logoColor=white)](agent/)
[![Docker](https://img.shields.io/badge/Docker-required-2496ED?logo=docker&logoColor=white)](https://docs.docker.com/get-docker/)
[![FastAPI](https://img.shields.io/badge/FastAPI-009688?logo=fastapi&logoColor=white)](facade/)
[![LangGraph](https://img.shields.io/badge/agent-LangGraph-1C3C3C)](https://langchain-ai.github.io/langgraph/)
[![Ollama](https://img.shields.io/badge/backend-Ollama-000000?logo=ollama&logoColor=white)](https://ollama.com)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS-lightgrey?logo=linux&logoColor=white)](#hardware--images)

[![Stars](https://img.shields.io/github/stars/raiyanyahya/llmaker?logo=github&color=yellow)](https://github.com/raiyanyahya/llmaker/stargazers)
[![Last commit](https://img.shields.io/github/last-commit/raiyanyahya/llmaker?logo=git&logoColor=white)](https://github.com/raiyanyahya/llmaker/commits/master)
[![Issues](https://img.shields.io/github/issues/raiyanyahya/llmaker?logo=github)](https://github.com/raiyanyahya/llmaker/issues)
[![Code size](https://img.shields.io/github/languages/code-size/raiyanyahya/llmaker?logo=github)](https://github.com/raiyanyahya/llmaker)
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](#contributing)

[Quickstart](#quickstart) · [Why llmaker](#why-self-host-your-llm-stack) · [Stacks](#stacks) · [The agent](#the-agent) · [Architecture](#architecture) · [CLI](#cli-reference) · [Roadmap](#roadmap)

</div>

---

## Overview

Running a model locally is easy. Shipping an *application* is not. A production
retrieval system needs a vector database, an embeddings service, a caching layer,
an orchestration layer, and observability — each containerized, networked, and
configured to discover the others. Assembling that is a recurring tax: a sprawl of
`docker run` flags, a brittle Compose file, and hundreds of lines of framework glue.

llmaker removes that tax. It treats the modern LLM stack as a first-class unit —
choose an application, provision it, and use it:

```bash
# 1 · Choose what to build. Each is a complete, self-hosted stack:
llmaker stack init rag        #  Document Q&A (RAG) — grounded answers with sources
#                  chatbot    #  A multi-turn conversational assistant
#                  faq        #  A knowledge-base / support bot
#                  recommend  #  A semantic recommendation engine (no LLM required)

# 2 · Provision it. One command brings up the whole stack — model, vector DB,
#     embeddings, the retrieval agent, and tracing — networked and ready:
llmaker apply                 #  reads stack.yaml; --prune reconciles to it

# 3 · Use it. Ingest your data, then query the running stack over HTTP:
AGENT=$(llmaker service ls --json | jq -r '.[] | select(.service=="agent").url')

curl "$AGENT/api/ingest" -F file=@employee-handbook.pdf
curl "$AGENT/api/chat"   -d '{"question": "What is our refund policy?"}'
# → {"answer":  "Refunds are issued within 30 days of purchase…",
#    "sources": [{"source": "employee-handbook.pdf", "score": 0.88}]}
```

That single `apply` provisions containers that already know how to find each
other — an LLM, a vector database, an embeddings server, a
[LangGraph](https://langchain-ai.github.io/langgraph/) retrieval agent, a Postgres,
and [Langfuse](https://langfuse.com) for tracing — on a private network, with no
manual configuration. Everything runs on your hardware.

> Prefer to compose it yourself, or just run a single model? llmaker does that
> too — see the [quickstart](#quickstart). Every piece is also available
> à la carte via `llmaker service add` and `llmaker up`.

---

## Highlights

| | |
|---|---|
| **The complete stack, curated** | Models **and** the infrastructure around them — vector databases (Qdrant, Chroma, pgvector, Weaviate), Redis, embeddings, Langfuse — from one versioned catalog. |
| **Automatic service discovery** | Every model and service joins a private Docker network and resolves by name. Your application reaches `chat:8080` and `qdrant:6333` with zero IP wiring. |
| **A retrieval agent, built in** | A FastAPI + LangGraph service implementing `rewrite → retrieve → rerank → generate` — multi-turn, MMR reranking — plus a semantic recommendation API. |
| **Observability by default** | The RAG stack ships Langfuse; every query is traced (retrieval hits and scores, generation model and token usage) with no setup. |
| **Declarative, reconcilable** | Define your stack in one file. `llmaker apply` brings it to the desired state in dependency order; `--prune` removes what's no longer declared. |
| **OpenAI-compatible** | Each model exposes a stable `/v1/*` API (chat, completions, embeddings, streaming). Backend-agnostic — swap Ollama for llama.cpp with a flag, not a rewrite. |
| **Private by design** | Containers bind to `127.0.0.1` by default. Your documents, embeddings, and traces never leave your infrastructure. No per-token cost, no vendor lock-in. |
| **Operable** | A single static Go binary, a labeled-container model with no state file to drift, `--json` output everywhere, and a live `top` dashboard. |

---

## Why self-host your LLM stack?

- **Data ownership.** Proprietary documents, customer data, and prompts stay on
  hardware you control. Nothing is sent to a third-party API.
- **No assembly tax.** The vector DB, embeddings, cache, agent, and tracing come
  pre-integrated and networked — not as a Compose file you maintain by hand.
- **Predictable cost.** Inference and retrieval run on infrastructure you already
  pay for. No per-token billing, no rate limits.
- **Portability.** The same `stack.yaml` runs on a laptop, a CI runner, or a
  server. Swap the model or the vector database without touching your application.

| | Model runners<br/>(Ollama, LM&nbsp;Studio) | DIY<br/>Docker&nbsp;Compose | Frameworks<br/>(LangChain) | **llmaker** |
|---|:---:|:---:|:---:|:---:|
| Run local models, OpenAI-compatible | ✓ | — | — | ✓ |
| Vector DB, embeddings, cache — curated | — | manual | — | ✓ |
| Service discovery between containers | — | manual | n/a | ✓ |
| One-command application (RAG, recsys) | — | — | — | ✓ |
| Built-in retrieval & recommendation agent | — | — | you code it | ✓ |
| Observability / tracing integrated | — | manual | manual | ✓ |
| Declarative provisioning & reconciliation | — | partial | — | ✓ |

---

## Installation

> Requires [Docker](https://docs.docker.com/get-docker/). Run `llmaker doctor` afterward to validate your environment.

```bash
# Prebuilt binary (Linux / macOS)
curl -fsSL https://raw.githubusercontent.com/raiyanyahya/llmaker/master/scripts/install.sh | sh

# Go toolchain
go install github.com/raiyanyahya/llmaker/cmd/llmaker@latest

# From source
git clone https://github.com/raiyanyahya/llmaker && cd llmaker && make build
```

<sub>Homebrew and `winget` packages are on the [roadmap](#roadmap). The agent image is built locally with `make image-agent` until it is published to a registry.</sub>

---

## Quickstart

Provision and run a complete retrieval-augmented generation stack:

```bash
llmaker stack init rag        # generate stack.yaml (rag | chatbot | faq | recommend)
make image-agent              # build the agent image once
llmaker apply -f stack.yaml   # provision the stack — model + services, networked
llmaker ls                    # inspect models and services in one view
```

Resolve the agent endpoint and use it:

```bash
AGENT=$(llmaker service ls --json | jq -r '.[] | select(.service=="agent").url')

curl "$AGENT/api/ingest" -F file=@handbook.pdf                     # ingest documents
curl "$AGENT/api/chat"   -d '{"question":"…","history":[],"top_k":4}'   # query, with sources
```

llmaker also runs individual models — the easiest way to expose a local,
OpenAI-compatible endpoint:

```bash
llmaker up --model llama3:8b          # provision a model instance
```
```python
from openai import OpenAI
client = OpenAI(base_url="http://127.0.0.1:11500/v1", api_key="not-needed")
client.chat.completions.create(model="llama3:8b",
    messages=[{"role": "user", "content": "Hello"}])
```

---

## Stacks

A stack is a model plus the services around it, provisioned together. Each
template is generated with `llmaker stack init <name>` and applied with
`llmaker apply`. All four are tested and verified end-to-end against live Docker.

| Template | Application | Components |
|---|---|---|
| `rag` | Document Q&A — ingest files, query with grounded answers and sources, fully traced | LLM · Qdrant · embeddings · agent · Langfuse · Postgres |
| `chatbot` | A multi-turn assistant with a web UI, extendable into RAG | LLM · agent |
| `faq` | A knowledge-base assistant tuned for short, grounded answers | LLM · Qdrant · embeddings · agent |
| `recommend` | A semantic recommendation engine — "more like this", no LLM required | Qdrant · embeddings · agent |

---

## The agent

The catalog's `agent` is a FastAPI + LangGraph service (`agent/`) that turns a
bare model and vector store into an application. It is a standard service on the
network, configured by environment to discover the others by name.

**Retrieval as an explicit graph** — `rewrite → retrieve → rerank → generate`:

- **rewrite** — collapses multi-turn history into a standalone query, so
  follow-ups that depend on context ("and when was *it* released?") resolve
  correctly. The model is only invoked when there is history to resolve.
- **retrieve** — embeds the query and retrieves a candidate set from the vector
  store.
- **rerank** — applies [Maximal Marginal Relevance](https://en.wikipedia.org/wiki/Maximal_marginal_relevance)
  for relevant, non-redundant context.
- **generate** — produces the answer from that context and the conversation.

```
POST /api/ingest      multipart file or text  →  chunk, embed, store
POST /api/chat        { question, history?, top_k? }  →  answer + sources
POST /api/items       { items: [{ id, text, metadata? }] }  →  index for recommendation
POST /api/recommend   { query }  or  { like: [id, …] }  →  ranked items
```

**Tracing.** The `rag` stack provisions Langfuse and the agent traces every query
to it, with zero configuration — each request appears as a `rag-chat` trace with
its retrieval and generation steps. Tracing is enabled by the template and is
otherwise opt-in via two environment variables.

**Recommendations** reuse the same embeddings and vector store, with no model
involved: index items once, then retrieve by free-text intent (`query`) or by
example (`like`, which averages the seed items into a profile and excludes them
from the results).

Full agent contract and configuration: [`agent/README.md`](agent/README.md).

---

## Services & networking

Compose a stack from the catalog directly, or let a template do it:

```bash
llmaker service catalog          # list available services
llmaker service add qdrant       # vector database     → qdrant:6333
llmaker service add redis        # cache / memory      → redis:6379
llmaker service add embeddings   # embeddings (HF TEI) → embeddings:80
llmaker service add langfuse     # observability       → langfuse:3000
```

| Category | Services |
|---|---|
| Vector databases | Qdrant · Chroma · pgvector (Postgres) · Weaviate |
| Cache / memory | Redis |
| Embeddings | HuggingFace Text-Embeddings-Inference |
| Observability | Langfuse |
| Agent | LangGraph retrieval & recommendation agent |

Every model and service joins a private Docker network (`llmaker-net`) and is
addressable there by name — service discovery without IPs, links, or a Compose
file. Applications running on the host or in their own container reach the stack
the same way:

```bash
docker run --rm --network llmaker-net redis:7-alpine redis-cli -h redis ping   # → PONG
```

Adding a service is a single entry in `internal/service/catalog.go`; the CLI,
fleet view, and declarative engine pick it up automatically.

---

## Declarative configuration

`stack init` generates one of these; it can also be authored by hand. `apply`
reconciles the running stack to the file — provisioning services before the
applications that depend on them — and `--prune` removes anything not declared.

```yaml
# stack.yaml  →  llmaker apply -f stack.yaml [--prune]
defaults: { backend: ollama }
instances:
  - { name: chat, model: llama3:8b, memory: 8g }   # → chat:8080
services:
  - use: qdrant                                    # → qdrant:6333
  - { name: cache, use: redis }                    # → cache:6379
  - { name: embeddings, use: embeddings, env: { MODEL_ID: BAAI/bge-small-en-v1.5 } }
  - use: agent                                     # → agent:8800
```

Unset ports are assigned automatically; a stack may be services-only. See
[`examples/stack.yaml`](examples/stack.yaml) and [`examples/llm.yaml`](examples/llm.yaml).

---

<a id="architecture"></a>

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│  llmaker CLI   (Go — single static binary)                            │
│  orchestration · Docker SDK · private networking · declarative apply  │
└───────────────────────────────┬──────────────────────────────────────┘
                                │  provision · start · stop · HTTP
                                ▼
   ════════════════ llmaker-net  (private network, DNS by name) ════════════════
    ┌── Model instance ───────────┐   ┌── Services ───────────────────────────┐
    │ engine ⇄ facade (FastAPI)   │   │ qdrant · embeddings · redis · pgvector │
    │ Ollama | llama.cpp          │   │ langfuse · …                           │
    │ OpenAI /v1/* · web UI       │   │ qdrant:6333   embeddings:80            │
    │ chat:8080                   │   └────────────────────────────────────────┘
    └─────────────────────────────┘                  ▲
                    ▲                                 │
                    └──────────────┬──────────────────┘
                    ┌── Agent (FastAPI + LangGraph) ───┐
                    │ rewrite → retrieve → rerank →     │   agent:8800
                    │ generate · ingest · recommend     │
                    └───────────────────────────────────┘
              host ports (127.0.0.1:PORT) mapped per container
```

The control plane is a single Go binary; the data plane is containers on a private
network. Orchestration logic is decoupled from Docker behind a `Runtime`
interface, and the fleet is tracked entirely through container labels — there is
no local state file to drift out of sync. Model facades and the agent are Python
(FastAPI), each communicating over the same HTTP contract.

---

## CLI reference

| Command | Description |
|---|---|
| `llmaker stack init <rag\|chatbot\|faq\|recommend>` | Generate a ready-to-apply stack definition |
| `llmaker apply -f stack.yaml` | Provision / reconcile a declarative stack — `--prune` |
| `llmaker up [preset]` | Provision a model instance — preset, flags, or interactive wizard |
| `llmaker service catalog` | List available services |
| `llmaker service add <type> [name]` | Provision a service — `--env`, `--port`, `--memory` |
| `llmaker service ls \| rm \| stop \| start` | Manage services — `--json` |
| `llmaker ls` | List the fleet — models and services — `--json`, `--quiet` |
| `llmaker top` | Live resource dashboard across the fleet |
| `llmaker status <name>` | Detailed instance status — `--json` |
| `llmaker pull <model> --on <name>` | Download a model with progress — `--default` |
| `llmaker chat [name]` | Interactive or one-shot chat — `--message`, stdin |
| `llmaker open <name>` | Open a container's web UI — `--print` |
| `llmaker logs <name> -f` | Stream logs from any container |
| `llmaker stop \| start \| rm <name>…` | Lifecycle management — `rm --force` |
| `llmaker doctor` | Validate the environment (Docker, GPU, platform caveats) |

---

## Configuration

| Setting | Where | Default |
|---|---|---|
| backend / model | `--backend` · `--model` · `stack.yaml` | `ollama` · backend default |
| memory · cpus · gpu | flags · `stack.yaml` | host-derived |
| port · host | `--port` · `--host` | auto · `127.0.0.1` |
| service environment | `service add --env` · `env:` in `stack.yaml` | per-service defaults |
| `API_KEY` · `CORS_ORIGINS` · `KEEP_ALIVE` | `--api-key` · `--cors` · `--keep-alive` | open · `*` · `5m` |

Per-service and agent configuration (model URLs, chunking, reranking, tracing
keys) is documented in [`agent/README.md`](agent/README.md) and
[`facade/README.md`](facade/README.md).

---

## Security

Every container binds to `127.0.0.1` by default; nothing is exposed until you opt
in, and exposure pairs with authentication:

```bash
llmaker up --host 0.0.0.0 --api-key "$(openssl rand -hex 16)"
```

When `API_KEY` is set, every `/v1/*` and `/api/*` request requires a bearer token
(liveness probes excepted). The agent enforces its own `API_KEY` identically. The
Langfuse keys and database password in the catalog are **development defaults** —
rotate them before exposing a stack beyond localhost.

---

## Hardware & images

Docker on macOS cannot pass through the Apple GPU; a containerized engine runs
CPU-only. `llmaker doctor` detects and reports this. On Linux with NVIDIA, `--gpu`
reserves GPUs via the NVIDIA Container Toolkit.

| Image | Size | Use |
|---|---|---|
| `llmaker-ollama:latest` | ~8.5 GB | GPU-capable (Linux + NVIDIA) |
| `llmaker-ollama:cpu` | ~360 MB | CPU-only — laptops, CI, macOS |
| `llmaker-agent:latest` | ~480 MB | LangGraph retrieval & recommendation agent |

Images are resolved with a pull-if-missing policy, so locally built images
(`make image-agent`) are used directly without contacting a registry.

---

## Development

```bash
make build        # build ./bin/llmaker
make check        # gofmt + vet + go test (CI parity)

make facade-setup && make facade-test     # model facade (pytest)
make agent-setup  && make agent-test      # retrieval/recommendation agent (pytest)

make images       # build backend + agent images
```

The Go control plane is tested against an in-memory runtime (no Docker required).
The model facade and the agent — routes, the LangGraph pipeline, reranking,
tracing, and recommendation — are tested against in-memory fakes. CI runs Go race
tests, `gofmt`, a ruff-linted Python test matrix, and image builds on every push.

```
cmd/llmaker/            CLI entrypoint
internal/
  backend/              inference engines and image references
  service/              the service catalog
  engine/               domain model, ports, labels, Runtime interface
    dockerrt/           Docker implementation and the private network
    enginetest/         in-memory Runtime for tests
  config/               stack.yaml parsing and dependency ordering
  cli/ · ui/ · tui/     Cobra commands and the terminal interface
facade/                 model facade (FastAPI) + per-model web UI
agent/                  retrieval & recommendation agent (FastAPI + LangGraph)
images/                 backend and agent Dockerfiles
```

---

<a id="roadmap"></a>

## Roadmap

> **Status: alpha.** Every capability below is implemented, tested, and verified
> end-to-end against live Docker.

- [x] Model instances — OpenAI-compatible facade, per-model UI, fleet management
- [x] Service catalog — vector databases, cache, embeddings, observability
- [x] Private networking — automatic service discovery by name
- [x] Declarative stacks — `stack init` templates and reconciling `apply --prune`
- [x] Retrieval agent — LangGraph `rewrite → retrieve → rerank → generate`, multi-turn
- [x] Recommendation engine — semantic `query` and "more like this"
- [x] Integrated observability — Langfuse tracing
- [ ] Agent tooling — function calling, dedicated reranking, evaluation
- [ ] Additional backends — llama.cpp model management; Metal on macOS
- [ ] Distribution — multi-architecture images, package managers, releases

---

## Contributing

Contributions are welcome. Keep the suite green (`make check`, `make facade-test`,
`make agent-test`), match the surrounding style, and include tests. Adding a
service is a single catalog entry; adding a model backend is a single facade
adapter.

## License

[Apache 2.0](LICENSE) © Raiyan Yahya.
