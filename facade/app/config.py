"""Runtime configuration, sourced entirely from environment variables.

The Go CLI passes these into the container (see internal/cli/env.go), so the
facade is configured the same way whether it's launched by ``llmaker up`` or by
hand with ``docker run -e``.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field

from . import __version__


def _split_csv(value: str) -> list[str]:
    return [v.strip() for v in value.split(",") if v.strip()]


@dataclass
class Settings:
    """Resolved facade settings."""

    backend: str = "ollama"
    name: str = "llmaker"
    default_model: str = ""
    facade_port: int = 8080
    api_key: str = ""
    cors_origins: list[str] = field(default_factory=lambda: ["*"])
    keep_alive: str = "5m"
    ollama_url: str = "http://127.0.0.1:11434"
    llamacpp_url: str = "http://127.0.0.1:8081"
    version: str = __version__

    @classmethod
    def from_env(cls, environ: dict[str, str] | None = None) -> "Settings":
        env = dict(os.environ if environ is None else environ)
        return cls(
            backend=env.get("LLMAKER_BACKEND", "ollama").strip().lower() or "ollama",
            name=env.get("LLMAKER_NAME", "llmaker"),
            default_model=env.get("LLMAKER_DEFAULT_MODEL", "").strip(),
            facade_port=_int(env.get("FACADE_PORT"), 8080),
            api_key=env.get("API_KEY", "").strip(),
            cors_origins=_split_csv(env.get("CORS_ORIGINS", "*")) or ["*"],
            keep_alive=env.get("KEEP_ALIVE", "5m").strip() or "5m",
            ollama_url=env.get("OLLAMA_URL", "http://127.0.0.1:11434").rstrip("/"),
            llamacpp_url=env.get("LLAMACPP_URL", "http://127.0.0.1:8081").rstrip("/"),
        )


def _int(value: str | None, default: int) -> int:
    try:
        return int(value) if value not in (None, "") else default
    except (TypeError, ValueError):
        return default
