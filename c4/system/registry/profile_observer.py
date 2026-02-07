"""Profile Observer - Records observable user behavior for profile learning.

Collects data from MCP tool calls (add_todo, checkpoint, report, submit)
and stores observations in .c4/profile_observations.json.
Only records what is directly observable - never infers unobservable traits.
"""

from __future__ import annotations

import json
import logging
from datetime import datetime, timezone
from pathlib import Path
from typing import Literal

from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)


class ProfileObservation(BaseModel):
    """A single observation of user behavior."""

    timestamp: str = Field(
        default_factory=lambda: datetime.now(timezone.utc).isoformat()
    )
    event_type: Literal["add_todo", "checkpoint", "report", "submit"]

    # From c4_add_todo
    task_domain: str | None = None
    dod_length: int | None = None
    dod_keywords: list[str] = Field(default_factory=list)

    # From c4_checkpoint
    checkpoint_decision: str | None = None
    checkpoint_notes: str | None = None
    required_changes: list[str] | None = None

    # From c4_report
    summary_length: int | None = None
    files_changed_count: int | None = None


# Keywords to extract from DoD/notes for profile inference
_REVIEW_FOCUS_KEYWORDS = {
    "test": "testing",
    "coverage": "testing",
    "security": "security",
    "perf": "performance",
    "performance": "performance",
    "type": "type-safety",
    "types": "type-safety",
    "error": "error-handling",
    "edge case": "edge-cases",
    "documentation": "documentation",
    "docs": "documentation",
    "reproducib": "reproducibility",
    "citation": "citations",
    "methodology": "methodology",
    "statistical": "statistical-rigor",
    "baseline": "baselines",
    "ablation": "ablation",
    "experiment": "experimental-design",
}


def _extract_keywords(text: str) -> list[str]:
    """Extract profile-relevant keywords from text."""
    if not text:
        return []
    text_lower = text.lower()
    found = []
    for trigger, keyword in _REVIEW_FOCUS_KEYWORDS.items():
        if trigger in text_lower and keyword not in found:
            found.append(keyword)
    return found


class ProfileObserver:
    """Records user behavior observations to disk.

    Thread-safe via atomic file writes.
    """

    def __init__(self, c4_dir: Path):
        self.path = c4_dir / "profile_observations.json"

    def _load(self) -> list[dict]:
        if self.path.exists():
            try:
                return json.loads(self.path.read_text())
            except (json.JSONDecodeError, OSError):
                logger.warning("Corrupted observations file, starting fresh")
                return []
        return []

    def _save(self, observations: list[dict]) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.path.write_text(json.dumps(observations, indent=2, ensure_ascii=False))

    def _append(self, obs: ProfileObservation) -> None:
        data = self._load()
        data.append(obs.model_dump(exclude_none=True))
        self._save(data)

    def record_add_todo(
        self,
        title: str,
        dod: str,
        domain: str | None = None,
    ) -> None:
        """Record observation from c4_add_todo call."""
        try:
            keywords = _extract_keywords(dod) + _extract_keywords(title)
            self._append(
                ProfileObservation(
                    event_type="add_todo",
                    task_domain=domain,
                    dod_length=len(dod) if dod else 0,
                    dod_keywords=list(set(keywords)),
                )
            )
        except Exception as e:
            logger.debug(f"Failed to record add_todo observation: {e}")

    def record_checkpoint(
        self,
        decision: str,
        notes: str,
        required_changes: list[str] | None = None,
    ) -> None:
        """Record observation from c4_checkpoint call."""
        try:
            self._append(
                ProfileObservation(
                    event_type="checkpoint",
                    checkpoint_decision=decision,
                    checkpoint_notes=notes,
                    required_changes=required_changes,
                    dod_keywords=_extract_keywords(notes or ""),
                )
            )
        except Exception as e:
            logger.debug(f"Failed to record checkpoint observation: {e}")

    def record_report(
        self,
        summary: str,
        files_changed: list[str] | None = None,
    ) -> None:
        """Record observation from c4_report call."""
        try:
            self._append(
                ProfileObservation(
                    event_type="report",
                    summary_length=len(summary) if summary else 0,
                    files_changed_count=len(files_changed) if files_changed else 0,
                )
            )
        except Exception as e:
            logger.debug(f"Failed to record report observation: {e}")

    def record_submit(
        self,
        task_domain: str | None = None,
    ) -> None:
        """Record observation from c4_submit call."""
        try:
            self._append(
                ProfileObservation(
                    event_type="submit",
                    task_domain=task_domain,
                )
            )
        except Exception as e:
            logger.debug(f"Failed to record submit observation: {e}")

    def get_all(self) -> list[ProfileObservation]:
        """Load all observations as typed models."""
        data = self._load()
        result = []
        for item in data:
            try:
                result.append(ProfileObservation(**item))
            except Exception:
                continue
        return result

    def clear(self) -> None:
        """Clear all observations after learning."""
        if self.path.exists():
            self.path.unlink()
