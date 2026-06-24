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

    kinds = [o.kind for o in trace.observations]
    assert "span" in kinds and "generation" in kinds  # retrieve + generate

    retrieve = next(o for o in trace.observations if o.kind == "span")
    assert retrieve.ended is not None  # span was closed
    assert retrieve.ended["output"]["hits"] == 1

    gen = next(o for o in trace.observations if o.kind == "generation")
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
