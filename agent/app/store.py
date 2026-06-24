"""Vector store wrapper over Qdrant.

The collection is created lazily on first upsert using the embedding dimension we
actually see, so the agent works with any embedding model without being told its
size up front.
"""

from __future__ import annotations

import uuid

from qdrant_client import AsyncQdrantClient, models


class VectorStore:
    def __init__(self, url: str, collection: str, client: AsyncQdrantClient | None = None) -> None:
        self._collection = collection
        self._client = client or AsyncQdrantClient(url=url)
        self._ready = False

    async def aclose(self) -> None:
        await self._client.close()

    async def ensure(self, dim: int) -> None:
        """Create the collection if it doesn't exist yet."""
        if self._ready:
            return
        if not await self._client.collection_exists(self._collection):
            await self._client.create_collection(
                collection_name=self._collection,
                vectors_config=models.VectorParams(size=dim, distance=models.Distance.COSINE),
            )
        self._ready = True

    async def upsert(self, vectors: list[list[float]], texts: list[str], source: str) -> int:
        """Store chunks with their vectors; returns the number stored."""
        if not vectors:
            return 0
        await self.ensure(len(vectors[0]))
        points = [
            models.PointStruct(
                id=str(uuid.uuid4()),
                vector=vec,
                payload={"text": text, "source": source},
            )
            for vec, text in zip(vectors, texts, strict=True)
        ]
        await self._client.upsert(collection_name=self._collection, points=points)
        return len(points)

    async def search(self, vector: list[float], top_k: int) -> list[dict]:
        """Return the top_k most similar chunks as {text, source, score}.

        Degrades to an empty result (rather than erroring the whole chat) when
        the vector DB is unreachable or empty — so the agent still answers while
        Qdrant is starting, or when a stack runs without one."""
        try:
            if not await self._client.collection_exists(self._collection):
                return []
            hits = await self._client.query_points(
                collection_name=self._collection, query=vector, limit=top_k, with_payload=True
            )
        except Exception:
            return []
        out = []
        for h in hits.points:
            payload = h.payload or {}
            out.append(
                {
                    "text": payload.get("text", ""),
                    "source": payload.get("source", ""),
                    "score": h.score,
                }
            )
        return out

    async def count(self) -> int:
        try:
            if not await self._client.collection_exists(self._collection):
                return 0
            return (await self._client.count(self._collection)).count
        except Exception:
            return 0
