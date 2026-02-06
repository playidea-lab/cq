"""@c4_track decorator - automatic experiment tracking.

Wraps a function to capture:
1. Code features (AST analysis) - once at decoration time
2. Input data profiles (numpy/pandas/torch)
3. stdout metrics (regex parsing, pass-through)
4. Git context (commit, branch, dirty)
5. Execution environment (Python, OS, GPU)
6. Wall-clock time

Results are assembled into an ExecutionStats model.
No PiQHub dependency - everything is local.
"""

from __future__ import annotations

import functools
import inspect
import io
import logging
import os
import sys
import time
from typing import Any, Callable

from c4.models.task import ExecutionStats

from .analyzer import analyze_code
from .capture import parse_metrics_from_lines
from .context import capture_context
from .data_inspector import inspect_data
from .git_context import capture_git_context

logger = logging.getLogger(__name__)


class LocalTracker:
    """Local experiment tracker - implements ExperimentTracker ABC pattern.

    Collects execution stats locally without any remote API calls.
    """

    def __init__(self) -> None:
        self._runs: dict[str, dict[str, Any]] = {}

    def start_run(self, task_id: str, code_features: dict[str, Any]) -> str:
        """Start a tracked run."""
        run_id = f"run-{task_id}-{int(time.time())}"
        self._runs[run_id] = {
            "task_id": task_id,
            "code_features": code_features,
            "metrics": {},
            "start_time": time.time(),
        }
        return run_id

    def log_metrics(self, run_id: str, metrics: dict[str, Any]) -> None:
        """Log metrics for a run."""
        if run_id in self._runs:
            self._runs[run_id]["metrics"].update(metrics)

    def end_run(self, run_id: str, final_stats: ExecutionStats | None = None) -> None:
        """End a tracked run."""
        if run_id in self._runs:
            run = self._runs[run_id]
            run["end_time"] = time.time()
            run["final_stats"] = final_stats

    def get_run(self, run_id: str) -> dict[str, Any] | None:
        """Get run data."""
        return self._runs.get(run_id)


class _StdoutCapture:
    """Tee stdout to capture lines while passing through to original."""

    def __init__(self, original: Any) -> None:
        self._original = original
        self._lines: list[str] = []
        self._buffer = io.StringIO()

    def write(self, text: str) -> int:
        self._original.write(text)
        self._buffer.write(text)
        # Split on newlines to capture complete lines
        content = self._buffer.getvalue()
        if "\n" in content:
            parts = content.split("\n")
            # Complete lines (all except last which may be partial)
            for line in parts[:-1]:
                if line.strip():
                    self._lines.append(line)
            # Keep partial line in buffer
            self._buffer = io.StringIO()
            self._buffer.write(parts[-1])
        return len(text)

    def flush(self) -> None:
        self._original.flush()

    @property
    def captured_lines(self) -> list[str]:
        # Flush remaining buffer
        remaining = self._buffer.getvalue().strip()
        if remaining:
            return self._lines + [remaining]
        return list(self._lines)


def c4_track(
    task_id: str | None = None,
    capture_stdout: bool = True,
    capture_code: bool = True,
    capture_data: bool = True,
    capture_git: bool = True,
) -> Callable:
    """Decorator to track experiment execution.

    Usage:
        @c4_track(task_id="T-001-0")
        def train_model(X_train, y_train):
            for epoch in range(100):
                loss = model.train_step(X_train, y_train)
                print(f"epoch: {epoch}, loss: {loss:.4f}")
            return model

    Args:
        task_id: C4 task ID (or read from C4_TASK_ID env var)
        capture_stdout: Parse metrics from stdout
        capture_code: Analyze source code with AST
        capture_data: Profile input data (numpy/pandas/torch)
        capture_git: Capture git context

    Returns:
        Decorator function
    """

    def decorator(func: Callable) -> Callable:
        # Analyze code once at decoration time
        _code_features: dict[str, Any] = {}
        if capture_code:
            try:
                source = inspect.getsource(func)
                _code_features = analyze_code(source)
            except (OSError, TypeError):
                pass

        @functools.wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            resolved_task_id = task_id or os.environ.get("C4_TASK_ID", "unknown")
            stats = ExecutionStats()

            # Code features (pre-computed)
            if _code_features:
                stats.code_features = _code_features

            # Data profiling
            if capture_data:
                stats.data_profile = _profile_args(func, args, kwargs)

            # Git context
            if capture_git:
                stats.git_context = capture_git_context()

            # Execution environment
            stats.env_context = capture_context()

            # Stdout capture
            stdout_capture = None
            if capture_stdout:
                stdout_capture = _StdoutCapture(sys.stdout)
                sys.stdout = stdout_capture

            start = time.time()
            try:
                result = func(*args, **kwargs)
                return result
            finally:
                elapsed = time.time() - start
                stats.run_time_sec = round(elapsed, 3)

                # Restore stdout and parse metrics
                if stdout_capture is not None:
                    sys.stdout = stdout_capture._original
                    lines = stdout_capture.captured_lines
                    if lines:
                        stats.metrics = parse_metrics_from_lines(lines)

                # Store stats on the wrapper for retrieval
                wrapper._last_stats = stats
                wrapper._last_task_id = resolved_task_id

                logger.debug(
                    "c4_track: task=%s, time=%.1fs, metrics=%d",
                    resolved_task_id,
                    elapsed,
                    len(stats.metrics),
                )

        # Initialize attributes
        wrapper._last_stats = None
        wrapper._last_task_id = None
        wrapper._code_features = _code_features
        return wrapper

    return decorator


def _profile_args(func: Callable, args: tuple, kwargs: dict) -> dict[str, Any]:
    """Profile function arguments that look like data."""
    profile: dict[str, Any] = {}
    sig = inspect.signature(func)
    params = list(sig.parameters.keys())

    # Profile positional args
    for i, arg in enumerate(args):
        name = params[i] if i < len(params) else f"arg_{i}"
        info = inspect_data(arg, name)
        if info:
            profile[name] = info

    # Profile keyword args
    for name, arg in kwargs.items():
        info = inspect_data(arg, name)
        if info:
            profile[name] = info

    return profile
