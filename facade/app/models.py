"""Pydantic schemas for the normalized facade contract.

These mirror the Go client structs in internal/facade/types.go — the two sides
are kept in lockstep so the CLI and the facade agree on the wire format.
"""

from __future__ import annotations

from pydantic import BaseModel, Field


class HealthResponse(BaseModel):
    status: str = "ok"
    ready: bool = True


class GPUInfo(BaseModel):
    name: str
    utilization: float = 0.0
    memory_used: int = 0
    memory_total: int = 0


class SystemInfo(BaseModel):
    cpu_percent: float = 0.0
    memory_used: int = 0
    memory_total: int = 0
    memory_percent: float = 0.0
    gpus: list[GPUInfo] = Field(default_factory=list)


class InstalledModel(BaseModel):
    name: str
    size: int = 0
    modified: str = ""


class RunningModel(BaseModel):
    name: str
    size: int = 0
    vram: int = 0


class ModelsInfo(BaseModel):
    default: str = ""
    running: list[RunningModel] = Field(default_factory=list)
    installed: list[InstalledModel] = Field(default_factory=list)


class InstanceInfo(BaseModel):
    name: str = ""
    backend: str = ""
    version: str = ""
    uptime_seconds: float = 0.0
    default_model: str = ""


class MetricsInfo(BaseModel):
    tokens_per_second: float = 0.0
    requests_total: int = 0


class StatusResponse(BaseModel):
    instance: InstanceInfo
    system: SystemInfo
    models: ModelsInfo
    metrics: MetricsInfo


class ModelList(BaseModel):
    default: str = ""
    models: list[InstalledModel] = Field(default_factory=list)


class PullRequest(BaseModel):
    model: str


class ModelActionRequest(BaseModel):
    model: str


class PullEvent(BaseModel):
    status: str = ""
    digest: str = ""
    completed: int = 0
    total: int = 0
    error: str = ""
