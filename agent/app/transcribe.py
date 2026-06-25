"""Speech-to-text by proxying to an OpenAI-compatible Whisper endpoint.

The agent doesn't run the model itself — it forwards uploaded audio to the
in-network ``whisper`` service (faster-whisper-server, which exposes the OpenAI
``/v1/audio/transcriptions`` contract) and returns its JSON. Enabled only when
WHISPER_URL is set, like the other opt-in integrations. A client can be injected
for tests; otherwise one is created per call so nothing is left open.
"""

from __future__ import annotations

from .config import Settings


class Transcriber:
    def __init__(self, settings: Settings, client=None) -> None:
        self._settings = settings
        self._client = client

    async def transcribe(
        self, filename: str, content: bytes, content_type: str | None = None
    ) -> dict:
        url = self._settings.whisper_url.rstrip("/") + "/v1/audio/transcriptions"
        files = {"file": (filename or "audio", content, content_type or "application/octet-stream")}
        data = {"model": self._settings.whisper_model, "response_format": "json"}

        async def _post(http) -> dict:
            resp = await http.post(url, files=files, data=data)
            resp.raise_for_status()
            return resp.json()

        if self._client is not None:
            return await _post(self._client)
        import httpx

        async with httpx.AsyncClient(timeout=120.0) as http:
            return await _post(http)
