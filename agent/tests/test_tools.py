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
)


def test_safe_eval_arithmetic():
    assert safe_eval("(2+3)*4") == 20
    assert safe_eval("2**10") == 1024
    assert safe_eval("-7 + 1.5") == -5.5


def test_safe_eval_rejects_code():
    for expr in ["__import__('os')", "open('x')", "a + 1", "1; 2"]:
        with pytest.raises((ValueError, SyntaxError)):
            safe_eval(expr)


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


def test_build_tools_includes_sql_only_when_configured():
    embedder, store = FakeEmbedder(), FakeStore()
    names = {t.name for t in build_tools(Settings(), store, embedder)}
    assert {"calculator", "current_time", "knowledge_base"} <= names
    assert "sql" not in names

    names_sql = {t.name for t in build_tools(Settings(sql_dsn="postgres://x"), store, embedder)}
    assert "sql" in names_sql


def test_tool_schema_is_openai_shape():
    s = calculator.schema()
    assert s["type"] == "function"
    assert s["function"]["name"] == "calculator"
    assert "expression" in s["function"]["parameters"]["properties"]
