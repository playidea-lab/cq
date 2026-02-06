"""Artifact auto-detection - scan task workspace for outputs.

Scans outputs/, checkpoints/ and matches patterns like *.pt, *.pkl, metrics.json.
"""

from __future__ import annotations

import fnmatch
import logging
from pathlib import Path

logger = logging.getLogger(__name__)

# Default detection patterns
DEFAULT_PATTERNS = [
    "outputs/**",
    "checkpoints/**",
    "results/**",
    "*.pt",
    "*.pth",
    "*.pkl",
    "*.joblib",
    "*.onnx",
    "metrics.json",
    "results.json",
]


def scan_outputs(
    workspace: str | Path,
    patterns: list[str] | None = None,
    max_size_mb: int = 500,
) -> list[dict]:
    """Scan workspace for artifact candidates.

    Args:
        workspace: Directory to scan
        patterns: Glob patterns to match (default: DEFAULT_PATTERNS)
        max_size_mb: Maximum file size in MB

    Returns:
        List of detected artifact dicts (name, path, size_bytes, type)
    """
    workspace = Path(workspace)
    if not workspace.exists():
        return []

    patterns = patterns or DEFAULT_PATTERNS
    max_bytes = max_size_mb * 1024 * 1024
    found: list[dict] = []
    seen_paths: set[Path] = set()

    for pattern in patterns:
        for path in workspace.glob(pattern):
            if not path.is_file():
                continue
            if path in seen_paths:
                continue
            seen_paths.add(path)

            size = path.stat().st_size
            if size > max_bytes:
                logger.debug("Skipping %s (%.1f MB > %d MB)", path, size / 1e6, max_size_mb)
                continue

            artifact_type = _infer_type(path, workspace)
            found.append({
                "name": path.name,
                "path": str(path),
                "size_bytes": size,
                "type": artifact_type,
            })

    return found


def _infer_type(path: Path, workspace: Path) -> str:
    """Infer artifact type from path and extension."""
    rel = str(path.relative_to(workspace))

    # Source code
    if path.suffix in {".py", ".sh", ".yaml", ".yml", ".toml"}:
        return "source"

    # Data files
    if fnmatch.fnmatch(rel, "data/**") or path.suffix in {".csv", ".parquet", ".arrow"}:
        return "data"

    # Everything else is output
    return "output"
