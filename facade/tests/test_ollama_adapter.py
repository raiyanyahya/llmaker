"""Tests for the Ollama adapter's translation of Ollama's native API into the
facade contract, using an httpx MockTransport so no real Ollama is required."""

from __future__ import annotations

import httpx
import pytest

from app.adapters.ollama import OllamaAdapter
from app.config import Settings


def _handler(request: httpx.Request) -> httpx.Response:
    path = request.url.path
    if path == "/api/tags":
        return httpx.Response(200, json={"models": [{"name": "llama3:8b", "size": 4096, "modified_at": "2024-05-01"}]})
    if path == "/api/ps":
        return httpx.Response(200, json={"models": [{"name": "llama3:8b", "size": 4096, "size_vram": 8192}]})
    if path == "/api/pull":
        body = (
            b'{"status":"pulling manifest"}\n'
            b'{"status":"downloading","completed":1,"total":2}\n'
            b'{"status":"success"}\n'
        )
        return httpx.Response(200, content=body)
    if path == "/api/delete":
        return httpx.Response(200)
    if path == "/v1/chat/completions":
        return httpx.Response(200, json={"choices": [{"message": {"role": "assistant", "content": "hi"}}]})
    if path == "/v1/embeddings":
        return httpx.Response(200, json={"data": [{"embedding": [0.5]}]})
    if path == "/v1/models":
        return httpx.Response(200, json={"object": "list", "data": [{"id": "llama3:8b"}]})
    return httpx.Response(404)


@pytest.fixture
def adapter() -> OllamaAdapter:
    client = httpx.AsyncClient(transport=httpx.MockTransport(_handler))
    return OllamaAdapter(Settings(), client=client)


async def test_health(adapter):
    assert await adapter.health() is True


async def test_list_models(adapter):
    models = await adapter.list_models()
    assert models[0].name == "llama3:8b"
    assert models[0].size == 4096


async def test_running_models(adapter):
    running = await adapter.running_models()
    assert running[0].vram == 8192


async def test_pull_maps_progress(adapter):
    events = [e async for e in adapter.pull("llama3:8b")]
    assert events[0].status == "pulling manifest"
    assert events[1].completed == 1 and events[1].total == 2
    assert events[-1].status == "success"


async def test_chat_non_streaming(adapter):
    result = await adapter.chat({"model": "m", "messages": []})
    assert result["choices"][0]["message"]["content"] == "hi"


async def test_delete_ok(adapter):
    await adapter.delete("llama3:8b")  # should not raise


async def test_embeddings(adapter):
    result = await adapter.embeddings({"model": "m", "input": "x"})
    assert result["data"][0]["embedding"] == [0.5]
