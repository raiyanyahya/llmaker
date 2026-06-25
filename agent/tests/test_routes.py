from fakes import (
    FakeEmbedder,
    FakeEvaluator,
    FakeExtractor,
    FakeItemStore,
    FakeMemory,
    FakePipeline,
    FakeStore,
    FakeSummarizer,
    FakeToolAgent,
    FakeTranscriber,
)
from fastapi.testclient import TestClient

from app.config import Settings
from app.main import create_app


def _fakes() -> dict:
    embedder = FakeEmbedder()
    store = FakeStore()
    return {
        "embedder": embedder,
        "store": store,
        "item_store": FakeItemStore(),
        "pipeline": FakePipeline(store, embedder),
        "tool_agent": FakeToolAgent(),
        "evaluator": FakeEvaluator(),
        "summarizer": FakeSummarizer(),
        "extractor": FakeExtractor(),
        "transcriber": FakeTranscriber(),
        "memory": FakeMemory(),
    }


def make_client(api_key: str = "") -> TestClient:
    return TestClient(create_app(Settings(api_key=api_key), **_fakes()))


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


def test_summarize_endpoint():
    with make_client() as c:
        r = c.post("/api/summarize", json={"text": "a longish piece of text", "max_words": 20})
        assert r.status_code == 200, r.text
        assert r.json()["summary"]
        assert c.post("/api/summarize", json={"text": "  "}).status_code == 400


def test_extract_endpoint():
    with make_client() as c:
        r = c.post("/api/extract", json={"text": "Ada, 1843", "fields": {"name": "person"}})
        assert r.status_code == 200, r.text
        assert "name" in r.json()["data"]
        assert c.post("/api/extract", json={"text": "x", "fields": {}}).status_code == 400


def test_transcribe_endpoint():
    with make_client() as c:
        r = c.post("/api/transcribe", files={"file": ("clip.wav", b"RIFF", "audio/wav")})
        assert r.status_code == 200, r.text
        assert "transcript" in r.json()["text"]


def test_transcribe_requires_whisper_configured():
    f = _fakes()
    f["transcriber"] = None  # WHISPER_URL unset → endpoint disabled
    with TestClient(create_app(Settings(), **f)) as c:
        r = c.post("/api/transcribe", files={"file": ("clip.wav", b"RIFF", "audio/wav")})
        assert r.status_code == 400


def test_memory_persists_history_across_requests():
    f = _fakes()
    with TestClient(create_app(Settings(), **f)) as c:
        c.post("/api/chat", json={"question": "first?", "session_id": "s1"})
        c.post("/api/chat", json={"question": "second?", "session_id": "s1"})
    # Two turns persisted server-side (user + assistant, twice).
    saved = f["memory"].sessions["s1"]
    assert len(saved) == 4
    assert saved[0] == {"role": "user", "content": "first?"}


def test_no_memory_without_session_id():
    f = _fakes()
    with TestClient(create_app(Settings(), **f)) as c:
        c.post("/api/chat", json={"question": "anon"})
    assert f["memory"].sessions == {}  # nothing stored without a session_id


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
