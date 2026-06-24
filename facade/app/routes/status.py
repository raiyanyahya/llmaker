"""Aggregate status — the single endpoint that powers both the web UI and
`llmaker top`/`llmaker status`."""

from __future__ import annotations

from fastapi import APIRouter, Depends, Request

from ..deps import require_auth
from ..metrics import system_metrics
from ..models import InstanceInfo, MetricsInfo, ModelsInfo, StatusResponse

router = APIRouter(prefix="/api", dependencies=[Depends(require_auth)])


async def build_status(app) -> StatusResponse:
    """Assemble a status snapshot. Backend hiccups degrade to empty model lists
    rather than failing the whole endpoint, so the dashboard stays responsive
    while a model loads or the engine warms up."""
    settings = app.state.settings
    adapter = app.state.adapter
    st = app.state.app_state

    try:
        installed = await adapter.list_models()
    except Exception:
        installed = []
    try:
        running = await adapter.running_models()
    except Exception:
        running = []

    return StatusResponse(
        instance=InstanceInfo(
            name=settings.name,
            backend=settings.backend,
            version=settings.version,
            uptime_seconds=st.uptime_seconds(),
            default_model=st.default_model,
        ),
        system=system_metrics(),
        models=ModelsInfo(default=st.default_model, running=running, installed=installed),
        metrics=MetricsInfo(
            tokens_per_second=st.tokens_per_second,
            requests_total=st.requests,
        ),
    )


@router.get("/status", response_model=StatusResponse)
async def status(request: Request) -> StatusResponse:
    return await build_status(request.app)
