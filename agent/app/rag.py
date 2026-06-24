"""The RAG pipeline, as a LangGraph state graph.

Two nodes — ``retrieve`` then ``generate`` — make the flow explicit and easy to
extend (add reranking, query rewriting, tool calls, …) without touching the
HTTP layer. State flows question → context → answer.
"""

from __future__ import annotations

from typing import TypedDict

from langgraph.graph import END, StateGraph
from openai import AsyncOpenAI

from .config import Settings
from .embed import Embedder
from .store import VectorStore

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


class RagPipeline:
    def __init__(
        self,
        settings: Settings,
        store: VectorStore,
        embedder: Embedder,
        llm: AsyncOpenAI | None = None,
    ) -> None:
        self._settings = settings
        self._store = store
        self._embedder = embedder
        self._llm = llm or AsyncOpenAI(base_url=settings.llm_base_url, api_key=settings.llm_api_key)
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
        vec = await self._embedder.embed_one(state["question"])
        context = await self._store.search(vec, k) if vec else []
        return {"context": context}

    async def _generate(self, state: RagState) -> RagState:
        context = state.get("context", [])
        joined = "\n\n---\n\n".join(c["text"] for c in context) or "(no documents found)"
        resp = await self._llm.chat.completions.create(
            model=self._settings.llm_model,
            messages=[
                {"role": "system", "content": SYSTEM_PROMPT.format(context=joined)},
                {"role": "user", "content": state["question"]},
            ],
        )
        return {"answer": resp.choices[0].message.content or ""}

    async def answer(self, question: str, top_k: int | None = None) -> RagState:
        """Run the graph and return the final state (answer + context/sources)."""
        result = await self._graph.ainvoke(
            {"question": question, "top_k": top_k or self._settings.top_k}
        )
        return result
