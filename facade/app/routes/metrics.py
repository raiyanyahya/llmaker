"""Prometheus metrics — a text exposition endpoint so every instance is
scrapeable by Prometheus/Grafana.

It exposes the same serving and host signals the dashboard already uses
(requests, tokens/sec, CPU/RAM/GPU), formatted as Prometheus text. Like
``/api/health`` it is intentionally unauthenticated so a scraper needs no
credentials; it carries only aggregate counters and host gauges — no secrets —
and the facade binds to loopback by default.
"""

from __future__ import annotations

from fastapi import APIRouter, Request, Response

from ..metrics import system_metrics

router = APIRouter()

# The Prometheus text exposition content type (version 0.0.4).
CONTENT_TYPE = "text/plain; version=0.0.4; charset=utf-8"


def _escape(value: str) -> str:
    """Escape a Prometheus label value (backslash, quote, newline)."""
    return value.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")


def _series(
    lines: list[str], name: str, mtype: str, help_: str, samples: list[tuple[str, object]]
) -> None:
    """Append one metric family: its HELP/TYPE header and every labeled sample."""
    lines.append(f"# HELP {name} {help_}")
    lines.append(f"# TYPE {name} {mtype}")
    for labels, value in samples:
        lines.append(f"{name}{labels} {value}")


@router.get("/metrics", include_in_schema=False)
async def metrics(request: Request) -> Response:
    st = request.app.state.app_state
    settings = request.app.state.settings
    sys = system_metrics()

    lines: list[str] = []
    info_labels = (
        f'{{name="{_escape(settings.name)}",'
        f'backend="{_escape(settings.backend)}",'
        f'version="{_escape(settings.version)}"}}'
    )
    _series(lines, "llmaker_info", "gauge", "Instance metadata (always 1).", [(info_labels, 1)])
    _series(lines, "llmaker_up", "gauge", "1 when the facade is serving.", [("", 1)])
    _series(
        lines,
        "llmaker_uptime_seconds",
        "gauge",
        "Facade uptime in seconds.",
        [("", f"{st.uptime_seconds():.0f}")],
    )
    _series(
        lines,
        "llmaker_requests_total",
        "counter",
        "Total inference requests handled.",
        [("", st.requests)],
    )
    _series(
        lines,
        "llmaker_tokens_per_second",
        "gauge",
        "Recent generation throughput (tokens/sec).",
        [("", f"{st.tokens_per_second:.3f}")],
    )
    _series(
        lines,
        "llmaker_cpu_percent",
        "gauge",
        "Host CPU utilization (0-100).",
        [("", f"{sys.cpu_percent:.1f}")],
    )
    _series(
        lines,
        "llmaker_memory_used_bytes",
        "gauge",
        "Host memory used, in bytes.",
        [("", sys.memory_used)],
    )
    _series(
        lines,
        "llmaker_memory_total_bytes",
        "gauge",
        "Host memory total, in bytes.",
        [("", sys.memory_total)],
    )

    if sys.gpus:

        def by_gpu(attr: str) -> list[tuple[str, object]]:
            return [(f'{{gpu="{i}"}}', getattr(g, attr)) for i, g in enumerate(sys.gpus)]

        _series(
            lines,
            "llmaker_gpu_utilization",
            "gauge",
            "GPU utilization (0-100).",
            by_gpu("utilization"),
        )
        _series(
            lines,
            "llmaker_gpu_memory_used_bytes",
            "gauge",
            "GPU memory used, in bytes.",
            by_gpu("memory_used"),
        )
        _series(
            lines,
            "llmaker_gpu_memory_total_bytes",
            "gauge",
            "GPU memory total, in bytes.",
            by_gpu("memory_total"),
        )

    return Response(content="\n".join(lines) + "\n", media_type=CONTENT_TYPE)
