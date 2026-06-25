"""Extractor: requested-fields-only JSON, with defensive parsing."""

from fakes import ScriptedChat

from app.config import Settings
from app.extract import Extractor, _json_object
from app.tracing import Tracer


def _extractor(content):
    return Extractor(Settings(), llm=ScriptedChat([content]), tracer=Tracer(Settings()))


async def test_extract_returns_requested_fields():
    ex = _extractor('{"name": "Ada", "year": 1843}')
    out = await ex.extract("Ada Lovelace, 1843", {"name": "the person", "year": "the year"})
    assert out["data"] == {"name": "Ada", "year": 1843}


async def test_extract_fills_missing_with_null_and_drops_extras():
    ex = _extractor('{"name": "Ada", "bogus": 1}')
    out = await ex.extract("x", {"name": "n", "email": "e"})
    assert out["data"] == {"name": "Ada", "email": None}  # email→null, bogus dropped


async def test_extract_tolerates_prose_and_fences():
    ex = _extractor('Sure! ```json\n{"a": 1}\n```')
    out = await ex.extract("x", {"a": "the a"})
    assert out["data"] == {"a": 1}


async def test_extract_handles_unparseable_output():
    ex = _extractor("sorry, I can't")
    out = await ex.extract("x", {"a": "the a"})
    assert out["data"] == {"a": None}  # degrades, never raises


def test_json_object_parsing():
    assert _json_object('prefix {"x": 1} suffix') == {"x": 1}
    assert _json_object("no object here") == {}
    assert _json_object("[1, 2, 3]") == {}  # arrays are not objects
