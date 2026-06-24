# llmaker RAG agent

A small LangGraph retrieval-augmented chat service that ties an llmaker stack
together: an LLM instance, a vector database, and an embeddings endpoint. It
ingests documents and answers questions grounded in them.

It's just another container on the llmaker network, so its defaults point at the
conventional in-network names (`chat`, `qdrant`, `embeddings`) — a stack from
`llmaker stack init rag` wires itself up with no extra config.

## API

| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/health` | liveness (unauthenticated) |
| `GET`  | `/api/stats` | collection name, chunk count, models |
| `POST` | `/api/ingest` | ingest a `file` upload or `text` form field → chunk, embed, store |
| `POST` | `/api/chat` | `{ "question": "...", "top_k": 4 }` → grounded answer + sources |
| `GET`  | `/` | self-contained web UI (ingest + ask) |

The pipeline is a LangGraph state graph: `retrieve` (embed the question, search
the vector store) → `generate` (prompt the LLM with the retrieved context).

## Configuration (env)

| Var | Default | Purpose |
|---|---|---|
| `LLM_BASE_URL` | `http://chat:8080/v1` | OpenAI-compatible LLM (an llmaker instance) |
| `LLM_MODEL` | `llama3:8b` | chat model |
| `LLM_API_KEY` | `not-needed` | bearer token for the LLM, if it requires one |
| `EMBEDDINGS_URL` | `http://embeddings:80` | OpenAI-compatible embeddings endpoint |
| `EMBEDDINGS_MODEL` | `BAAI/bge-small-en-v1.5` | embedding model |
| `QDRANT_URL` | `http://qdrant:6333` | vector database |
| `COLLECTION` | `llmaker` | Qdrant collection name |
| `TOP_K` | `4` | chunks retrieved per query |
| `CHUNK_SIZE` / `CHUNK_OVERLAP` | `1000` / `150` | ingestion chunking |
| `PORT` | `8800` | server port |
| `API_KEY` | — | when set, require `Authorization: Bearer <key>` on `/api/*` |

## Develop

```bash
python3 -m venv .venv && . .venv/bin/activate
pip install -e ".[dev]"
pytest -q                 # tests run against in-memory fakes (no services needed)
python -m app             # run locally (needs the services it points at)
```
