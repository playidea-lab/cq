"""Checkpoint snapshot - periodic state capture during long experiments.

Saves intermediate metrics and model state at configurable intervals.
Controlled by config.tracker.snapshot_interval.
"""

from __future__ import annotations

import json
import logging
import time
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


class SnapshotManager:
    """Manages periodic checkpoint snapshots during experiment execution.

    Usage:
        sm = SnapshotManager(snapshot_dir=".c4/snapshots/T-001-0", interval=100)
        for step in range(1000):
            train_step()
            sm.maybe_snapshot(step, {"loss": loss_val, "lr": current_lr})
    """

    def __init__(
        self,
        snapshot_dir: str | Path,
        interval: int = 0,
    ) -> None:
        """Initialize snapshot manager.

        Args:
            snapshot_dir: Directory to store snapshots
            interval: Steps between snapshots (0 = disabled)
        """
        self.snapshot_dir = Path(snapshot_dir)
        self.interval = interval
        self._last_snapshot_step: int | None = None
        self._snapshots: list[dict[str, Any]] = []

        if interval > 0:
            self.snapshot_dir.mkdir(parents=True, exist_ok=True)

    @property
    def enabled(self) -> bool:
        return self.interval > 0

    def maybe_snapshot(
        self,
        step: int,
        metrics: dict[str, Any],
        extra: dict[str, Any] | None = None,
    ) -> bool:
        """Take a snapshot if interval has elapsed.

        Args:
            step: Current training step
            metrics: Current metrics to snapshot
            extra: Additional data to include

        Returns:
            True if snapshot was taken
        """
        if not self.enabled:
            return False

        if self._last_snapshot_step is not None and step - self._last_snapshot_step < self.interval:
            return False

        self._take_snapshot(step, metrics, extra)
        self._last_snapshot_step = step
        return True

    def _take_snapshot(
        self,
        step: int,
        metrics: dict[str, Any],
        extra: dict[str, Any] | None = None,
    ) -> None:
        """Save a snapshot to disk."""
        snapshot = {
            "step": step,
            "timestamp": time.time(),
            "metrics": metrics,
        }
        if extra:
            snapshot["extra"] = extra

        self._snapshots.append(snapshot)

        # Write to disk
        snapshot_path = self.snapshot_dir / f"snapshot_{step:08d}.json"
        try:
            snapshot_path.write_text(json.dumps(snapshot, indent=2, default=str))
            logger.debug("Snapshot saved: step=%d -> %s", step, snapshot_path)
        except Exception as e:
            logger.warning("Failed to save snapshot at step %d: %s", step, e)

    def list_snapshots(self) -> list[dict[str, Any]]:
        """List all snapshots taken in this session."""
        return list(self._snapshots)

    def load_snapshots(self) -> list[dict[str, Any]]:
        """Load all snapshots from disk."""
        if not self.snapshot_dir.exists():
            return []

        snapshots = []
        for path in sorted(self.snapshot_dir.glob("snapshot_*.json")):
            try:
                data = json.loads(path.read_text())
                snapshots.append(data)
            except Exception as e:
                logger.warning("Failed to load snapshot %s: %s", path, e)

        return snapshots
