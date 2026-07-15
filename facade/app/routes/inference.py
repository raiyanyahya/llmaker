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
    ``model`` unset (or null). An explicit (non-blank) model is always respected.
    """
    model = payload.get("model")
    if isinstance(model, str) and model.strip():
        return
    if model is not None and not isinstance(model, str):
        raise HTTPException(status_code=400, detail="model must be a string")
    # The live default is the single source of truth: seeded from settings at
    # startup, changed by /api/models/default, and deliberately cleared when the
    # default model is deleted — falling back to settings here would resurrect
    # a deleted model.
    default = app.state.app_state.default_model
    if default:
        payload["model"] = default


async def _tracked_stream(result, state):
    """Relay a streamed response, keeping it counted as in-flight until drained.

    The real adapters build their streams lazily (an async generator that only
    contacts the backend on first iteration), so this is also where streaming
    failures surface — count them, since _serve's except can never see them.
    """
    try:
        async for chunk in result:
            if chunk:
                yield chunk
    except Exception:
        state.incr_errors()
        raise
    finally:
        state.leave_request()


def _record_usage(state, result, elapsed: float) -> None:
    """Feed token throughput from a non-streaming result into the rolling
    tokens/sec metric. Streaming responses are async iterators and carry no
    usage here, so they're skipped. Only ``completion_tokens`` counts —
    ``total_tokens`` includes prompt tokens and would inflate the completion
    counter. Never raises: metrics must not fail a request."""
    if not isinstance(result, dict):
        return
    try:
        usage = result.get("usage") or {}
        state.record_completion(int(usage.get("completion_tokens") or 0), elapsed)
    except (AttributeError, TypeError, ValueError):
        pass


async def _serve(request: Request, method):
    """Shared path for every /v1 inference endpoint: parse, inject the default
    model, account requests/errors/in-flight, and stream or return JSON.

    ``method`` selects the adapter coroutine (e.g. ``lambda a: a.chat``) — a
    real attribute reference, so a rename shows up in editors and tests, unlike
    a method-name string."""
    payload = await _read_json(request)
    app = request.app
    state = app.state.app_state
    _fill_default_model(payload, app)
    state.incr_requests()
    state.enter_request()
    start = time.monotonic()
    streaming = False
    try:
        result = await method(app.state.adapter)(payload)
        if hasattr(result, "__aiter__"):
            # Streaming: _tracked_stream owns the in-flight count from here on.
            response = StreamingResponse(
                _tracked_stream(result, state), media_type="text/event-stream"
            )
            streaming = True
            return response
        _record_usage(state, result, time.monotonic() - start)
        return JSONResponse(result)
    except Exception:
        state.incr_errors()
        raise
    finally:
        # The finally (not the except) releases in-flight, so even cancellation
        # (client disconnect mid-handler) can't leak the gauge.
        if not streaming:
            state.leave_request()


@router.post("/chat/completions")
async def chat_completions(request: Request):
    return await _serve(request, lambda a: a.chat)


@router.post("/completions")
async def completions(request: Request):
    return await _serve(request, lambda a: a.completions)


@router.post("/embeddings")
async def embeddings(request: Request):
    return await _serve(request, lambda a: a.embeddings)


@router.get("/models")
async def models(request: Request):
    return JSONResponse(await request.app.state.adapter.openai_models())
