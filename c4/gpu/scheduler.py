"""GPU Job Scheduler - subprocess execution with GPU allocation.

Absorbed from piq/core/scheduler.py.
Handles job queuing, GPU allocation, process management,
timeout enforcement, failure detection, and PID-based recovery.
"""

from __future__ import annotations

import logging
import os
import shutil
import signal as sig
import subprocess
import threading
from datetime import datetime, timezone
from enum import Enum
from pathlib import Path

from .monitor import BaseGpuMonitor, get_gpu_monitor

logger = logging.getLogger(__name__)


class JobStatus(str, Enum):
    """GPU job execution status."""

    QUEUED = "queued"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    CANCELLED = "cancelled"
    TIMEOUT = "timeout"


class FailureType(str, Enum):
    """Classification of job failure cause."""

    OOM = "oom"
    SIGNAL = "signal"
    TIMEOUT = "timeout"
    RUNTIME = "runtime"


class GpuJob:
    """A GPU job tracked by the scheduler."""

    def __init__(
        self,
        job_id: str,
        task_id: str,
        command: str,
        workdir: str = ".",
        gpu_count: int = 1,
        min_vram_gb: float = 8.0,
        gpu_model_pattern: str | None = None,
        parallelism: str = "single",
        timeout_minutes: int = 60,
        env: dict[str, str] | None = None,
    ) -> None:
        self.job_id = job_id
        self.task_id = task_id
        self.command = command
        self.workdir = workdir
        self.gpu_count = gpu_count
        self.min_vram_gb = min_vram_gb
        self.gpu_model_pattern = gpu_model_pattern
        self.parallelism = parallelism
        self.timeout_minutes = timeout_minutes
        self.env = env or {}
        self.status = JobStatus.QUEUED
        self.failure_type: FailureType | None = None
        self.exit_code: int | None = None
        self.pid: int | None = None
        self.gpu_indices: list[int] = []
        self.created_at = datetime.now(tz=timezone.utc)
        self.started_at: datetime | None = None
        self.finished_at: datetime | None = None

    @property
    def requires_gpu(self) -> bool:
        return self.gpu_count > 0 and self.parallelism != "single" or self.gpu_count >= 1

    @property
    def is_terminal(self) -> bool:
        return self.status in (JobStatus.SUCCEEDED, JobStatus.FAILED, JobStatus.CANCELLED, JobStatus.TIMEOUT)


class GpuJobScheduler:
    """GPU-aware job scheduler with subprocess management.

    Features:
    - VRAM-based GPU allocation
    - Subprocess execution with CUDA_VISIBLE_DEVICES
    - Log capture to file
    - Timeout enforcement
    - OOM/SIGNAL/TIMEOUT/RUNTIME failure classification
    - PID-based orphan process recovery
    """

    def __init__(
        self,
        jobs_dir: str | Path = ".c4/gpu_jobs",
        gpu_monitor: BaseGpuMonitor | None = None,
        max_concurrent: int = 4,
        poll_interval: float = 2.0,
    ) -> None:
        self.jobs_dir = Path(jobs_dir)
        self.jobs_dir.mkdir(parents=True, exist_ok=True)
        self.gpu = gpu_monitor or get_gpu_monitor()
        self.max_concurrent = max_concurrent
        self.poll_interval = poll_interval

        self._jobs: dict[str, GpuJob] = {}
        self._processes: dict[str, subprocess.Popen] = {}
        self._log_files: dict[str, object] = {}
        self._lock = threading.Lock()
        self._gpu_lock = threading.Lock()
        self._thread: threading.Thread | None = None
        self._stop = threading.Event()
        self._job_counter = 0

    # ========== Job Submission ==========

    def submit(
        self,
        task_id: str,
        command: str,
        workdir: str = ".",
        gpu_count: int = 1,
        min_vram_gb: float = 8.0,
        gpu_model_pattern: str | None = None,
        parallelism: str = "single",
        timeout_minutes: int = 60,
        env: dict[str, str] | None = None,
    ) -> GpuJob:
        """Submit a GPU job to the queue.

        Returns:
            GpuJob instance with status QUEUED
        """
        self._job_counter += 1
        job_id = f"gpu-{task_id}-{self._job_counter}"

        job = GpuJob(
            job_id=job_id,
            task_id=task_id,
            command=command,
            workdir=workdir,
            gpu_count=gpu_count,
            min_vram_gb=min_vram_gb,
            gpu_model_pattern=gpu_model_pattern,
            parallelism=parallelism,
            timeout_minutes=timeout_minutes,
            env=env,
        )

        job_dir = self._get_job_dir(job_id)
        job_dir.mkdir(parents=True, exist_ok=True)

        with self._lock:
            self._jobs[job_id] = job

        logger.info("GPU job submitted: %s (task=%s, gpus=%d)", job_id, task_id, gpu_count)
        return job

    def cancel(self, job_id: str) -> GpuJob | None:
        """Cancel a job."""
        job = self._jobs.get(job_id)
        if not job:
            return None

        if job.status == JobStatus.RUNNING:
            self._kill_job(job_id)

        if not job.is_terminal:
            job.status = JobStatus.CANCELLED
            job.finished_at = datetime.now(tz=timezone.utc)
            logger.info("GPU job cancelled: %s", job_id)

        return job

    def get_job(self, job_id: str) -> GpuJob | None:
        """Get job by ID."""
        return self._jobs.get(job_id)

    def list_jobs(self, task_id: str | None = None) -> list[GpuJob]:
        """List jobs, optionally filtered by task_id."""
        jobs = list(self._jobs.values())
        if task_id:
            jobs = [j for j in jobs if j.task_id == task_id]
        return sorted(jobs, key=lambda j: j.created_at, reverse=True)

    # ========== Scheduler Loop ==========

    def start(self, recover: bool = True) -> None:
        """Start scheduler background thread."""
        if self._thread and self._thread.is_alive():
            return

        if recover:
            self._recover_running_jobs()

        self._stop.clear()
        self._thread = threading.Thread(
            target=self._loop,
            daemon=True,
            name="c4-gpu-scheduler",
        )
        self._thread.start()
        logger.info("GPU scheduler started (max_concurrent=%d)", self.max_concurrent)

    def stop(self, timeout: float = 10.0) -> None:
        """Stop scheduler and kill remaining processes."""
        self._stop.set()
        if self._thread:
            self._thread.join(timeout=timeout)

        with self._lock:
            for job_id in list(self._processes.keys()):
                self._kill_job(job_id)

        logger.info("GPU scheduler stopped")

    @property
    def is_running(self) -> bool:
        return self._thread is not None and self._thread.is_alive()

    @property
    def running_count(self) -> int:
        return len(self._processes)

    def _loop(self) -> None:
        """Main scheduler loop."""
        while not self._stop.is_set():
            try:
                self._check_running_jobs()
                self._schedule_queued_jobs()
            except Exception as e:
                logger.error("Scheduler loop error: %s", e, exc_info=True)
            self._stop.wait(self.poll_interval)

    def _schedule_queued_jobs(self) -> None:
        """Start queued jobs if resources available."""
        running = len(self._processes)
        if running >= self.max_concurrent:
            return

        available = self.max_concurrent - running
        queued = [j for j in self._jobs.values() if j.status == JobStatus.QUEUED]
        queued.sort(key=lambda j: j.created_at)

        for job in queued[:available]:
            with self._gpu_lock:
                gpu_indices = self._allocate_gpus(job)
                if gpu_indices is not None:
                    self._start_job(job, gpu_indices)

    def _check_running_jobs(self) -> None:
        """Check running jobs for completion or timeout."""
        completed: list[tuple[str, int]] = []
        timed_out: list[str] = []

        with self._lock:
            for job_id, process in self._processes.items():
                if process.poll() is not None:
                    completed.append((job_id, process.returncode))
                elif self._is_timed_out(job_id):
                    timed_out.append(job_id)

        for job_id, exit_code in completed:
            self._complete_job(job_id, exit_code)

        for job_id in timed_out:
            self._timeout_job(job_id)

    # ========== GPU Allocation ==========

    def _allocate_gpus(self, job: GpuJob) -> list[int] | None:
        """Allocate GPUs for a job based on VRAM requirements.

        Returns:
            List of GPU indices, or None if insufficient resources.
        """
        if job.gpu_count == 0:
            return []

        gpus = self.gpu.find_multiple_gpus(
            count=job.gpu_count,
            min_vram_gb=job.min_vram_gb,
            gpu_model_pattern=job.gpu_model_pattern,
        )
        if not gpus:
            return None
        return [g.index for g in gpus]

    # ========== Job Execution ==========

    def _start_job(self, job: GpuJob, gpu_indices: list[int]) -> None:
        """Start job as subprocess with GPU environment."""
        if not self._check_disk_space():
            logger.warning("Skipping job %s: low disk space", job.job_id)
            return

        job_dir = self._get_job_dir(job.job_id)
        log_path = job_dir / "output.log"

        env = os.environ.copy()
        env.update(job.env)
        env["C4_TASK_ID"] = job.task_id
        env["C4_JOB_ID"] = job.job_id

        if gpu_indices:
            env["CUDA_VISIBLE_DEVICES"] = ",".join(map(str, gpu_indices))

            if job.parallelism in ("ddp", "fsdp", "deepspeed") or len(gpu_indices) > 1:
                env["WORLD_SIZE"] = str(len(gpu_indices))
                env["LOCAL_WORLD_SIZE"] = str(len(gpu_indices))
                env["MASTER_ADDR"] = "localhost"
                env["MASTER_PORT"] = str(self._get_free_port())
                env["RANK"] = "0"
                env["LOCAL_RANK"] = "0"

        log_file = open(log_path, "w")  # noqa: SIM115

        try:
            command = self._build_launch_command(job, env)

            process = subprocess.Popen(
                command,
                shell=True,
                cwd=job.workdir,
                env=env,
                stdout=log_file,
                stderr=subprocess.STDOUT,
                preexec_fn=os.setsid,
            )

            with self._lock:
                self._processes[job.job_id] = process
                self._log_files[job.job_id] = log_file

            job.status = JobStatus.RUNNING
            job.pid = process.pid
            job.gpu_indices = gpu_indices
            job.started_at = datetime.now(tz=timezone.utc)

            self._save_pid_file(job.job_id, process.pid)

            gpu_str = ",".join(map(str, gpu_indices)) if gpu_indices else "none"
            logger.info("GPU job started: %s (pid=%d, gpus=[%s])", job.job_id, process.pid, gpu_str)

        except Exception as e:
            # Close log_file only if it wasn't registered in _log_files (process failed to start)
            with self._lock:
                if job.job_id not in self._log_files:
                    log_file.close()
            logger.error("Failed to start GPU job %s: %s", job.job_id, e)
            job.status = JobStatus.FAILED
            job.failure_type = FailureType.RUNTIME
            job.finished_at = datetime.now(tz=timezone.utc)

    def _build_launch_command(self, job: GpuJob, env: dict[str, str]) -> str:
        """Build launch command based on parallelism type.

        For multi-GPU with torchrun (DDP/FSDP), wraps the command.
        For DeepSpeed, uses deepspeed launcher.
        Otherwise, returns the raw command.
        """
        gpu_count = len(job.gpu_indices) if job.gpu_indices else 1

        if job.parallelism in ("ddp", "fsdp") and gpu_count > 1:
            master_port = env.get("MASTER_PORT", "29500")
            return (
                f"torchrun --nproc_per_node={gpu_count} "
                f"--master_addr=localhost --master_port={master_port} "
                f"{job.command}"
            )

        if job.parallelism == "deepspeed" and gpu_count > 1:
            return (
                f"deepspeed --num_gpus={gpu_count} "
                f"{job.command}"
            )

        return job.command

    def _complete_job(self, job_id: str, exit_code: int) -> None:
        """Handle job completion."""
        with self._lock:
            self._processes.pop(job_id, None)
            log_file = self._log_files.pop(job_id, None)
            if log_file:
                log_file.close()

        job = self._jobs.get(job_id)
        if not job:
            return

        job.exit_code = exit_code
        job.finished_at = datetime.now(tz=timezone.utc)

        if exit_code == 0:
            job.status = JobStatus.SUCCEEDED
        else:
            job.status = JobStatus.FAILED
            job.failure_type = self._detect_failure_type(job_id, exit_code)

        logger.info("GPU job completed: %s (status=%s, exit=%d)", job_id, job.status.value, exit_code)

    def _timeout_job(self, job_id: str) -> None:
        """Kill a timed-out job."""
        logger.warning("GPU job %s exceeded timeout, killing...", job_id)
        self._kill_job(job_id)

        with self._lock:
            self._processes.pop(job_id, None)
            log_file = self._log_files.pop(job_id, None)
            if log_file:
                log_file.close()

        job = self._jobs.get(job_id)
        if job:
            job.status = JobStatus.TIMEOUT
            job.failure_type = FailureType.TIMEOUT
            job.exit_code = -1
            job.finished_at = datetime.now(tz=timezone.utc)

    def _kill_job(self, job_id: str) -> None:
        """Kill job process group."""
        with self._lock:
            process = self._processes.get(job_id)
            if not process:
                return

        try:
            os.killpg(os.getpgid(process.pid), sig.SIGTERM)
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(os.getpgid(process.pid), sig.SIGKILL)
            except Exception:
                pass
        except Exception as e:
            logger.warning("Error killing GPU job %s: %s", job_id, e)

    def _is_timed_out(self, job_id: str) -> bool:
        """Check if a running job exceeded its timeout."""
        job = self._jobs.get(job_id)
        if not job or not job.started_at:
            return False
        elapsed = (datetime.now(tz=timezone.utc) - job.started_at).total_seconds()
        return elapsed > job.timeout_minutes * 60

    # ========== Failure Detection ==========

    def _detect_failure_type(self, job_id: str, exit_code: int) -> FailureType:
        """Classify failure from exit code and log patterns."""
        if self._detect_oom_from_log(job_id):
            return FailureType.OOM

        if exit_code == -9 or exit_code == 137:  # noqa: PLR1714
            return FailureType.OOM
        elif exit_code < 0:
            return FailureType.SIGNAL
        return FailureType.RUNTIME

    def _detect_oom_from_log(self, job_id: str) -> bool:
        """Detect OOM patterns in job log."""
        log_path = self._get_job_dir(job_id) / "output.log"
        if not log_path.exists():
            return False

        oom_patterns = [
            "cuda out of memory",
            "torch.cuda.outofmemoryerror",
            "memoryerror",
            "cannot allocate memory",
            "killed",
        ]

        try:
            with open(log_path) as f:
                lines = f.readlines()[-50:]
                text = "".join(lines).lower()
                return any(p in text for p in oom_patterns)
        except Exception:
            return False

    # ========== Recovery ==========

    def _recover_running_jobs(self) -> None:
        """Recover orphaned RUNNING jobs from previous session via PID files."""
        if not self.jobs_dir.exists():
            return

        for pid_file in self.jobs_dir.rglob("pid"):
            job_id = pid_file.parent.name
            try:
                pid = int(pid_file.read_text().strip())
                if pid <= 0:
                    continue
            except (ValueError, OSError):
                continue

            if self._is_process_alive(pid):
                logger.info("Found orphaned process for %s (pid=%d), attaching...", job_id, pid)
                # We don't have the full GpuJob metadata after restart,
                # but we can track the PID for cleanup
            else:
                logger.debug("Stale PID file for %s (pid=%d), cleaning up", job_id, pid)
                pid_file.unlink(missing_ok=True)

    def _save_pid_file(self, job_id: str, pid: int) -> None:
        """Save PID for recovery."""
        try:
            pid_path = self._get_job_dir(job_id) / "pid"
            pid_path.write_text(str(pid))
        except Exception as e:
            logger.warning("Failed to save PID for %s: %s", job_id, e)

    @staticmethod
    def _is_process_alive(pid: int) -> bool:
        """Check if process is alive."""
        try:
            os.kill(pid, 0)
            return True
        except (OSError, ProcessLookupError):
            return False

    # ========== Log Access ==========

    def read_log(self, job_id: str, tail: int = 200) -> list[str]:
        """Read job log lines (last N lines)."""
        log_path = self._get_job_dir(job_id) / "output.log"
        if not log_path.exists():
            return []
        try:
            with open(log_path) as f:
                lines = f.readlines()
            return [line.rstrip() for line in lines[-tail:]]
        except Exception:
            return []

    # ========== Utilities ==========

    def _get_job_dir(self, job_id: str) -> Path:
        return self.jobs_dir / job_id

    def _check_disk_space(self, min_gb: float = 5.0) -> bool:
        """Check if sufficient disk space is available."""
        try:
            stat = shutil.disk_usage(self.jobs_dir)
            return stat.free / (1024**3) >= min_gb
        except Exception:
            return True

    @staticmethod
    def _get_free_port() -> int:
        """Get a free port for DDP MASTER_PORT."""
        import socket

        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
            s.bind(("", 0))
            s.listen(1)
            return s.getsockname()[1]
