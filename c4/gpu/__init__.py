"""C4 GPU Support - GPU monitoring, scheduling, and multi-GPU launchers.

Absorbed from piq/core/gpu.py and piq/core/scheduler.py.
"""

from .models import GpuBackend, GpuInfo
from .monitor import detect_backend, get_gpu_monitor
from .scheduler import GpuJobScheduler

__all__ = [
    "GpuBackend",
    "GpuInfo",
    "GpuJobScheduler",
    "detect_backend",
    "get_gpu_monitor",
]
