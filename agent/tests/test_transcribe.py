"""Transcriber: proxies audio to an OpenAI-compatible Whisper endpoint."""

from app.config import Settings
from app.transcribe import Transcriber


class _Resp:
    def __init__(self, payload):
        self._payload = payload

    def raise_for_status(self):
        pass

    def json(self):
        return self._payload


class _FakeHTTP:
    def __init__(self, payload):
        self._payload = payload
        self.posts = []

    async def post(self, url, files=None, data=None):
        self.posts.append((url, files, data))
        return _Resp(self._payload)


async def test_transcribe_proxies_to_whisper():
    http = _FakeHTTP({"text": "hello world"})
    settings = Settings(whisper_url="http://whisper:8000/", whisper_model="small")
    t = Transcriber(settings, client=http)

    out = await t.transcribe("clip.wav", b"RIFF....", "audio/wav")

    assert out == {"text": "hello world"}
    url, files, data = http.posts[0]
    assert url == "http://whisper:8000/v1/audio/transcriptions"
    assert data["model"] == "small"
    assert files["file"][0] == "clip.wav"
    assert files["file"][1] == b"RIFF...."
