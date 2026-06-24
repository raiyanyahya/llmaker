"""Entrypoint: ``python -m app`` / ``llmaker-facade``.

Starts uvicorn bound to all interfaces inside the container on FACADE_PORT.
"""

from __future__ import annotations

from .config import Settings


def main() -> None:
    import uvicorn

    settings = Settings.from_env()
    uvicorn.run(
        "app.main:app",
        host="0.0.0.0",  # noqa: S104 - bound inside the container; host maps the port
        port=settings.facade_port,
        log_level="info",
        access_log=False,
    )


if __name__ == "__main__":
    main()
