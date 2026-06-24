"""Maximal Marginal Relevance (MMR) reranking.

Vector search returns the most *similar* chunks, which are often near-duplicates.
MMR re-picks a set that balances relevance to the query against diversity among
the picks, so the LLM sees broader context. It needs only the candidate vectors
we already have — no extra model.
"""

from __future__ import annotations

import math


def _cosine(a: list[float], b: list[float]) -> float:
    if not a or not b:
        return 0.0
    dot = sum(x * y for x, y in zip(a, b, strict=False))
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(y * y for y in b))
    if na == 0 or nb == 0:
        return 0.0
    return dot / (na * nb)


def mmr(
    query_vec: list[float],
    candidates: list[dict],
    k: int,
    lambda_: float = 0.5,
) -> list[dict]:
    """Select up to k candidates by MMR.

    candidates each carry a ``vector``. lambda_ trades relevance (1.0) against
    diversity (0.0). Candidates without a vector fall back to plain order.
    """
    pool = [c for c in candidates if c.get("vector")]
    if not pool or not query_vec:
        return candidates[:k]

    relevance = {id(c): _cosine(query_vec, c["vector"]) for c in pool}
    selected: list[dict] = []
    remaining = list(pool)

    while remaining and len(selected) < k:
        best, best_score = None, -math.inf
        for c in remaining:
            redundancy = max((_cosine(c["vector"], s["vector"]) for s in selected), default=0.0)
            score = lambda_ * relevance[id(c)] - (1 - lambda_) * redundancy
            if score > best_score:
                best, best_score = c, score
        selected.append(best)
        remaining.remove(best)
    return selected
