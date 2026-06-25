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

from .agent_graph import ToolAgent
from .config import Settings, load_settings
from .embed import Embedder
from .eval import Evaluator
from .ingest import chunk_text, extract_text
from .rag import RagPipeline
from .recommend import centroid
from .store import VectorStore
from .tools import build_tools

STATIC_DIR = Path(__file__).parent / "static"


class Message(BaseModel):
    role: str
    content: str


class ChatRequest(BaseModel):
    question: str
    top_k: int | None = None
    history: list[Message] | None = None  # prior turns, for multi-turn chat


class Item(BaseModel):
    id: str
    text: str
    metadata: dict | None = None


class ItemsRequest(BaseModel):
    items: list[Item]


class RecommendRequest(BaseModel):
    query: str | None = None  # recommend by free-text intent
    like: list[str] | None = None  # …or "more like these" item ids
    k: int = 5


class AgentRequest(BaseModel):
    question: str
    history: list[Message] | None = None
    max_steps: int | None = None


class EvalCase(BaseModel):
    question: str
    reference: str | None = None  # a gold answer, to also score correctness
    expected_sources: list[str] | None = None  # sources retrieval should surface


class EvalRequest(BaseModel):
    cases: list[EvalCase]
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
    if getattr(app.state, "item_store", None) is None:
        app.state.item_store = VectorStore(settings.qdrant_url, settings.items_collection)
        owned.append(app.state.item_store)
    if getattr(app.state, "pipeline", None) is None:
        app.state.pipeline = RagPipeline(settings, app.state.store, app.state.embedder)
    if getattr(app.state, "tool_agent", None) is None:
        tools = build_tools(settings, app.state.store, app.state.embedder)
        app.state.tool_agent = ToolAgent(settings, tools)
    if getattr(app.state, "evaluator", None) is None:
        app.state.evaluator = Evaluator(settings, app.state.pipeline)
    try:
        yield
    finally:
        for component in (app.state.pipeline, app.state.tool_agent, app.state.evaluator):
            flush = getattr(component, "flush", None)
            if callable(flush):
                flush()
        for c in owned:
            await c.aclose()


def create_app(
    settings: Settings | None = None,
    *,
    embedder=None,
    store=None,
    item_store=None,
    pipeline=None,
    tool_agent=None,
    evaluator=None,
) -> FastAPI:
    settings = settings or load_settings()
    app = FastAPI(title="llmaker RAG agent", version="0.1.0", lifespan=lifespan)
    app.state.settings = settings
    app.state.embedder = embedder
    app.state.store = store
    app.state.item_store = item_store
    app.state.pipeline = pipeline
    app.state.tool_agent = tool_agent
    app.state.evaluator = evaluator

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
        history = [m.model_dump() for m in (req.history or [])]
        result = await app.state.pipeline.answer(req.question, req.top_k, history=history)
        sources = [
            {"source": c.get("source", ""), "score": c.get("score", 0.0)}
            for c in result.get("context", [])
        ]
        return {"answer": result.get("answer", ""), "sources": sources}

    @app.post("/api/agent", dependencies=[Depends(require_auth)])
    async def agent(req: AgentRequest) -> dict:
        if not req.question.strip():
            raise HTTPException(status_code=400, detail="question is required")
        history = [m.model_dump() for m in (req.history or [])]
        result = await app.state.tool_agent.run(req.question, history, req.max_steps)
        # steps records each tool call the model made (name, args, result).
        return {"answer": result.get("answer", ""), "steps": result.get("steps", [])}

    @app.post("/api/eval", dependencies=[Depends(require_auth)])
    async def evaluate(req: EvalRequest) -> dict:
        if not req.cases:
            raise HTTPException(status_code=400, detail="provide at least one eval case")
        cases = [c.model_dump() for c in req.cases]
        # Per-case scores (groundedness, relevance, optional correctness /
        # context_recall) plus an aggregate summary.
        return await app.state.evaluator.evaluate(cases, req.top_k)

    @app.post("/api/items", dependencies=[Depends(require_auth)])
    async def add_items(req: ItemsRequest) -> dict:
        if not req.items:
            raise HTTPException(status_code=400, detail="no items provided")
        vectors = await app.state.embedder.embed([it.text for it in req.items])
        payload = [
            {"id": it.id, "text": it.text, "vector": v, "metadata": it.metadata or {}}
            for it, v in zip(req.items, vectors, strict=True)
        ]
        stored = await app.state.item_store.upsert_items(payload)
        return {"added": stored}

    @app.post("/api/recommend", dependencies=[Depends(require_auth)])
    async def recommend(req: RecommendRequest) -> dict:
        # Build the query vector from free text, or from the liked items'
        # centroid ("more like these").
        if req.query and req.query.strip():
            vector = await app.state.embedder.embed_one(req.query)
            exclude: list[str] = []
        elif req.like:
            vectors = await app.state.item_store.item_vectors(req.like)
            if not vectors:
                raise HTTPException(status_code=404, detail="none of the liked items were found")
            vector = centroid(vectors)
            exclude = req.like
        else:
            raise HTTPException(
                status_code=400, detail="provide a query or a list of liked item ids"
            )
        recs = await app.state.item_store.recommend(vector, req.k, exclude_ids=exclude)
        return {"recommendations": recs}

    @app.get("/")
    async def index() -> FileResponse:
        return FileResponse(STATIC_DIR / "index.html")

    return app


app = create_app()
