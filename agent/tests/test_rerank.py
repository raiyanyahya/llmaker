from app.rerank import mmr


def _c(vec, text):
    return {"vector": vec, "text": text, "source": "s"}


def test_mmr_picks_relevant_first():
    query = [1.0, 0.0]
    cands = [_c([0.1, 1.0], "off"), _c([1.0, 0.0], "on"), _c([0.9, 0.1], "near")]
    out = mmr(query, cands, k=1, lambda_=1.0)  # pure relevance
    assert out[0]["text"] == "on"


def test_mmr_prefers_diversity_over_duplicates():
    query = [1.0, 0.0, 0.0]
    # "a" and "dupe" are identical (redundancy 1.0); "diverse" is a bit less
    # relevant but adds new information.
    cands = [
        _c([0.9, 0.1, 0.0], "a"),
        _c([0.9, 0.1, 0.0], "dupe"),
        _c([0.5, 0.0, 0.5], "diverse"),
    ]
    out = mmr(query, cands, k=2, lambda_=0.5)
    texts = [c["text"] for c in out]
    # The second pick is the diverse chunk, not the exact duplicate.
    assert "a" in texts
    assert "diverse" in texts
    assert "dupe" not in texts


def test_mmr_handles_missing_vectors():
    out = mmr([1.0, 0.0], [{"text": "x"}, {"text": "y"}], k=1)
    assert len(out) == 1


def test_mmr_empty_query_falls_back_to_order():
    cands = [_c([1.0], "a"), _c([1.0], "b")]
    assert mmr([], cands, k=1) == cands[:1]
