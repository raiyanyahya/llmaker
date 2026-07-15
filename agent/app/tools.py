"""Tools the agent can call.

Each tool is a name, an OpenAI-format JSON-schema description, and an async
function. The agent (agent_graph.py) advertises the schemas to the model and
executes whatever the model decides to call. Tools are deliberately small and
side-effect-light; the SQL tool is read-only and opt-in.

Built-in tools:
  - calculator      safe arithmetic, no code execution
  - current_time    the current UTC time
  - knowledge_base  retrieval over the ingested documents (RAG, as a tool)
  - web_search      public-web search via self-hosted SearXNG (only if SEARCH_URL set)
  - sql             read-only SQL over a configured database (only if SQL_DSN set)
"""

from __future__ import annotations

import ast
import datetime
import json
import math
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

# Cap exponents so a crafted expression (e.g. 9**9**9) can't pin the CPU / blow
# up memory building an astronomically large integer.
_MAX_POW_EXP = 1000
# Cap the magnitude of any intermediate result too. The exponent cap alone is not
# enough: nested powers (e.g. ((9**999)**999)**50) keep every exponent small while
# the value explodes super-exponentially. ~10k bits (≈3000 digits) is far beyond
# any real calculator use and keeps every operation cheap.
_MAX_RESULT_BITS = 10_000


def _magnitude_bits(value) -> float:
    """Approximate log2(|value|) — an upper bound used to reject a result that
    would be too large to compute cheaply, before actually computing it."""
    if isinstance(value, int):
        return value.bit_length()  # ≥ log2(|value|)
    v = abs(value)
    return math.log2(v) if v > 1.0 else 0.0


def safe_eval(expression: str) -> float:
    """Evaluate an arithmetic expression over numbers only. Anything else (names,
    calls, attributes) is rejected, so this never executes arbitrary code."""

    def _eval(node: ast.AST):
        # bool is an int subclass; True/False are not calculator numbers.
        if (
            isinstance(node, ast.Constant)
            and isinstance(node.value, int | float)
            and not isinstance(node.value, bool)
        ):
            return node.value
        if isinstance(node, ast.BinOp) and type(node.op) in _BIN_OPS:
            left, right = _eval(node.left), _eval(node.right)
            if isinstance(node.op, ast.Pow):
                if abs(right) > _MAX_POW_EXP:
                    raise ValueError("exponent too large")
                # Reject before computing: an astronomically large pow never
                # runs. Only positive exponents grow the result — a negative
                # one shrinks it toward zero, however large the base.
                if right > 0 and _magnitude_bits(left) * right > _MAX_RESULT_BITS:
                    raise ValueError("result too large")
            try:
                result = _BIN_OPS[type(node.op)](left, right)
            except OverflowError:
                # Float overflow (e.g. 10.0**400, or an int too big to convert
                # for a float op) raises OverflowError; keep the contract that
                # size rejections are always ValueError.
                raise ValueError("result too large") from None
            # Backstop for growth that isn't a single pow (e.g. a long multiply
            # chain): reject once an intermediate integer exceeds the cap.
            if isinstance(result, int) and result.bit_length() > _MAX_RESULT_BITS:
                raise ValueError("result too large")
            return result
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
    # datetime.timezone.utc (not datetime.UTC, which is 3.11+) keeps Python 3.10 happy.
    return datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")


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


# --- web search (self-hosted via SearXNG; opt-in) ---


def web_search_tool(search_url: str, client=None, max_results: int = 5) -> Tool:
    """A web_search tool backed by a SearXNG-compatible JSON endpoint.

    SearXNG aggregates public search engines and, with the JSON format enabled,
    returns ``{"results": [{title, url, content}, …]}``. The agent stays fully
    self-hosted: queries go to the in-network ``searxng`` service, not a paid API.
    A ``client`` (httpx.AsyncClient) can be injected for tests; otherwise one is
    created per call so nothing is left open between searches.
    """
    base = search_url.rstrip("/")

    async def _query(http, query: str) -> list[dict]:
        resp = await http.get(f"{base}/search", params={"q": query, "format": "json"})
        resp.raise_for_status()
        return resp.json().get("results") or []

    async def _search(args: dict) -> str:
        query = str(args.get("query", "")).strip()
        if not query:
            return "error: query required"
        if client is not None:
            results = await _query(client, query)
        else:
            import httpx

            async with httpx.AsyncClient(timeout=15.0) as http:
                results = await _query(http, query)
        results = results[:max_results]
        if not results:
            return "No web results found."
        return "\n\n".join(
            f"{r.get('title', '').strip()} — {r.get('url', '').strip()}\n"
            f"{(r.get('content') or '').strip()}"
            for r in results
        )

    return Tool(
        "web_search",
        "Search the public web and return the top results (title, URL, snippet). "
        "Use for current events or facts not in the knowledge base.",
        {
            "type": "object",
            "properties": {"query": {"type": "string", "description": "the search query"}},
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


def sql_tool(dsn: str, row_limit: int = 50, timeout: float = 15.0) -> Tool:
    async def _query(args: dict) -> str:
        query = str(args.get("query", "")).strip()
        if not is_read_only_sql(query):
            return "error: only a single read-only SELECT/WITH query is allowed"
        import asyncpg

        conn = await asyncpg.connect(dsn, timeout=timeout)
        try:
            # Enforce read-only at the server too: a READ ONLY transaction rejects
            # any write — including DML smuggled into a CTE (WITH … DELETE …),
            # which the textual guard above can't catch on its own.
            async with conn.transaction(readonly=True):
                rows = await conn.fetch(query, timeout=timeout)
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
    if settings.search_url:
        tools.append(web_search_tool(settings.search_url, max_results=settings.search_results))
    if settings.sql_dsn:
        tools.append(sql_tool(settings.sql_dsn))
    return tools
