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

    async def search(
        self, vector: list[float], top_k: int, with_vectors: bool = False
    ) -> list[dict]:
        """Return the top_k most similar chunks as {text, source, score} (and the
        stored ``vector`` when with_vectors, for downstream reranking).

        Degrades to an empty result (rather than erroring the whole chat) when
        the vector DB is unreachable or empty — so the agent still answers while
        Qdrant is starting, or when a stack runs without one."""
        try:
            if not await self._client.collection_exists(self._collection):
                return []
            hits = await self._client.query_points(
                collection_name=self._collection,
                query=vector,
                limit=top_k,
                with_payload=True,
                with_vectors=with_vectors,
            )
        except Exception:
            return []
        out = []
        for h in hits.points:
            payload = h.payload or {}
            item = {
                "text": payload.get("text", ""),
                "source": payload.get("source", ""),
                "score": h.score,
            }
            if with_vectors and h.vector is not None:
                item["vector"] = h.vector
            out.append(item)
        return out

    async def count(self) -> int:
        try:
            if not await self._client.collection_exists(self._collection):
                return 0
            return (await self._client.count(self._collection)).count
        except Exception:
            return 0

    # --- recommendation: one vector per item, addressable by caller-chosen id ---

    async def upsert_items(self, items: list[dict]) -> int:
        """Store items for recommendation: one point per item, keyed by a
        deterministic id derived from the caller's item id (so the same item id
        updates in place and can be looked up later). Each item is
        {id, text, vector, metadata?}."""
        if not items:
            return 0
        await self.ensure(len(items[0]["vector"]))
        points = [
            models.PointStruct(
                id=_point_id(it["id"]),
                vector=it["vector"],
                payload={
                    "item_id": str(it["id"]),
                    "text": it.get("text", ""),
                    "metadata": it.get("metadata") or {},
                },
            )
            for it in items
        ]
        await self._client.upsert(collection_name=self._collection, points=points)
        return len(points)

    async def item_vectors(self, item_ids: list[str]) -> list[list[float]]:
        """Return the stored vectors for the given item ids (missing ids skipped)."""
        if not item_ids:
            return []
        try:
            recs = await self._client.retrieve(
                collection_name=self._collection,
                ids=[_point_id(i) for i in item_ids],
                with_vectors=True,
            )
        except Exception:
            return []
        return [r.vector for r in recs if r.vector is not None]

    async def recommend(
        self, vector: list[float], k: int, exclude_ids: list[str] | None = None
    ) -> list[dict]:
        """Return up to k items most similar to a vector, dropping any whose
        item_id is in exclude_ids (e.g. the seeds in a 'more like this' query)."""
        exclude = set(exclude_ids or [])
        try:
            if not await self._client.collection_exists(self._collection):
                return []
            hits = await self._client.query_points(
                collection_name=self._collection,
                query=vector,
                limit=k + len(exclude),
                with_payload=True,
            )
        except Exception:
            return []
        out = []
        for h in hits.points:
            payload = h.payload or {}
            item_id = payload.get("item_id", "")
            if item_id in exclude:
                continue
            out.append(
                {
                    "id": item_id,
                    "text": payload.get("text", ""),
                    "metadata": payload.get("metadata") or {},
                    "score": h.score,
                }
            )
            if len(out) >= k:
                break
        return out


_ITEM_NS = uuid.UUID("6f3a1e2c-7b4d-5e6f-8a9b-0c1d2e3f4a5b")


def _point_id(item_id) -> str:
    """Deterministic UUID point id from a caller-supplied item id, so Qdrant
    (which needs UUID/int ids) can use arbitrary string ids stably."""
    return str(uuid.uuid5(_ITEM_NS, str(item_id)))
