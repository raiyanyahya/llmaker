"""Liveness/readiness — intentionally unauthenticated so container health checks
and `llmaker up` can poll it before any API key is presented."""

from __future__ import annotations

from fastapi import APIRouter, Request, Response

from ..models import HealthResponse

router = APIRouter()


@router.get("/api/health", response_model=HealthResponse)
async def health(request: Request, response: Response) -> HealthResponse:
    adapter = request.app.state.adapter
    try:
        ready = await adapter.health()
    except Exception:
        ready = False
    if not ready:
        response.status_code = 503
        return HealthResponse(status="starting", ready=False)
    return HealthResponse(status="ok", ready=True)
