"""GPU data models."""

from __future__ import annotations

from enum import Enum

from pydantic import BaseModel, Field


class GpuBackend(str, Enum):
    """GPU backend types."""

    CUDA = "cuda"
    MPS = "mps"
    NONE = "none"


class GpuInfo(BaseModel):
    """GPU device information."""

    index: int = Field(description="GPU device index")
    name: str = Field(default="", description="GPU model name")
    backend: GpuBackend = Field(default=GpuBackend.NONE)

    # Memory (GB)
    vram_total_gb: float = Field(default=0.0, ge=0)
    vram_used_gb: float = Field(default=0.0, ge=0)
    vram_free_gb: float = Field(default=0.0, ge=0)

    # Utilization (%)
    gpu_utilization: float = Field(default=0.0, ge=0, le=100)
    memory_utilization: float = Field(default=0.0, ge=0, le=100)

    # Hardware
    temperature_c: float | None = None
    power_draw_w: float | None = None
    compute_capability: str | None = Field(
        None, description="CUDA compute capability (e.g., '8.0' for Ampere)"
    )

    @property
    def is_available(self) -> bool:
        """Check if GPU has usable free VRAM."""
        return self.vram_free_gb > 0.5  # At least 500MB free


class MigInstanceInfo(BaseModel):
    """Multi-Instance GPU (MIG) partition info for Ampere+ GPUs."""

    parent_gpu_index: int
    gi_id: int = Field(description="GPU Instance ID")
    ci_id: int = Field(description="Compute Instance ID")
    vram_gb: float = Field(ge=0)
    name: str = ""
