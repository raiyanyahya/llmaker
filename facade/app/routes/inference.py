"""OpenAI-compatible inference. These endpoints are a thin shim over the backend
adapter; streaming chat/completions come back as Server-Sent Events."""

from __future__ import annotations

from fastapi import APIRouter, Depends, Request
from fastapi.responses import JSONResponse, StreamingResponse

from ..deps import require_auth

router = APIRouter(prefix="/v1", dependencies=[Depends(require_auth)])


def _respond(result):
    # Adapters return an async byte iterator for streamed responses, else a dict.
    if hasattr(result, "__aiter__"):
        return StreamingResponse(result, media_type="text/event-stream")
    return JSONResponse(result)


@router.post("/chat/completions")
async def chat_completions(request: Request):
    payload = await request.json()
    request.app.state.app_state.incr_requests()
    return _respond(await request.app.state.adapter.chat(payload))


@router.post("/completions")
async def completions(request: Request):
    payload = await request.json()
    request.app.state.app_state.incr_requests()
    return _respond(await request.app.state.adapter.completions(payload))


@router.post("/embeddings")
async def embeddings(request: Request):
    payload = await request.json()
    return JSONResponse(await request.app.state.adapter.embeddings(payload))


@router.get("/models")
async def models(request: Request):
    return JSONResponse(await request.app.state.adapter.openai_models())
