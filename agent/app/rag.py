"""The RAG pipeline, as a LangGraph state graph.

Two nodes — ``retrieve`` then ``generate`` — make the flow explicit and easy to
extend (add reranking, query rewriting, tool calls, …) without touching the
HTTP layer. State flows question → context → answer.

Each run is traced to Langfuse when configured: a top-level ``rag-chat`` trace
with a ``retrieve`` span (hits + scores) and a ``generate`` generation (model +
token usage). The trace handle rides through the graph state so the nodes can
attach their own spans; tracing is entirely best-effort (see tracing.py).
"""

from __future__ import annotations

from typing import Any, TypedDict

from langgraph.graph import END, StateGraph
from openai import AsyncOpenAI

from .config import Settings
from .embed import Embedder
from .store import VectorStore
from .tracing import Tracer, safe_end, safe_generation, safe_span, safe_update

SYSTEM_PROMPT = (
    "You are a helpful assistant. Answer the user's question using ONLY the "
    "context below. If the context doesn't contain the answer, say you don't "
    "know based on the provided documents — do not make things up.\n\n"
    "Context:\n{context}"
)


class RagState(TypedDict, total=False):
    question: str
    top_k: int
    context: list[dict]
    answer: str
    trace: Any  # Langfuse trace handle (or None); carried through the graph


class RagPipeline:
    def __init__(
        self,
        settings: Settings,
        store: VectorStore,
        embedder: Embedder,
        llm: AsyncOpenAI | None = None,
        tracer: Tracer | None = None,
    ) -> None:
        self._settings = settings
        self._store = store
        self._embedder = embedder
        self._llm = llm or AsyncOpenAI(base_url=settings.llm_base_url, api_key=settings.llm_api_key)
        self._tracer = tracer or Tracer(settings)
        self._graph = self._build()

    def _build(self):
        g = StateGraph(RagState)
        g.add_node("retrieve", self._retrieve)
        g.add_node("generate", self._generate)
        g.set_entry_point("retrieve")
        g.add_edge("retrieve", "generate")
        g.add_edge("generate", END)
        return g.compile()

    async def _retrieve(self, state: RagState) -> RagState:
        k = state.get("top_k") or self._settings.top_k
        span = safe_span(state.get("trace"), name="retrieve", input=state["question"])
        vec = await self._embedder.embed_one(state["question"])
        context = await self._store.search(vec, k) if vec else []
        safe_end(
            span,
            output={"hits": len(context), "scores": [c.get("score") for c in context]},
        )
        return {"context": context}

    async def _generate(self, state: RagState) -> RagState:
        context = state.get("context", [])
        joined = "\n\n---\n\n".join(c["text"] for c in context) or "(no documents found)"
        messages = [
            {"role": "system", "content": SYSTEM_PROMPT.format(context=joined)},
            {"role": "user", "content": state["question"]},
        ]
        gen = safe_generation(
            state.get("trace"), name="generate", model=self._settings.llm_model, input=messages
        )
        resp = await self._llm.chat.completions.create(
            model=self._settings.llm_model, messages=messages
        )
        answer = resp.choices[0].message.content or ""
        safe_end(gen, output=answer, usage=_usage(resp))
        return {"answer": answer}

    async def answer(self, question: str, top_k: int | None = None) -> RagState:
        """Run the graph and return the final state (answer + context/sources)."""
        trace = self._tracer.trace(name="rag-chat", input={"question": question})
        result = await self._graph.ainvoke(
            {"question": question, "top_k": top_k or self._settings.top_k, "trace": trace}
        )
        safe_update(
            trace,
            output={
                "answer": result.get("answer", ""),
                "sources": [c.get("source") for c in result.get("context", [])],
            },
        )
        return result

    def flush(self) -> None:
        """Flush any buffered traces (called on shutdown)."""
        self._tracer.flush()


def _usage(resp) -> dict | None:
    """Pull token usage out of an OpenAI response if present (for Langfuse)."""
    u = getattr(resp, "usage", None)
    if not u:
        return None
    return {
        "input": getattr(u, "prompt_tokens", None),
        "output": getattr(u, "completion_tokens", None),
        "total": getattr(u, "total_tokens", None),
    }
