"""FastAPI surface for the RAG agent: health, document ingestion, and a
retrieval-augmented chat endpoint, plus a tiny self-contained web UI.

The heavy lifting lives in the embedder, the vector store, and the LangGraph
pipeline; this module is just wiring and HTTP."""

from __future__ import annotations

from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import Depends, FastAPI, File, Form, HTTPException, Request, UploadFile
from fastapi.responses import FileResponse, JSONResponse
from pydantic import BaseModel

from .config import Settings, load_settings
from .embed import Embedder
from .ingest import chunk_text, extract_text
from .rag import RagPipeline
from .store import VectorStore

STATIC_DIR = Path(__file__).parent / "static"


class ChatRequest(BaseModel):
    question: str
    top_k: int | None = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings: Settings = app.state.settings
    # Components may be pre-injected (tests); only build/own what's missing so we
    # don't open real network clients under test.
    owned = []
    if getattr(app.state, "embedder", None) is None:
        app.state.embedder = Embedder(settings)
        owned.append(app.state.embedder)
    if getattr(app.state, "store", None) is None:
        app.state.store = VectorStore(settings.qdrant_url, settings.collection)
        owned.append(app.state.store)
    if getattr(app.state, "pipeline", None) is None:
        app.state.pipeline = RagPipeline(settings, app.state.store, app.state.embedder)
    try:
        yield
    finally:
        for c in owned:
            await c.aclose()


def create_app(
    settings: Settings | None = None, *, embedder=None, store=None, pipeline=None
) -> FastAPI:
    settings = settings or load_settings()
    app = FastAPI(title="llmaker RAG agent", version="0.1.0", lifespan=lifespan)
    app.state.settings = settings
    app.state.embedder = embedder
    app.state.store = store
    app.state.pipeline = pipeline

    def require_auth(request: Request) -> None:
        key = settings.api_key
        if not key:
            return
        header = request.headers.get("authorization", "")
        if header != f"Bearer {key}":
            raise HTTPException(status_code=401, detail="missing or invalid bearer token")

    @app.get("/health")
    async def health() -> dict:
        return {"status": "ok"}

    @app.get("/api/stats", dependencies=[Depends(require_auth)])
    async def stats() -> dict:
        return {
            "collection": settings.collection,
            "chunks": await app.state.store.count(),
            "llm_model": settings.llm_model,
            "embeddings_model": settings.embeddings_model,
        }

    @app.post("/api/ingest", dependencies=[Depends(require_auth)])
    async def ingest(
        file: UploadFile | None = File(default=None),
        text: str | None = Form(default=None),
        source: str | None = Form(default=None),
    ) -> JSONResponse:
        if file is not None:
            raw = await file.read()
            content = extract_text(file.filename or "upload", raw)
            src = source or file.filename or "upload"
        elif text:
            content = text
            src = source or "inline"
        else:
            raise HTTPException(status_code=400, detail="provide a file or text to ingest")

        chunks = chunk_text(content, settings.chunk_size, settings.chunk_overlap)
        if not chunks:
            raise HTTPException(status_code=400, detail="no text found to ingest")
        vectors = await app.state.embedder.embed(chunks)
        stored = await app.state.store.upsert(vectors, chunks, src)
        return JSONResponse({"ingested": stored, "source": src})

    @app.post("/api/chat", dependencies=[Depends(require_auth)])
    async def chat(req: ChatRequest) -> dict:
        if not req.question.strip():
            raise HTTPException(status_code=400, detail="question is required")
        result = await app.state.pipeline.answer(req.question, req.top_k)
        sources = [
            {"source": c.get("source", ""), "score": c.get("score", 0.0)}
            for c in result.get("context", [])
        ]
        return {"answer": result.get("answer", ""), "sources": sources}

    @app.get("/")
    async def index() -> FileResponse:
        return FileResponse(STATIC_DIR / "index.html")

    return app


app = create_app()
