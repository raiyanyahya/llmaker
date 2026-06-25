"""In-memory fakes so the agent's routes and pipeline can be tested without a
real LLM, embeddings server, or Qdrant."""

from __future__ import annotations


class FakeEmbedder:
    """Deterministic toy embeddings: a tiny bag-of-words vector over a fixed
    vocabulary, enough for cosine similarity to behave sensibly in tests."""

    VOCAB = ["llmaker", "rag", "vector", "cat", "dog", "fast", "local", "model"]

    def __init__(self) -> None:
        self.calls = 0

    def _vec(self, text: str) -> list[float]:
        t = text.lower()
        return [float(t.count(w)) + 0.1 for w in self.VOCAB]

    async def embed(self, texts: list[str]) -> list[list[float]]:
        self.calls += 1
        return [self._vec(t) for t in texts]

    async def embed_one(self, text: str) -> list[float]:
        return (await self.embed([text]))[0]

    async def aclose(self) -> None:
        pass


class FakeStore:
    def __init__(self) -> None:
        self.docs: list[dict] = []

    async def upsert(self, vectors, texts, source) -> int:
        for vec, text in zip(vectors, texts, strict=True):
            self.docs.append({"vector": vec, "text": text, "source": source})
        return len(texts)

    async def search(self, vector, top_k, with_vectors=False) -> list[dict]:
        def dot(a, b):
            return sum(x * y for x, y in zip(a, b, strict=False))

        ranked = sorted(self.docs, key=lambda d: dot(d["vector"], vector), reverse=True)
        out = []
        for d in ranked[:top_k]:
            item = {"text": d["text"], "source": d["source"], "score": 0.9}
            if with_vectors:
                item["vector"] = d["vector"]
            out.append(item)
        return out

    async def count(self) -> int:
        return len(self.docs)

    async def aclose(self) -> None:
        pass


class FakeItemStore:
    """In-memory recommendation store: one vector per item id."""

    def __init__(self) -> None:
        self.items: dict[str, dict] = {}

    async def upsert_items(self, items) -> int:
        for it in items:
            self.items[str(it["id"])] = {
                "vector": it["vector"],
                "text": it.get("text", ""),
                "metadata": it.get("metadata") or {},
            }
        return len(items)

    async def item_vectors(self, item_ids) -> list[list[float]]:
        return [self.items[i]["vector"] for i in item_ids if i in self.items]

    async def recommend(self, vector, k, exclude_ids=None) -> list[dict]:
        def dot(a, b):
            return sum(x * y for x, y in zip(a, b, strict=False))

        exclude = set(exclude_ids or [])
        ranked = sorted(
            ((i, d) for i, d in self.items.items() if i not in exclude),
            key=lambda kv: dot(kv[1]["vector"], vector),
            reverse=True,
        )
        return [
            {"id": i, "text": d["text"], "metadata": d["metadata"], "score": 0.9}
            for i, d in ranked[:k]
        ]

    async def aclose(self) -> None:
        pass


class FakePipeline:
    """Stands in for the LangGraph pipeline; echoes retrieved context."""

    def __init__(self, store: FakeStore, embedder: FakeEmbedder) -> None:
        self._store = store
        self._embedder = embedder

    async def answer(
        self, question: str, top_k: int | None = None, history: list | None = None
    ) -> dict:
        vec = await self._embedder.embed_one(question)
        context = await self._store.search(vec, top_k or 4)
        snippet = context[0]["text"] if context else "no documents"
        return {"answer": f"Based on the docs: {snippet}", "context": context}


class FakeToolAgent:
    """Stands in for the tool-calling agent; echoes a canned tool run."""

    async def run(self, question, history=None, max_steps=None) -> dict:
        return {
            "answer": f"answer to: {question}",
            "steps": [{"tool": "calculator", "args": {"expression": "2+2"}, "result": "4"}],
        }


class _FakeObservation:
    def __init__(self, kind: str, kwargs: dict, sink: list) -> None:
        self.kind = kind
        self.input = kwargs
        self.ended = None
        self._sink = sink
        sink.append(self)

    def end(self, **kwargs) -> None:
        self.ended = kwargs


class FakeTrace:
    def __init__(self, kwargs: dict) -> None:
        self.input = kwargs
        self.observations: list[_FakeObservation] = []
        self.updated = None

    def span(self, **kwargs) -> _FakeObservation:
        return _FakeObservation("span", kwargs, self.observations)

    def generation(self, **kwargs) -> _FakeObservation:
        return _FakeObservation("generation", kwargs, self.observations)

    def update(self, **kwargs) -> None:
        self.updated = kwargs


class FakeLangfuse:
    """Records the trace/span/generation calls the pipeline makes."""

    def __init__(self) -> None:
        self.traces: list[FakeTrace] = []
        self.flushed = 0

    def trace(self, **kwargs) -> FakeTrace:
        t = FakeTrace(kwargs)
        self.traces.append(t)
        return t

    def flush(self) -> None:
        self.flushed += 1
