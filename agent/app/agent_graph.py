"""The tool-calling agent, as a LangGraph loop.

    call_model ──(tool calls?)──▶ tools ──▶ call_model
         │
         └──(final answer)──▶ END

The model is advertised the tool schemas and decides what to call; the `tools`
node executes the calls and feeds the results back, until the model produces a
plain answer or the step budget is exhausted. This is the standard agentic loop,
built on the same OpenAI-compatible client and Langfuse tracing as the RAG
pipeline. Knowledge-base retrieval is itself a tool, so the model chooses when to
search your documents versus compute, look up the time, or query the database.
"""

from __future__ import annotations

import json
from typing import Any, TypedDict

from langgraph.graph import END, StateGraph
from openai import AsyncOpenAI

from .config import Settings
from .tools import Tool
from .tracing import Tracer, safe_end, safe_generation, safe_span, safe_update

SYSTEM_PROMPT = (
    "You are a helpful assistant with access to tools. Use a tool when it helps "
    "answer the question — search the knowledge base for facts about the user's "
    "documents, calculate when math is involved, and so on. When you have what you "
    "need, reply to the user in plain language."
)


class AgentState(TypedDict, total=False):
    messages: list[dict]
    steps: list[dict]
    iterations: int
    max_steps: int
    answer: str
    trace: Any


class ToolAgent:
    def __init__(
        self,
        settings: Settings,
        tools: list[Tool],
        llm: AsyncOpenAI | None = None,
        tracer: Tracer | None = None,
    ) -> None:
        self._settings = settings
        self._tools = {t.name: t for t in tools}
        self._schemas = [t.schema() for t in tools]
        self._llm = llm or AsyncOpenAI(base_url=settings.llm_base_url, api_key=settings.llm_api_key)
        self._tracer = tracer or Tracer(settings)
        self._graph = self._build()

    def _build(self):
        g = StateGraph(AgentState)
        g.add_node("call_model", self._call_model)
        g.add_node("tools", self._run_tools)
        g.set_entry_point("call_model")
        g.add_conditional_edges("call_model", self._route, {"tools": "tools", END: END})
        g.add_edge("tools", "call_model")
        return g.compile()

    def _route(self, state: AgentState) -> str:
        last = state["messages"][-1]
        budget = state.get("max_steps", self._settings.agent_max_steps)
        if last.get("tool_calls") and state.get("iterations", 0) < budget:
            return "tools"
        return END

    async def _call_model(self, state: AgentState) -> AgentState:
        gen = safe_generation(
            state.get("trace"),
            name="call_model",
            model=self._settings.llm_model,
            input=state["messages"],
        )
        resp = await self._llm.chat.completions.create(
            model=self._settings.llm_model,
            messages=state["messages"],
            tools=self._schemas,
            tool_choice="auto",
        )
        msg = resp.choices[0].message
        assistant: dict = {"role": "assistant", "content": msg.content or ""}
        tool_calls = getattr(msg, "tool_calls", None)
        if tool_calls:
            assistant["tool_calls"] = [
                {
                    "id": tc.id,
                    "type": "function",
                    "function": {"name": tc.function.name, "arguments": tc.function.arguments},
                }
                for tc in tool_calls
            ]
        safe_end(gen, output=assistant.get("tool_calls") or assistant["content"])
        return {"messages": [*state["messages"], assistant], "answer": msg.content or ""}

    async def _run_tools(self, state: AgentState) -> AgentState:
        last = state["messages"][-1]
        new_messages: list[dict] = []
        steps = list(state.get("steps", []))
        for tc in last.get("tool_calls", []):
            name = tc["function"]["name"]
            try:
                args = json.loads(tc["function"].get("arguments") or "{}")
            except json.JSONDecodeError:
                args = {}
            span = safe_span(state.get("trace"), name=f"tool:{name}", input=args)
            tool = self._tools.get(name)
            result = await tool.run(args) if tool else f"error: unknown tool '{name}'"
            safe_end(span, output=result[:500])
            new_messages.append({"role": "tool", "tool_call_id": tc["id"], "content": result})
            steps.append({"tool": name, "args": args, "result": result})
        return {
            "messages": [*state["messages"], *new_messages],
            "steps": steps,
            "iterations": state.get("iterations", 0) + 1,
        }

    async def run(
        self, question: str, history: list[dict] | None = None, max_steps: int | None = None
    ) -> dict:
        messages = [
            {"role": "system", "content": SYSTEM_PROMPT},
            *(history or []),
            {"role": "user", "content": question},
        ]
        trace = self._tracer.trace(name="tool-agent", input={"question": question})
        result = await self._graph.ainvoke(
            {
                "messages": messages,
                "steps": [],
                "iterations": 0,
                "max_steps": max_steps or self._settings.agent_max_steps,
                "trace": trace,
            }
        )
        answer = result.get("answer", "")
        steps = result.get("steps", [])
        safe_update(trace, output={"answer": answer, "tools_used": [s["tool"] for s in steps]})
        return {"answer": answer, "steps": steps}

    def flush(self) -> None:
        self._tracer.flush()
