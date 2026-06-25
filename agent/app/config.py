"""Runtime configuration, all from the environment.

The defaults point at the conventional in-network names llmaker gives stack
components (``chat``, ``qdrant``, ``embeddings``), so a stack created with
``llmaker stack init rag`` wires itself together with no extra config.
"""

from __future__ import annotations

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="", case_sensitive=False)

    # LLM (OpenAI-compatible — an llmaker instance's facade).
    llm_base_url: str = "http://chat:8080/v1"
    llm_model: str = "llama3:8b"
    llm_api_key: str = "not-needed"

    # Embeddings (OpenAI-compatible — the llmaker embeddings service / TEI).
    embeddings_url: str = "http://embeddings:80"
    embeddings_model: str = "BAAI/bge-small-en-v1.5"

    # Vector store.
    qdrant_url: str = "http://qdrant:6333"
    collection: str = "llmaker"
    items_collection: str = "items"  # separate collection for recommendation items

    # Retrieval / chunking.
    top_k: int = 4
    chunk_size: int = 1000
    chunk_overlap: int = 150
    # Reranking: fetch top_k * fetch_multiplier candidates, then MMR down to
    # top_k. mmr_lambda trades relevance (1.0) for diversity (0.0).
    fetch_multiplier: int = 3
    mmr_lambda: float = 0.5
    rewrite_queries: bool = True  # rewrite follow-ups into standalone queries

    # Observability (Langfuse). Tracing turns on only when both keys are set,
    # so the agent runs fine without it. Defaults point at the in-network
    # Langfuse service llmaker can run.
    langfuse_host: str = "http://langfuse:3000"
    langfuse_public_key: str = ""
    langfuse_secret_key: str = ""

    # Tool-calling agent.
    agent_max_steps: int = 4  # max tool-call rounds before forcing an answer
    sql_dsn: str = ""  # when set, expose a read-only SQL tool against this database
    # Web search: when set, expose a web_search tool backed by a SearXNG-compatible
    # JSON endpoint (the in-network "searxng" service). Empty → no web_search tool,
    # the same opt-in shape as the SQL tool.
    search_url: str = ""
    search_results: int = 5  # results returned per web_search call

    # Evaluation harness (/api/eval). The judge reuses the chat LLM by default;
    # point eval_model at a stronger model to grade with it instead.
    eval_model: str = ""

    # Conversation memory. When REDIS_URL is set, /api/chat and /api/agent persist
    # per-session history in Redis (pass "session_id"); otherwise history is only
    # what the client sends on each request.
    redis_url: str = ""  # e.g. redis://redis:6379
    memory_max_turns: int = 20  # cap stored user+assistant pairs per session
    memory_ttl_seconds: int = 604800  # expire idle sessions after 7 days

    # Summarization (/api/summarize). Long inputs are map-reduced in chunks.
    summarize_chunk_size: int = 3000

    # Speech-to-text (/api/transcribe). When WHISPER_URL is set, uploaded audio is
    # proxied to this OpenAI-compatible endpoint (the in-network whisper service).
    whisper_url: str = ""
    whisper_model: str = "Systran/faster-whisper-small"

    # Server.
    port: int = 8800
    api_key: str = ""  # when set, require Authorization: Bearer <key>
    max_upload_mb: int = 25  # reject larger /api/ingest and /api/transcribe uploads

    def embeddings_endpoint(self) -> str:
        return self.embeddings_url.rstrip("/") + "/v1/embeddings"

    def tracing_enabled(self) -> bool:
        return bool(self.langfuse_public_key and self.langfuse_secret_key)

    def judge_model(self) -> str:
        """Model used by the evaluation harness as a judge (defaults to the chat model)."""
        return self.eval_model or self.llm_model


def load_settings() -> Settings:
    return Settings()
