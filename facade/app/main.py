"""FastAPI application factory and the module-level ``app`` for uvicorn."""

from __future__ import annotations

from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles

from . import metrics
from .adapters import Adapter, build_adapter
from .config import Settings
from .routes import health, inference, status, ws
from .routes import metrics as metrics_routes
from .routes import models as models_routes
from .state import AppState

STATIC_DIR = Path(__file__).parent / "static"


def create_app(settings: Settings | None = None, adapter: Adapter | None = None) -> FastAPI:
    """Build the facade app. Tests pass a fake adapter; production reads env."""
    settings = settings or Settings.from_env()

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        metrics.prime()
        yield
        if app.state.adapter is not None:
            await app.state.adapter.aclose()

    app = FastAPI(
        title="llmaker facade",
        version=settings.version,
        summary="Normalized OpenAI-compatible control plane for a self-hosted LLM.",
        lifespan=lifespan,
    )

    app.state.settings = settings
    app.state.app_state = AppState(default_model=settings.default_model)
    app.state.adapter = adapter if adapter is not None else build_adapter(settings)

    # Only enable cross-origin access when origins are explicitly configured.
    # With none set (the default), the browser blocks cross-site calls, so a
    # random page the user visits can't drive the loopback facade (e.g. pull or
    # delete models). The bundled web UI is same-origin and unaffected.
    if settings.cors_origins:
        app.add_middleware(
            CORSMiddleware,
            allow_origins=settings.cors_origins,
            allow_methods=["*"],
            allow_headers=["*"],
            allow_credentials=False,
        )

    app.include_router(health.router)
    app.include_router(metrics_routes.router)
    app.include_router(status.router)
    app.include_router(models_routes.router)
    app.include_router(inference.router)
    app.include_router(ws.router)

    _mount_ui(app)
    return app


def _mount_ui(app: FastAPI) -> None:
    if not STATIC_DIR.exists():
        return

    @app.get("/", include_in_schema=False)
    async def index() -> FileResponse:
        return FileResponse(STATIC_DIR / "index.html")

    app.mount("/static", StaticFiles(directory=STATIC_DIR), name="static")


# Module-level app so `uvicorn app.main:app` works inside the container.
app = create_app()
