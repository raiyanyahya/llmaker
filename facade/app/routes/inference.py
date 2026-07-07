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
        payload = await request.json()
    except Exception:
        raise HTTPException(status_code=400, detail="invalid JSON body") from None
    # A valid-JSON non-object (e.g. [] or "x") would crash the downstream
    # payload.get(...) with a 500; reject it cleanly here instead.
    if not isinstance(payload, dict):
        raise HTTPException(status_code=400, detail="request body must be a JSON object")
    return payload


def _fill_default_model(payload: dict, app) -> None:
    """Inject the instance's default model when the client omits one.

    A self-hosted instance usually serves a single model, so requiring callers
    to name it is friction: any OpenAI client can point at the facade and leave
    ``model`` unset. An explicit (non-blank) model is always respected. Uses the
    live default (which ``/api/models/default`` can change) over the static one.
    """
    model = payload.get("model")
    if isinstance(model, str) and model.strip():
        return
    default = app.state.app_state.default_model or app.state.settings.default_model
    if default:
        payload["model"] = default


async def _tracked_stream(result, state):
    """Relay a streamed response, keeping it counted as in-flight until drained."""
    try:
        async for chunk in result:
            if chunk:
                yield chunk
    finally:
        state.leave_request()


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


async def _serve(request: Request, method_name: str):
    """Shared path for chat + text completions: parse, inject the default model,
    account requests/errors/in-flight, and stream or return JSON."""
    payload = await _read_json(request)
    app = request.app
    state = app.state.app_state
    _fill_default_model(payload, app)
    state.incr_requests()
    state.enter_request()
    start = time.monotonic()
    try:
        result = await getattr(app.state.adapter, method_name)(payload)
    except Exception:
        state.incr_errors()
        state.leave_request()
        raise
    # Streaming: the byte iterator stays in-flight until the client drains it.
    if hasattr(result, "__aiter__"):
        return StreamingResponse(_tracked_stream(result, state), media_type="text/event-stream")
    _record_usage(state, result, time.monotonic() - start)
    state.leave_request()
    return JSONResponse(result)


@router.post("/chat/completions")
async def chat_completions(request: Request):
    return await _serve(request, "chat")


@router.post("/completions")
async def completions(request: Request):
    return await _serve(request, "completions")


@router.post("/embeddings")
async def embeddings(request: Request):
    payload = await _read_json(request)
    _fill_default_model(payload, request.app)
    return JSONResponse(await request.app.state.adapter.embeddings(payload))


@router.get("/models")
async def models(request: Request):
    return JSONResponse(await request.app.state.adapter.openai_models())
