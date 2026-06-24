"""The adapter contract every backend implements."""

from __future__ import annotations

from abc import ABC, abstractmethod
from collections.abc import AsyncIterator

from ..models import InstalledModel, PullEvent, RunningModel

# A chat/completions result is either a full JSON dict (non-streaming) or an
# async iterator of Server-Sent-Event byte chunks (streaming). Routes duck-type
# on ``__aiter__`` to decide how to respond.


class Adapter(ABC):
    """Normalizes one inference engine to the facade's stable contract."""

    name: str = "unknown"

    @abstractmethod
    async def health(self) -> bool:
        """Return True when the backend is ready to serve."""

    @abstractmethod
    async def list_models(self) -> list[InstalledModel]:
        """Models installed on disk."""

    @abstractmethod
    async def running_models(self) -> list[RunningModel]:
        """Models currently loaded in (V)RAM."""

    @abstractmethod
    def pull(self, model: str) -> AsyncIterator[PullEvent]:
        """Download a model, yielding normalized progress events."""

    @abstractmethod
    async def delete(self, model: str) -> None:
        """Remove an installed model."""

    @abstractmethod
    async def chat(self, payload: dict):
        """OpenAI-compatible chat. Returns a dict or an async byte iterator."""

    @abstractmethod
    async def completions(self, payload: dict):
        """OpenAI-compatible text completion."""

    @abstractmethod
    async def embeddings(self, payload: dict) -> dict:
        """OpenAI-compatible embeddings (never streamed)."""

    @abstractmethod
    async def openai_models(self) -> dict:
        """OpenAI ``/v1/models`` listing."""

    async def aclose(self) -> None:
        """Release any resources (HTTP clients). Default: no-op."""
        return None
