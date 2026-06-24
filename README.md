<div align="center">

# 🦙 llmaker

### Self-host your whole LLM stack — and ship working AI apps — from your terminal.

**Not just a model server. A model *plus* the vector DB, embeddings, cache,
observability, and a RAG/agent layer — wired together and running in one command.**

`llmaker stack init rag && llmaker apply` gives you a document-grounded chatbot,
end to end. RAG, FAQ bots, and recommendation engines are one command each. It
feels like `docker compose up` for local AI — not a weekend of glue code.

<br/>

[![CI](https://github.com/raiyanyahya/llmaker/actions/workflows/ci.yml/badge.svg)](https://github.com/raiyanyahya/llmaker/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/raiyanyahya/llmaker)](https://goreportcard.com/report/github.com/raiyanyahya/llmaker)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Python](https://img.shields.io/badge/services-Python%203.10%2B-3776AB?logo=python&logoColor=white)](agent/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

[![Built with Charm](https://img.shields.io/badge/built%20with-Charm_🤍-ff69b4)](https://charm.sh)
[![LangGraph](https://img.shields.io/badge/agent-LangGraph-1C3C3C)](https://langchain-ai.github.io/langgraph/)
[![Docker](https://img.shields.io/badge/Docker-required-2496ED?logo=docker&logoColor=white)](https://docs.docker.com/get-docker/)
[![Status](https://img.shields.io/badge/status-alpha-orange.svg)](#roadmap)

<br/>

[**Install**](#-install) · [**Build an app**](#-from-zero-to-a-rag-app-in-one-command) · [**Why?**](#-why-llmaker) · [**The agent**](#-the-built-in-agent) · [**Commands**](#-commands) · [**Architecture**](#architecture) · [**Roadmap**](#roadmap)

</div>

---

## 🚀 From zero to a RAG app in one command

```console
$ llmaker stack init rag && llmaker apply -f stack.yaml
✓ qdrant        ready  → qdrant:6333
✓ embeddings    ready  → embeddings:80
✓ pgvector      ready  → pgvector:5432
✓ langfuse      ready  → langfuse:3000
✓ chat (llama3) ready  → chat:8080
✓ agent         ready  → agent:8800
✓ Applied stack.yaml: 6 created

$ curl $AGENT/api/ingest -F file=@employee-handbook.pdf
{"ingested": 42, "source": "employee-handbook.pdf"}

$ curl $AGENT/api/chat -d '{"question":"what is our refund policy?"}'
{"answer":"Refunds are issued within 30 days of purchase…",
 "sources":[{"source":"employee-handbook.pdf","score":0.88}]}
```

One command brought up **six containers that already know how to find each other**:
an LLM, a vector database, an embeddings server, a [LangGraph](https://langchain-ai.github.io/langgraph/)
RAG agent, plus Langfuse so every answer is traced. No `docker run` flags, no IP
wrangling, no framework boilerplate. Everything runs **locally**, on your machine.

---

## 🤔 Why llmaker?

You rarely just want "a model." You want the **thing you build with it** — a
chatbot over your docs, an FAQ bot, a recommender. And the moment you start, the
model is the easy 10%. The other 90% is plumbing:

> spin up a vector DB · stand up an embeddings endpoint · get them on a network ·
> make your app discover them · write the chunk/embed/retrieve/generate loop ·
> add a cache · bolt on tracing so you can see what the LLM actually did …

That's a pile of `docker run` flags, a hand-rolled compose file, and a few
hundred lines of LangChain — before you've answered a single question.

**`llmaker` is the part where you skip all of that.** It ships the whole stack as
a curated, networked, one-command system — and a built-in agent that already
implements RAG and recommendations — so you go from nothing to a working,
observable AI app in a minute, and *then* customize.

The space is crowded, so here's the honest wedge — it's the **whole stack**, not a piece:

| | Model runners<br/>(Ollama · LM&nbsp;Studio) | DIY<br/>`docker compose` | Frameworks<br/>(LangChain) | 🦙 **llmaker** |
|---|:---:|:---:|:---:|:---:|
| Run local models, OpenAI-compatible | ✅ | — | — | ✅ |
| Vector DB · embeddings · cache, curated | ❌ | manual | ❌ | ✅ |
| Containers discover each other by name | ❌ | manual | n/a | ✅ |
| **One-command working app** (RAG · recsys) | ❌ | ❌ | ❌ | ✅ |
| Built-in agent (LangGraph RAG + recommender) | ❌ | ❌ | code it | ✅ |
| Observability / tracing baked in | ❌ | manual | manual | ✅ |
| Fleet view · live TUI · declarative `apply` | ❌ | ⚠️ | ❌ | ✅ |

> *Ollama gives you a model. llmaker gives you the app — model, memory, retrieval,
> and the agent that ties them together.*

---

## 🧰 What you can build

Each is one command (`llmaker stack init <name>` → `llmaker apply`):

| Stack | What you get | Pieces |
|---|---|---|
| 🔎 **`rag`** | Document Q&A — ingest PDFs/text, ask, get answers **with sources**, fully traced | LLM · Qdrant · embeddings · agent · Langfuse |
| 💬 **`chatbot`** | A minimal multi-turn assistant with a web UI, easy to extend into RAG | LLM · agent |
| ❓ **`faq`** | A knowledge-base bot tuned for short, grounded answers | LLM · Qdrant · embeddings · agent |
| 🎯 **`recommend`** | A semantic recommendation engine — "more like this", **no LLM needed** | Qdrant · embeddings · agent |

All four are real, tested, and verified end-to-end against live Docker.

---

## ✨ Features

|   |   |
|---|---|
| 🧱 **The whole stack, curated** | LLMs **and** the infra around them — vector DBs (Qdrant, Chroma, pgvector, Weaviate), Redis, an embeddings server, Langfuse — from one catalog. `llmaker service add qdrant`. |
| 🕸️ **They find each other** | Every model and service joins one Docker network, reachable by name. Your app talks to `chat:8080` **and** `qdrant:6333` — zero IP wrangling. |
| 🤖 **A real agent, built in** | A FastAPI + LangGraph app: `rewrite → retrieve → rerank → generate`, multi-turn, MMR reranking, plus a `/api/recommend` recommender. Ingest, chat, recommend — over HTTP. |
| 📊 **Observable by default** | The `rag` stack wires in Langfuse; every query is traced (retrieve hits/scores → generate model/tokens) with **zero setup**. |
| 📜 **Declarative stacks** | One `stack.yaml` for LLMs **and** services; `llmaker apply` reconciles it (services first), `--prune` reaps the rest. |
| 🔌 **Backend-agnostic, OpenAI-compatible** | Each model serves a stable `/v1/*` API (chat, completions, embeddings, SSE). Ollama today, llama.cpp next — a `--backend` flag, your code unchanged. |
| 🏷️ **Fleet = state** | No state file. Models and services are labeled containers, so `llmaker ls` / `top` are always reality. |
| 🎨 **Beautiful terminal UX** | Cobra + [Charm](https://charm.sh): a wizard, live progress, a `top` dashboard. `--json` everywhere for scripts. |
| 🖥️ **Honest about hardware** | Auto-detects GPUs; warns about the Docker-on-macOS no-Metal reality before you hit mystery latency. A 360 MB CPU image for laptops/CI. |

---

## 📦 Install

> Requires **[Docker](https://docs.docker.com/get-docker/)**. Run `llmaker doctor` afterward to check your environment.

```bash
# 1) Prebuilt binary (Linux / macOS)
curl -fsSL https://raw.githubusercontent.com/raiyanyahya/llmaker/master/scripts/install.sh | sh

# 2) With the Go toolchain
go install github.com/raiyanyahya/llmaker/cmd/llmaker@latest

# 3) From source
git clone https://github.com/raiyanyahya/llmaker && cd llmaker && make build
# → ./bin/llmaker
```

<sub>Homebrew tap & `winget` are on the [roadmap](#roadmap). The RAG agent image
is built locally with `make image-agent` until it lands on GHCR.</sub>

---

## 🏁 Quickstart

**Build an app (the main event):**

```bash
llmaker stack init rag        # scaffold stack.yaml (rag | chatbot | faq | recommend)
make image-agent              # build the agent image once (until it's on GHCR)
llmaker apply -f stack.yaml   # bring the whole stack up, wired together
llmaker ls                    # see your models AND services in one view
```

**Or just run a model** — llmaker is still the easiest way to do that:

```bash
llmaker up chat               # a preset: obvious model, sane defaults, zero flags
llmaker up --model llama3:8b  # …or explicit
llmaker chat chat             # sanity-check it in the terminal
llmaker open chat             # its own web UI in the browser
```

Either way you get a stable OpenAI-compatible endpoint — point any client at it:

```python
from openai import OpenAI
client = OpenAI(base_url="http://127.0.0.1:11500/v1", api_key="not-needed")
client.chat.completions.create(model="llama3:8b",
    messages=[{"role": "user", "content": "Hello"}])
```

---

## 🤖 The built-in agent

The catalog's `agent` is a small FastAPI + LangGraph app (`agent/`) that turns a
bare LLM + vector DB into an application. It's just another service on the
network, with its env pointed at the others by name, so it self-wires.

**RAG, as an explicit graph** — `rewrite → retrieve → rerank → generate`:

- **rewrite** folds multi-turn history into one standalone query, so follow-ups
  like *"and when was **it** released?"* resolve correctly (only calls the LLM
  when there's history, so single-shot stays fast).
- **retrieve** embeds the query and pulls a wide candidate set.
- **rerank** uses [MMR](https://en.wikipedia.org/wiki/Maximal_marginal_relevance)
  to keep relevant-but-diverse context (no near-duplicate chunks).
- **generate** answers with that context plus the conversation.

```bash
A=$(llmaker service ls --json | jq -r '.[]|select(.service=="agent").url')
curl "$A/api/ingest"  -F file=@docs.pdf                                   # or -F text='…'
curl "$A/api/chat"    -d '{"question":"…","history":[…],"top_k":4}'       # answer + sources
```

**Observability, free.** The `rag` stack also runs Langfuse; every query is traced:

```
rag-chat  "what is our refund policy?"
├─ retrieve   qdrant · 4 hits · scores [0.88, 0.74, …]   12ms
└─ generate   llama3 · 318 tokens                         2.1s
```

Langfuse boots with fixed dev keys (sign in `admin@llmaker.local` / `llmaker-dev`),
so it works with zero setup; tracing is opt-in via two env vars the template sets.

**Recommendations, no LLM.** The same embeddings + vector store power a recommender:

```bash
curl "$A/api/items"     -d '{"items":[{"id":"sku1","text":"waterproof hiking boots"}, …]}'
curl "$A/api/recommend" -d '{"query":"something for a trail run"}'   # by intent
curl "$A/api/recommend" -d '{"like":["sku1","sku2"]}'               # taste profile, seeds excluded
```

Full agent contract: [`agent/README.md`](agent/README.md).

---

## 🧩 Services & networking

The stack is assembled from a curated catalog — add pieces à la carte or let a
template do it:

```bash
llmaker service catalog          # everything available
llmaker service add qdrant       # vector DB         → qdrant:6333
llmaker service add redis        # cache / memory    → redis:6379
llmaker service add embeddings   # HF TEI embeddings → embeddings:80
llmaker service add langfuse     # observability     → langfuse:3000
```

| Category | Services |
|---|---|
| **Vector databases** | Qdrant · Chroma · pgvector (Postgres) · Weaviate |
| **Cache / memory** | Redis (chat memory, sessions, semantic cache) |
| **Embeddings** | HuggingFace Text-Embeddings-Inference |
| **Observability** | Langfuse (tracing, prompt analytics, evals) |
| **Agent** | LangGraph RAG + recommendation agent |

**The point is the wiring.** Every model and service joins one Docker network
(`llmaker-net`) and resolves there by name — no IPs, no `--link`, no compose file:

```bash
# proof: a throwaway container reaches a service by name
docker run --rm --network llmaker-net redis:7-alpine redis-cli -h redis ping   # → PONG
```

Adding a service is one entry in `internal/service/catalog.go` — the CLI, `ls`,
`top`, and `apply` pick it up for free.

---

## 📜 Declarative stacks

`stack init` writes one of these; you can also hand-write it. `apply` reconciles
the running stack to the file (services come up first, since your app dials them
at boot), and `--prune` removes anything not declared.

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

Ports left unset are auto-assigned; a stack can be services-only. See
[`examples/stack.yaml`](examples/stack.yaml) and [`examples/llm.yaml`](examples/llm.yaml).

---

## 🔌 Backends — one API, many engines

Each model container runs a Python/FastAPI **facade** that normalizes its engine
to the same OpenAI-compatible contract, so your app never cares what's underneath:

| Backend | Status | Why |
|---|:---:|---|
| **Ollama** | ✅ default | Easiest path; rich model library; simple pull/run |
| **llama.cpp** | 🧱 structural | Max control: GGUF, quantization, perf flags |
| vLLM · TGI · mlx | 🛣️ future | Throughput / Apple-native — same contract, new adapter |

Adding a backend is one adapter (`facade/app/adapters/`). Full facade contract:
[`facade/README.md`](facade/README.md).

---

<a id="architecture"></a>

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│  llmaker CLI   (Go, single static binary)                            │
│  Cobra + Charm TUI · Docker Go SDK · fleet & stacks via labels       │
└───────────────────────────────┬─────────────────────────────────────┘
                                │  create / start / stop · HTTP
                                ▼
        ╔═══════════════ llmaker-net (Docker network, DNS by name) ═══════════════╗
        ║                                                                          ║
        ║   ┌── LLM instance ────────────┐     ┌── Services ──────────────────┐   ║
        ║   │ backend engine ⇄ facade     │     │ qdrant   embeddings  redis   │   ║
        ║   │ Ollama|llamacpp  /v1/* · UI │     │ langfuse pgvector  …         │   ║
        ║   │ chat:8080                   │     │ qdrant:6333  embeddings:80   │   ║
        ║   └─────────────────────────────┘     └──────────────────────────────┘   ║
        ║                    ▲                              ▲                       ║
        ║                    └──────────┬───────────────────┘                      ║
        ║                    ┌── agent (FastAPI + LangGraph) ──┐                    ║
        ║                    │ rewrite→retrieve→rerank→generate │  agent:8800       ║
        ║                    │ /api/ingest · /chat · /recommend │                   ║
        ║                    └──────────────────────────────────┘                   ║
        ╚══════════════════════════════════════════════════════════════════════════╝
                       host ports (127.0.0.1:PORT) mapped per container
```

- **CLI in Go, services in Python** — decoupled by HTTP. The Go side owns
  orchestration (Docker SDK, the only place it's imported), networking, and the
  TUI; the Python side owns the model facade and the agent.
- **No local state file.** Every model and service is a labeled container on a
  shared network, so the fleet view is always reality and DNS does the wiring.

---

## 🧭 Commands

| Command | What it does |
|---|---|
| `llmaker stack init <rag\|chatbot\|faq\|recommend>` | scaffold a ready-to-apply whole-stack `stack.yaml` |
| `llmaker apply -f stack.yaml` | bring up / reconcile a declarative stack (LLMs **+** services) — `--prune` |
| `llmaker up [preset]` | create + start an LLM — preset (`chat`, `code`, `small`, `embed`, `vision`), flags, or a wizard |
| `llmaker service catalog` | list the services you can run |
| `llmaker service add <type> [name]` | create + start a service — `--env`, `--port`, `--memory` |
| `llmaker service ls` / `rm` / `stop` / `start` | manage running services — `--json` |
| `llmaker ls` | the whole fleet — models **and** services — `--json`, `--quiet` |
| `llmaker top` | live, animated dashboard across the fleet |
| `llmaker status <name>` | detailed status: gauges, loaded/installed models — `--json` |
| `llmaker pull <model> --on <name>` | download a model with a live progress bar — `--default` |
| `llmaker chat [name]` | interactive or one-shot chat — `--message`, or pipe `stdin` |
| `llmaker open <name>` | open a container's web UI — `--print` to just emit the URL |
| `llmaker logs <name> -f` | stream logs (instance **or** service) |
| `llmaker stop` / `start` / `rm <name>…` | lifecycle — `rm --force` |
| `llmaker doctor` | environment check (Docker, GPU, the macOS caveat) |

---

## ⚙️ Configuration

| Setting | Where | Default |
|---|---|---|
| backend / model | `--backend` / `--model` / `stack.yaml` | `ollama` / backend default |
| memory / cpus / gpu | flags / `stack.yaml` | host-derived |
| port / host | `--port` / `--host` | auto / `127.0.0.1` |
| service env (e.g. `MODEL_ID`, agent wiring) | `service add --env`, or `env:` in `stack.yaml` | per-catalog defaults |
| `API_KEY` / `CORS_ORIGINS` / `KEEP_ALIVE` | `--api-key` / `--cors` / `--keep-alive` | open / `*` / `5m` |

The agent and each service expose their own env (LLM/embeddings/Qdrant URLs,
chunking, MMR, Langfuse keys, …) — see [`agent/README.md`](agent/README.md) and
[`facade/README.md`](facade/README.md).

---

<a id="hardware"></a>

## 🖥️ Hardware & images

**Docker on macOS can't pass through the Apple GPU** — a containerized engine
runs CPU-only and slower. `llmaker doctor` detects this and warns you; `--native`
Metal mode is on the roadmap. On Linux with NVIDIA, `--gpu` reserves GPUs via the
NVIDIA Container Toolkit.

| Image | Size | Use |
|---|---|---|
| `llmaker-ollama:latest` | ~8.5 GB | GPU-capable — Linux + NVIDIA |
| `llmaker-ollama:cpu` | **~360 MB** | slim, CPU-only — laptops, CI, Docker-on-macOS |
| `llmaker-agent:latest` | ~480 MB | the LangGraph RAG / recommendation agent |

`llmaker` uses a *pull-if-missing* policy, so a locally-built image (e.g. `make
image-agent`) is used as-is without contacting a registry.

---

## 🔒 Security

Every container binds to **`127.0.0.1`** by default — nothing is exposed until
you opt in, and exposure pairs with auth:

```bash
llmaker up --host 0.0.0.0 --api-key "$(openssl rand -hex 16)"
```

When `API_KEY` is set, every `/v1/*` and `/api/*` call requires a bearer token
(health probes stay open). The agent honors its own `API_KEY` the same way. The
Langfuse dev keys and pgvector password are **development defaults** — rotate them
before exposing a stack beyond localhost.

---

## 🧪 Development & testing

```bash
make build          # build ./bin/llmaker
make check          # gofmt + vet + go test  (what CI runs)

make facade-setup && make facade-test     # the model facade (pytest)
make agent-setup  && make agent-test      # the RAG/recsys agent (pytest)

make images             # build the backend + agent images
make image-ollama-cpu   # slim CPU image
make image-agent        # the agent image
```

Everything is tested without needing live services: Go command logic runs against
an in-memory fake runtime; the facade's routes + Ollama adapter use an `httpx`
mock; the agent's routes, the LangGraph pipeline, MMR, tracing, and the
recommender run against in-memory fakes. CI runs Go race tests + `gofmt` + a
ruff-linted Python matrix (facade and agent) + a no-push image build on every push.

---

## 🗂️ Project layout

```
cmd/llmaker/            CLI entrypoint
internal/
  backend/              inference engines + image refs
  service/              catalog of stack services (vector DBs, cache, agent, …)
  engine/               domain model (instances + services), ports, labels, Runtime
    dockerrt/           Docker SDK impl + the llmaker-net network (only Docker import)
    enginetest/         in-memory Runtime for tests
  config/               stack.yaml / llm.yaml parsing + service tiers
  cli/                  Cobra commands (up, service, stack, apply, top, …)
  ui/ · tui/            Lip Gloss theme + the `top` Bubble Tea dashboard
facade/                 Python / FastAPI model facade + per-model web UI
agent/                  Python / FastAPI + LangGraph RAG & recommendation agent
images/                 backend + agent Dockerfiles
```

---

## ❓ FAQ

<details>
<summary><b>Is everything really local?</b></summary>

Yes. Every model and service runs in a local Docker container bound to
`127.0.0.1` by default. Your documents, vectors, and traces never leave your
machine unless you choose to expose a port.
</details>

<details>
<summary><b>Do I need a GPU?</b></summary>

No. The slim `:cpu` image runs anywhere Docker does, and the whole RAG stack runs
on a laptop CPU with a small model. A GPU just makes large models faster.
</details>

<details>
<summary><b>How is this different from LangChain / LlamaIndex?</b></summary>

Those are libraries you write code with — you still have to host the vector DB,
embeddings, and tracing yourself. llmaker is the *infrastructure*: it runs and
wires those pieces, and ships a working agent on top. Use llmaker to host the
stack; use a framework inside your own app if you want — they point at the same
endpoints.
</details>

<details>
<summary><b>Can I customize the agent / use my own app?</b></summary>

Both. Tune the agent via env (model, chunking, MMR, top-k, tracing), or ignore it
and point your own app at `qdrant:6333` / `chat:8080` on the network — it's all
standard HTTP and OpenAI-compatible.
</details>

<details>
<summary><b>Something's wrong — where do I look first?</b></summary>

`llmaker doctor` (environment), `llmaker ls` / `top` (what's up), `llmaker logs
<name> -f` (any container, model or service).
</details>

---

<a id="roadmap"></a>

## 🛣️ Status & roadmap

> **Alpha, but real.** Every capability below is implemented, tested, and verified
> end-to-end against live Docker — a full RAG stack ingesting a document, answering
> a grounded question, and tracing it; a recommender ranking items; clean teardown.

- [x] LLM instances — OpenAI-compatible facade, per-model web UI, fleet `ls`/`top`
- [x] **Stack services** — vector DBs, cache, embeddings, observability on a shared network
- [x] **Container networking** — everything discovers everything by name (`llmaker-net`)
- [x] **One-command stacks** — `stack init` (rag · chatbot · faq · recommend) + `apply --prune`
- [x] **Built-in agent** — LangGraph `rewrite→retrieve→rerank→generate`, multi-turn, MMR
- [x] **Recommendations** — `/api/items` + `/api/recommend` (query or "more like this")
- [x] **Observability** — Langfuse tracing, zero-setup dev keys
- [x] Slim CPU image variant
- [ ] Agent tooling — tool/function calls, a reranker model, an eval harness
- [ ] llama.cpp adapter — full model management; `--native` Metal on macOS
- [ ] Multi-arch images → GHCR + a release, Homebrew/`winget`, demo GIFs

---

## 🤝 Contributing

Issues and PRs welcome. Keep it green (`make check`, `make facade-test`,
`make agent-test`), match the surrounding style, add a test. Adding a service is
one catalog entry; adding a backend is one facade adapter — that's the whole job.

## 📄 License

[Apache 2.0](LICENSE) © Raiyan Yahya.

<div align="center">
<br/>
<sub>Built with 🦙, Go, LangGraph, and an unreasonable number of Docker labels.</sub>
</div>
