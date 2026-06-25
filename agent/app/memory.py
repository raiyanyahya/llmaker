"""Redis-backed conversation memory.

When REDIS_URL is configured, ``/api/chat`` and ``/api/agent`` persist per-session
history server-side: a request carrying a ``session_id`` has its prior turns
loaded and prepended, and the new turn is appended afterward. History is capped
and expires, so sessions don't grow unbounded. Every Redis call is best-effort —
if Redis is unreachable the agent still answers (with whatever the client sent),
mirroring how the vector store degrades.

The session is stored as a single JSON list under ``llmaker:session:<id>``, which
keeps the fake (and the wire format) trivial: just ``get``/``set``/``delete``.
"""

from __future__ import annotations

import json


class RedisMemory:
    def __init__(self, url: str, client=None, max_turns: int = 20, ttl: int = 604800) -> None:
        if client is None:
            import redis.asyncio as redis

            client = redis.from_url(url, decode_responses=True)
        self._client = client
        self._max_messages = max_turns * 2  # a turn is a user + an assistant message
        self._ttl = ttl

    @staticmethod
    def _key(session_id: str) -> str:
        return f"llmaker:session:{session_id}"

    async def load(self, session_id: str) -> list[dict]:
        try:
            raw = await self._client.get(self._key(session_id))
            return json.loads(raw) if raw else []
        except Exception:
            return []

    async def append(self, session_id: str, messages: list[dict]) -> None:
        if not messages:
            return
        try:
            history = await self.load(session_id)
            history.extend(messages)
            history = history[-self._max_messages :]
            await self._client.set(self._key(session_id), json.dumps(history), ex=self._ttl)
        except Exception:
            pass

    async def clear(self, session_id: str) -> None:
        try:
            await self._client.delete(self._key(session_id))
        except Exception:
            pass

    async def aclose(self) -> None:
        try:
            await self._client.aclose()
        except Exception:
            pass


def build_memory(settings):
    """Return a RedisMemory when REDIS_URL is configured, else None (memory off)."""
    if not settings.redis_url:
        return None
    return RedisMemory(
        settings.redis_url,
        max_turns=settings.memory_max_turns,
        ttl=settings.memory_ttl_seconds,
    )
