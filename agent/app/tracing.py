"""Optional Langfuse tracing for the RAG pipeline.

Observability must never break a chat, so every interaction with the Langfuse
SDK is best-effort: if the client can't be built, or a span call raises, we
quietly fall back to no tracing. Tracing is enabled only when both Langfuse keys
are configured.

The helpers operate on whatever the SDK's ``trace`` object is, which keeps the
pipeline code readable (``span = safe_span(trace, ...)``) and lets tests inject a
fake client that records the calls.
"""

from __future__ import annotations

from .config import Settings


class Tracer:
    def __init__(self, settings: Settings, client: object | None = None) -> None:
        self._client = client
        if self._client is None and settings.tracing_enabled():
            try:
                from langfuse import Langfuse

                self._client = Langfuse(
                    public_key=settings.langfuse_public_key,
                    secret_key=settings.langfuse_secret_key,
                    host=settings.langfuse_host,
                )
            except Exception:
                self._client = None

    @property
    def enabled(self) -> bool:
        return self._client is not None

    def trace(self, **kwargs) -> object | None:
        if self._client is None:
            return None
        try:
            return self._client.trace(**kwargs)
        except Exception:
            return None

    def flush(self) -> None:
        if self._client is None:
            return
        try:
            self._client.flush()
        except Exception:
            pass


def safe_span(trace: object | None, **kwargs) -> object | None:
    if trace is None:
        return None
    try:
        return trace.span(**kwargs)
    except Exception:
        return None


def safe_generation(trace: object | None, **kwargs) -> object | None:
    if trace is None:
        return None
    try:
        return trace.generation(**kwargs)
    except Exception:
        return None


def safe_end(obj: object | None, **kwargs) -> None:
    if obj is None:
        return
    try:
        obj.end(**kwargs)
    except Exception:
        pass


def safe_update(obj: object | None, **kwargs) -> None:
    if obj is None:
        return
    try:
        obj.update(**kwargs)
    except Exception:
        pass


def safe_score(trace: object | None, **kwargs) -> None:
    """Attach a numeric score to a trace (used by the evaluation harness). Like
    everything here, best-effort: a missing client or SDK quirk never fails a run."""
    if trace is None:
        return
    try:
        trace.score(**kwargs)
    except Exception:
        pass
