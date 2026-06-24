<div align="center">

# 🦙 llmaker

### Self-host your whole LLM stack — models **and** the infra around them — from your terminal.

**Local LLM stacks that feel like `docker compose up`, not a CUDA scavenger hunt.**

Pick a backend (Ollama, llama.cpp) and get a stable OpenAI-compatible API **+** a
web UI **per instance** — then add the rest of the stack (vector DBs, cache,
embeddings, observability) so they all find each other by name. Managed as one
fleet from a beautiful terminal. No memorizing `docker run` flags.

<br/>

[![CI](https://github.com/raiyanyahya/llmaker/actions/workflows/ci.yml/badge.svg)](https://github.com/raiyanyahya/llmaker/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/raiyanyahya/llmaker)](https://goreportcard.com/report/github.com/raiyanyahya/llmaker)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Facade](https://img.shields.io/badge/facade-Python%203.10%2B-3776AB?logo=python&logoColor=white)](facade/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

[![Built with Charm](https://img.shields.io/badge/built%20with-Charm_🤍-ff69b4)](https://charm.sh)
[![Docker](https://img.shields.io/badge/Docker-required-2496ED?logo=docker&logoColor=white)](https://docs.docker.com/get-docker/)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS-lightgrey)](#hardware)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](#-contributing)
[![Status](https://img.shields.io/badge/status-alpha-orange.svg)](#roadmap)

<br/>

[**Install**](#-install) · [**Quickstart**](#-quickstart) · [**Why?**](#-why-llmaker) · [**Commands**](#-commands) · [**Architecture**](#architecture) · [**Roadmap**](#roadmap)

</div>

---

```console
$ llmaker up --backend ollama --model llama3:8b --memory 8g --cpus 4
➜ Starting brave-llama (Ollama · llama3:8b)
✓ Image ready
✓ Creating brave-llama
✓ Starting brave-llama
✓ Waiting for brave-llama facade
model  ████████████████████████████  100%  4.7 GB / 4.7 GB
✓ Pulled llama3:8b

╭ ✓ Instance ready ───────────────────╮
│ name      brave-llama               │
│ backend   Ollama                    │
│ model     llama3:8b                 │
│ endpoint  http://127.0.0.1:11500/v1 │
│ web UI    http://127.0.0.1:11500    │
│ port      11500                     │
╰─────────────────────────────────────╯

Next:
  llmaker chat brave-llama   # quick test in the terminal
  llmaker open brave-llama   # open the web UI
  llmaker top                # live fleet dashboard
```

One command. A real OpenAI-compatible endpoint, a browser UI, and live status —
in under a minute. Then `llmaker top` shows load across **every** instance.

---

## 🤔 Why llmaker?

You just wanted to run Llama 3 locally. Simple, right?

Forty browser tabs later, you're three pages deep in a 2019 NVIDIA forum thread,
your `docker run` command has fourteen `-e` flags, *something* is already wedged
on port 8080, the Mac you're on can't see its own GPU through Docker, and you
**still** don't have a UI. Running a second model? Start over.

**`llmaker` is the part where you stop doing that.**

It treats local LLM servers like what they are — services you want to *launch,
observe, and throw away* — and wraps every backend in one stable API so your app
never cares whether Ollama or llama.cpp runs underneath.

The space is crowded, so let's be honest about the wedge — it's the *combination*:

| | Ollama CLI | `docker model run` | LM&nbsp;Studio&nbsp;/&nbsp;Jan | 🦙 **llmaker** |
|---|:---:|:---:|:---:|:---:|
| Multi-backend behind **one** API | ❌ | ❌ | ❌ | ✅ |
| Per-instance **isolation** | ❌ | ⚠️ | ❌ | ✅ |
| API **and** web UI per instance | ❌ | ❌ | ⚠️ | ✅ |
| **Fleet** view + resource limits | ❌ | ❌ | ❌ | ✅ |
| Terminal-first (live TUI dashboard) | ⚠️ | ❌ | ❌ | ✅ |
| Honest about the Mac-GPU problem | n/a | ⚠️ | ✅ | ✅ |

> *"htop for your local LLM fleet" — plus the part where you start the fleet.*

---

## ✨ Features

|   |   |
|---|---|
| 🔌 **Backend-agnostic** | Ollama today, llama.cpp next. Switching is a `--backend` flag — nothing in your app changes. |
| 🏷️ **Fleet = state** | Tracked entirely through Docker labels. `llmaker ls` can't drift out of sync, because the containers **are** the source of truth. |
| 🎨 **Beautiful terminal UX** | Cobra + [Charm](https://charm.sh) (Bubble Tea · Lip Gloss · Bubbles · Huh): a wizard, live progress bars, and `llmaker top`. |
| 🌐 **A UI in every box** | Each instance serves its own dark-mode dashboard — gauges, model management, a chat tester, copy-paste snippets. |
| 🧠 **One stable API** | OpenAI-compatible `/v1/*` (with SSE streaming) for chat, completions, and embeddings — point any OpenAI client at it. |
| 🧱 **Not just LLMs — the whole stack** | A curated catalog of the services around your models: vector DBs (Qdrant, Chroma, pgvector, Weaviate), Redis, a dedicated embeddings server, Langfuse. `llmaker service add qdrant`. |
| 🕸️ **They find each other** | Every instance and service joins one network, reachable by name — your app talks to `chat:8080` **and** `qdrant:6333` with zero IP-wrangling. |
| 🖥️ **Honest about hardware** | Auto-detects GPUs; warns about the Docker-on-macOS no-Metal reality **before** you hit mystery latency. |
| 📜 **Declarative stacks** | `llm.yaml` + `llmaker apply` — compose-like, but LLM-aware: LLMs **and** services in one file, with a `--prune` reconcile. |
| 🪶 **Slim by choice** | A 357 MB CPU-only image (vs 8.5 GB GPU) for laptops, CI, and Macs. |
| 🤖 **Script-friendly** | `--json` output, `NO_COLOR`/non-TTY aware, sane exit codes. Pretty for humans, parseable for robots. |

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

<sub>Homebrew tap & `winget` are on the [roadmap](#roadmap).</sub>

---

## 🚀 Quickstart

```bash
llmaker up chat                # fastest path: a preset — obvious model, sane defaults, zero flags
llmaker up                     # interactive wizard with host-derived defaults
llmaker up --model llama3:8b   # …or go straight to it with explicit flags
llmaker ls                     # styled fleet table
llmaker chat brave-llama       # sanity-check it in the terminal
llmaker open brave-llama       # open the web UI in your browser
llmaker top                    # live dashboard across the whole fleet
```

**Presets** are one-word shortcuts for the obvious cases — pick a model and go, no flags:

```bash
llmaker up chat      # general chat            → llama3:8b
llmaker up code      # coding                  → qwen2.5-coder:7b
llmaker up small     # tiny & fast (CPU/low-RAM) → llama3.2:1b
llmaker up embed     # embeddings for RAG      → nomic-embed-text
llmaker up vision    # images + text           → llava:7b
```

Any flag overrides a preset (and skips the wizard): `llmaker up code --gpu --memory 16g`.

Then use it from **any** OpenAI client — it's just an endpoint:

```python
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:11500/v1", api_key="not-needed")
print(client.chat.completions.create(
    model="llama3:8b",
    messages=[{"role": "user", "content": "Hello"}],
).choices[0].message.content)
```

Switch the whole thing to llama.cpp later? `--backend llamacpp`. Your code doesn't move.

---

## 🔗 Drop it into the tools you already use

It's an OpenAI-compatible endpoint, so anything that speaks OpenAI works
unchanged — just point it at `http://127.0.0.1:11500/v1` with any non-empty key.

<details>
<summary><b>aider</b> — AI pair programming in your repo</summary>

```bash
export OPENAI_API_BASE=http://127.0.0.1:11500/v1
export OPENAI_API_KEY=not-needed
aider --model openai/qwen2.5-coder:7b      # llmaker up code
```
</details>

<details>
<summary><b>Continue.dev</b> — the VS Code / JetBrains copilot</summary>

```json
// ~/.continue/config.json
{
  "models": [{
    "title": "llmaker",
    "provider": "openai",
    "model": "llama3:8b",
    "apiBase": "http://127.0.0.1:11500/v1",
    "apiKey": "not-needed"
  }]
}
```
</details>

<details>
<summary><b>LangChain</b> / <b>LlamaIndex</b></summary>

```python
# LangChain
from langchain_openai import ChatOpenAI
llm = ChatOpenAI(base_url="http://127.0.0.1:11500/v1",
                 api_key="not-needed", model="llama3:8b")

# LlamaIndex
from llama_index.llms.openai_like import OpenAILike
llm = OpenAILike(api_base="http://127.0.0.1:11500/v1",
                 api_key="not-needed", model="llama3:8b", is_chat_model=True)
```
</details>

<details>
<summary><b>curl</b> — no SDK at all</summary>

```bash
curl http://127.0.0.1:11500/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"llama3:8b","messages":[{"role":"user","content":"hi"}]}'
```
</details>

> RAG? `llmaker up embed` gives you a `/v1/embeddings` endpoint on the same
> contract — point your vector store at it the same way.

---

## 🧭 Commands

| Command | What it does |
|---|---|
| `llmaker up [preset]` | create + start an instance — a preset (`chat`, `code`, `small`, `embed`, `vision`) for an instant zero-flag start, an explicit set of flags, or a [Huh](https://github.com/charmbracelet/huh) wizard when run with neither |
| `llmaker ls` | styled fleet table — `--json`, `--quiet` |
| `llmaker top` | live, animated dashboard across the fleet |
| `llmaker status <name>` | detailed status: gauges, loaded/installed models — `--json` |
| `llmaker pull <model> --on <name>` | download a model with a live progress bar — `--default` |
| `llmaker chat [name]` | interactive or one-shot chat — `--message`, or pipe `stdin` |
| `llmaker open <name>` | open the instance's web UI — `--print` to just emit the URL |
| `llmaker logs <name> -f` | stream container logs (instance **or** service) |
| `llmaker stop` / `start` / `rm <name>…` | lifecycle — `rm --force` |
| `llmaker service catalog` | list the services you can run (vector DBs, cache, embeddings, observability) |
| `llmaker service add <type> [name]` | create + start a service — `--env`, `--port`, `--memory` |
| `llmaker service ls` / `rm` / `stop` / `start` | manage running services — `--json` |
| `llmaker stack init <rag\|chatbot\|faq>` | scaffold a ready-to-apply whole-stack `stack.yaml` |
| `llmaker apply -f stack.yaml` | reconcile a declarative stack (LLMs **+** services) — `--prune` |
| `llmaker doctor` | environment check (Docker, GPU, the macOS caveat) |
| `llmaker build` | **advanced**: generate a custom image build context |

---

<a id="architecture"></a>

## 🏗️ Architecture

```
┌────────────────────────────────────────────────────────────┐
│  llmaker CLI  (Go, single static binary)                    │
│  Cobra + Charm TUI · Docker Go SDK · fleet via labels       │
└───────────────┬─────────────────────────────┬──────────────┘
                │ create / start / stop        │ HTTP: status,
                │ (Docker SDK)                 │ pull, chat, models
                ▼                              ▼
   ┌──────────────────────────────────────────────────────────┐
   │  Container instance                                       │
   │   ┌────────────────┐   ┌───────────────────────────────┐ │
   │   │ Backend engine │◀─▶│ Facade (Python / FastAPI)      │ │
   │   │ Ollama|llamacpp│   │  /v1/* · /api/status · models  │ │
   │   │ (loopback only)│   │  web UI · /ws/status           │ │
   │   └────────────────┘   └──────────────┬────────────────┘ │
   │     model volume                 :8080 → host :PORT       │
   └──────────────────────────────────────────────────────────┘
```

- **CLI in Go, facade in Python** — decoupled by the HTTP contract, each using
  the best ecosystem for its job (Charm's TUI stack; FastAPI's async + SSE +
  WebSockets + Pydantic + `psutil`/`pynvml`).
- The CLI polls `/api/status` and streams pulls over plain HTTP; the facade's
  WebSocket exists for the **browser** UI. Both front doors hit the *same* API.
- **No local state file.** Every instance is a labeled container, so the fleet
  view is always reality.

📄 The full facade contract — every endpoint and env var — lives in
[`facade/README.md`](facade/README.md).

---

## 🔌 Backends — one API, many engines

The facade inside each container normalizes every engine to the same contract:

| Backend | Status | Why |
|---|:---:|---|
| **Ollama** | ✅ default | Easiest path; rich model library; simple pull/run |
| **llama.cpp** | 🧱 structural | Max control: GGUF, quantization, fine-grained perf flags |
| vLLM · TGI · mlx | 🛣️ future | Throughput / Apple-native — same contract, new adapter |

Adding a backend is one adapter (`facade/app/adapters/`) — the CLI, the UI, and
your app never change.

---

## 🧱 Beyond LLMs — the whole stack

A real chatbot, FAQ bot, or recommender isn't just a model — it's a model **plus**
a vector store, a cache, an embedding service, and something to watch it all.
`llmaker` runs those too, from one curated catalog:

```bash
llmaker service catalog          # see what's available
llmaker service add qdrant       # vector DB         → qdrant:6333
llmaker service add redis        # cache / memory    → redis:6379
llmaker service add embeddings   # HF TEI embeddings → embeddings:80
llmaker service add langfuse     # LLM observability → langfuse:3000
llmaker ls                       # instances *and* services, one view
```

| Category | Services |
|---|---|
| **Vector databases** | Qdrant · Chroma · pgvector (Postgres) · Weaviate |
| **Cache / memory** | Redis (chat memory, sessions, semantic cache) |
| **Embeddings** | HuggingFace Text-Embeddings-Inference |
| **Observability** | Langfuse (tracing, prompt analytics, evals) |
| **Agent** | LangGraph RAG agent — ingest docs, answer grounded questions |

**The point is the wiring.** Every instance and service joins one Docker network
(`llmaker-net`) and is reachable there by its name — no IPs, no `--link`, no
hand-rolled compose file. Your RAG app, running as its own container or on your
host, talks to `chat:8080` and `qdrant:6333` and it just resolves:

```bash
# proof: a throwaway container reaches the service by name
docker run --rm --network llmaker-net redis:7-alpine redis-cli -h redis ping
# → PONG
```

Adding a service is one entry in the catalog (`internal/service/catalog.go`) —
the CLI, `ls`, `top`, and `apply` pick it up for free.

---

## 🤖 A whole RAG stack in one command

`llmaker stack init` scaffolds a ready-to-apply stack — an LLM **plus** a vector
DB, an embeddings server, and a built-in [LangGraph](https://langchain-ai.github.io/langgraph/)
RAG agent that ingests your documents and answers questions grounded in them:

```bash
llmaker stack init rag        # writes stack.yaml (rag | chatbot | faq)
make image-agent              # build the agent image once
llmaker apply -f stack.yaml   # chat + qdrant + embeddings + agent, all wired up
```

That brings up four containers that already know how to find each other —
`agent:8800` talks to `chat:8080`, `qdrant:6333`, and `embeddings:80` by name.
Then ingest and ask (or use the agent's web UI):

```bash
A=$(llmaker service ls --json | jq -r '.[]|select(.service=="agent").url')
curl -s "$A/api/ingest" -F text='llmaker runs your whole local LLM stack.'
curl -s "$A/api/chat" -H 'content-type: application/json' \
     -d '{"question":"what does llmaker do?"}'
# → {"answer":"It runs your whole local LLM stack.","sources":[...]}
```

The agent is a small FastAPI + LangGraph app (`agent/`): a `retrieve → generate`
graph over any OpenAI-compatible LLM and embeddings endpoint, with Qdrant for
storage. Swap the model or the vector DB — the graph doesn't change.

---

## 📜 Declarative stacks

One file brings up the **whole stack** — LLMs and the services around them —
and `apply` reconciles to it (services first, since your app may dial them at
boot):

```yaml
# stack.yaml  →  llmaker apply -f stack.yaml [--prune]
defaults: { backend: ollama }
instances:
  - { name: chat, model: llama3:8b, memory: 8g }   # → chat:8080
services:
  - use: qdrant                                    # → qdrant:6333
  - { name: cache, use: redis }                    # → cache:6379
  - { name: embeddings, use: embeddings, env: { MODEL_ID: BAAI/bge-small-en-v1.5 } }
```

Ports left unset are auto-assigned. A stack can be services-only, too. See
[`examples/stack.yaml`](examples/stack.yaml) and [`examples/llm.yaml`](examples/llm.yaml).

---

## ⚙️ Configuration

| Setting | Where | Default |
|---|---|---|
| backend | `--backend` / `llm.yaml` | `ollama` |
| model | `--model` | backend default |
| memory / cpus / gpu | flags | host-derived |
| port / host | `--port` / `--host` | auto / `127.0.0.1` |
| `API_KEY` | `--api-key` | empty (open) |
| `CORS_ORIGINS` | `--cors` | `*` |
| `KEEP_ALIVE` | `--keep-alive` | `5m` |

---

<a id="hardware"></a>

## 🖥️ Hardware & image variants

**Docker on macOS cannot pass through the Apple GPU** — a containerized engine
runs CPU-only and is much slower. `llmaker doctor` detects this and warns you,
`llmaker up --gpu` on a Mac tells you up front, and native Metal mode (`--native`)
is on the roadmap. On Linux with NVIDIA, `--gpu` reserves your GPUs via the
NVIDIA Container Toolkit.

The Ollama backend ships in two flavors — the GPU base bundles ~4 GB of
CUDA/ROCm/MLX libraries that CPU hosts don't need:

| Tag | Size | Use |
|---|---|---|
| `llmaker-ollama:latest` | ~8.5 GB | GPU-capable — Linux + NVIDIA |
| `llmaker-ollama:cpu` | **~360 MB** | slim, CPU-only — laptops, CI, Docker-on-macOS |

```bash
make image-ollama-cpu     # docker build -f images/ollama/Dockerfile.cpu ...
llmaker up --image ghcr.io/raiyanyahya/llmaker-ollama:cpu --model qwen2.5:0.5b
```

`llmaker` uses a *pull-if-missing* policy, so a locally-built image is used as-is
without contacting a registry.

---

## 🔒 Security

The facade binds to **`127.0.0.1`** by default. Exposing it is opt-in and pairs
with auth:

```bash
llmaker up --host 0.0.0.0 --api-key "$(openssl rand -hex 16)"
```

When `API_KEY` is set, every `/v1/*` and `/api/*` call requires
`Authorization: Bearer <key>` — except `/api/health`, kept open for container
probes. `CORS_ORIGINS` tightens the browser surface.

---

## 🧪 Development & testing

```bash
make build          # build ./bin/llmaker
make test           # go test ./...
make check          # gofmt + vet + test  (what CI runs)
make cover          # coverage summary

make facade-setup   # venv + install the Python facade
make facade-test    # pytest
make agent-setup    # venv + install the RAG agent
make agent-test     # pytest

make images             # build the backend + agent images
make image-ollama-cpu   # build the slim CPU image
make image-agent        # build the RAG agent image
```

All three parts are tested: Go command logic runs against an in-memory fake runtime
(no Docker needed), the facade's routes + Ollama adapter are covered with `pytest`
(the adapter via an `httpx` mock transport), and the RAG agent's routes + LangGraph
pipeline run against in-memory fakes. CI runs Go race tests + `gofmt` + a Python
matrix (facade and agent, ruff-linted) + a no-push image build on every push.

---

## 🗂️ Project layout

```
cmd/llmaker/            CLI entrypoint
internal/
  backend/              supported inference engines + image refs
  service/              catalog of stack services (vector DBs, cache, …)
  engine/               domain model (instances + services), ports, labels, Runtime
    dockerrt/           Docker SDK implementation (the only Docker import)
    enginetest/         in-memory Runtime for tests
  facade/               Go client for the facade contract
  config/               stack.yaml / llm.yaml parsing
  ui/                   Lip Gloss theme, tables, gauges, spinners
  tui/                  `llmaker top` Bubble Tea dashboard
  cli/                  Cobra commands
facade/                 Python / FastAPI control-plane + web UI
agent/                  Python / FastAPI + LangGraph RAG agent
images/                 backend + agent Dockerfiles + entrypoints
```

---

## ❓ FAQ

<details>
<summary><b>Do I need a GPU?</b></summary>

No. The slim `:cpu` image runs anywhere Docker does. A GPU just makes large
models faster — `llmaker up small` is happy on a laptop CPU.
</details>

<details>
<summary><b>Why a Python facade instead of hitting Ollama directly?</b></summary>

So your app targets **one** contract forever. Swap Ollama for llama.cpp (or a
future vLLM/TGI) and the endpoint, the web UI, and your client code never change
— only a `--backend` flag does.
</details>

<details>
<summary><b>Is my model data sent anywhere?</b></summary>

No. Everything runs in a local container bound to `127.0.0.1` by default.
Exposing it is opt-in (`--host 0.0.0.0`) and pairs with `--api-key`.
</details>

<details>
<summary><b>Can I run several models at once?</b></summary>

Yes — that's the point. Each is its own isolated instance on its own port;
`llmaker ls` and `llmaker top` show the whole fleet. Define them once in
`llm.yaml` and `llmaker apply`.
</details>

<details>
<summary><b>Something's wrong — where do I look first?</b></summary>

`llmaker doctor` (environment), `llmaker status <name>` (the instance),
`llmaker logs <name> -f` (the container). On macOS, remember Docker can't see
the Apple GPU — `doctor` will say so.
</details>

---

<a id="roadmap"></a>

## 🛣️ Status & roadmap

> **Alpha, but real.** The Go CLI, the Python facade, the Ollama backend, and the
> web UI are implemented, tested, and verified against live Docker — `up`, a model
> pull, real inference, and teardown all work end to end.

- [x] Core CLI + normalized facade (`up · ls · status · pull · chat · open · logs · stop/start/rm`)
- [x] Live progress, `top` dashboard, the `up` wizard
- [x] Declarative stacks (`apply --prune`) — LLMs **and** services in one file
- [x] **Stack services** — vector DBs, cache, embeddings, observability on a shared network
- [x] **One-command RAG** — `stack init` templates + a LangGraph agent image wired to the stack
- [x] Slim CPU image variant
- [ ] More agent graphs (rerank, tools, multi-step) + Langfuse tracing baked in
- [ ] llama.cpp adapter — full model management
- [ ] `--native` Metal mode on macOS
- [ ] Multi-arch images → GHCR (`amd64` + `arm64`)
- [ ] Homebrew tap, `winget`, demo GIFs

---

## 🤝 Contributing

Issues and PRs welcome. Keep it green (`make check` + `make facade-test`), match
the surrounding style, and add a test. Adding a backend? Implement one adapter in
`facade/app/adapters/` and register it — that's the whole job.

## 📄 License

[Apache 2.0](LICENSE) © Raiyan Yahya.

<div align="center">
<br/>
<sub>Built with 🦙, Go, and an unreasonable number of Docker labels.</sub>
</div>
