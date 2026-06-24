"""Model management: list / pull (streamed) / delete / set-default."""

from __future__ import annotations

from fastapi import APIRouter, Depends, Request
from fastapi.responses import StreamingResponse

from ..deps import require_auth
from ..models import ModelActionRequest, ModelList, PullRequest

router = APIRouter(prefix="/api", dependencies=[Depends(require_auth)])


@router.get("/models", response_model=ModelList)
async def list_models(request: Request) -> ModelList:
    adapter = request.app.state.adapter
    st = request.app.state.app_state
    installed = await adapter.list_models()
    return ModelList(default=st.default_model, models=installed)


@router.post("/models/pull")
async def pull(req: PullRequest, request: Request) -> StreamingResponse:
    """Stream NDJSON progress. The CLI renders this as a live bar; the web UI
    consumes the same stream via fetch()."""
    adapter = request.app.state.adapter

    async def gen():
        async for event in adapter.pull(req.model):
            yield event.model_dump_json() + "\n"

    return StreamingResponse(gen(), media_type="application/x-ndjson")


@router.post("/models/delete")
async def delete(req: ModelActionRequest, request: Request) -> dict:
    adapter = request.app.state.adapter
    st = request.app.state.app_state
    await adapter.delete(req.model)
    if st.default_model == req.model:
        st.default_model = ""
    return {"status": "ok"}


@router.post("/models/default")
async def set_default(req: ModelActionRequest, request: Request) -> dict:
    st = request.app.state.app_state
    st.default_model = req.model
    return {"status": "ok", "default": req.model}
