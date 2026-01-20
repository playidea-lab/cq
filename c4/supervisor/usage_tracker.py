"""Usage Tracker - Token and cost tracking for LLM API calls.

Provides session and cumulative tracking of token usage and costs,
with support for budgets and logging.

Example:
    >>> tracker = UsageTracker()
    >>> tracker.record_usage("claude-sonnet-4-20250514", 1000, 500)
    >>> print(tracker.session_cost)  # $0.0105
    >>> tracker.save_report("usage_report.json")
"""

import json
import logging
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Callable

from .claude_models import estimate_cost, get_model_preset

logger = logging.getLogger(__name__)


@dataclass
class UsageRecord:
    """Single usage record for an API call."""

    timestamp: datetime
    model: str
    input_tokens: int
    output_tokens: int
    total_tokens: int
    cost: float | None
    request_id: str | None = None
    metadata: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        """Convert to dictionary for serialization."""
        return {
            "timestamp": self.timestamp.isoformat(),
            "model": self.model,
            "input_tokens": self.input_tokens,
            "output_tokens": self.output_tokens,
            "total_tokens": self.total_tokens,
            "cost": self.cost,
            "request_id": self.request_id,
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict) -> "UsageRecord":
        """Create from dictionary."""
        return cls(
            timestamp=datetime.fromisoformat(data["timestamp"]),
            model=data["model"],
            input_tokens=data["input_tokens"],
            output_tokens=data["output_tokens"],
            total_tokens=data["total_tokens"],
            cost=data.get("cost"),
            request_id=data.get("request_id"),
            metadata=data.get("metadata", {}),
        )


@dataclass
class UsageSummary:
    """Summary of usage statistics."""

    total_requests: int
    total_input_tokens: int
    total_output_tokens: int
    total_tokens: int
    total_cost: float
    by_model: dict[str, dict]
    first_request: datetime | None
    last_request: datetime | None

    def to_dict(self) -> dict:
        """Convert to dictionary for serialization."""
        return {
            "total_requests": self.total_requests,
            "total_input_tokens": self.total_input_tokens,
            "total_output_tokens": self.total_output_tokens,
            "total_tokens": self.total_tokens,
            "total_cost": self.total_cost,
            "by_model": self.by_model,
            "first_request": self.first_request.isoformat() if self.first_request else None,
            "last_request": self.last_request.isoformat() if self.last_request else None,
        }


# Type alias for budget callback
BudgetCallback = Callable[[float, float, float], None]  # (current, budget, percentage)


class UsageTracker:
    """Track token usage and costs for LLM API calls.

    Maintains session-level and persistent tracking of API usage,
    with support for budgets and alerts.

    Attributes:
        session_records: List of usage records for current session
        budget: Optional budget limit in USD
        budget_callback: Called when budget threshold is reached

    Example:
        >>> tracker = UsageTracker(budget=10.0)
        >>> tracker.set_budget_callback(lambda c, b, p: print(f"Used {p:.0%} of budget"))
        >>> tracker.record_usage("claude-sonnet-4-20250514", 1000, 500)
    """

    def __init__(
        self,
        budget: float | None = None,
        budget_warning_threshold: float = 0.8,
        persistent_file: Path | None = None,
    ):
        """Initialize usage tracker.

        Args:
            budget: Optional budget limit in USD
            budget_warning_threshold: Trigger warning at this percentage (default 80%)
            persistent_file: Optional file for persistent storage
        """
        self.budget = budget
        self.budget_warning_threshold = budget_warning_threshold
        self.persistent_file = persistent_file

        self._session_records: list[UsageRecord] = []
        self._persistent_records: list[UsageRecord] = []
        self._budget_callback: BudgetCallback | None = None
        self._budget_exceeded_notified = False

        # Load persistent records if file exists
        if persistent_file and persistent_file.exists():
            self._load_persistent()

    def set_budget_callback(self, callback: BudgetCallback) -> None:
        """Set callback for budget alerts.

        Args:
            callback: Function called with (current_cost, budget, percentage)
        """
        self._budget_callback = callback

    def record_usage(
        self,
        model: str,
        input_tokens: int,
        output_tokens: int,
        cost: float | None = None,
        request_id: str | None = None,
        metadata: dict | None = None,
    ) -> UsageRecord:
        """Record a new usage entry.

        Args:
            model: Model identifier
            input_tokens: Number of input tokens
            output_tokens: Number of output tokens
            cost: Optional pre-calculated cost (estimated if not provided)
            request_id: Optional request identifier
            metadata: Optional additional metadata

        Returns:
            Created UsageRecord
        """
        # Estimate cost if not provided
        if cost is None:
            cost = estimate_cost(model, input_tokens, output_tokens)

        record = UsageRecord(
            timestamp=datetime.now(),
            model=model,
            input_tokens=input_tokens,
            output_tokens=output_tokens,
            total_tokens=input_tokens + output_tokens,
            cost=cost,
            request_id=request_id,
            metadata=metadata or {},
        )

        self._session_records.append(record)
        self._persistent_records.append(record)

        # Log usage
        self._log_usage(record)

        # Check budget
        self._check_budget()

        # Auto-save if persistent file is set
        if self.persistent_file:
            self._save_persistent()

        return record

    @property
    def session_records(self) -> list[UsageRecord]:
        """Get all session records."""
        return self._session_records.copy()

    @property
    def session_cost(self) -> float:
        """Get total cost for current session."""
        return sum(r.cost or 0 for r in self._session_records)

    @property
    def session_tokens(self) -> int:
        """Get total tokens for current session."""
        return sum(r.total_tokens for r in self._session_records)

    @property
    def total_cost(self) -> float:
        """Get total cost including persistent records."""
        return sum(r.cost or 0 for r in self._persistent_records)

    @property
    def total_tokens(self) -> int:
        """Get total tokens including persistent records."""
        return sum(r.total_tokens for r in self._persistent_records)

    def get_session_summary(self) -> UsageSummary:
        """Get summary of session usage."""
        return self._build_summary(self._session_records)

    def get_total_summary(self) -> UsageSummary:
        """Get summary of all usage (including persistent)."""
        return self._build_summary(self._persistent_records)

    def _build_summary(self, records: list[UsageRecord]) -> UsageSummary:
        """Build usage summary from records."""
        if not records:
            return UsageSummary(
                total_requests=0,
                total_input_tokens=0,
                total_output_tokens=0,
                total_tokens=0,
                total_cost=0.0,
                by_model={},
                first_request=None,
                last_request=None,
            )

        by_model: dict[str, dict] = {}
        for record in records:
            if record.model not in by_model:
                by_model[record.model] = {
                    "requests": 0,
                    "input_tokens": 0,
                    "output_tokens": 0,
                    "total_tokens": 0,
                    "cost": 0.0,
                }
            by_model[record.model]["requests"] += 1
            by_model[record.model]["input_tokens"] += record.input_tokens
            by_model[record.model]["output_tokens"] += record.output_tokens
            by_model[record.model]["total_tokens"] += record.total_tokens
            by_model[record.model]["cost"] += record.cost or 0

        return UsageSummary(
            total_requests=len(records),
            total_input_tokens=sum(r.input_tokens for r in records),
            total_output_tokens=sum(r.output_tokens for r in records),
            total_tokens=sum(r.total_tokens for r in records),
            total_cost=sum(r.cost or 0 for r in records),
            by_model=by_model,
            first_request=min(r.timestamp for r in records),
            last_request=max(r.timestamp for r in records),
        )

    def _log_usage(self, record: UsageRecord) -> None:
        """Log usage record."""
        preset = get_model_preset(record.model)
        model_name = preset.display_name if preset else record.model

        cost_str = f", ${record.cost:.4f}" if record.cost else ""
        logger.info(
            f"API Usage [{model_name}]: "
            f"{record.input_tokens:,} in / {record.output_tokens:,} out "
            f"({record.total_tokens:,} total{cost_str})"
        )

        # Log cumulative session stats
        logger.debug(f"Session total: {self.session_tokens:,} tokens, ${self.session_cost:.4f}")

    def _check_budget(self) -> None:
        """Check if budget threshold is reached."""
        if self.budget is None or self._budget_callback is None:
            return

        current = self.session_cost
        percentage = current / self.budget

        # Check if we've crossed the warning threshold
        if percentage >= self.budget_warning_threshold:
            if not self._budget_exceeded_notified:
                self._budget_callback(current, self.budget, percentage)

                if percentage >= 1.0:
                    self._budget_exceeded_notified = True
                    logger.warning(
                        f"Budget EXCEEDED: ${current:.2f} / ${self.budget:.2f} ({percentage:.0%})"
                    )
                else:
                    logger.warning(
                        f"Budget warning: ${current:.2f} / ${self.budget:.2f} ({percentage:.0%})"
                    )

    def reset_session(self) -> UsageSummary:
        """Reset session records and return summary.

        Returns:
            Summary of reset session
        """
        summary = self.get_session_summary()
        self._session_records.clear()
        self._budget_exceeded_notified = False
        logger.info(f"Session reset. Previous: {summary.total_tokens:,} tokens")
        return summary

    def save_report(
        self,
        path: Path | str,
        include_records: bool = True,
        session_only: bool = False,
    ) -> None:
        """Save usage report to file.

        Args:
            path: Output file path
            include_records: Include individual records
            session_only: Only include session records
        """
        path = Path(path)

        records = self._session_records if session_only else self._persistent_records
        summary = self.get_session_summary() if session_only else self.get_total_summary()

        report = {
            "generated_at": datetime.now().isoformat(),
            "summary": summary.to_dict(),
            "budget": {
                "limit": self.budget,
                "used": self.session_cost if session_only else self.total_cost,
                "remaining": (
                    (self.budget - (self.session_cost if session_only else self.total_cost))
                    if self.budget
                    else None
                ),
            },
        }

        if include_records:
            report["records"] = [r.to_dict() for r in records]

        path.write_text(json.dumps(report, indent=2))
        logger.info(f"Usage report saved to {path}")

    def _load_persistent(self) -> None:
        """Load persistent records from file."""
        if not self.persistent_file or not self.persistent_file.exists():
            return

        try:
            data = json.loads(self.persistent_file.read_text())
            if "records" in data:
                self._persistent_records = [UsageRecord.from_dict(r) for r in data["records"]]
                logger.debug(f"Loaded {len(self._persistent_records)} persistent records")
        except (json.JSONDecodeError, KeyError) as e:
            logger.warning(f"Failed to load persistent records: {e}")

    def _save_persistent(self) -> None:
        """Save persistent records to file."""
        if not self.persistent_file:
            return

        try:
            self.persistent_file.parent.mkdir(parents=True, exist_ok=True)
            data = {
                "updated_at": datetime.now().isoformat(),
                "records": [r.to_dict() for r in self._persistent_records],
            }
            self.persistent_file.write_text(json.dumps(data, indent=2))
        except OSError as e:
            logger.warning(f"Failed to save persistent records: {e}")

    def format_summary(self, session_only: bool = True) -> str:
        """Format usage summary as human-readable string.

        Args:
            session_only: Show only session stats

        Returns:
            Formatted summary string
        """
        summary = self.get_session_summary() if session_only else self.get_total_summary()

        lines = [
            "=" * 50,
            "Usage Summary",
            "=" * 50,
            f"Total Requests: {summary.total_requests:,}",
            f"Total Tokens:   {summary.total_tokens:,}",
            f"  - Input:      {summary.total_input_tokens:,}",
            f"  - Output:     {summary.total_output_tokens:,}",
            f"Total Cost:     ${summary.total_cost:.4f}",
        ]

        if summary.by_model:
            lines.append("")
            lines.append("By Model:")
            for model, stats in summary.by_model.items():
                preset = get_model_preset(model)
                name = preset.display_name if preset else model
                lines.append(
                    f"  {name}: {stats['requests']} req, "
                    f"{stats['total_tokens']:,} tokens, ${stats['cost']:.4f}"
                )

        if self.budget:
            lines.append("")
            used = self.session_cost if session_only else self.total_cost
            remaining = self.budget - used
            pct = (used / self.budget) * 100
            lines.append(f"Budget: ${used:.2f} / ${self.budget:.2f} ({pct:.1f}%)")
            lines.append(f"Remaining: ${remaining:.2f}")

        lines.append("=" * 50)
        return "\n".join(lines)


def create_usage_tracker(
    budget: float | None = None,
    persistent_file: Path | str | None = None,
) -> UsageTracker:
    """Create a configured usage tracker.

    Args:
        budget: Optional budget limit in USD
        persistent_file: Optional file for persistent storage

    Returns:
        Configured UsageTracker
    """
    return UsageTracker(
        budget=budget,
        persistent_file=Path(persistent_file) if persistent_file else None,
    )
