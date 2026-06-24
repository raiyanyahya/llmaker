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

    # Server.
    port: int = 8800
    api_key: str = ""  # when set, require Authorization: Bearer <key>

    def embeddings_endpoint(self) -> str:
        return self.embeddings_url.rstrip("/") + "/v1/embeddings"


def load_settings() -> Settings:
    return Settings()
