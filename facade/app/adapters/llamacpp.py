"""llama.cpp adapter.

llama.cpp's ``server`` is already near-OpenAI, so inference proxies straight
through. Its model story differs from Ollama: a server typically serves a single
GGUF that's mounted/baked rather than pulled at runtime, so ``pull`` reports an
actionable error instead of pretending. This adapter is intentionally minimal —
it establishes the shape; richer multi-model management can grow behind it.
"""

from __future__ import annotations

from collections.abc import AsyncIterator

import httpx

from ..config import Settings
from ..models import InstalledModel, PullEvent, RunningModel
from .base import Adapter


class LlamaCppAdapter(Adapter):
    name = "llamacpp"

    def __init__(self, settings: Settings, client: httpx.AsyncClient | None = None) -> None:
        self.base = settings.llamacpp_url
        self._client = client or httpx.AsyncClient(timeout=None)

    async def aclose(self) -> None:
        await self._client.aclose()

    async def health(self) -> bool:
        try:
            r = await self._client.get(f"{self.base}/health", timeout=5.0)
            return r.status_code == 200
        except httpx.HTTPError:
            return False

    async def list_models(self) -> list[InstalledModel]:
        try:
            r = await self._client.get(f"{self.base}/v1/models", timeout=15.0)
            r.raise_for_status()
        except httpx.HTTPError:
            return []
        return [InstalledModel(name=m.get("id", "")) for m in r.json().get("data", [])]

    async def running_models(self) -> list[RunningModel]:
        # The server serves whatever model it was started with.
        models = await self.list_models()
        return [RunningModel(name=m.name) for m in models[:1]]

    async def pull(self, model: str) -> AsyncIterator[PullEvent]:
        yield PullEvent(
            status="error",
            error=(
                "the llama.cpp backend does not pull models at runtime; "
                "mount or bake a GGUF and set --model to its name"
            ),
        )

    async def delete(self, model: str) -> None:
        raise NotImplementedError("llama.cpp models are files; manage them on disk")

    async def chat(self, payload: dict):
        return await self._openai("/chat/completions", payload)

    async def completions(self, payload: dict):
        return await self._openai("/completions", payload)

    async def embeddings(self, payload: dict) -> dict:
        r = await self._client.post(f"{self.base}/v1/embeddings", json=payload)
        r.raise_for_status()
        return r.json()

    async def openai_models(self) -> dict:
        r = await self._client.get(f"{self.base}/v1/models", timeout=15.0)
        r.raise_for_status()
        return r.json()

    async def _openai(self, path: str, payload: dict):
        if payload.get("stream"):
            return self._stream_openai(path, payload)
        r = await self._client.post(f"{self.base}/v1{path}", json=payload)
        r.raise_for_status()
        return r.json()

    async def _stream_openai(self, path: str, payload: dict) -> AsyncIterator[bytes]:
        async with self._client.stream("POST", f"{self.base}/v1{path}", json=payload) as resp:
            resp.raise_for_status()
            async for chunk in resp.aiter_raw():
                if chunk:
                    yield chunk
