"""Route-level tests against the in-memory FakeAdapter."""

from __future__ import annotations

import json

from app.config import Settings
from app.main import create_app
from fake_adapter import FakeAdapter
from fastapi.testclient import TestClient


def test_health_ok(client):
    r = client.get("/api/health")
    assert r.status_code == 200
    assert r.json()["ready"] is True


def test_health_starting(client, adapter):
    adapter.healthy = False
    r = client.get("/api/health")
    assert r.status_code == 503
    assert r.json() == {"status": "starting", "ready": False}


def test_status(client):
    r = client.get("/api/status")
    assert r.status_code == 200
    d = r.json()
    assert d["instance"]["name"] == "test-inst"
    assert d["instance"]["backend"] == "fake"
    assert d["models"]["default"] == "llama3:8b"
    assert any(m["name"] == "llama3:8b" for m in d["models"]["installed"])
    assert "cpu_percent" in d["system"]


def test_list_models(client):
    r = client.get("/api/models")
    assert r.status_code == 200
    d = r.json()
    assert d["default"] == "llama3:8b"
    assert d["models"][0]["name"] == "llama3:8b"


def test_pull_streams_ndjson(client):
    r = client.post("/api/models/pull", json={"model": "qwen2.5:7b"})
    assert r.status_code == 200
    lines = [json.loads(l) for l in r.text.strip().splitlines() if l.strip()]
    assert lines[0]["status"] == "downloading"
    assert lines[-1]["status"] == "success"
    # The model becomes installed afterwards.
    assert any(m["name"] == "qwen2.5:7b" for m in client.get("/api/models").json()["models"])


def test_delete_model(client, adapter):
    r = client.post("/api/models/delete", json={"model": "llama3:8b"})
    assert r.status_code == 200
    assert "llama3:8b" in adapter.deleted


def test_set_default(client):
    r = client.post("/api/models/default", json={"model": "newdefault"})
    assert r.status_code == 200
    assert r.json()["default"] == "newdefault"
    assert client.get("/api/status").json()["models"]["default"] == "newdefault"


def test_chat_non_streaming(client):
    r = client.post("/v1/chat/completions", json={"model": "m", "messages": [{"role": "user", "content": "hi"}]})
    assert r.status_code == 200
    assert r.json()["choices"][0]["message"]["content"] == "Hello world"


def test_chat_streaming_sse(client):
    r = client.post(
        "/v1/chat/completions",
        json={"model": "m", "stream": True, "messages": [{"role": "user", "content": "hi"}]},
    )
    assert r.status_code == 200
    assert "text/event-stream" in r.headers["content-type"]
    body = r.text
    assert "Hello" in body and "world" in body and "[DONE]" in body


def test_embeddings(client):
    r = client.post("/v1/embeddings", json={"model": "m", "input": "hello"})
    assert r.status_code == 200
    assert r.json()["data"][0]["embedding"]


def test_openai_models(client):
    r = client.get("/v1/models")
    assert r.status_code == 200
    assert r.json()["data"][0]["id"] == "llama3:8b"


def test_requests_counter_increments(client):
    before = client.get("/api/status").json()["metrics"]["requests_total"]
    client.post("/v1/chat/completions", json={"model": "m", "messages": []})
    after = client.get("/api/status").json()["metrics"]["requests_total"]
    assert after == before + 1


def test_web_ui_served(client):
    r = client.get("/")
    assert r.status_code == 200
    assert "llmaker" in r.text.lower()


def test_auth_enforced_when_key_set():
    app = create_app(settings=Settings(api_key="secret", backend="fake"), adapter=FakeAdapter())
    with TestClient(app) as c:
        assert c.get("/api/status").status_code == 401
        assert c.get("/api/status", headers={"Authorization": "Bearer secret"}).status_code == 200
        # Health stays open for container probes even when a key is configured.
        assert c.get("/api/health").status_code == 200
        # Wrong key rejected.
        assert c.get("/api/status", headers={"Authorization": "Bearer nope"}).status_code == 401


def test_websocket_status(client):
    with client.websocket_connect("/ws/status") as ws:
        msg = ws.receive_json()
        assert msg["instance"]["name"] == "test-inst"
        assert "system" in msg
