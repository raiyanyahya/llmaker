"""Tools the agent can call.

Each tool is a name, an OpenAI-format JSON-schema description, and an async
function. The agent (agent_graph.py) advertises the schemas to the model and
executes whatever the model decides to call. Tools are deliberately small and
side-effect-light; the SQL tool is read-only and opt-in.

Built-in tools:
  - calculator      safe arithmetic, no code execution
  - current_time    the current UTC time
  - knowledge_base  retrieval over the ingested documents (RAG, as a tool)
  - sql             read-only SQL over a configured database (only if SQL_DSN set)
"""

from __future__ import annotations

import ast
import datetime
import json
import operator
from collections.abc import Awaitable, Callable

ToolFn = Callable[[dict], Awaitable[str]]


class Tool:
    def __init__(self, name: str, description: str, parameters: dict, fn: ToolFn) -> None:
        self.name = name
        self.description = description
        self.parameters = parameters
        self._fn = fn

    def schema(self) -> dict:
        """OpenAI tool/function schema."""
        return {
            "type": "function",
            "function": {
                "name": self.name,
                "description": self.description,
                "parameters": self.parameters,
            },
        }

    async def run(self, args: dict) -> str:
        try:
            return await self._fn(args)
        except Exception as exc:  # tools never raise into the loop
            return f"error: {exc}"


# --- calculator (safe arithmetic; no eval of arbitrary code) ---

_BIN_OPS = {
    ast.Add: operator.add,
    ast.Sub: operator.sub,
    ast.Mult: operator.mul,
    ast.Div: operator.truediv,
    ast.FloorDiv: operator.floordiv,
    ast.Mod: operator.mod,
    ast.Pow: operator.pow,
}
_UNARY_OPS = {ast.UAdd: operator.pos, ast.USub: operator.neg}


def safe_eval(expression: str) -> float:
    """Evaluate an arithmetic expression over numbers only. Anything else (names,
    calls, attributes) is rejected, so this never executes arbitrary code."""

    def _eval(node: ast.AST):
        if isinstance(node, ast.Constant) and isinstance(node.value, int | float):
            return node.value
        if isinstance(node, ast.BinOp) and type(node.op) in _BIN_OPS:
            return _BIN_OPS[type(node.op)](_eval(node.left), _eval(node.right))
        if isinstance(node, ast.UnaryOp) and type(node.op) in _UNARY_OPS:
            return _UNARY_OPS[type(node.op)](_eval(node.operand))
        raise ValueError("unsupported expression")

    return _eval(ast.parse(expression, mode="eval").body)


async def _calculator(args: dict) -> str:
    expr = str(args.get("expression", "")).strip()
    if not expr:
        return "error: expression required"
    return str(safe_eval(expr))


calculator = Tool(
    "calculator",
    "Evaluate an arithmetic expression and return the numeric result.",
    {
        "type": "object",
        "properties": {"expression": {"type": "string", "description": "e.g. (2+3)*4 or 12.5/4"}},
        "required": ["expression"],
    },
    _calculator,
)


# --- current time ---


async def _current_time(_: dict) -> str:
    return datetime.datetime.now(datetime.UTC).isoformat(timespec="seconds")


current_time = Tool(
    "current_time",
    "Get the current date and time in UTC (ISO-8601).",
    {"type": "object", "properties": {}},
    _current_time,
)


# --- knowledge base (retrieval as a tool) ---


def knowledge_base_tool(store, embedder, top_k: int) -> Tool:
    async def _search(args: dict) -> str:
        query = str(args.get("query", "")).strip()
        if not query:
            return "error: query required"
        vec = await embedder.embed_one(query)
        hits = await store.search(vec, top_k) if vec else []
        if not hits:
            return "No matching documents found."
        return "\n\n".join(f"[{h.get('source', '')}] {h.get('text', '')}" for h in hits)

    return Tool(
        "knowledge_base",
        "Search the ingested document collection for passages relevant to a query.",
        {
            "type": "object",
            "properties": {"query": {"type": "string", "description": "what to look up"}},
            "required": ["query"],
        },
        _search,
    )


# --- read-only SQL (opt-in) ---


def is_read_only_sql(query: str) -> bool:
    """Allow only a single SELECT/WITH statement. Rejects writes/DDL and stacked
    statements (a semicolon anywhere but a single trailing one)."""
    q = query.strip().rstrip(";").strip()
    if ";" in q:  # no stacked statements
        return False
    head = q.lower()
    return head.startswith("select ") or head.startswith("with ")


def sql_tool(dsn: str, row_limit: int = 50) -> Tool:
    async def _query(args: dict) -> str:
        query = str(args.get("query", "")).strip()
        if not is_read_only_sql(query):
            return "error: only a single read-only SELECT/WITH query is allowed"
        import asyncpg

        conn = await asyncpg.connect(dsn)
        try:
            rows = await conn.fetch(query)
        finally:
            await conn.close()
        data = [dict(r) for r in rows[:row_limit]]
        return json.dumps(data, default=str)

    return Tool(
        "sql",
        "Run a single read-only SQL query (SELECT/WITH) against the database and "
        "return the rows as JSON.",
        {
            "type": "object",
            "properties": {"query": {"type": "string", "description": "a SELECT statement"}},
            "required": ["query"],
        },
        _query,
    )


def build_tools(settings, store, embedder) -> list[Tool]:
    """Assemble the tool set for the agent from configuration."""
    tools = [calculator, current_time, knowledge_base_tool(store, embedder, settings.top_k)]
    if settings.sql_dsn:
        tools.append(sql_tool(settings.sql_dsn))
    return tools
