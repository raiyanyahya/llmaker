"""Exercises the real LangGraph pipeline (retrieve → generate) with fake I/O."""

from fakes import FakeEmbedder, FakeStore

from app.config import Settings
from app.rag import RagPipeline


class _Msg:
    def __init__(self, content):
        self.message = type("M", (), {"content": content})


class _Resp:
    def __init__(self, content):
        self.choices = [_Msg(content)]


class FakeLLM:
    """Minimal AsyncOpenAI stand-in: records every prompt and echoes the input."""

    def __init__(self):
        self.last_system = None
        self.calls: list[list[dict]] = []
        self.chat = type("C", (), {"completions": self})()

    async def create(self, model, messages):  # noqa: A002 - matches OpenAI SDK
        self.calls.append(messages)
        self.last_system = messages[0]["content"]
        return _Resp("answer using: " + messages[-1]["content"])


async def test_pipeline_retrieves_then_generates():
    embedder = FakeEmbedder()
    store = FakeStore()
    await store.upsert(
        await embedder.embed(["llmaker is a local rag stack", "cats are unrelated"]),
        ["llmaker is a local rag stack", "cats are unrelated"],
        "doc",
    )
    llm = FakeLLM()
    pipe = RagPipeline(Settings(), store, embedder, llm=llm)

    result = await pipe.answer("tell me about llmaker rag", top_k=1)

    # The graph ran generate (we got an answer) after retrieve (context present).
    assert result["answer"].startswith("answer using:")
    assert result["context"], "expected retrieved context"
    # The most relevant chunk (not the cat one) was retrieved and put in the prompt.
    assert "llmaker" in llm.last_system
    assert "cats are unrelated" not in llm.last_system


async def test_no_history_skips_the_rewrite_call():
    embedder = FakeEmbedder()
    store = FakeStore()
    await store.upsert(await embedder.embed(["llmaker rag"]), ["llmaker rag"], "doc")
    llm = FakeLLM()
    pipe = RagPipeline(Settings(), store, embedder, llm=llm)

    await pipe.answer("what is llmaker?", top_k=1)
    # Single-shot: only the generate call hits the LLM (rewrite is skipped).
    assert len(llm.calls) == 1


async def test_multi_turn_rewrites_and_carries_history():
    embedder = FakeEmbedder()
    store = FakeStore()
    await store.upsert(await embedder.embed(["llmaker rag stack"]), ["llmaker rag stack"], "doc")
    llm = FakeLLM()
    pipe = RagPipeline(Settings(), store, embedder, llm=llm)

    history = [
        {"role": "user", "content": "what is llmaker?"},
        {"role": "assistant", "content": "It runs a local LLM stack."},
    ]
    result = await pipe.answer("does it do rag?", top_k=1, history=history)

    # Two LLM calls now: the rewrite, then the generate.
    assert len(llm.calls) == 2
    # The final generate call carried the prior turns through to the model.
    generate_msgs = llm.calls[-1]
    contents = [m["content"] for m in generate_msgs]
    assert "It runs a local LLM stack." in contents
    assert result["answer"]
