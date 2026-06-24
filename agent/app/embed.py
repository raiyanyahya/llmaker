"""Embeddings client.

Talks to any OpenAI-compatible ``/v1/embeddings`` endpoint — the llmaker
embeddings service (HuggingFace TEI) by default, but equally an llmaker LLM
instance that serves embeddings. Kept deliberately thin (one httpx call) so the
agent has no hard dependency on a particular embeddings backend.
"""

from __future__ import annotations

import httpx

from .config import Settings


class Embedder:
    def __init__(self, settings: Settings, client: httpx.AsyncClient | None = None) -> None:
        self._settings = settings
        self._client = client or httpx.AsyncClient(timeout=60.0)

    async def aclose(self) -> None:
        await self._client.aclose()

    async def embed(self, texts: list[str]) -> list[list[float]]:
        """Return one embedding vector per input text."""
        if not texts:
            return []
        payload = {"input": texts, "model": self._settings.embeddings_model}
        r = await self._client.post(self._settings.embeddings_endpoint(), json=payload)
        r.raise_for_status()
        data = r.json().get("data", [])
        # Preserve request order (OpenAI returns an "index" per item).
        ordered = sorted(data, key=lambda d: d.get("index", 0))
        return [d["embedding"] for d in ordered]

    async def embed_one(self, text: str) -> list[float]:
        vecs = await self.embed([text])
        return vecs[0] if vecs else []
