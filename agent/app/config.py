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

    # Server.
    port: int = 8800
    api_key: str = ""  # when set, require Authorization: Bearer <key>

    def embeddings_endpoint(self) -> str:
        return self.embeddings_url.rstrip("/") + "/v1/embeddings"

    def tracing_enabled(self) -> bool:
        return bool(self.langfuse_public_key and self.langfuse_secret_key)


def load_settings() -> Settings:
    return Settings()
