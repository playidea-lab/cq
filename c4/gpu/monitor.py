"""GPU monitoring - multi-backend GPU detection and allocation.

Absorbed from piq/core/gpu.py.
Supports CUDA (pynvml), Apple MPS, and CPU-only fallback.
"""

from __future__ import annotations

import logging
import re
import threading
from abc import ABC, abstractmethod

from c4.interfaces import GpuScheduler

from .models import GpuBackend, GpuInfo

logger = logging.getLogger(__name__)

# Thread-safe singleton
_monitor_lock = threading.Lock()
_monitor_instance: BaseGpuMonitor | None = None


class BaseGpuMonitor(ABC):
    """Abstract GPU monitor interface."""

    @abstractmethod
    def get_gpu_count(self) -> int:
        """Return number of available GPUs."""
        ...

    @abstractmethod
    def get_gpu_info(self, index: int) -> GpuInfo:
        """Get info for a specific GPU."""
        ...

    def get_all_gpus(self) -> list[GpuInfo]:
        """Get info for all GPUs."""
        return [self.get_gpu_info(i) for i in range(self.get_gpu_count())]

    def find_best_gpu(self, min_vram_gb: float = 0) -> GpuInfo | None:
        """Find GPU with most free VRAM meeting minimum requirement."""
        gpus = [g for g in self.get_all_gpus() if g.vram_free_gb >= min_vram_gb]
        if not gpus:
            return None
        return max(gpus, key=lambda g: g.vram_free_gb)

    def find_multiple_gpus(
        self,
        count: int,
        min_vram_gb: float = 0,
        gpu_model_pattern: str | None = None,
    ) -> list[GpuInfo]:
        """Find multiple GPUs meeting requirements.

        Args:
            count: Number of GPUs needed
            min_vram_gb: Minimum free VRAM per GPU
            gpu_model_pattern: Regex pattern to match GPU model name

        Returns:
            List of matching GPUs sorted by free VRAM (descending)
        """
        gpus = self.get_all_gpus()

        # Filter by VRAM
        gpus = [g for g in gpus if g.vram_free_gb >= min_vram_gb]

        # Filter by model pattern
        if gpu_model_pattern:
            pattern = re.compile(gpu_model_pattern, re.IGNORECASE)
            gpus = [g for g in gpus if pattern.search(g.name)]

        # Sort by free VRAM descending
        gpus.sort(key=lambda g: g.vram_free_gb, reverse=True)

        if len(gpus) < count:
            return []

        return gpus[:count]


class CudaGpuMonitor(BaseGpuMonitor):
    """NVIDIA GPU monitor via pynvml."""

    def __init__(self) -> None:
        try:
            import pynvml

            pynvml.nvmlInit()
            self._pynvml = pynvml
            self._device_count = pynvml.nvmlDeviceGetCount()
            logger.info("CUDA GPU monitor: %d device(s) found", self._device_count)
        except Exception as e:
            raise RuntimeError(f"Failed to initialize pynvml: {e}") from e

    def get_gpu_count(self) -> int:
        return self._device_count

    def get_gpu_info(self, index: int) -> GpuInfo:
        pynvml = self._pynvml
        handle = pynvml.nvmlDeviceGetHandleByIndex(index)
        name = pynvml.nvmlDeviceGetName(handle)
        if isinstance(name, bytes):
            name = name.decode("utf-8")

        mem = pynvml.nvmlDeviceGetMemoryInfo(handle)
        total_gb = mem.total / (1024**3)
        used_gb = mem.used / (1024**3)
        free_gb = mem.free / (1024**3)

        # Utilization
        try:
            util = pynvml.nvmlDeviceGetUtilizationRates(handle)
            gpu_util = float(util.gpu)
            mem_util = float(util.memory)
        except Exception:
            gpu_util = 0.0
            mem_util = 0.0

        # Temperature
        try:
            temp = float(pynvml.nvmlDeviceGetTemperature(handle, 0))
        except Exception:
            temp = None

        # Power
        try:
            power = pynvml.nvmlDeviceGetPowerUsage(handle) / 1000.0
        except Exception:
            power = None

        # Compute capability
        try:
            major, minor = pynvml.nvmlDeviceGetCudaComputeCapability(handle)
            cc = f"{major}.{minor}"
        except Exception:
            cc = None

        return GpuInfo(
            index=index,
            name=name,
            backend=GpuBackend.CUDA,
            vram_total_gb=round(total_gb, 2),
            vram_used_gb=round(used_gb, 2),
            vram_free_gb=round(free_gb, 2),
            gpu_utilization=gpu_util,
            memory_utilization=mem_util,
            temperature_c=temp,
            power_draw_w=power,
            compute_capability=cc,
        )


class MpsGpuMonitor(BaseGpuMonitor):
    """Apple Metal Performance Shaders GPU monitor."""

    def __init__(self) -> None:
        try:
            import subprocess

            result = subprocess.run(
                ["system_profiler", "SPDisplaysDataType"],
                capture_output=True,
                text=True,
                timeout=5,
            )
            self._gpu_name = "Apple GPU"
            for line in result.stdout.splitlines():
                line = line.strip()
                if "Chipset Model:" in line:
                    self._gpu_name = line.split(":", 1)[1].strip()
                    break

            # Estimate VRAM from system memory (unified memory)
            import os

            total_mem = os.sysconf("SC_PAGE_SIZE") * os.sysconf("SC_PHYS_PAGES")
            self._total_gb = total_mem / (1024**3)
            # MPS typically can use ~75% of unified memory
            self._usable_gb = self._total_gb * 0.75
            logger.info("MPS GPU monitor: %s (%.1f GB unified)", self._gpu_name, self._total_gb)
        except Exception as e:
            raise RuntimeError(f"Failed to detect MPS: {e}") from e

    def get_gpu_count(self) -> int:
        return 1

    def get_gpu_info(self, index: int) -> GpuInfo:
        if index != 0:
            raise IndexError(f"MPS only has 1 GPU, got index {index}")

        # Estimate free memory (rough - unified memory makes this imprecise)
        try:
            import psutil

            mem = psutil.virtual_memory()
            free_gb = mem.available / (1024**3) * 0.75
        except ImportError:
            free_gb = self._usable_gb * 0.5  # Rough estimate

        return GpuInfo(
            index=0,
            name=self._gpu_name,
            backend=GpuBackend.MPS,
            vram_total_gb=round(self._usable_gb, 2),
            vram_used_gb=round(self._usable_gb - free_gb, 2),
            vram_free_gb=round(free_gb, 2),
        )


class NullGpuMonitor(BaseGpuMonitor):
    """CPU-only fallback monitor (no GPU available)."""

    def get_gpu_count(self) -> int:
        return 0

    def get_gpu_info(self, index: int) -> GpuInfo:
        raise IndexError("No GPUs available (NullGpuMonitor)")


def detect_backend() -> GpuBackend:
    """Auto-detect available GPU backend."""
    # Try CUDA first
    try:
        import pynvml

        pynvml.nvmlInit()
        count = pynvml.nvmlDeviceGetCount()
        if count > 0:
            pynvml.nvmlShutdown()
            return GpuBackend.CUDA
        pynvml.nvmlShutdown()
    except Exception:
        pass

    # Try MPS (Apple Silicon)
    try:
        import torch

        if hasattr(torch.backends, "mps") and torch.backends.mps.is_available():
            return GpuBackend.MPS
    except ImportError:
        pass

    # Fallback: check for Apple Silicon without torch
    try:
        import platform

        if platform.processor() == "arm" and platform.system() == "Darwin":
            return GpuBackend.MPS
    except Exception:
        pass

    return GpuBackend.NONE


def get_gpu_monitor(backend: GpuBackend | None = None) -> BaseGpuMonitor:
    """Get or create thread-safe singleton GPU monitor.

    Args:
        backend: Force specific backend (auto-detect if None)

    Returns:
        GPU monitor instance
    """
    global _monitor_instance

    with _monitor_lock:
        if _monitor_instance is not None:
            return _monitor_instance

        if backend is None:
            backend = detect_backend()

        if backend == GpuBackend.CUDA:
            try:
                _monitor_instance = CudaGpuMonitor()
            except Exception:
                logger.warning("CUDA init failed, falling back to NullGpuMonitor")
                _monitor_instance = NullGpuMonitor()
        elif backend == GpuBackend.MPS:
            try:
                _monitor_instance = MpsGpuMonitor()
            except Exception:
                logger.warning("MPS init failed, falling back to NullGpuMonitor")
                _monitor_instance = NullGpuMonitor()
        else:
            _monitor_instance = NullGpuMonitor()

        return _monitor_instance


def reset_gpu_monitor() -> None:
    """Reset singleton (for testing)."""
    global _monitor_instance
    with _monitor_lock:
        _monitor_instance = None


class LocalGpuScheduler(GpuScheduler):
    """Local GPU scheduler - single machine implementation."""

    def __init__(self, monitor: BaseGpuMonitor | None = None) -> None:
        self._monitor = monitor or get_gpu_monitor()
        self._allocated: set[int] = set()

    def detect_gpus(self) -> list[dict]:
        return [g.model_dump() for g in self._monitor.get_all_gpus()]

    def allocate(self, gpu_count: int = 1, min_vram_gb: float = 8.0) -> list[int]:
        gpus = self._monitor.find_multiple_gpus(
            count=gpu_count, min_vram_gb=min_vram_gb
        )
        if not gpus:
            raise RuntimeError(
                f"Cannot allocate {gpu_count} GPU(s) with {min_vram_gb}GB VRAM"
            )
        ids = [g.index for g in gpus]
        self._allocated.update(ids)
        return ids

    def release(self, gpu_ids: list[int]) -> None:
        self._allocated -= set(gpu_ids)
