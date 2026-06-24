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

    async def search(self, vector, top_k) -> list[dict]:
        def dot(a, b):
            return sum(x * y for x, y in zip(a, b, strict=False))

        ranked = sorted(self.docs, key=lambda d: dot(d["vector"], vector), reverse=True)
        return [{"text": d["text"], "source": d["source"], "score": 0.9} for d in ranked[:top_k]]

    async def count(self) -> int:
        return len(self.docs)

    async def aclose(self) -> None:
        pass


class FakePipeline:
    """Stands in for the LangGraph pipeline; echoes retrieved context."""

    def __init__(self, store: FakeStore, embedder: FakeEmbedder) -> None:
        self._store = store
        self._embedder = embedder

    async def answer(self, question: str, top_k: int | None = None) -> dict:
        vec = await self._embedder.embed_one(question)
        context = await self._store.search(vec, top_k or 4)
        snippet = context[0]["text"] if context else "no documents"
        return {"answer": f"Based on the docs: {snippet}", "context": context}


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
