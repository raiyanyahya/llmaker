from fakes import (
    FakeEmbedder,
    FakeEvaluator,
    FakeItemStore,
    FakePipeline,
    FakeStore,
    FakeToolAgent,
)
from fastapi.testclient import TestClient

from app.config import Settings
from app.main import create_app


def make_client(api_key: str = "") -> TestClient:
    embedder = FakeEmbedder()
    store = FakeStore()
    pipeline = FakePipeline(store, embedder)
    settings = Settings(api_key=api_key)
    app = create_app(
        settings,
        embedder=embedder,
        store=store,
        item_store=FakeItemStore(),
        pipeline=pipeline,
        tool_agent=FakeToolAgent(),
        evaluator=FakeEvaluator(),
    )
    return TestClient(app)


def test_health_is_open():
    with make_client() as c:
        r = c.get("/health")
        assert r.status_code == 200
        assert r.json()["status"] == "ok"


def test_ingest_then_chat_round_trip():
    with make_client() as c:
        r = c.post("/api/ingest", data={"text": "llmaker runs a local rag stack", "source": "doc1"})
        assert r.status_code == 200, r.text
        assert r.json()["ingested"] >= 1

        stats = c.get("/api/stats").json()
        assert stats["chunks"] >= 1

        r = c.post("/api/chat", json={"question": "what does llmaker rag do?"})
        assert r.status_code == 200, r.text
        body = r.json()
        assert "llmaker" in body["answer"].lower()
        assert body["sources"] and body["sources"][0]["source"] == "doc1"


def test_agent_endpoint_returns_answer_and_steps():
    with make_client() as c:
        r = c.post("/api/agent", json={"question": "what is 2+2?"})
        assert r.status_code == 200, r.text
        body = r.json()
        assert body["answer"]
        assert body["steps"] and body["steps"][0]["tool"] == "calculator"


def test_agent_requires_question():
    with make_client() as c:
        assert c.post("/api/agent", json={"question": "  "}).status_code == 400


def test_eval_endpoint_scores_cases():
    with make_client() as c:
        r = c.post("/api/eval", json={"cases": [{"question": "q1"}, {"question": "q2"}]})
        assert r.status_code == 200, r.text
        body = r.json()
        assert body["summary"]["cases"] == 2
        assert body["results"][0]["scores"]["groundedness"] == 1.0


def test_eval_requires_cases():
    with make_client() as c:
        assert c.post("/api/eval", json={"cases": []}).status_code == 400


def test_ingest_requires_content():
    with make_client() as c:
        r = c.post("/api/ingest", data={})
        assert r.status_code == 400


def test_chat_requires_question():
    with make_client() as c:
        r = c.post("/api/chat", json={"question": "  "})
        assert r.status_code == 400


def test_file_ingest():
    with make_client() as c:
        r = c.post(
            "/api/ingest",
            files={"file": ("notes.txt", b"vectors make rag fast and local", "text/plain")},
        )
        assert r.status_code == 200, r.text
        assert r.json()["source"] == "notes.txt"


def test_auth_enforced_when_key_set():
    with make_client(api_key="secret") as c:
        assert c.get("/health").status_code == 200  # health stays open
        assert c.post("/api/chat", json={"question": "hi"}).status_code == 401
        ok = c.post(
            "/api/chat", json={"question": "hi"}, headers={"Authorization": "Bearer secret"}
        )
        assert ok.status_code == 200
