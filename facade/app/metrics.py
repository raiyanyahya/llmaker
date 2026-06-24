"""System metrics: CPU/RAM via psutil, GPU/VRAM via NVML when available.

GPU support is optional and best-effort — the facade must run fine on a CPU-only
host (and inside Docker on macOS), so a missing or failing NVML simply yields an
empty GPU list rather than an error.
"""

from __future__ import annotations

from .models import GPUInfo, SystemInfo

try:  # psutil is a hard dependency, but guard so imports never crash the app.
    import psutil
except Exception:  # pragma: no cover - psutil is installed in practice
    psutil = None  # type: ignore[assignment]


def prime() -> None:
    """Prime psutil's CPU counter so the first real reading isn't 0.0."""
    if psutil is not None:
        try:
            psutil.cpu_percent(interval=None)
        except Exception:
            pass


def system_metrics() -> SystemInfo:
    if psutil is None:
        return SystemInfo()
    try:
        cpu = float(psutil.cpu_percent(interval=None))
        vm = psutil.virtual_memory()
        return SystemInfo(
            cpu_percent=cpu,
            memory_used=int(vm.total - vm.available),
            memory_total=int(vm.total),
            memory_percent=float(vm.percent),
            gpus=gpu_metrics(),
        )
    except Exception:
        return SystemInfo()


def gpu_metrics() -> list[GPUInfo]:
    """Return per-GPU utilization/VRAM via NVML, or [] when unavailable."""
    try:
        import pynvml  # provided by the optional nvidia-ml-py package
    except Exception:
        return []

    gpus: list[GPUInfo] = []
    try:
        pynvml.nvmlInit()
        count = pynvml.nvmlDeviceGetCount()
        for i in range(count):
            handle = pynvml.nvmlDeviceGetHandleByIndex(i)
            name = pynvml.nvmlDeviceGetName(handle)
            if isinstance(name, bytes):
                name = name.decode()
            util = pynvml.nvmlDeviceGetUtilizationRates(handle)
            mem = pynvml.nvmlDeviceGetMemoryInfo(handle)
            gpus.append(
                GPUInfo(
                    name=str(name),
                    utilization=float(util.gpu),
                    memory_used=int(mem.used),
                    memory_total=int(mem.total),
                )
            )
    except Exception:
        return gpus
    finally:
        try:
            pynvml.nvmlShutdown()
        except Exception:
            pass
    return gpus
