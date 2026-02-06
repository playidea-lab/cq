"""Tests for GPU job scheduler."""

import os
from unittest.mock import MagicMock

import pytest

from c4.gpu.models import GpuBackend, GpuInfo
from c4.gpu.scheduler import (
    FailureType,
    GpuJob,
    GpuJobScheduler,
    JobStatus,
)


@pytest.fixture
def jobs_dir(tmp_path):
    return tmp_path / "gpu_jobs"


@pytest.fixture
def mock_monitor():
    """GPU monitor that reports 2 fake GPUs."""
    monitor = MagicMock()
    gpus = [
        GpuInfo(index=0, name="FakeGPU-0", backend=GpuBackend.CUDA, vram_total_gb=16, vram_used_gb=2, vram_free_gb=14),
        GpuInfo(index=1, name="FakeGPU-1", backend=GpuBackend.CUDA, vram_total_gb=16, vram_used_gb=8, vram_free_gb=8),
    ]
    monitor.get_all_gpus.return_value = gpus
    monitor.get_gpu_count.return_value = 2
    monitor.find_multiple_gpus.return_value = gpus
    return monitor


@pytest.fixture
def scheduler(jobs_dir, mock_monitor):
    return GpuJobScheduler(
        jobs_dir=jobs_dir,
        gpu_monitor=mock_monitor,
        max_concurrent=2,
        poll_interval=0.1,
    )


class TestGpuJob:
    def test_job_creation(self):
        job = GpuJob(
            job_id="gpu-T-001-0-1",
            task_id="T-001-0",
            command="python train.py",
            gpu_count=2,
            min_vram_gb=8.0,
            parallelism="ddp",
        )
        assert job.status == JobStatus.QUEUED
        assert job.requires_gpu is True
        assert job.is_terminal is False
        assert job.gpu_count == 2

    def test_terminal_states(self):
        job = GpuJob(job_id="j1", task_id="T-001-0", command="echo hi")
        assert job.is_terminal is False

        job.status = JobStatus.SUCCEEDED
        assert job.is_terminal is True

        job.status = JobStatus.FAILED
        assert job.is_terminal is True

        job.status = JobStatus.CANCELLED
        assert job.is_terminal is True


class TestGpuJobScheduler:
    def test_submit(self, scheduler):
        job = scheduler.submit(
            task_id="T-001-0",
            command="python train.py",
            gpu_count=1,
        )
        assert job.status == JobStatus.QUEUED
        assert job.task_id == "T-001-0"
        assert "T-001-0" in job.job_id

    def test_submit_creates_job_dir(self, scheduler):
        job = scheduler.submit(task_id="T-002-0", command="echo test")
        job_dir = scheduler._get_job_dir(job.job_id)
        assert job_dir.exists()

    def test_cancel_queued_job(self, scheduler):
        job = scheduler.submit(task_id="T-003-0", command="sleep 100")
        result = scheduler.cancel(job.job_id)
        assert result.status == JobStatus.CANCELLED
        assert result.is_terminal is True

    def test_cancel_nonexistent(self, scheduler):
        assert scheduler.cancel("nonexistent") is None

    def test_list_jobs(self, scheduler):
        scheduler.submit(task_id="T-001-0", command="echo a")
        scheduler.submit(task_id="T-001-0", command="echo b")
        scheduler.submit(task_id="T-002-0", command="echo c")

        all_jobs = scheduler.list_jobs()
        assert len(all_jobs) == 3

        task1_jobs = scheduler.list_jobs(task_id="T-001-0")
        assert len(task1_jobs) == 2

    def test_get_job(self, scheduler):
        job = scheduler.submit(task_id="T-001-0", command="echo test")
        found = scheduler.get_job(job.job_id)
        assert found is not None
        assert found.job_id == job.job_id

    def test_get_job_not_found(self, scheduler):
        assert scheduler.get_job("nonexistent") is None


class TestGpuAllocation:
    def test_allocate_gpus_success(self, scheduler, mock_monitor):
        job = GpuJob(
            job_id="j1", task_id="T-001-0", command="echo",
            gpu_count=2, min_vram_gb=8.0,
        )
        indices = scheduler._allocate_gpus(job)
        assert indices is not None
        assert len(indices) == 2
        mock_monitor.find_multiple_gpus.assert_called_once()

    def test_allocate_gpus_insufficient(self, scheduler, mock_monitor):
        mock_monitor.find_multiple_gpus.return_value = []
        job = GpuJob(
            job_id="j1", task_id="T-001-0", command="echo",
            gpu_count=4, min_vram_gb=16.0,
        )
        assert scheduler._allocate_gpus(job) is None

    def test_allocate_zero_gpus(self, scheduler):
        job = GpuJob(
            job_id="j1", task_id="T-001-0", command="echo",
            gpu_count=0,
        )
        assert scheduler._allocate_gpus(job) == []


class TestBuildLaunchCommand:
    def test_single_gpu_command(self, scheduler):
        job = GpuJob(
            job_id="j1", task_id="T-001-0",
            command="python train.py", parallelism="single",
        )
        job.gpu_indices = [0]
        cmd = scheduler._build_launch_command(job, {})
        assert cmd == "python train.py"

    def test_ddp_multi_gpu(self, scheduler):
        job = GpuJob(
            job_id="j1", task_id="T-001-0",
            command="python train.py", parallelism="ddp",
        )
        job.gpu_indices = [0, 1]
        cmd = scheduler._build_launch_command(job, {"MASTER_PORT": "29500"})
        assert "torchrun" in cmd
        assert "--nproc_per_node=2" in cmd

    def test_deepspeed_multi_gpu(self, scheduler):
        job = GpuJob(
            job_id="j1", task_id="T-001-0",
            command="python train.py", parallelism="deepspeed",
        )
        job.gpu_indices = [0, 1, 2, 3]
        cmd = scheduler._build_launch_command(job, {})
        assert "deepspeed" in cmd
        assert "--num_gpus=4" in cmd


class TestFailureDetection:
    def test_oom_exit_code_137(self, scheduler):
        ft = scheduler._detect_failure_type("nonexistent", 137)
        assert ft == FailureType.OOM

    def test_oom_exit_code_neg9(self, scheduler):
        ft = scheduler._detect_failure_type("nonexistent", -9)
        assert ft == FailureType.OOM

    def test_signal_negative_exit(self, scheduler):
        ft = scheduler._detect_failure_type("nonexistent", -15)
        assert ft == FailureType.SIGNAL

    def test_runtime_error(self, scheduler):
        ft = scheduler._detect_failure_type("nonexistent", 1)
        assert ft == FailureType.RUNTIME

    def test_oom_from_log(self, scheduler):
        job = scheduler.submit(task_id="T-001-0", command="echo test")
        log_path = scheduler._get_job_dir(job.job_id) / "output.log"
        log_path.write_text("Training...\nCUDA out of memory\nExiting\n")
        assert scheduler._detect_oom_from_log(job.job_id) is True

    def test_no_oom_from_log(self, scheduler):
        job = scheduler.submit(task_id="T-001-0", command="echo test")
        log_path = scheduler._get_job_dir(job.job_id) / "output.log"
        log_path.write_text("Training completed successfully\n")
        assert scheduler._detect_oom_from_log(job.job_id) is False


class TestLogAccess:
    def test_read_log(self, scheduler):
        job = scheduler.submit(task_id="T-001-0", command="echo test")
        log_path = scheduler._get_job_dir(job.job_id) / "output.log"
        log_path.write_text("line 1\nline 2\nline 3\n")
        lines = scheduler.read_log(job.job_id)
        assert len(lines) == 3
        assert lines[0] == "line 1"

    def test_read_log_tail(self, scheduler):
        job = scheduler.submit(task_id="T-001-0", command="echo test")
        log_path = scheduler._get_job_dir(job.job_id) / "output.log"
        log_path.write_text("\n".join(f"line {i}" for i in range(100)))
        lines = scheduler.read_log(job.job_id, tail=10)
        assert len(lines) == 10
        assert lines[-1] == "line 99"

    def test_read_log_nonexistent(self, scheduler):
        assert scheduler.read_log("nonexistent") == []


class TestRecovery:
    def test_save_and_check_pid(self, scheduler):
        job = scheduler.submit(task_id="T-001-0", command="echo test")
        scheduler._save_pid_file(job.job_id, os.getpid())

        pid_path = scheduler._get_job_dir(job.job_id) / "pid"
        assert pid_path.exists()
        assert int(pid_path.read_text().strip()) == os.getpid()

    def test_is_process_alive(self):
        # Current process should be alive
        assert GpuJobScheduler._is_process_alive(os.getpid()) is True
        # Non-existent PID
        assert GpuJobScheduler._is_process_alive(999999999) is False


class TestDiskSpaceCheck:
    def test_disk_space_check(self, scheduler):
        # Should pass with very low threshold
        assert scheduler._check_disk_space(min_gb=0.001) is True

    def test_disk_space_low(self, scheduler):
        # Should fail with impossibly high threshold
        assert scheduler._check_disk_space(min_gb=999999) is False


class TestSchedulerStartStop:
    def test_start_stop(self, scheduler):
        scheduler.start(recover=False)
        assert scheduler.is_running is True
        scheduler.stop(timeout=2)
        assert scheduler.is_running is False

    def test_double_start(self, scheduler):
        scheduler.start(recover=False)
        scheduler.start(recover=False)  # Should be idempotent
        assert scheduler.is_running is True
        scheduler.stop(timeout=2)
