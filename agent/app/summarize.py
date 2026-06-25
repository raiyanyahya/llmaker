"""Summarization, with map-reduce for inputs too long for one pass.

Short text is summarized in a single LLM call. Long text is split into chunks,
each summarized (the "map"), and those partial summaries are summarized together
(the "reduce") — so a whole report or transcript condenses without blowing the
context window. Each call is traced to Langfuse when configured.
"""

from __future__ import annotations

from openai import AsyncOpenAI

from .config import Settings
from .ingest import chunk_text
from .tracing import Tracer, safe_end, safe_generation, safe_update

SYSTEM_PROMPT = "You are a precise summarizer. Capture the key points faithfully and concisely."


class Summarizer:
    def __init__(
        self,
        settings: Settings,
        llm: AsyncOpenAI | None = None,
        tracer: Tracer | None = None,
    ) -> None:
        self._settings = settings
        self._llm = llm or AsyncOpenAI(base_url=settings.llm_base_url, api_key=settings.llm_api_key)
        self._tracer = tracer or Tracer(settings)

    async def summarize(
        self, text: str, instructions: str | None = None, max_words: int | None = None
    ) -> dict:
        """Summarize text; returns {"summary", "chunks"} (chunks = map-reduce passes)."""
        trace = self._tracer.trace(name="summarize", input={"chars": len(text)})
        chunks = chunk_text(text, self._settings.summarize_chunk_size, 0) or [text]

        if len(chunks) <= 1:
            summary = await self._one(chunks[0], instructions, max_words, trace)
            safe_update(trace, output={"chunks": 1})
            return {"summary": summary, "chunks": 1}

        # map: summarize each chunk; reduce: summarize the partial summaries.
        partials = [await self._one(c, instructions, None, trace) for c in chunks]
        reduce_instr = instructions or "Combine these section summaries into one coherent summary."
        summary = await self._one("\n\n".join(partials), reduce_instr, max_words, trace)
        safe_update(trace, output={"chunks": len(chunks)})
        return {"summary": summary, "chunks": len(chunks)}

    async def _one(self, text, instructions, max_words, trace) -> str:
        directive = instructions or "Summarize the following."
        if max_words:
            directive += f" Keep it under about {max_words} words."
        messages = [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": f"{directive}\n\n{text}"},
        ]
        gen = safe_generation(
            trace, name="summarize", model=self._settings.llm_model, input=directive
        )
        resp = await self._llm.chat.completions.create(
            model=self._settings.llm_model, messages=messages
        )
        out = (resp.choices[0].message.content or "").strip()
        safe_end(gen, output=out)
        return out

    def flush(self) -> None:
        self._tracer.flush()
