"""Structured extraction: turn free text into a typed JSON object.

The caller names the fields it wants (each with a short description) and gets back
a JSON object with exactly those keys — the everyday "pull the order number, date
and total out of this email" task. The model is asked for JSON only, and the
reply is parsed defensively (fences/prose tolerated; missing fields become null),
so a chatty model never breaks the contract.
"""

from __future__ import annotations

import json

from openai import AsyncOpenAI

from .config import Settings
from .tracing import Tracer, safe_end, safe_generation, safe_update


class Extractor:
    def __init__(
        self,
        settings: Settings,
        llm: AsyncOpenAI | None = None,
        tracer: Tracer | None = None,
    ) -> None:
        self._settings = settings
        self._llm = llm or AsyncOpenAI(base_url=settings.llm_base_url, api_key=settings.llm_api_key)
        self._tracer = tracer or Tracer(settings)

    async def extract(self, text: str, fields: dict[str, str]) -> dict:
        """Extract `fields` (name → description) from text; returns {"data": {...}}
        with exactly the requested keys (null where not found)."""
        spec = "\n".join(f"- {name}: {desc}" for name, desc in fields.items())
        shape = "{" + ", ".join(f'"{name}": ...' for name in fields) + "}"
        prompt = (
            "Extract the following fields from the text and return ONLY a JSON object "
            f"with exactly these keys (use null when a field is absent):\n{spec}\n\n"
            f"TEXT:\n{text}\n\nJSON {shape}:"
        )
        trace = self._tracer.trace(name="extract", input={"fields": list(fields)})
        gen = safe_generation(
            trace, name="extract", model=self._settings.llm_model, input=list(fields)
        )
        try:
            resp = await self._llm.chat.completions.create(
                model=self._settings.llm_model, messages=[{"role": "user", "content": prompt}]
            )
            raw = resp.choices[0].message.content or ""
        except Exception:
            raw = ""
        parsed = _json_object(raw)
        data = {name: parsed.get(name) for name in fields}
        safe_end(gen, output=data)
        safe_update(trace, output={"data": data})
        return {"data": data}

    def flush(self) -> None:
        self._tracer.flush()


def _json_object(raw: str) -> dict:
    """Best-effort: pull the first {...} object out of a model reply."""
    start, end = raw.find("{"), raw.rfind("}")
    if start == -1 or end <= start:
        return {}
    try:
        obj = json.loads(raw[start : end + 1])
    except (json.JSONDecodeError, ValueError):
        return {}
    return obj if isinstance(obj, dict) else {}
