# Project Plan: `llm` — A CLI for Self-Hosted LLM API Containers

> **Note (2026-06-24):** The implemented binary is named **`llmaker`** (see `README.md`). Throughout this original plan, read `llm` as `llmaker`.

> **One-liner:** A single, beautiful Go CLI that spins up isolated, self-hosted LLM servers as containers — pick a backend (Ollama, llama.cpp, more later), set resource limits, and get a stable OpenAI-compatible API **plus** a built-in web UI per instance. The CLI orchestrates the whole fleet from your terminal with rich styling and a live TUI dashboard. No memorizing `docker run` flags.

- **Status:** Planning
- **Owner:** Raiyan Yahya
- **Created:** 2026-06-16 · **Revised:** 2026-06-16 (CLI-first redesign)
- **Category:** Developer Tooling / Local AI Infrastructure / CLI
- **Binary:** `llm` · **Codename:** `llm`

### What changed in this revision
The original plan exposed the product as a Docker image users ran with `docker run`. That's the weakest part of the DX. This revision makes a **Go CLI the primary interface**: it orchestrates prebuilt, parameterized containers, supports **multiple backends behind one stable API**, manages a **fleet** of instances, and renders status/animations in the terminal. The container's self-contained API + web UI from the original design are preserved — the CLI drives them.

---

## 1. Vision & Principles

Running local LLMs should feel like `vercel dev` or `docker compose up` — one friendly command, sane defaults, instant feedback. The product is a CLI (`llm`) that:

1. Launches **isolated LLM servers** as containers, each exposing a stable **OpenAI-compatible API** + **status/health APIs** + its **own web UI**.
2. Lets you choose the **backend** (Ollama / llama.cpp / future vLLM, TGI, mlx) with **sane defaults**, so apps never care which engine runs underneath.
3. Manages a whole **fleet** — list, inspect, scale, stop, and watch instances live from the terminal.
4. Is **beautiful and animated** — spinners, live progress bars, a `top`-style TUI — built in Go.

**Design principles (the non-negotiables):**

- **Config over codegen.** The common path *runs prebuilt images* with flags/env — fast, reproducible, supportable. Dockerfile generation + image building is an **advanced escape hatch**, not the default (see §4).
- **Backend-agnostic API.** The CLI and your apps talk to a *normalized* API; the engine (Ollama vs llama.cpp) is an implementation detail behind it (see §3).
- **Each container is self-sufficient.** It carries its own API + UI + health endpoints; the CLI is a fleet view *over* those APIs, never a single point the instances depend on.
- **Terminal-first, but not terminal-only.** Rich CLI/TUI for power users; each instance still serves a browser UI for everyone else.
- **Honest about hardware.** GPU acceleration is surfaced and handled explicitly (see §7) — no silently-slow CPU inference.

---

## 2. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│  `llm` CLI  (Go, single static binary)                            │
│  - Cobra command tree + Charm TUI (Bubble Tea / Lip Gloss)        │
│  - Talks to Docker via the Docker Go SDK                          │
│  - Talks to each container's normalized API for status/control    │
│  - Tracks the fleet via Docker labels (no drift-prone state file) │
└───────────────┬───────────────────────────────┬──────────────────┘
                │ create / start / stop          │ HTTP/WS: status,
                │ (Docker SDK)                    │ pull, chat, models
                ▼                                 ▼
   ┌───────────────────────────────────────────────────────────────┐
   │  Container instance  (one per model server)                    │
   │  ┌───────────────────┐   ┌─────────────────────────────────┐  │
   │  │ Backend engine     │   │ Control-plane facade             │  │
   │  │ Ollama | llama.cpp │◀─▶│  - /v1/* OpenAI-compatible       │  │
   │  │ (internal only)    │   │  - /api/status /api/health       │  │
   │  └───────────────────┘   │  - /api/models/* (pull/stop/…)   │  │
   │                          │  - serves the web UI             │  │
   │   model volume           │  - WS: live status + pull progress│ │
   │   (persisted)            └──────────────┬──────────────────┘  │
   │                                  :PORT (exposed, auto-assigned)│
   └───────────────────────────────────────────────────────────────┘
```

- **The CLI is a thin, opinionated orchestrator over Docker** — it does *not* reimplement Docker. It uses the Docker Go SDK to create/run/stop containers and talks HTTP/WS to each instance's facade for status and control.
- **The facade normalizes every backend** to the same contract, so the CLI, the web UI, and your apps are backend-agnostic.

---

## 3. Backends — One API, Many Engines

The control-plane facade inside each container exposes a **stable, normalized contract** regardless of engine:

| Backend | Why | Native quirks the facade hides |
|---|---|---|
| **Ollama** (default) | Easiest; great model library; simple pull/run | Maps Ollama's `/api/*` to our normalized endpoints |
| **llama.cpp** | Max control: GGUF, quantization, fine-grained perf flags | llama.cpp's server is already near-OpenAI; facade aligns status/model mgmt |
| **vLLM / TGI / mlx** (future) | Throughput / Apple-native | Same facade contract, new adapter |

**The contract (identical across backends):**

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings` | OpenAI-compatible inference (streaming via SSE) |
| `GET` | `/api/status` | aggregate: running/installed models, default model, CPU/RAM/GPU/VRAM load, uptime |
| `GET` | `/api/health` | liveness/readiness → 200 / 503 |
| `GET/POST/DELETE` | `/api/models*` | list / pull (with job + progress) / stop (unload) / set-default / delete |
| `WS` | `/ws/status`, `/ws/pull/{job}` | live gauges + download progress (powers UI **and** the CLI's `llm top`) |

> **Key consequence:** adding a backend never changes the CLI, the UI, or anyone's app code — only a new adapter behind the facade. "Options for Ollama or llama.cpp and others" stays clean instead of becoming a pile of special cases.

---

## 4. Prebuilt Images First, Build as an Escape Hatch

**The decisive design call: don't generate a Dockerfile and build on every run.** Per-invocation builds are slow, need build context, and leave every user with a slightly different generated file — a reproducibility and support nightmare. Almost everything users want (model, RAM/CPU, GPU, ports, default model) is **runtime configuration**, not build-time.

**So:**
- **`llm up` runs a prebuilt, parameterized image** — one per backend (`ghcr.io/raiyanyahya/llm-ollama`, `…/llm-llamacpp`), selected by `--backend`. Pulled once, cached, runs in seconds. This is the 95% path.
- **`llm build` is the advanced escape hatch** — *then* the CLI generates a Dockerfile and builds a custom image, for: a custom/bleeding-edge backend, a CPU-optimized llama.cpp compile, a model baked into the image, or a custom base. Reserved for people who explicitly need it.

Config over codegen for the common path; codegen only when config genuinely can't express it.

---

## 5. The CLI — Command Surface & Look/Feel

### 5.1 Go stack (specifically chosen for styling + animation)
- **Cobra** — command tree / flags / help (the Go CLI standard).
- **Charm Bubble Tea** — the TUI runtime for interactive flows and `llm top`.
- **Lip Gloss** — styling: colors, borders, padding, layout (the "nice styling").
- **Bubbles** — ready-made animated components: **spinners, progress bars, tables, viewports**.
- **Huh** — interactive forms/wizards for `llm up` when run without flags.
- **Glamour** — render markdown (help, model cards) beautifully in the terminal.
- **Docker Go SDK** (`github.com/docker/docker/client`) — create/run/stop containers programmatically (no brittle shelling out).
- Ships as a **single static cross-platform binary** (`go build`); install via Homebrew, `curl | sh`, or `go install`.

### 5.2 Animation & styling, concretely
- **`llm up`:** animated spinner while pulling the image; a **live progress bar** for model download (CLI subscribes to the container's `/ws/pull/{job}`); a green ✓ summary card (name, backend, model, port, URL) on success.
- **`llm top`:** a Bubble Tea **live TUI dashboard** across the whole fleet — animated CPU/RAM/GPU/VRAM gauges, currently-loaded model, tokens/sec, all refreshed from each instance's `/ws/status`. "htop for your local LLM fleet."
- **`llm ls`:** a styled, color-coded fleet table (health = green/amber/red, load bars inline).
- **`llm up` (no flags):** a Huh wizard — pick backend, model, memory/CPU, GPU — with sane defaults preselected.
- Consistent theme via Lip Gloss; respects `NO_COLOR` and non-TTY (plain output when piped).

### 5.3 Command surface
```
llm up --backend ollama --model llama3:8b --memory 8g --cpus 4 --gpu
                                  # create + run a prebuilt image (seconds)
llm up                            # interactive Huh wizard with sane defaults
llm ls                            # styled fleet table: name, backend, model, health, port, load
llm top                           # live animated TUI dashboard across the fleet
llm status <name>                 # detailed status (from /api/status)
llm pull llama3:70b --on <name>   # download a model into an instance (live bar)
llm chat <name>                   # quick interactive chat in the terminal (sanity test)
llm open <name>                   # open that instance's web UI in the browser
llm logs <name> -f                # stream container logs
llm stop / start / rm <name>      # lifecycle
llm apply llm.yaml                # declarative multi-instance fleet (compose-like)
llm build --backend llama.cpp --custom ...   # ADVANCED: codegen Dockerfile + build
llm doctor                        # environment check (Docker present? GPU? Mac caveat?)
```

### 5.4 Fleet state & networking
- **State via Docker labels**, not a separate file: every instance is tagged `managed-by=llm`, plus `llm.backend`, `llm.model`, `llm.port`. `llm ls`/`top` just filter by label — **no local registry to drift out of sync** with reality.
- **Auto port allocation:** the CLI picks a free host port per instance and records it in labels/UI, so running several instances never collides on 8080.
- **Declarative fleets:** `llm.yaml` describes multiple instances; `llm apply` reconciles to it (compose-like, but LLM-aware) for reproducible setups.

---

## 6. Resource Allocation

Map directly to Docker runtime limits, with sane defaults the CLI picks from host inspection:

| Flag | Default | Maps to |
|---|---|---|
| `--memory 8g` | derived from host RAM | container memory limit |
| `--cpus 4` | derived from host cores | CPU quota |
| `--gpu` / `--gpus all` | auto-detect NVIDIA | GPU reservation (NVIDIA Container Toolkit) |
| `--port` | auto-assigned | host port for the facade |
| `--model` | `llama3:8b` (or backend default) | preloaded on first boot |

`llm doctor` reports detected RAM/cores/GPU and warns before you over-allocate.

---

## 7. The Hardware Reality: Apple Silicon + Docker (handled, not hidden)

**Docker on macOS cannot pass through the Apple GPU/Metal** — a containerized Ollama/llama.cpp on a Mac runs **CPU-only** and is dramatically slower. On the most common dev laptop, the naive containerized approach is the slow one. This plan handles it explicitly:

- **`--native` mode (Mac):** the CLI manages a **native** Ollama/llama.cpp process (full Metal acceleration) instead of a container, while still wrapping it with the same facade (API + UI + status) so the CLI/apps don't notice the difference. Best perf on Macs.
- **Containers on Linux / NVIDIA:** full isolation + GPU passthrough as designed.
- **`llm doctor` + first-run warning:** detects macOS + Docker and recommends `--native`; never lets a user discover the slowdown via mysterious latency.

This is the one place the "everything in a container" ideal bends to physics — and the CLI is exactly the right layer to abstract it (container vs native becomes a runtime detail).

---

## 8. The Per-Instance Web UI (preserved from the original design)

Each container still serves a minimal, modern, dark-first dashboard at its assigned port (`llm open <name>`):
- Status header (online, uptime, version) + **live gauges** (CPU/RAM/GPU/VRAM via `/ws/status`).
- **Running models** (VRAM, default star, Stop) and **installed models** (size, Set default / Delete / Run).
- **Install panel** with searchable catalog + free-text + **live download progress**.
- **Built-in chat tester** and a **"use from your app"** copy-paste snippet (curl/Python/JS prefilled with host + model).
- Implementation: single static `index.html` + Alpine.js + Tailwind (CDN) — zero build step — served by the facade.

The CLI and the per-instance UI are two front doors onto the **same** facade APIs; everything the UI shows, `llm top` can show too.

---

## 9. Configuration Reference

| Setting | Where | Default | Purpose |
|---|---|---|---|
| backend | `--backend` / `llm.yaml` | `ollama` | engine selection |
| model | `--model` | `llama3:8b` | preloaded model |
| memory / cpus / gpu | flags | host-derived | resource limits |
| port | `--port` | auto | facade host port |
| runtime | `--native` (mac) / container | auto-detect | container vs native (GPU) |
| `API_KEY` | env (passed through) | empty | require bearer auth on the facade |
| `CORS_ORIGINS` | env | `*` | hardening knob |
| `KEEP_ALIVE` | env | `5m` | model keep-alive in VRAM |

---

## 10. Roadmap

**Phase 0 — Spike (½–1 day):** `llm up` runs a prebuilt Ollama image with `--memory/--cpus/--port`; `llm ls` lists via labels; `llm rm`. Prove the orchestration loop + facade `/api/health`.

**Phase 1 — Core CLI + facade (1 wk):** Normalized facade (`/v1`, `/api/status`, model mgmt); `llm up/ls/status/pull/chat/open/logs/stop/start/rm`; auto-port; spinners + live pull progress bar (Charm).

**Phase 2 — Multi-backend (1 wk):** llama.cpp image + adapter behind the same facade; `--backend`; `llm doctor`.

**Phase 3 — TUI + fleet (1 wk):** `llm top` live dashboard; `llm.yaml` + `llm apply`; the `llm up` Huh wizard; polished Lip Gloss theme.

**Phase 4 — Hardware + advanced (1–2 wks):** `--native` Metal mode on macOS; `llm build` codegen/escape hatch; GPU auto-detect; multi-arch prebuilt images → GHCR.

**Phase 5 — Polish:** auth/CORS passthrough, Prometheus `/metrics`, Homebrew tap + install script, docs with terminal GIFs.

**Definition of done:** a developer runs `llm up`, watches an animated download, and within a minute has an OpenAI-compatible endpoint + a browser UI; `llm top` shows live load across every instance; switching from Ollama to llama.cpp is a `--backend` flag and nothing in their app changes; on a Mac, `--native` gives full GPU speed.

---

## 11. Risks & Tradeoffs

| Risk | Impact | Mitigation |
|---|---|---|
| Per-run Dockerfile builds (the tempting wrong default) | Slow, fragile, unsupportable | Prebuilt images by default; `llm build` only as escape hatch (§4). |
| Apple Silicon + Docker = no GPU | Slow on the most common laptop | `--native` Metal mode + `llm doctor` warning (§7). |
| CLI reimplements Docker / scope creep | Maintenance sink | Thin wrapper over the Docker Go SDK; lean on Docker + compose-like `apply`, don't rebuild orchestration. |
| Backend API divergence | Special-case sprawl | One normalized facade contract; backends are adapters (§3). |
| Crowded space (Ollama CLI, Docker Model Runner, LM Studio, Jan, llamafile) | "Why not just Ollama?" | Wedge = multi-backend behind one API + per-instance isolation + each instance has API **and** UI + fleet-level resource control & terminal observability. |
| State drift (which containers are ours?) | Confusing fleet view | Track via Docker labels, not a local file. |
| Port collisions across instances | Failures | Auto-assign + record in labels. |

---

## 12. Competitive Landscape (be honest)
- **Ollama CLI** — single backend, no per-instance isolation/UI, no fleet/resource orchestration.
- **Docker Model Runner (`docker model run`)** — Docker-native, but single-pattern, no rich CLI/TUI or multi-backend facade.
- **LM Studio / Jan / GPT4All** — GUI-first desktop apps, not CLI/fleet/server-oriented.
- **llamafile / llama-swap** — single-binary serving / model swapping, narrower scope.
- **Our wedge:** *one CLI to spin up isolated, observable, API-**and**-UI-equipped LLM servers on any backend, with resource limits, managed as a fleet from a beautiful terminal.* That combination isn't offered elsewhere — but the multi-backend + fleet + observability + DX story has to stay sharp.

---

## 13. Decisions & Open Questions

### Resolved (locked 2026-06-16)
- ✅ **Facade language: Python / FastAPI.** The CLI (Go) and the facade are decoupled by the HTTP contract, so they needn't share a language — and for the facade's job (LLM proxying/normalization, model management, system metrics, serving the UI) Python wins decisively: richest backend-client ecosystem (Ollama client, OpenAI SDK, `llama-cpp-python`), FastAPI's async + SSE + WebSockets + Pydantic + auto OpenAPI docs, and best-in-class metrics via `psutil`/`pynvml`. Image-size cost is marginal since the backend + model dominate the image anyway (mitigate with a slim base + minimal deps). **CLI in Go, facade in Python.**

### Still open
1. **Native mode scope:** macOS only at first, or also a native Linux path? *(Lean: macOS-only native to start — it's the GPU-in-Docker pain point.)*
2. **`llm.yaml` ↔ `llm apply` semantics:** full reconcile (create/update/delete to match file) or additive only? *(Lean: reconcile, with a `--prune` guard.)*
3. **Distribution:** Homebrew + curl script first; Windows/winget when?
4. **Model catalog:** curated per-backend lists baked in, or fetched live?
5. **Auth defaults:** keep open by default (easy) with `API_KEY` opt-in, or secure-by-default on non-loopback binds?

---

## 14. References / Tech to Evaluate
- CLI/TUI (Go): Cobra; Charm **Bubble Tea**, **Lip Gloss**, **Bubbles**, **Huh**, **Glamour**.
- Orchestration: Docker Go SDK (`github.com/docker/docker/client`); NVIDIA Container Toolkit.
- Backends: Ollama HTTP API; llama.cpp server (OpenAI-compatible); vLLM / TGI / mlx (future adapters).
- Facade: **Python / FastAPI** (locked) — async, SSE + WebSockets for streaming/progress, Pydantic, auto OpenAPI docs; `psutil` + `pynvml` for metrics.
- Comparables: Ollama CLI, Docker Model Runner, LM Studio, Jan, llamafile, llama-swap.
