"""Run the agent: ``python -m app`` (or the ``llmaker-agent`` script)."""

from __future__ import annotations

import uvicorn

from .config import load_settings


def main() -> None:
    settings = load_settings()
    uvicorn.run("app.main:app", host="0.0.0.0", port=settings.port, log_level="info")


if __name__ == "__main__":
    main()
