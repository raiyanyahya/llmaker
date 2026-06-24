"""An in-memory Adapter for exercising the facade routes without a backend."""

from __future__ import annotations

from app.adapters.base import Adapter
from app.models import InstalledModel, PullEvent, RunningModel


class FakeAdapter(Adapter):
    name = "fake"

    def __init__(self) -> None:
        self.healthy = True
        self.deleted: list[str] = []
        self._installed = [
            InstalledModel(name="llama3:8b", size=4_000_000_000, modified="2024-01-01")
        ]

    async def health(self) -> bool:
        return self.healthy

    async def list_models(self) -> list[InstalledModel]:
        return list(self._installed)

    async def running_models(self) -> list[RunningModel]:
        return [RunningModel(name="llama3:8b", size=4_000_000_000, vram=5_000_000_000)]

    async def pull(self, model: str):
        yield PullEvent(status="downloading", completed=50, total=100)
        yield PullEvent(status="success")
        self._installed.append(InstalledModel(name=model))

    async def delete(self, model: str) -> None:
        self.deleted.append(model)
        self._installed = [m for m in self._installed if m.name != model]

    async def chat(self, payload: dict):
        if payload.get("stream"):

            async def gen():
                yield b'data: {"choices":[{"delta":{"content":"Hello"}}]}\n\n'
                yield b'data: {"choices":[{"delta":{"content":" world"}}]}\n\n'
                yield b"data: [DONE]\n\n"

            return gen()
        return {
            "choices": [{"message": {"role": "assistant", "content": "Hello world"}}],
            "usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
        }

    async def completions(self, payload: dict):
        return {"choices": [{"text": "hello"}]}

    async def embeddings(self, payload: dict) -> dict:
        return {"data": [{"embedding": [0.1, 0.2, 0.3]}]}

    async def openai_models(self) -> dict:
        return {"object": "list", "data": [{"id": "llama3:8b", "object": "model"}]}
