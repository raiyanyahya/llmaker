"""Summarizer: single-pass and map-reduce, with a scripted LLM."""

from fakes import ScriptedChat

from app.config import Settings
from app.summarize import Summarizer
from app.tracing import Tracer


async def test_summarize_single_pass():
    llm = ScriptedChat(["a concise summary"])
    s = Summarizer(Settings(), llm=llm, tracer=Tracer(Settings()))

    out = await s.summarize("a short piece of text to condense")

    assert out == {"summary": "a concise summary", "chunks": 1}
    assert len(llm.calls) == 1


async def test_summarize_map_reduce_for_long_text():
    # Tiny chunk size forces several map chunks + one reduce call.
    llm = ScriptedChat(["partial", "partial", "FINAL"])
    s = Summarizer(Settings(summarize_chunk_size=40), llm=llm, tracer=Tracer(Settings()))

    out = await s.summarize("sentence number one. " * 20, max_words=50)

    assert out["chunks"] > 1
    assert out["summary"] == "FINAL"
    assert len(llm.calls) == out["chunks"] + 1  # one per chunk, plus the reduce


async def test_summarize_passes_instructions_and_word_cap():
    llm = ScriptedChat(["ok"])
    s = Summarizer(Settings(), llm=llm, tracer=Tracer(Settings()))

    await s.summarize("text", instructions="as three bullet points", max_words=25)

    sent = llm.calls[0][-1]["content"]
    assert "as three bullet points" in sent
    assert "25 words" in sent
