"""Backend adapters.

Each adapter normalizes one inference engine to the facade contract. Adding a
backend means writing one adapter here — nothing else in the facade, the CLI, or
a user's app changes.
"""

from __future__ import annotations

from ..config import Settings
from .base import Adapter


def build_adapter(settings: Settings) -> Adapter:
    """Construct the adapter for the configured backend."""
    backend = settings.backend.lower()
    if backend in ("llamacpp", "llama.cpp", "llama-cpp"):
        from .llamacpp import LlamaCppAdapter

        return LlamaCppAdapter(settings)
    # Default and fallback: Ollama.
    from .ollama import OllamaAdapter

    return OllamaAdapter(settings)


__all__ = ["Adapter", "build_adapter"]
