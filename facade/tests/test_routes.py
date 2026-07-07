"""Route-level tests against the in-memory FakeAdapter."""

from __future__ import annotations

import json

from fake_adapter import FakeAdapter
from fastapi.testclient import TestClient

from app.config import Settings
from app.main import create_app


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
    lines = [json.loads(ln) for ln in r.text.strip().splitlines() if ln.strip()]
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
    r = client.post(
        "/v1/chat/completions", json={"model": "m", "messages": [{"role": "user", "content": "hi"}]}
    )
    assert r.status_code == 200
    assert r.json()["choices"][0]["message"]["content"] == "Hello world"


def test_chat_rejects_non_object_body(client):
    # Valid JSON that isn't an object must be a clean 400, not a 500.
    for body in ([], "hi", 5):
        r = client.post("/v1/chat/completions", json=body)
        assert r.status_code == 400


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


def test_tokens_per_second_recorded(client):
    # Starts at zero, then a completion with usage populates the rolling metric.
    assert client.get("/api/status").json()["metrics"]["tokens_per_second"] == 0
    client.post(
        "/v1/chat/completions", json={"model": "m", "messages": [{"role": "user", "content": "hi"}]}
    )
    assert client.get("/api/status").json()["metrics"]["tokens_per_second"] > 0


def test_default_model_injected_when_omitted(client, adapter):
    # A client that omits `model` gets the instance default filled in.
    r = client.post("/v1/chat/completions", json={"messages": [{"role": "user", "content": "hi"}]})
    assert r.status_code == 200
    assert adapter.last_payload["model"] == "llama3:8b"


def test_explicit_model_is_respected(client, adapter):
    client.post("/v1/chat/completions", json={"model": "custom:1b", "messages": []})
    assert adapter.last_payload["model"] == "custom:1b"


def test_default_model_tracks_runtime_change(client, adapter):
    client.post("/api/models/default", json={"model": "switched:3b"})
    client.post("/v1/completions", json={"prompt": "hi"})
    assert adapter.last_payload["model"] == "switched:3b"


def test_default_model_injected_for_embeddings(client, adapter):
    client.post("/v1/embeddings", json={"input": "hello"})
    assert adapter.last_payload["model"] == "llama3:8b"


def test_metrics_errors_and_in_flight(settings, adapter):
    # A failing backend increments errors_total; in-flight settles back to 0.
    adapter.fail = True
    app = create_app(settings=settings, adapter=adapter)
    with TestClient(app, raise_server_exceptions=False) as c:
        assert c.post("/v1/chat/completions", json={"messages": []}).status_code == 500
        body = c.get("/metrics").text
        errors = next(ln for ln in body.splitlines() if ln.startswith("llmaker_errors_total "))
        in_flight = next(
            ln for ln in body.splitlines() if ln.startswith("llmaker_requests_in_flight ")
        )
        assert int(errors.split()[1]) == 1
        assert int(in_flight.split()[1]) == 0


def test_metrics_completion_tokens_total(client):
    client.post("/v1/chat/completions", json={"messages": [{"role": "user", "content": "hi"}]})
    body = client.get("/metrics").text
    line = next(ln for ln in body.splitlines() if ln.startswith("llmaker_completion_tokens_total "))
    assert int(line.split()[1]) >= 2  # the fake reports completion_tokens=2


def test_invalid_json_returns_400(client):
    r = client.post(
        "/v1/chat/completions",
        content="{not valid json",
        headers={"Content-Type": "application/json"},
    )
    assert r.status_code == 400


def test_metrics_prometheus_exposition(client):
    r = client.get("/metrics")
    assert r.status_code == 200
    assert "text/plain" in r.headers["content-type"]
    body = r.text
    assert "# TYPE llmaker_requests_total counter" in body
    assert "llmaker_up 1" in body
    assert "llmaker_info{" in body


def test_metrics_requests_counter_reflects_traffic(client):
    client.post("/v1/chat/completions", json={"model": "m", "messages": []})
    body = client.get("/metrics").text
    line = next(line for line in body.splitlines() if line.startswith("llmaker_requests_total "))
    assert int(line.split()[1]) >= 1


def test_metrics_open_even_with_api_key():
    # Scrapers carry no credentials; /metrics stays open like /api/health.
    app = create_app(settings=Settings(api_key="secret", backend="fake"), adapter=FakeAdapter())
    with TestClient(app) as c:
        assert c.get("/metrics").status_code == 200


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
