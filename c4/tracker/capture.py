"""Metric capture from stdout - parse key:value patterns from log output.

Absorbed from piq/piqr/capture.py.
Supports: key: value, key = value, key - value patterns.
Handles percentages (92% → 0.92), scientific notation (1e-4 → 0.0001).
"""

from __future__ import annotations

import re

# Patterns for metric extraction
_METRIC_PATTERNS = [
    # key: value (most common in ML)
    re.compile(
        r"(?:^|[\s|,])"  # boundary
        r"([\w./%-]+)"  # key
        r"\s*[:=\-]\s*"  # separator
        r"([+-]?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?%?)"  # value
    ),
]

# Keys to skip (noise)
_SKIP_KEYS = frozenset({
    "pid", "port", "http", "https", "v", "version",
    "python", "cuda", "gpu", "cpu", "node", "rank",
})

# Progress keys (extract separately)
_PROGRESS_KEYS = frozenset({"epoch", "step", "iteration", "batch", "iter"})


def parse_metrics(line: str) -> dict[str, float]:
    """Extract metrics from a single log line.

    Args:
        line: A single line of stdout output

    Returns:
        Dict of metric_name → numeric_value

    Examples:
        >>> parse_metrics("epoch: 10, loss: 0.234, accuracy: 92%")
        {'epoch': 10.0, 'loss': 0.234, 'accuracy': 0.92}
        >>> parse_metrics("lr: 1e-4")
        {'lr': 0.0001}
    """
    metrics: dict[str, float] = {}

    for pattern in _METRIC_PATTERNS:
        for match in pattern.finditer(line):
            key = match.group(1).lower().strip(".-_")
            raw_value = match.group(2)

            if key in _SKIP_KEYS:
                continue

            try:
                value = _parse_value(raw_value)
                if value is not None:
                    metrics[key] = value
            except (ValueError, OverflowError):
                continue

    return metrics


def parse_metrics_from_lines(lines: list[str]) -> dict[str, float]:
    """Extract latest metrics from multiple lines.

    Later lines override earlier ones for same key.

    Args:
        lines: List of stdout lines

    Returns:
        Dict of latest metric values
    """
    result: dict[str, float] = {}
    for line in lines:
        result.update(parse_metrics(line))
    return result


def extract_progress(line: str) -> dict[str, int]:
    """Extract progress indicators (epoch, step, etc.) from a line.

    Args:
        line: A single log line

    Returns:
        Dict of progress_key → integer_value
    """
    progress: dict[str, int] = {}
    metrics = parse_metrics(line)
    for key in _PROGRESS_KEYS:
        if key in metrics:
            progress[key] = int(metrics[key])
    return progress


def _parse_value(raw: str) -> float | None:
    """Parse a numeric value string, handling percentages and scientific notation."""
    if not raw:
        return None

    if raw.endswith("%"):
        return float(raw[:-1]) / 100.0

    val = float(raw)

    # Reject unreasonable values
    if abs(val) > 1e15:
        return None

    return val
