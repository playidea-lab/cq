"""Execution context capture - runtime environment information.

Absorbed from piq/piqr/context_extractor.py.
"""

from __future__ import annotations

import os
import platform
import sys
from datetime import datetime, timezone


def capture_context() -> dict:
    """Capture current execution environment.

    Returns:
        Dict with python, os, cpu, memory, gpu, timestamp info
    """
    ctx: dict = {
        "timestamp": datetime.now(tz=timezone.utc).isoformat(),
        "python_version": sys.version.split()[0],
        "platform": platform.platform(),
        "os": platform.system(),
        "arch": platform.machine(),
        "cwd": os.getcwd(),
    }

    # CPU info
    try:
        ctx["cpu_count"] = os.cpu_count() or 0
    except Exception:
        ctx["cpu_count"] = 0

    # Memory
    try:
        import psutil

        mem = psutil.virtual_memory()
        ctx["ram_total_gb"] = round(mem.total / (1024**3), 1)
        ctx["ram_available_gb"] = round(mem.available / (1024**3), 1)
    except ImportError:
        pass

    # GPU backend
    try:
        from c4.gpu import detect_backend

        ctx["gpu_backend"] = detect_backend().value
    except Exception:
        ctx["gpu_backend"] = "unknown"

    return ctx
