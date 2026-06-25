"""RedisMemory: round-trip, capping, clear, and graceful degradation."""

from app.config import Settings
from app.memory import RedisMemory, build_memory


class FakeRedis:
    """Minimal async Redis stand-in (get/set/delete) over a dict."""

    def __init__(self):
        self.kv = {}
        self.ttls = {}

    async def get(self, key):
        return self.kv.get(key)

    async def set(self, key, value, ex=None):
        self.kv[key] = value
        self.ttls[key] = ex

    async def delete(self, key):
        self.kv.pop(key, None)

    async def aclose(self):
        pass


async def test_memory_round_trip_and_ttl():
    redis = FakeRedis()
    m = RedisMemory("redis://x", client=redis, ttl=123)
    assert await m.load("s1") == []

    turn = [{"role": "user", "content": "hi"}, {"role": "assistant", "content": "yo"}]
    await m.append("s1", turn)

    assert await m.load("s1") == turn
    assert redis.ttls["llmaker:session:s1"] == 123  # expiry was set


async def test_memory_caps_history_to_recent_turns():
    m = RedisMemory("redis://x", client=FakeRedis(), max_turns=1)  # keep 2 messages
    await m.append("s", [{"role": "user", "content": "a"}, {"role": "assistant", "content": "b"}])
    await m.append("s", [{"role": "user", "content": "c"}, {"role": "assistant", "content": "d"}])

    hist = await m.load("s")
    assert len(hist) == 2
    assert hist[-1]["content"] == "d"  # oldest turn evicted


async def test_memory_clear():
    m = RedisMemory("redis://x", client=FakeRedis())
    await m.append("s", [{"role": "user", "content": "x"}])
    await m.clear("s")
    assert await m.load("s") == []


async def test_memory_degrades_when_redis_errors():
    class Boom:
        async def get(self, key):
            raise RuntimeError("redis down")

        async def set(self, key, value, ex=None):
            raise RuntimeError("redis down")

        async def delete(self, key):
            raise RuntimeError("redis down")

    m = RedisMemory("redis://x", client=Boom())
    assert await m.load("s") == []  # never raises
    await m.append("s", [{"role": "user", "content": "x"}])  # swallowed
    await m.clear("s")  # swallowed


def test_build_memory_is_off_without_url():
    assert build_memory(Settings()) is None
