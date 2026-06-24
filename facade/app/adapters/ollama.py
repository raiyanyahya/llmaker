"""Ollama adapter.

Ollama already exposes an OpenAI-compatible ``/v1`` surface, so inference is a
transparent reverse proxy. Only the control plane (model list/pull/delete,
running models) needs translation from Ollama's native ``/api`` shape into the
facade contract.
"""

from __future__ import annotations

import json
from collections.abc import AsyncIterator

import httpx

from ..config import Settings
from ..models import InstalledModel, PullEvent, RunningModel
from .base import Adapter


class OllamaAdapter(Adapter):
    name = "ollama"

    def __init__(self, settings: Settings, client: httpx.AsyncClient | None = None) -> None:
        self.base = settings.ollama_url
        # timeout=None: pulls and long generations must not be cut off.
        self._client = client or httpx.AsyncClient(timeout=None)

    async def aclose(self) -> None:
        await self._client.aclose()

    # --- control plane ---

    async def health(self) -> bool:
        try:
            r = await self._client.get(f"{self.base}/api/tags", timeout=5.0)
            return r.status_code == 200
        except httpx.HTTPError:
            return False

    async def list_models(self) -> list[InstalledModel]:
        r = await self._client.get(f"{self.base}/api/tags", timeout=15.0)
        r.raise_for_status()
        out: list[InstalledModel] = []
        for m in r.json().get("models", []):
            out.append(
                InstalledModel(
                    name=m.get("name", ""),
                    size=int(m.get("size", 0) or 0),
                    modified=m.get("modified_at", "") or "",
                )
            )
        return out

    async def running_models(self) -> list[RunningModel]:
        try:
            r = await self._client.get(f"{self.base}/api/ps", timeout=10.0)
            r.raise_for_status()
        except httpx.HTTPError:
            return []
        out: list[RunningModel] = []
        for m in r.json().get("models", []):
            out.append(
                RunningModel(
                    name=m.get("name", ""),
                    size=int(m.get("size", 0) or 0),
                    vram=int(m.get("size_vram", 0) or 0),
                )
            )
        return out

    async def pull(self, model: str) -> AsyncIterator[PullEvent]:
        """Stream Ollama's pull progress, normalized to PullEvent."""
        payload = {"model": model, "stream": True}
        try:
            async with self._client.stream("POST", f"{self.base}/api/pull", json=payload) as resp:
                if resp.status_code != 200:
                    body = (await resp.aread()).decode(errors="replace")
                    yield PullEvent(status="error", error=f"pull failed ({resp.status_code}): {body}")
                    return
                async for line in resp.aiter_lines():
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        m = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    if m.get("error"):
                        yield PullEvent(status="error", error=str(m["error"]))
                        return
                    status = m.get("status", "")
                    event = PullEvent(
                        status="success" if status == "success" else (status or "downloading"),
                        digest=m.get("digest", "") or "",
                        completed=int(m.get("completed", 0) or 0),
                        total=int(m.get("total", 0) or 0),
                    )
                    yield event
        except httpx.HTTPError as exc:
            yield PullEvent(status="error", error=str(exc))

    async def delete(self, model: str) -> None:
        # Ollama's delete takes the model in the body of a DELETE request.
        r = await self._client.request(
            "DELETE", f"{self.base}/api/delete", json={"name": model}, timeout=30.0
        )
        if r.status_code not in (200, 404):
            r.raise_for_status()

    # --- inference (transparent OpenAI proxy) ---

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
        """Proxy an OpenAI endpoint, streaming when the caller asked for it."""
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
