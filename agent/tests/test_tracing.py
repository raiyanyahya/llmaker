"""Verifies the RAG pipeline emits a well-formed Langfuse trace when tracing is
enabled, and stays silent (and unbroken) when it isn't."""

from fakes import FakeEmbedder, FakeLangfuse, FakeStore
from test_rag import FakeLLM

from app.config import Settings
from app.rag import RagPipeline
from app.tracing import Tracer


async def _seed_pipeline(tracer):
    embedder = FakeEmbedder()
    store = FakeStore()
    await store.upsert(
        await embedder.embed(["llmaker is a local rag stack"]),
        ["llmaker is a local rag stack"],
        "doc",
    )
    return RagPipeline(Settings(), store, embedder, llm=FakeLLM(), tracer=tracer)


async def test_emits_trace_with_retrieve_and_generate():
    lf = FakeLangfuse()
    tracer = Tracer(Settings(), client=lf)
    assert tracer.enabled

    pipe = await _seed_pipeline(tracer)
    await pipe.answer("tell me about llmaker", top_k=1)

    assert len(lf.traces) == 1
    trace = lf.traces[0]
    assert trace.input["name"] == "rag-chat"

    by_name = {o.input.get("name"): o for o in trace.observations}
    # The graph emitted retrieve + rerank spans and a generate generation.
    assert {"retrieve", "rerank", "generate"} <= set(by_name)

    retrieve = by_name["retrieve"]
    assert retrieve.kind == "span" and retrieve.ended is not None
    assert retrieve.ended["output"]["fetched"] >= 1

    rerank = by_name["rerank"]
    assert rerank.ended["output"]["kept"] == 1  # MMR down to top_k=1

    gen = by_name["generate"]
    assert gen.kind == "generation"
    assert gen.input["model"] == Settings().llm_model
    assert gen.ended is not None

    # The top-level trace was updated with the final answer.
    assert trace.updated is not None
    assert "answer" in trace.updated["output"]


async def test_disabled_tracer_is_silent_and_safe():
    tracer = Tracer(Settings())  # no langfuse keys → disabled
    assert not tracer.enabled
    pipe = await _seed_pipeline(tracer)
    # Must still produce an answer with tracing off.
    result = await pipe.answer("tell me about llmaker", top_k=1)
    assert result["answer"]
    pipe.flush()  # no-op, must not raise
