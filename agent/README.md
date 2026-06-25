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
| `POST` | `/api/chat` | `{ "question": "...", "top_k": 4, "history": [{role, content}, …] }` → grounded answer + sources |
| `POST` | `/api/agent` | `{ "question": "...", "history": [...], "max_steps": 4 }` → tool-using answer + the tool calls it made |
| `POST` | `/api/eval` | `{ "cases": [{question, reference?, expected_sources?}, …], "top_k"? }` → per-case scores + summary |
| `POST` | `/api/items` | `{ "items": [{id, text, metadata?}, …] }` → embed + store items for recommendations |
| `POST` | `/api/recommend` | `{ "query": "..." }` or `{ "like": ["id", …] }`, `k` → similar items |
| `GET`  | `/` | self-contained web UI (ingest + ask) |

Beyond RAG, the same embeddings + vector store power a **recommender**: load
items once, then get recommendations by free-text intent (`query`) or from a set
of liked items (`like` — their average becomes a taste profile, and the seeds are
excluded). No LLM required.

The pipeline is a LangGraph state graph with four nodes:

```
rewrite → retrieve → rerank → generate
```

- **rewrite** — fold a multi-turn conversation into one standalone search query
  (only calls the LLM when there's `history` to resolve, so single-shot stays fast).
- **retrieve** — embed the query and pull a wide candidate set (`top_k × fetch_multiplier`).
- **rerank** — [MMR](https://en.wikipedia.org/wiki/Maximal_marginal_relevance) down to
  `top_k`, trading relevance for diversity so the LLM sees broad, non-redundant context.
- **generate** — answer using the context plus the conversation so far.

## Tool calling (`/api/agent`)

`/api/agent` runs a second LangGraph graph — a tool-calling loop — where the
model decides which tools to invoke and the loop executes them until it produces
an answer (bounded by `AGENT_MAX_STEPS`):

```
call_model ──(tool calls?)──▶ tools ──▶ call_model ──▶ … ──▶ answer
```

Built-in tools:

- **calculator** — safe arithmetic (no code execution).
- **knowledge_base** — retrieval over your ingested documents (RAG, exposed as a
  tool, so the model searches only when it decides to).
- **current_time** — the current UTC time.
- **web_search** — public-web search via a self-hosted SearXNG endpoint, so the
  model can answer current/external questions without a paid API (enabled only
  when `SEARCH_URL` is set; `llmaker stack init research` wires it).
- **sql** — a single read-only `SELECT`/`WITH` query against `SQL_DSN`
  (enabled only when that variable is set).

The response includes the answer and a `steps` array recording each tool call
(name, arguments, result). Requires a tool-capable model (e.g. `qwen2.5`,
`llama3.1`). Adding a tool is one entry in `app/tools.py`.

## Evaluation (`/api/eval`)

Self-hosting a retrieval stack is only worthwhile if it answers well. `/api/eval`
runs a dataset of questions through the *same* pipeline that serves `/api/chat`
and grades each answer, so quality is measurable and regressions are visible:

```jsonc
POST /api/eval
{
  "cases": [
    { "question": "What is the refund window?",
      "reference": "30 days",                  // optional gold answer
      "expected_sources": ["policy.pdf"] }     // optional retrieval target
  ],
  "top_k": 4
}
```

Each case is scored from 0.0 to 1.0:

- **groundedness** — is every claim in the answer supported by the retrieved context? *(LLM judge)*
- **relevance** — does the answer address the question? *(LLM judge)*
- **correctness** — does the answer match the `reference`? *(judge; only when a reference is given)*
- **context_recall** — fraction of `expected_sources` that retrieval surfaced *(deterministic; only when expected_sources are given)*

The response is the per-case results plus an aggregate `summary` of mean scores.
The judge is the chat model by default (set `EVAL_MODEL` to grade with a stronger
one). When tracing is configured, every case is also sent to Langfuse with its
scores attached — so your eval set lands right next to your live traces.

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
| `TOP_K` | `4` | chunks kept per query (after reranking) |
| `FETCH_MULTIPLIER` | `3` | candidates fetched before MMR (`TOP_K × this`) |
| `MMR_LAMBDA` | `0.5` | rerank relevance↔diversity trade-off (1.0 = pure relevance) |
| `REWRITE_QUERIES` | `true` | rewrite multi-turn follow-ups into standalone queries |
| `AGENT_MAX_STEPS` | `4` | max tool-call rounds in `/api/agent` |
| `SQL_DSN` | — | when set, expose a read-only SQL tool against this database |
| `SEARCH_URL` | — | when set, expose a web_search tool backed by this SearXNG JSON endpoint |
| `SEARCH_RESULTS` | `5` | results returned per web_search call |
| `EVAL_MODEL` | — | judge model for `/api/eval` (defaults to `LLM_MODEL`) |
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
