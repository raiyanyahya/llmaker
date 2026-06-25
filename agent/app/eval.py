"""An evaluation harness for the RAG pipeline.

Self-hosting a retrieval stack is only half the job — you also need to know
whether it actually answers well. This runs a dataset of questions through the
same pipeline that serves ``/api/chat`` and grades each answer, so quality is
measurable and regressions are visible.

Metrics, per case:

- **groundedness** — is every claim in the answer supported by the retrieved
  context? (LLM-as-judge, 0–1)
- **relevance** — does the answer actually address the question? (judge, 0–1)
- **correctness** — does the answer agree with a provided ``reference``? (judge,
  0–1; only scored when a reference is given)
- **context_recall** — fraction of the case's ``expected_sources`` that retrieval
  surfaced (deterministic, no LLM; only when expected_sources are given)

The judge is the chat model by default (override with ``EVAL_MODEL``). Every case
is traced to Langfuse with its scores attached when tracing is configured, so the
harness doubles as an evaluation dataset in the same place the live traces land.
"""

from __future__ import annotations

import json

from openai import AsyncOpenAI

from .config import Settings
from .tracing import Tracer, safe_end, safe_score, safe_span, safe_update

# Plain English for each metric, woven into the judge prompt.
_RUBRIC = {
    "groundedness": "is every claim in the answer supported by the context?",
    "relevance": "does the answer address the question?",
    "correctness": "does the answer agree with the reference answer?",
}


class Evaluator:
    def __init__(
        self,
        settings: Settings,
        pipeline,
        judge: AsyncOpenAI | None = None,
        tracer: Tracer | None = None,
    ) -> None:
        self._settings = settings
        self._pipeline = pipeline
        self._judge = judge or AsyncOpenAI(
            base_url=settings.llm_base_url, api_key=settings.llm_api_key
        )
        self._tracer = tracer or Tracer(settings)

    async def evaluate(self, cases: list[dict], top_k: int | None = None) -> dict:
        """Run every case and return per-case results plus an aggregate summary."""
        results = [await self._score_case(c, top_k) for c in cases]
        return {"results": results, "summary": _summarize(results)}

    async def _score_case(self, case: dict, top_k: int | None) -> dict:
        question = case["question"]
        reference = case.get("reference")
        expected = case.get("expected_sources") or []
        trace = self._tracer.trace(name="rag-eval", input={"question": question})

        state = await self._pipeline.answer(question, top_k)
        answer = state.get("answer", "")
        context = state.get("context", [])
        sources = [c.get("source", "") for c in context]

        scores = await self._judge_answer(question, context, answer, reference, trace)
        if expected:
            scores["context_recall"] = _context_recall(sources, expected)

        for name, value in scores.items():
            safe_score(trace, name=name, value=value)
        safe_update(trace, output={"answer": answer, "scores": scores})
        return {"question": question, "answer": answer, "sources": sources, "scores": scores}

    async def _judge_answer(
        self, question: str, context: list[dict], answer: str, reference: str | None, trace
    ) -> dict:
        metrics = ["groundedness", "relevance"]
        if reference:
            metrics.append("correctness")
        joined = "\n\n".join(c.get("text", "") for c in context) or "(no context retrieved)"
        rubric = "\n".join(f"- {m}: {_RUBRIC[m]}" for m in metrics)
        example = "{" + ", ".join(f'"{m}": 0.0' for m in metrics) + "}"
        prompt = (
            "You are a strict evaluator of a retrieval-augmented answer. Score each "
            f"metric from 0.0 (fails) to 1.0 (perfect):\n{rubric}\n\n"
            f"QUESTION:\n{question}\n\nCONTEXT:\n{joined}\n\nANSWER:\n{answer}\n"
            + (f"\nREFERENCE ANSWER:\n{reference}\n" if reference else "")
            + f"\nReply with ONLY a JSON object of the scores, e.g. {example}"
        )
        span = safe_span(trace, name="judge", input={"metrics": metrics})
        try:
            resp = await self._judge.chat.completions.create(
                model=self._settings.judge_model(),
                messages=[{"role": "user", "content": prompt}],
            )
            raw = resp.choices[0].message.content or ""
        except Exception:
            raw = ""
        scores = _parse_scores(raw, metrics)
        safe_end(span, output=scores)
        return scores

    def flush(self) -> None:
        self._tracer.flush()


def _context_recall(sources: list[str], expected: list[str]) -> float:
    """Fraction of expected sources that retrieval surfaced. Matching is by
    substring either way, so ``handbook.pdf`` matches a chunk sourced from
    ``handbook.pdf#3`` and vice versa."""
    if not expected:
        return 0.0
    present = [s for s in sources if s]
    hits = sum(1 for exp in expected if any(exp in s or s in exp for s in present))
    return round(hits / len(expected), 3)


def _parse_scores(raw: str, metrics: list[str]) -> dict:
    """Pull the requested metrics out of the judge's reply, clamped to [0, 1].
    Tolerates code fences and surrounding prose; missing/garbled scores → 0."""
    data = _extract_json(raw)
    return {m: _clamp(data.get(m)) for m in metrics}


def _extract_json(raw: str) -> dict:
    start, end = raw.find("{"), raw.rfind("}")
    if start == -1 or end <= start:
        return {}
    try:
        obj = json.loads(raw[start : end + 1])
    except (json.JSONDecodeError, ValueError):
        return {}
    return obj if isinstance(obj, dict) else {}


def _clamp(value) -> float:
    try:
        return max(0.0, min(1.0, float(value)))
    except (TypeError, ValueError):
        return 0.0


def _summarize(results: list[dict]) -> dict:
    """Mean of each metric across all cases, plus the case count."""
    totals: dict[str, float] = {}
    counts: dict[str, int] = {}
    for r in results:
        for name, value in r["scores"].items():
            totals[name] = totals.get(name, 0.0) + value
            counts[name] = counts.get(name, 0) + 1
    means = {name: round(total / counts[name], 3) for name, total in totals.items()}
    return {"cases": len(results), "means": means}
