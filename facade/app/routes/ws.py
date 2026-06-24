"""WebSocket endpoints for the browser UI's live gauges. The CLI doesn't use
these — it polls /api/status — so this exists purely to give the web UI smooth,
push-based updates."""

from __future__ import annotations

import asyncio

from fastapi import APIRouter, WebSocket, WebSocketDisconnect

from ..deps import token_matches
from .status import build_status

router = APIRouter()


@router.websocket("/ws/status")
async def ws_status(ws: WebSocket) -> None:
    settings = ws.app.state.settings
    if settings.api_key and not token_matches(ws.query_params.get("api_key", ""), settings.api_key):
        await ws.close(code=1008)  # policy violation
        return

    await ws.accept()
    try:
        while True:
            snapshot = await build_status(ws.app)
            await ws.send_json(snapshot.model_dump())
            await asyncio.sleep(1.0)
    except WebSocketDisconnect:
        return
    except Exception:
        try:
            await ws.close()
        except Exception:
            pass
