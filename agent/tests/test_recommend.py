"""Recommendation endpoints (/api/items, /api/recommend) and the profile math."""

from fakes import FakeEmbedder, FakeItemStore, FakePipeline, FakeStore
from fastapi.testclient import TestClient

from app.config import Settings
from app.main import create_app
from app.recommend import centroid


def _client() -> TestClient:
    embedder = FakeEmbedder()
    store = FakeStore()
    return create_app(
        Settings(),
        embedder=embedder,
        store=store,
        item_store=FakeItemStore(),
        pipeline=FakePipeline(store, embedder),
    )


def test_centroid_averages():
    assert centroid([[2.0, 0.0], [0.0, 4.0]]) == [1.0, 2.0]
    assert centroid([]) == []


def test_add_items_then_recommend_by_query():
    with TestClient(_client()) as c:
        r = c.post(
            "/api/items",
            json={
                "items": [
                    {"id": "a", "text": "fast local llm inference"},
                    {"id": "b", "text": "cat grooming tips"},
                    {"id": "c", "text": "running llm models locally"},
                ]
            },
        )
        assert r.status_code == 200, r.text
        assert r.json()["added"] == 3

        rec = c.post("/api/recommend", json={"query": "run a local model", "k": 2})
        assert rec.status_code == 200, rec.text
        ids = [x["id"] for x in rec.json()["recommendations"]]
        assert "b" not in ids  # the cat item is irrelevant
        assert ids and ids[0] in {"a", "c"}


def test_recommend_more_like_this_excludes_seed():
    with TestClient(_client()) as c:
        c.post(
            "/api/items",
            json={
                "items": [
                    {"id": "a", "text": "local llm fast"},
                    {"id": "c", "text": "llm models local"},
                    {"id": "b", "text": "cat dog pet"},
                ]
            },
        )
        rec = c.post("/api/recommend", json={"like": ["a"], "k": 5})
        assert rec.status_code == 200, rec.text
        ids = [x["id"] for x in rec.json()["recommendations"]]
        assert "a" not in ids  # the seed itself is excluded
        assert "c" in ids  # the related item is recommended


def test_recommend_requires_query_or_like():
    with TestClient(_client()) as c:
        assert c.post("/api/recommend", json={"k": 3}).status_code == 400


def test_recommend_unknown_like_id():
    with TestClient(_client()) as c:
        r = c.post("/api/recommend", json={"like": ["nope"], "k": 3})
        assert r.status_code == 404
