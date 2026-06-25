"""FastAPI surface for the RAG agent: health, document ingestion, and a
retrieval-augmented chat endpoint, plus a tiny self-contained web UI.

The heavy lifting lives in the embedder, the vector store, and the LangGraph
pipeline; this module is just wiring and HTTP."""

from __future__ import annotations

import hmac
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import Depends, FastAPI, File, Form, HTTPException, Request, UploadFile
from fastapi.responses import FileResponse, JSONResponse
from pydantic import BaseModel

from .agent_graph import ToolAgent
from .config import Settings, load_settings
from .embed import Embedder
from .eval import Evaluator
from .extract import Extractor
from .ingest import chunk_text, extract_text
from .memory import build_memory
from .rag import RagPipeline
from .recommend import centroid
from .store import VectorStore
from .summarize import Summarizer
from .tools import build_tools
from .transcribe import Transcriber

STATIC_DIR = Path(__file__).parent / "static"


async def _read_capped(upload: UploadFile, max_mb: int) -> bytes:
    """Read an upload, rejecting anything over the limit before it's all in memory.
    Reading max+1 bytes bounds memory regardless of the client-declared size."""
    limit = max_mb * 1024 * 1024
    raw = await upload.read(limit + 1)
    if len(raw) > limit:
        raise HTTPException(status_code=413, detail=f"upload exceeds the {max_mb} MB limit")
    return raw


class Message(BaseModel):
    role: str
    content: str


class ChatRequest(BaseModel):
    question: str
    top_k: int | None = None
    history: list[Message] | None = None  # prior turns, for multi-turn chat
    session_id: str | None = None  # when set + memory enabled, persist/resume history


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
    session_id: str | None = None


class SummarizeRequest(BaseModel):
    text: str
    instructions: str | None = None  # e.g. "as 3 bullet points" / "for an executive"
    max_words: int | None = None


class ExtractRequest(BaseModel):
    text: str
    fields: dict[str, str]  # field name → what to pull out


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
    if getattr(app.state, "summarizer", None) is None:
        app.state.summarizer = Summarizer(settings)
    if getattr(app.state, "extractor", None) is None:
        app.state.extractor = Extractor(settings)
    if getattr(app.state, "transcriber", None) is None and settings.whisper_url:
        app.state.transcriber = Transcriber(settings)
    if getattr(app.state, "memory", None) is None and settings.redis_url:
        app.state.memory = build_memory(settings)
        owned.append(app.state.memory)
    try:
        yield
    finally:
        for component in (
            app.state.pipeline,
            app.state.tool_agent,
            app.state.evaluator,
            app.state.summarizer,
            app.state.extractor,
        ):
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
    summarizer=None,
    extractor=None,
    transcriber=None,
    memory=None,
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
    app.state.summarizer = summarizer
    app.state.extractor = extractor
    app.state.transcriber = transcriber
    app.state.memory = memory

    def require_auth(request: Request) -> None:
        key = settings.api_key
        if not key:
            return
        header = request.headers.get("authorization", "")
        # Constant-time compare so a wrong token can't be inferred by timing.
        if not hmac.compare_digest(header.encode("utf-8"), f"Bearer {key}".encode()):
            raise HTTPException(status_code=401, detail="missing or invalid bearer token")

    async def resume(session_id: str | None, history) -> list[dict]:
        """Prepend any stored history for this session to the request's history."""
        msgs = [m.model_dump() for m in (history or [])]
        mem = app.state.memory
        if mem and session_id:
            return await mem.load(session_id) + msgs
        return msgs

    async def remember(session_id: str | None, question: str, answer: str) -> None:
        """Persist the latest turn (no-op unless memory is enabled and a session given)."""
        mem = app.state.memory
        if mem and session_id:
            await mem.append(
                session_id,
                [{"role": "user", "content": question}, {"role": "assistant", "content": answer}],
            )

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
            raw = await _read_capped(file, settings.max_upload_mb)
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
        history = await resume(req.session_id, req.history)
        result = await app.state.pipeline.answer(req.question, req.top_k, history=history)
        answer = result.get("answer", "")
        await remember(req.session_id, req.question, answer)
        sources = [
            {"source": c.get("source", ""), "score": c.get("score", 0.0)}
            for c in result.get("context", [])
        ]
        return {"answer": answer, "sources": sources}

    @app.post("/api/agent", dependencies=[Depends(require_auth)])
    async def agent(req: AgentRequest) -> dict:
        if not req.question.strip():
            raise HTTPException(status_code=400, detail="question is required")
        history = await resume(req.session_id, req.history)
        result = await app.state.tool_agent.run(req.question, history, req.max_steps)
        answer = result.get("answer", "")
        await remember(req.session_id, req.question, answer)
        # steps records each tool call the model made (name, args, result).
        return {"answer": answer, "steps": result.get("steps", [])}

    @app.post("/api/eval", dependencies=[Depends(require_auth)])
    async def evaluate(req: EvalRequest) -> dict:
        if not req.cases:
            raise HTTPException(status_code=400, detail="provide at least one eval case")
        cases = [c.model_dump() for c in req.cases]
        # Per-case scores (groundedness, relevance, optional correctness /
        # context_recall) plus an aggregate summary.
        return await app.state.evaluator.evaluate(cases, req.top_k)

    @app.post("/api/summarize", dependencies=[Depends(require_auth)])
    async def summarize(req: SummarizeRequest) -> dict:
        if not req.text.strip():
            raise HTTPException(status_code=400, detail="text is required")
        return await app.state.summarizer.summarize(req.text, req.instructions, req.max_words)

    @app.post("/api/extract", dependencies=[Depends(require_auth)])
    async def extract(req: ExtractRequest) -> dict:
        if not req.text.strip():
            raise HTTPException(status_code=400, detail="text is required")
        if not req.fields:
            raise HTTPException(status_code=400, detail="provide at least one field to extract")
        return await app.state.extractor.extract(req.text, req.fields)

    @app.post("/api/transcribe", dependencies=[Depends(require_auth)])
    async def transcribe(file: UploadFile = File(...)) -> dict:
        if app.state.transcriber is None:
            raise HTTPException(
                status_code=400, detail="transcription not configured (set WHISPER_URL)"
            )
        raw = await _read_capped(file, settings.max_upload_mb)
        return await app.state.transcriber.transcribe(
            file.filename or "audio", raw, file.content_type
        )

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
