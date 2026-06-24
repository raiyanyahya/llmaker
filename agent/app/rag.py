"""The RAG pipeline, as a LangGraph state graph.

Four nodes make the flow explicit and easy to extend:

    rewrite → retrieve → rerank → generate

- rewrite:  fold a multi-turn conversation into one standalone search query
            (only calls the LLM when there's history to resolve).
- retrieve: embed the query and pull a wide candidate set from the vector store.
- rerank:   MMR down to top_k for relevant-but-diverse context.
- generate: answer using the context plus the conversation so far.

Each run is traced to Langfuse when configured: a ``rag-chat`` trace with a span
per node (and a generation for each LLM call). The trace handle rides through the
graph state; tracing is entirely best-effort (see tracing.py).
"""

from __future__ import annotations

from typing import Any, TypedDict

from langgraph.graph import END, StateGraph
from openai import AsyncOpenAI

from .config import Settings
from .embed import Embedder
from .rerank import mmr
from .store import VectorStore
from .tracing import Tracer, safe_end, safe_generation, safe_span, safe_update

SYSTEM_PROMPT = (
    "You are a helpful assistant. Answer the user's question using ONLY the "
    "context below. If the context doesn't contain the answer, say you don't "
    "know based on the provided documents — do not make things up.\n\n"
    "Context:\n{context}"
)

REWRITE_PROMPT = (
    "Given the conversation so far and a follow-up question, rewrite the "
    "follow-up as a single, standalone search query that captures what to look "
    "up. Reply with ONLY the query, no preamble."
)


class RagState(TypedDict, total=False):
    question: str
    history: list[dict]
    top_k: int
    search_query: str
    query_vec: list[float]
    candidates: list[dict]
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
        g.add_node("rewrite", self._rewrite)
        g.add_node("retrieve", self._retrieve)
        g.add_node("rerank", self._rerank)
        g.add_node("generate", self._generate)
        g.set_entry_point("rewrite")
        g.add_edge("rewrite", "retrieve")
        g.add_edge("retrieve", "rerank")
        g.add_edge("rerank", "generate")
        g.add_edge("generate", END)
        return g.compile()

    async def _rewrite(self, state: RagState) -> RagState:
        question = state["question"]
        history = state.get("history") or []
        # No history (or rewriting disabled) → search the question as-is, no call.
        if not history or not self._settings.rewrite_queries:
            return {"search_query": question}
        span = safe_span(state.get("trace"), name="rewrite", input={"question": question})
        convo = _format_history(history)
        messages = [
            {"role": "system", "content": REWRITE_PROMPT},
            {"role": "user", "content": f"Conversation:\n{convo}\n\nFollow-up: {question}"},
        ]
        try:
            resp = await self._llm.chat.completions.create(
                model=self._settings.llm_model, messages=messages
            )
            query = (resp.choices[0].message.content or question).strip() or question
        except Exception:
            query = question
        safe_end(span, output=query)
        return {"search_query": query}

    async def _retrieve(self, state: RagState) -> RagState:
        k = state.get("top_k") or self._settings.top_k
        query = state.get("search_query") or state["question"]
        fetch = max(k * self._settings.fetch_multiplier, k)
        span = safe_span(state.get("trace"), name="retrieve", input=query)
        vec = await self._embedder.embed_one(query)
        candidates = await self._store.search(vec, fetch, with_vectors=True) if vec else []
        safe_end(span, output={"fetched": len(candidates)})
        return {"candidates": candidates, "search_query": query, "query_vec": vec}

    async def _rerank(self, state: RagState) -> RagState:
        k = state.get("top_k") or self._settings.top_k
        candidates = state.get("candidates", [])
        query_vec = state.get("query_vec", [])
        span = safe_span(state.get("trace"), name="rerank", input={"candidates": len(candidates)})
        context = mmr(query_vec, candidates, k, self._settings.mmr_lambda)
        # Drop the bulky vectors before they travel further / into the prompt.
        context = [{kk: v for kk, v in c.items() if kk != "vector"} for c in context]
        safe_end(span, output={"kept": len(context), "scores": [c.get("score") for c in context]})
        return {"context": context}

    async def _generate(self, state: RagState) -> RagState:
        context = state.get("context", [])
        joined = "\n\n---\n\n".join(c["text"] for c in context) or "(no documents found)"
        messages = [{"role": "system", "content": SYSTEM_PROMPT.format(context=joined)}]
        messages += state.get("history") or []
        messages.append({"role": "user", "content": state["question"]})
        gen = safe_generation(
            state.get("trace"), name="generate", model=self._settings.llm_model, input=messages
        )
        resp = await self._llm.chat.completions.create(
            model=self._settings.llm_model, messages=messages
        )
        answer = resp.choices[0].message.content or ""
        safe_end(gen, output=answer, usage=_usage(resp))
        return {"answer": answer}

    async def answer(
        self, question: str, top_k: int | None = None, history: list[dict] | None = None
    ) -> RagState:
        """Run the graph and return the final state (answer + context/sources)."""
        trace = self._tracer.trace(
            name="rag-chat", input={"question": question, "turns": len(history or [])}
        )
        result = await self._graph.ainvoke(
            {
                "question": question,
                "history": history or [],
                "top_k": top_k or self._settings.top_k,
                "trace": trace,
            }
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


def _format_history(history: list[dict]) -> str:
    return "\n".join(f"{m.get('role', 'user')}: {m.get('content', '')}" for m in history)


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
