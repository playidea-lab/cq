"""Usage Tracking - Monitor and enforce usage limits."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timedelta
from enum import Enum
from typing import Any


class UsageMetric(str, Enum):
    """Tracked usage metrics."""

    TASKS_COMPLETED = "tasks_completed"
    TOKENS_USED = "tokens_used"
    WORKERS_SPAWNED = "workers_spawned"
    API_CALLS = "api_calls"


@dataclass
class UsageRecord:
    """Single usage record.

    Attributes:
        metric: Type of metric
        value: Usage amount
        timestamp: When recorded
        project_id: Associated project
        metadata: Additional context
    """

    metric: UsageMetric
    value: int
    timestamp: datetime
    project_id: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class UsageSummary:
    """Aggregated usage for a period.

    Attributes:
        period_start: Start of period
        period_end: End of period
        tasks_completed: Total tasks
        tokens_used: Total tokens
        workers_spawned: Worker count
        api_calls: API call count
    """

    period_start: datetime
    period_end: datetime
    tasks_completed: int = 0
    tokens_used: int = 0
    workers_spawned: int = 0
    api_calls: int = 0


class UsageTracker:
    """
    Tracks usage metrics and enforces limits.

    Features:
    - Record usage events
    - Calculate daily/monthly totals
    - Check against plan limits
    - Export for billing

    Example:
        tracker = UsageTracker(user_id="user-123")
        tracker.record(UsageMetric.TASKS_COMPLETED, 1)
        tracker.record(UsageMetric.TOKENS_USED, 1500)

        if tracker.check_limit("tokens_per_month", plan):
            # Continue processing
    """

    def __init__(self, user_id: str):
        """Initialize usage tracker.

        Args:
            user_id: User to track usage for
        """
        self._user_id = user_id
        self._records: list[UsageRecord] = []

    @property
    def user_id(self) -> str:
        """Get user ID."""
        return self._user_id

    # =========================================================================
    # Recording
    # =========================================================================

    def record(
        self,
        metric: UsageMetric,
        value: int,
        project_id: str | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> UsageRecord:
        """Record a usage event.

        Args:
            metric: Metric type
            value: Usage amount
            project_id: Optional project ID
            metadata: Optional metadata

        Returns:
            Created usage record
        """
        record = UsageRecord(
            metric=metric,
            value=value,
            timestamp=datetime.now(),
            project_id=project_id,
            metadata=metadata or {},
        )
        self._records.append(record)
        return record

    def record_task(self, project_id: str, task_id: str) -> UsageRecord:
        """Record a completed task."""
        return self.record(
            UsageMetric.TASKS_COMPLETED,
            value=1,
            project_id=project_id,
            metadata={"task_id": task_id},
        )

    def record_tokens(
        self,
        tokens: int,
        project_id: str | None = None,
        model: str | None = None,
    ) -> UsageRecord:
        """Record token usage."""
        return self.record(
            UsageMetric.TOKENS_USED,
            value=tokens,
            project_id=project_id,
            metadata={"model": model} if model else {},
        )

    def record_worker(self, project_id: str, worker_id: str) -> UsageRecord:
        """Record a worker spawn."""
        return self.record(
            UsageMetric.WORKERS_SPAWNED,
            value=1,
            project_id=project_id,
            metadata={"worker_id": worker_id},
        )

    def record_api_call(self, endpoint: str) -> UsageRecord:
        """Record an API call."""
        return self.record(
            UsageMetric.API_CALLS,
            value=1,
            metadata={"endpoint": endpoint},
        )

    # =========================================================================
    # Aggregation
    # =========================================================================

    def get_records(
        self,
        metric: UsageMetric | None = None,
        since: datetime | None = None,
        until: datetime | None = None,
    ) -> list[UsageRecord]:
        """Get filtered usage records.

        Args:
            metric: Filter by metric type
            since: Filter by start time
            until: Filter by end time

        Returns:
            Filtered records
        """
        records = self._records

        if metric:
            records = [r for r in records if r.metric == metric]

        if since:
            records = [r for r in records if r.timestamp >= since]

        if until:
            records = [r for r in records if r.timestamp <= until]

        return records

    def get_total(
        self,
        metric: UsageMetric,
        since: datetime | None = None,
        until: datetime | None = None,
    ) -> int:
        """Get total usage for a metric.

        Args:
            metric: Metric to sum
            since: Optional start time
            until: Optional end time

        Returns:
            Total usage
        """
        records = self.get_records(metric, since, until)
        return sum(r.value for r in records)

    def get_daily_usage(self, date: datetime | None = None) -> UsageSummary:
        """Get usage summary for a day.

        Args:
            date: Day to summarize (default: today)

        Returns:
            Usage summary for the day
        """
        if date is None:
            date = datetime.now()

        start = date.replace(hour=0, minute=0, second=0, microsecond=0)
        end = start + timedelta(days=1)

        return UsageSummary(
            period_start=start,
            period_end=end,
            tasks_completed=self.get_total(UsageMetric.TASKS_COMPLETED, start, end),
            tokens_used=self.get_total(UsageMetric.TOKENS_USED, start, end),
            workers_spawned=self.get_total(UsageMetric.WORKERS_SPAWNED, start, end),
            api_calls=self.get_total(UsageMetric.API_CALLS, start, end),
        )

    def get_monthly_usage(
        self,
        year: int | None = None,
        month: int | None = None,
    ) -> UsageSummary:
        """Get usage summary for a month.

        Args:
            year: Year (default: current)
            month: Month (default: current)

        Returns:
            Usage summary for the month
        """
        now = datetime.now()
        year = year or now.year
        month = month or now.month

        start = datetime(year, month, 1)
        if month == 12:
            end = datetime(year + 1, 1, 1)
        else:
            end = datetime(year, month + 1, 1)

        return UsageSummary(
            period_start=start,
            period_end=end,
            tasks_completed=self.get_total(UsageMetric.TASKS_COMPLETED, start, end),
            tokens_used=self.get_total(UsageMetric.TOKENS_USED, start, end),
            workers_spawned=self.get_total(UsageMetric.WORKERS_SPAWNED, start, end),
            api_calls=self.get_total(UsageMetric.API_CALLS, start, end),
        )

    # =========================================================================
    # Limit Checking
    # =========================================================================

    def check_daily_task_limit(self, limit: int) -> tuple[bool, int]:
        """Check daily task limit.

        Args:
            limit: Maximum tasks per day

        Returns:
            (is_within_limit, current_count)
        """
        if limit == 0:  # Unlimited
            return True, 0

        summary = self.get_daily_usage()
        return summary.tasks_completed < limit, summary.tasks_completed

    def check_monthly_token_limit(self, limit: int) -> tuple[bool, int]:
        """Check monthly token limit.

        Args:
            limit: Maximum tokens per month

        Returns:
            (is_within_limit, current_count)
        """
        if limit == 0:  # Unlimited
            return True, 0

        summary = self.get_monthly_usage()
        return summary.tokens_used < limit, summary.tokens_used

    # =========================================================================
    # Export
    # =========================================================================

    def export_for_billing(
        self,
        since: datetime,
        until: datetime,
    ) -> dict[str, Any]:
        """Export usage data for billing.

        Args:
            since: Period start
            until: Period end

        Returns:
            Billing data dict
        """
        return {
            "user_id": self._user_id,
            "period": {
                "start": since.isoformat(),
                "end": until.isoformat(),
            },
            "totals": {
                "tasks_completed": self.get_total(
                    UsageMetric.TASKS_COMPLETED, since, until
                ),
                "tokens_used": self.get_total(
                    UsageMetric.TOKENS_USED, since, until
                ),
                "workers_spawned": self.get_total(
                    UsageMetric.WORKERS_SPAWNED, since, until
                ),
                "api_calls": self.get_total(
                    UsageMetric.API_CALLS, since, until
                ),
            },
            "record_count": len(self.get_records(since=since, until=until)),
        }

    def clear(self) -> None:
        """Clear all records."""
        self._records = []
