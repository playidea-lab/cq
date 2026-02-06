"""GPU tool handlers for MCP.

Handles: c4_gpu_status, c4_job_submit, c4_job_status
"""

from typing import Any

from ..registry import register_tool


def _get_monitor():
    """Get GPU monitor instance (lazy import for optional deps)."""
    from c4.gpu.monitor import GpuMonitor

    return GpuMonitor()


def _get_scheduler():
    """Get or create a GPU job scheduler (lazy import)."""
    from c4.gpu.scheduler import GpuJobScheduler

    # Use module-level singleton
    if not hasattr(_get_scheduler, "_instance"):
        _get_scheduler._instance = GpuJobScheduler()
    return _get_scheduler._instance


@register_tool("c4_gpu_status")
def handle_gpu_status(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get GPU status - available GPUs, VRAM, utilization.

    Returns:
        GPU information list and summary.
    """
    try:
        monitor = _get_monitor()
        gpus = monitor.detect()

        gpu_list = []
        for gpu in gpus:
            gpu_list.append({
                "index": gpu.index,
                "name": gpu.name,
                "backend": gpu.backend,
                "total_vram_gb": round(gpu.total_vram_gb, 2),
                "free_vram_gb": round(gpu.free_vram_gb, 2),
                "utilization_pct": round(gpu.utilization_pct, 1),
            })

        return {
            "gpu_count": len(gpu_list),
            "gpus": gpu_list,
            "backend": gpus[0].backend if gpus else "cpu",
        }
    except Exception as e:
        return {"error": f"GPU status check failed: {e}"}


@register_tool("c4_job_submit")
def handle_job_submit(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Submit a GPU job.

    Args (via arguments):
        command: Command to execute (e.g., "python train.py")
        task_id: Optional C4 task ID to link
        gpu_count: Number of GPUs (default: 1)
        working_dir: Working directory for the job

    Returns:
        Job ID and status.
    """
    command = arguments.get("command")
    if not command:
        return {"error": "command is required"}

    task_id = arguments.get("task_id")
    gpu_count = arguments.get("gpu_count", 1)
    working_dir = arguments.get("working_dir")

    try:
        scheduler = _get_scheduler()
        job_id = scheduler.submit(
            task_id=task_id or "manual",
            command=command,
            gpu_count=gpu_count,
            workdir=working_dir or ".",
        )

        return {
            "success": True,
            "job_id": job_id,
            "task_id": task_id,
            "message": f"Job submitted: {job_id}",
        }
    except Exception as e:
        return {"error": f"Job submission failed: {e}"}


@register_tool("c4_job_status")
def handle_job_status(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get GPU job status.

    Args (via arguments):
        job_id: Job ID to check (optional, returns all if omitted)

    Returns:
        Job status details.
    """
    job_id = arguments.get("job_id")

    try:
        scheduler = _get_scheduler()

        if job_id:
            job = scheduler.get_job(job_id)
            if not job:
                return {"error": f"Job not found: {job_id}"}
            return {
                "job_id": job.id,
                "status": job.status.value,
                "task_id": job.task_id,
                "command": job.command,
                "gpu_ids": job.gpu_ids,
            }

        # List all jobs
        jobs = scheduler.list_jobs()
        return {
            "job_count": len(jobs),
            "jobs": [
                {
                    "job_id": j.id,
                    "status": j.status.value,
                    "task_id": j.task_id,
                    "command": j.command[:80],
                }
                for j in jobs
            ],
        }
    except Exception as e:
        return {"error": f"Job status check failed: {e}"}
