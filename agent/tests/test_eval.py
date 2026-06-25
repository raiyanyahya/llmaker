"""Exercises the evaluation harness with a scripted judge LLM (no network)."""

from fakes import FakeEmbedder, FakeLangfuse, FakePipeline, FakeStore

from app.config import Settings
from app.eval import Evaluator, _context_recall, _parse_scores
from app.tracing import Tracer


class _Msg:
    def __init__(self, content):
        self.content = content


class _Resp:
    def __init__(self, content):
        self.choices = [type("C", (), {"message": _Msg(content)})()]


class ScriptedJudge:
    """Returns canned judge replies in order (the last one repeats)."""

    def __init__(self, replies):
        self._replies = replies
        self.i = 0
        self.calls = []
        self.chat = type("X", (), {"completions": self})()

    async def create(self, model, messages):
        self.calls.append(messages)
        r = self._replies[min(self.i, len(self._replies) - 1)]
        self.i += 1
        return _Resp(r)


async def _pipeline_with(*docs):
    embedder, store = FakeEmbedder(), FakeStore()
    for i, text in enumerate(docs):
        await store.upsert(await embedder.embed([text]), [text], f"doc{i}")
    return FakePipeline(store, embedder)


async def test_evaluate_aggregates_scores():
    pipeline = await _pipeline_with("llmaker runs a local rag stack")
    judge = ScriptedJudge(['{"groundedness": 1.0, "relevance": 0.8}'])
    ev = Evaluator(Settings(), pipeline, judge=judge, tracer=Tracer(Settings()))

    out = await ev.evaluate([{"question": "what is llmaker?"}, {"question": "rag?"}])

    assert out["summary"]["cases"] == 2
    assert out["summary"]["means"]["groundedness"] == 1.0
    assert out["summary"]["means"]["relevance"] == 0.8
    # No reference → correctness is not scored.
    assert "correctness" not in out["results"][0]["scores"]


async def test_correctness_scored_only_with_reference():
    pipeline = await _pipeline_with("llmaker rag")
    judge = ScriptedJudge(['{"groundedness": 1.0, "relevance": 1.0, "correctness": 0.5}'])
    ev = Evaluator(Settings(), pipeline, judge=judge, tracer=Tracer(Settings()))

    out = await ev.evaluate([{"question": "q", "reference": "the gold answer"}])

    assert out["results"][0]["scores"]["correctness"] == 0.5


async def test_evaluate_includes_context_recall_when_expected_given():
    pipeline = await _pipeline_with("llmaker rag")
    judge = ScriptedJudge(['{"groundedness": 1.0, "relevance": 1.0}'])
    ev = Evaluator(Settings(), pipeline, judge=judge, tracer=Tracer(Settings()))

    out = await ev.evaluate([{"question": "llmaker?", "expected_sources": ["doc0"]}])

    assert out["results"][0]["scores"]["context_recall"] == 1.0


def test_context_recall_is_a_fraction_with_substring_matching():
    assert _context_recall(["handbook.pdf#1", "faq.txt"], ["handbook.pdf", "missing"]) == 0.5
    assert _context_recall([], ["x"]) == 0.0
    assert _context_recall(["a"], []) == 0.0


def test_parse_scores_tolerates_prose_fences_and_clamps():
    raw = 'Sure!\n```json\n{"groundedness": 0.9, "relevance": 1.2}\n```'
    s = _parse_scores(raw, ["groundedness", "relevance"])
    assert s["groundedness"] == 0.9
    assert s["relevance"] == 1.0  # clamped into [0, 1]
    # Unparseable output degrades to zeros rather than raising.
    assert _parse_scores("no json here", ["groundedness"]) == {"groundedness": 0.0}


async def test_judge_failure_yields_zero_scores():
    pipeline = await _pipeline_with("doc")

    class Boom:
        def __init__(self):
            self.chat = type("X", (), {"completions": self})()

        async def create(self, model, messages):
            raise RuntimeError("judge down")

    ev = Evaluator(Settings(), pipeline, judge=Boom(), tracer=Tracer(Settings()))
    out = await ev.evaluate([{"question": "q"}])
    assert out["results"][0]["scores"] == {"groundedness": 0.0, "relevance": 0.0}


async def test_eval_traces_scores_to_langfuse():
    pipeline = await _pipeline_with("doc")
    fake_lf = FakeLangfuse()
    judge = ScriptedJudge(['{"groundedness": 1.0, "relevance": 0.5}'])
    ev = Evaluator(Settings(), pipeline, judge=judge, tracer=Tracer(Settings(), client=fake_lf))

    await ev.evaluate([{"question": "q"}])

    assert fake_lf.traces
    names = {s["name"] for s in fake_lf.traces[0].scores}
    assert {"groundedness", "relevance"} <= names
