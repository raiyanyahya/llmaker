# llmaker facade

The per-instance control-plane that runs inside every llmaker container. It wraps
a backend engine (Ollama, llama.cpp, …) and exposes one normalized contract:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings` | OpenAI-compatible inference (SSE streaming) |
| `GET`  | `/v1/models` | OpenAI-style model list |
| `GET`  | `/api/health` | liveness/readiness → 200 / 503 (unauthenticated) |
| `GET`  | `/api/status` | aggregate instance + system + model status |
| `GET`  | `/api/models` | installed models + default |
| `POST` | `/api/models/pull` | pull a model (streamed NDJSON progress) |
| `POST` | `/api/models/delete` | delete a model |
| `POST` | `/api/models/default` | set the default model |
| `WS`   | `/ws/status` | live status push for the web UI |
| `GET`  | `/` | self-contained web UI |

## Configuration (env)

| Var | Default | Purpose |
|---|---|---|
| `LLMAKER_BACKEND` | `ollama` | which adapter to load |
| `LLMAKER_NAME` | `llmaker` | instance name shown in status |
| `LLMAKER_DEFAULT_MODEL` | — | initial default model |
| `FACADE_PORT` | `8080` | port the facade binds inside the container |
| `API_KEY` | — | when set, require `Authorization: Bearer <key>` |
| `CORS_ORIGINS` | `*` | comma-separated allowed origins |
| `OLLAMA_URL` | `http://127.0.0.1:11434` | Ollama backend address |

## Develop

```bash
python3 -m venv .venv && . .venv/bin/activate
pip install -e ".[dev]"
pytest -q                 # run the test suite
python -m app             # run locally (needs a backend on localhost)
```

## Add a backend

Implement `app.adapters.base.Adapter` and register it in `app.adapters.build_adapter`.
Nothing else — routes, the web UI, and the CLI are all backend-agnostic.
