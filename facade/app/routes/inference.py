"""OpenAI-compatible inference. These endpoints are a thin shim over the backend
adapter; streaming chat/completions come back as Server-Sent Events."""

from __future__ import annotations

import time

from fastapi import APIRouter, Depends, HTTPException, Request
from fastapi.responses import JSONResponse, StreamingResponse

from ..deps import require_auth

router = APIRouter(prefix="/v1", dependencies=[Depends(require_auth)])


async def _read_json(request: Request) -> dict:
    """Parse the JSON body, answering 400 (not 500) on malformed input."""
    try:
        return await request.json()
    except Exception:
        raise HTTPException(status_code=400, detail="invalid JSON body")


def _respond(result):
    # Adapters return an async byte iterator for streamed responses, else a dict.
    if hasattr(result, "__aiter__"):
        return StreamingResponse(result, media_type="text/event-stream")
    return JSONResponse(result)


def _record_usage(state, result, elapsed: float) -> None:
    """Feed token throughput from a non-streaming result into the rolling
    tokens/sec metric. Streaming responses are async iterators and carry no
    usage here, so they're skipped."""
    if not isinstance(result, dict):
        return
    usage = result.get("usage") or {}
    tokens = usage.get("completion_tokens")
    if tokens is None:
        tokens = usage.get("total_tokens", 0)
    try:
        state.record_completion(int(tokens or 0), elapsed)
    except (TypeError, ValueError):
        pass


@router.post("/chat/completions")
async def chat_completions(request: Request):
    payload = await _read_json(request)
    state = request.app.state.app_state
    state.incr_requests()
    start = time.monotonic()
    result = await request.app.state.adapter.chat(payload)
    _record_usage(state, result, time.monotonic() - start)
    return _respond(result)


@router.post("/completions")
async def completions(request: Request):
    payload = await _read_json(request)
    state = request.app.state.app_state
    state.incr_requests()
    start = time.monotonic()
    result = await request.app.state.adapter.completions(payload)
    _record_usage(state, result, time.monotonic() - start)
    return _respond(result)


@router.post("/embeddings")
async def embeddings(request: Request):
    payload = await _read_json(request)
    return JSONResponse(await request.app.state.adapter.embeddings(payload))


@router.get("/models")
async def models(request: Request):
    return JSONResponse(await request.app.state.adapter.openai_models())
