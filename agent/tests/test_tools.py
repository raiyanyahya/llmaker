import pytest
from fakes import FakeEmbedder, FakeStore

from app.config import Settings
from app.tools import (
    build_tools,
    calculator,
    current_time,
    is_read_only_sql,
    knowledge_base_tool,
    safe_eval,
    web_search_tool,
)


class _FakeResponse:
    def __init__(self, payload):
        self._payload = payload

    def raise_for_status(self):
        pass

    def json(self):
        return self._payload


class _FakeHTTP:
    """A stand-in httpx.AsyncClient that records requests and returns a payload."""

    def __init__(self, payload):
        self._payload = payload
        self.requests = []

    async def get(self, url, params=None):
        self.requests.append((url, params))
        return _FakeResponse(self._payload)


def test_safe_eval_arithmetic():
    assert safe_eval("(2+3)*4") == 20
    assert safe_eval("2**10") == 1024
    assert safe_eval("-7 + 1.5") == -5.5


def test_safe_eval_rejects_code():
    for expr in ["__import__('os')", "open('x')", "a + 1", "1; 2"]:
        with pytest.raises((ValueError, SyntaxError)):
            safe_eval(expr)


def test_safe_eval_rejects_huge_power():
    # A crafted exponent must be rejected, not computed (DoS guard).
    for expr in ["9**9**9", "2 ** 100000"]:
        with pytest.raises(ValueError):
            safe_eval(expr)
    # Ordinary exponents still work.
    assert safe_eval("2**10") == 1024


def test_safe_eval_rejects_nested_power_blowup():
    # Every exponent here is under the per-exponent cap, but nesting still
    # explodes the magnitude — must be rejected on result size, not computed.
    for expr in ["(9**999)**999", "((9**999)**999)**50"]:
        with pytest.raises(ValueError):
            safe_eval(expr)
    # A large-but-bounded result is still allowed.
    assert safe_eval("9**500") == 9**500


async def test_calculator_tool():
    assert await calculator.run({"expression": "6*7"}) == "42"
    # Bad input is reported, never raised.
    assert (await calculator.run({"expression": "nope"})).startswith("error")


async def test_current_time_tool():
    out = await current_time.run({})
    assert "T" in out and out[:4].isdigit()  # ISO-8601


async def test_knowledge_base_tool_searches():
    embedder = FakeEmbedder()
    store = FakeStore()
    await store.upsert(await embedder.embed(["llmaker rag stack"]), ["llmaker rag stack"], "doc")
    kb = knowledge_base_tool(store, embedder, top_k=2)
    out = await kb.run({"query": "what is llmaker"})
    assert "llmaker rag stack" in out
    assert "[doc]" in out


def test_sql_guard_allows_only_readonly_single_statements():
    assert is_read_only_sql("SELECT * FROM orders")
    assert is_read_only_sql("  with x as (select 1) select * from x  ")
    assert is_read_only_sql("SELECT count(*) FROM t;")  # single trailing semicolon ok
    for bad in [
        "DROP TABLE orders",
        "DELETE FROM orders",
        "INSERT INTO t VALUES (1)",
        "UPDATE t SET a=1",
        "SELECT 1; DROP TABLE t",  # stacked
        "SELECT 1; SELECT 2",
    ]:
        assert not is_read_only_sql(bad), bad


async def test_web_search_tool_formats_and_caps_results():
    http = _FakeHTTP(
        {
            "results": [
                {"title": "llmaker", "url": "https://x", "content": "self-host the LLM stack"},
                {"title": "second", "url": "https://y", "content": "another"},
            ]
        }
    )
    tool = web_search_tool("http://searxng:8080", client=http, max_results=1)

    out = await tool.run({"query": "llmaker"})

    assert "llmaker" in out and "https://x" in out
    assert "second" not in out  # capped at max_results=1
    url, params = http.requests[0]
    assert url.endswith("/search")
    assert params == {"q": "llmaker", "format": "json"}


async def test_web_search_tool_handles_empty_and_missing_query():
    tool = web_search_tool("http://searxng:8080/", client=_FakeHTTP({"results": []}))
    assert await tool.run({"query": "x"}) == "No web results found."
    assert (await tool.run({"query": " "})).startswith("error")


def test_build_tools_includes_sql_only_when_configured():
    embedder, store = FakeEmbedder(), FakeStore()
    names = {t.name for t in build_tools(Settings(), store, embedder)}
    assert {"calculator", "current_time", "knowledge_base"} <= names
    assert "sql" not in names

    names_sql = {t.name for t in build_tools(Settings(sql_dsn="postgres://x"), store, embedder)}
    assert "sql" in names_sql


def test_build_tools_includes_web_search_only_when_configured():
    embedder, store = FakeEmbedder(), FakeStore()
    assert "web_search" not in {t.name for t in build_tools(Settings(), store, embedder)}
    cfg = Settings(search_url="http://searxng:8080")
    assert "web_search" in {t.name for t in build_tools(cfg, store, embedder)}


def test_tool_schema_is_openai_shape():
    s = calculator.schema()
    assert s["type"] == "function"
    assert s["function"]["name"] == "calculator"
    assert "expression" in s["function"]["parameters"]["properties"]
