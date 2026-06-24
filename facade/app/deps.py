"""Shared FastAPI dependencies."""

from __future__ import annotations

from fastapi import HTTPException, Request


def _extract_token(request: Request) -> str:
    auth = request.headers.get("Authorization", "")
    if auth.lower().startswith("bearer "):
        return auth[7:].strip()
    # Browsers can't set headers on WebSocket/EventSource, so accept a query param.
    return request.query_params.get("api_key", "")


async def require_auth(request: Request) -> None:
    """Enforce the optional bearer token. A no-op when API_KEY is unset, so the
    default local experience stays friction-free; secure-by-config when set."""
    settings = request.app.state.settings
    if not settings.api_key:
        return
    if _extract_token(request) != settings.api_key:
        raise HTTPException(status_code=401, detail="invalid or missing API key")
