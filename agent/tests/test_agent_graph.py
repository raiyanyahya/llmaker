"""Exercises the real LangGraph tool-calling loop with a scripted LLM."""

from app.agent_graph import ToolAgent
from app.config import Settings
from app.tools import calculator


class _Fn:
    def __init__(self, name, arguments):
        self.name = name
        self.arguments = arguments


class _ToolCall:
    def __init__(self, id, name, arguments):
        self.id = id
        self.function = _Fn(name, arguments)


class _Msg:
    def __init__(self, content=None, tool_calls=None):
        self.content = content
        self.tool_calls = tool_calls


class _Resp:
    def __init__(self, msg):
        self.choices = [type("C", (), {"message": msg})()]


class ScriptedLLM:
    """Replays a list of assistant messages, recording each call's messages."""

    def __init__(self, responses):
        self._responses = responses
        self.i = 0
        self.calls = []
        self.chat = type("X", (), {"completions": self})()

    async def create(self, model, messages, tools=None, tool_choice=None):
        self.calls.append(messages)
        r = self._responses[min(self.i, len(self._responses) - 1)]
        self.i += 1
        return _Resp(r)


async def test_tool_agent_calls_tool_then_answers():
    llm = ScriptedLLM(
        [
            _Msg(
                content="", tool_calls=[_ToolCall("call_1", "calculator", '{"expression":"2+2"}')]
            ),
            _Msg(content="The result is 4.", tool_calls=None),
        ]
    )
    agent = ToolAgent(Settings(), [calculator], llm=llm)

    out = await agent.run("what is 2 + 2?")

    assert out["answer"] == "The result is 4."
    assert out["steps"][0]["tool"] == "calculator"
    assert out["steps"][0]["result"] == "4"
    # The tool's result was fed back into the model's next call.
    assert any(m.get("role") == "tool" and m.get("content") == "4" for m in llm.calls[-1])


async def test_tool_agent_respects_step_budget():
    # A model that always asks for a tool must still terminate.
    looping = ScriptedLLM(
        [_Msg(content="", tool_calls=[_ToolCall("c", "calculator", '{"expression":"1+1"}')])]
    )
    agent = ToolAgent(Settings(agent_max_steps=2), [calculator], llm=looping)

    out = await agent.run("loop forever?", max_steps=2)

    assert len(out["steps"]) == 2  # stopped at the budget, no infinite loop


async def test_tool_agent_answers_without_tools():
    llm = ScriptedLLM([_Msg(content="Hello!", tool_calls=None)])
    agent = ToolAgent(Settings(), [calculator], llm=llm)

    out = await agent.run("hi")

    assert out["answer"] == "Hello!"
    assert out["steps"] == []
    assert len(llm.calls) == 1  # answered directly, no tool round
