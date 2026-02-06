"""Tests for GPU monitor module."""

import pytest

from c4.gpu.models import GpuBackend, GpuInfo
from c4.gpu.monitor import (
    LocalGpuScheduler,
    NullGpuMonitor,
    reset_gpu_monitor,
)


@pytest.fixture(autouse=True)
def _reset_monitor():
    """Reset GPU monitor singleton between tests."""
    reset_gpu_monitor()
    yield
    reset_gpu_monitor()


class TestNullGpuMonitor:
    def test_gpu_count_is_zero(self):
        monitor = NullGpuMonitor()
        assert monitor.get_gpu_count() == 0

    def test_get_all_gpus_empty(self):
        monitor = NullGpuMonitor()
        assert monitor.get_all_gpus() == []

    def test_get_gpu_info_raises(self):
        monitor = NullGpuMonitor()
        with pytest.raises(IndexError):
            monitor.get_gpu_info(0)

    def test_find_best_gpu_returns_none(self):
        monitor = NullGpuMonitor()
        assert monitor.find_best_gpu() is None

    def test_find_multiple_gpus_empty(self):
        monitor = NullGpuMonitor()
        assert monitor.find_multiple_gpus(count=1) == []


class TestGpuInfo:
    def test_is_available_with_free_vram(self):
        gpu = GpuInfo(index=0, vram_free_gb=2.0)
        assert gpu.is_available is True

    def test_not_available_with_low_vram(self):
        gpu = GpuInfo(index=0, vram_free_gb=0.1)
        assert gpu.is_available is False

    def test_gpu_backend_enum(self):
        assert GpuBackend.CUDA.value == "cuda"
        assert GpuBackend.MPS.value == "mps"
        assert GpuBackend.NONE.value == "none"


class TestLocalGpuScheduler:
    def test_detect_gpus_empty_with_null_monitor(self):
        scheduler = LocalGpuScheduler(monitor=NullGpuMonitor())
        assert scheduler.detect_gpus() == []

    def test_allocate_fails_with_no_gpus(self):
        scheduler = LocalGpuScheduler(monitor=NullGpuMonitor())
        with pytest.raises(RuntimeError, match="Cannot allocate"):
            scheduler.allocate(gpu_count=1)

    def test_release_is_idempotent(self):
        scheduler = LocalGpuScheduler(monitor=NullGpuMonitor())
        scheduler.release([0, 1])  # Should not raise
