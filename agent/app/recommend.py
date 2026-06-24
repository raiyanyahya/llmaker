"""Recommendation helpers.

The recommender is pure vector similarity: items are embedded once, then a query
(free text) or a set of liked items (a taste profile) becomes a vector we search
the item set with. The only math here is building that profile vector.
"""

from __future__ import annotations


def centroid(vectors: list[list[float]]) -> list[float]:
    """Average several vectors into one 'taste profile'. Returns [] if empty."""
    vectors = [v for v in vectors if v]
    if not vectors:
        return []
    n = len(vectors)
    dim = len(vectors[0])
    out = [0.0] * dim
    for v in vectors:
        for i in range(min(dim, len(v))):
            out[i] += v[i]
    return [x / n for x in out]
