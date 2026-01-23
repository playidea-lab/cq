"""Activity Tracking Service.

Provides activity logging for usage reporting and billing.

Environment Variables:
    SUPABASE_URL: Supabase project URL
    SUPABASE_KEY: Supabase anon/service key
"""

from __future__ import annotations

import logging
import os
from contextlib import asynccontextmanager
from dataclasses import dataclass, field
from datetime import datetime, date
from typing import TYPE_CHECKING, Any, AsyncIterator

if TYPE_CHECKING:
    from supabase import Client

logger = logging.getLogger(__name__)


# =============================================================================
# Domain Types
# =============================================================================


@dataclass
class ActivityLog:
    """Activity log entry."""

    id: str
    team_id: str
    activity_type: str
    user_id: str | None = None
    workspace_id: str | None = None
    resource_type: str | None = None
    resource_id: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)
    started_at: datetime | None = None
    ended_at: datetime | None = None
    duration_seconds: int | None = None
    created_at: datetime | None = None


@dataclass
class UsageSummary:
    """Aggregated usage statistics."""

    team_id: str
    date: date
    activity_type: str
    activity_count: int
    total_seconds: int
    unique_users: int


@dataclass
class UsageReport:
    """Usage report for a team."""

    team_id: str
    start_date: date
    end_date: date
    total_activities: int
    total_duration_seconds: int
    by_type: dict[str, int]
    by_date: list[UsageSummary]


# =============================================================================
# Exceptions
# =============================================================================


class ActivityError(Exception):
    """Base exception for activity operations."""

    pass


# =============================================================================
# Activity Collector Service
# =============================================================================


class ActivityCollector:
    """Service for tracking user and worker activities.

    Uses Supabase for persistent storage with RLS support.
    """

    TABLE_ACTIVITY_LOGS = "activity_logs"

    def __init__(
        self,
        url: str | None = None,
        key: str | None = None,
    ) -> None:
        """Initialize activity collector.

        Args:
            url: Supabase project URL (or SUPABASE_URL env)
            key: Supabase anon key (or SUPABASE_KEY env)
        """
        self._url = url or os.environ.get("SUPABASE_URL", "")
        self._key = key or os.environ.get("SUPABASE_KEY", "")
        self._client: Client | None = None

    @property
    def client(self) -> Client:
        """Lazy-initialize Supabase client."""
        if self._client is None:
            if not self._url or not self._key:
                raise ValueError(
                    "Supabase URL and key required. "
                    "Set SUPABASE_URL and SUPABASE_KEY environment variables."
                )
            from supabase import create_client

            self._client = create_client(self._url, self._key)
        return self._client

    # =========================================================================
    # Logging Operations
    # =========================================================================

    async def log_activity(
        self,
        team_id: str,
        activity_type: str,
        *,
        user_id: str | None = None,
        workspace_id: str | None = None,
        resource_type: str | None = None,
        resource_id: str | None = None,
        metadata: dict[str, Any] | None = None,
        started_at: datetime | None = None,
        ended_at: datetime | None = None,
    ) -> ActivityLog:
        """Log an activity.

        Args:
            team_id: Team identifier
            activity_type: Type of activity (e.g., 'task_completed', 'pr_created')
            user_id: User who performed the activity (None for system/worker)
            workspace_id: Workspace/project ID
            resource_type: Type of resource (e.g., 'task', 'pr')
            resource_id: Resource identifier
            metadata: Additional context
            started_at: Activity start time
            ended_at: Activity end time

        Returns:
            Created ActivityLog entry
        """
        now = datetime.utcnow()
        activity_data = {
            "team_id": team_id,
            "activity_type": activity_type,
            "user_id": user_id,
            "workspace_id": workspace_id,
            "resource_type": resource_type,
            "resource_id": resource_id,
            "metadata": metadata or {},
            "started_at": (started_at or now).isoformat(),
            "ended_at": ended_at.isoformat() if ended_at else None,
        }

        response = (
            self.client.table(self.TABLE_ACTIVITY_LOGS)
            .insert(activity_data)
            .execute()
        )

        if not response.data:
            raise ActivityError(f"Failed to log activity: {activity_type}")

        activity = self._row_to_activity(response.data[0])

        logger.debug(
            f"Logged activity: {activity_type} for team {team_id} "
            f"(resource: {resource_type}/{resource_id})"
        )

        return activity

    @asynccontextmanager
    async def track_activity(
        self,
        team_id: str,
        activity_type: str,
        *,
        user_id: str | None = None,
        workspace_id: str | None = None,
        resource_type: str | None = None,
        resource_id: str | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> AsyncIterator[None]:
        """Context manager for tracking activity duration.

        Automatically records start and end times.

        Args:
            team_id: Team identifier
            activity_type: Type of activity
            user_id: User who performed the activity
            workspace_id: Workspace/project ID
            resource_type: Type of resource
            resource_id: Resource identifier
            metadata: Additional context

        Yields:
            None

        Example:
            async with collector.track_activity(
                team_id="123",
                activity_type="task_execution",
                resource_type="task",
                resource_id="T-001"
            ):
                # Do work here
                pass
            # Activity logged with duration automatically
        """
        started_at = datetime.utcnow()
        try:
            yield
        finally:
            ended_at = datetime.utcnow()
            await self.log_activity(
                team_id=team_id,
                activity_type=activity_type,
                user_id=user_id,
                workspace_id=workspace_id,
                resource_type=resource_type,
                resource_id=resource_id,
                metadata=metadata,
                started_at=started_at,
                ended_at=ended_at,
            )

    # =========================================================================
    # Query Operations
    # =========================================================================

    async def get_team_activities(
        self,
        team_id: str,
        *,
        start_date: date | None = None,
        end_date: date | None = None,
        activity_type: str | None = None,
        user_id: str | None = None,
        limit: int = 100,
    ) -> list[ActivityLog]:
        """Get activities for a team.

        Args:
            team_id: Team identifier
            start_date: Filter from date
            end_date: Filter to date
            activity_type: Filter by type
            user_id: Filter by user
            limit: Maximum results

        Returns:
            List of ActivityLog entries
        """
        query = (
            self.client.table(self.TABLE_ACTIVITY_LOGS)
            .select("*")
            .eq("team_id", team_id)
            .order("created_at", desc=True)
            .limit(limit)
        )

        if start_date:
            query = query.gte("started_at", start_date.isoformat())

        if end_date:
            query = query.lte("started_at", end_date.isoformat())

        if activity_type:
            query = query.eq("activity_type", activity_type)

        if user_id:
            query = query.eq("user_id", user_id)

        response = query.execute()

        return [self._row_to_activity(row) for row in response.data]

    async def get_team_usage(
        self,
        team_id: str,
        start_date: date,
        end_date: date,
    ) -> UsageReport:
        """Get aggregated usage report for a team.

        Args:
            team_id: Team identifier
            start_date: Report start date
            end_date: Report end date

        Returns:
            UsageReport with aggregated statistics
        """
        # Query activity logs within date range
        response = (
            self.client.table(self.TABLE_ACTIVITY_LOGS)
            .select("*")
            .eq("team_id", team_id)
            .gte("started_at", start_date.isoformat())
            .lte("started_at", end_date.isoformat())
            .execute()
        )

        activities = [self._row_to_activity(row) for row in response.data]

        # Aggregate statistics
        total_activities = len(activities)
        total_duration = sum(
            a.duration_seconds for a in activities if a.duration_seconds
        )

        # Group by type
        by_type: dict[str, int] = {}
        for activity in activities:
            by_type[activity.activity_type] = (
                by_type.get(activity.activity_type, 0) + 1
            )

        # Group by date (for daily summary)
        by_date_dict: dict[date, dict[str, Any]] = {}
        for activity in activities:
            if activity.started_at:
                d = activity.started_at.date()
                if d not in by_date_dict:
                    by_date_dict[d] = {
                        "count": 0,
                        "duration": 0,
                        "users": set(),
                        "types": {},
                    }
                by_date_dict[d]["count"] += 1
                by_date_dict[d]["duration"] += activity.duration_seconds or 0
                if activity.user_id:
                    by_date_dict[d]["users"].add(activity.user_id)
                by_date_dict[d]["types"][activity.activity_type] = (
                    by_date_dict[d]["types"].get(activity.activity_type, 0) + 1
                )

        by_date = [
            UsageSummary(
                team_id=team_id,
                date=d,
                activity_type="all",
                activity_count=data["count"],
                total_seconds=data["duration"],
                unique_users=len(data["users"]),
            )
            for d, data in sorted(by_date_dict.items())
        ]

        return UsageReport(
            team_id=team_id,
            start_date=start_date,
            end_date=end_date,
            total_activities=total_activities,
            total_duration_seconds=total_duration,
            by_type=by_type,
            by_date=by_date,
        )

    # =========================================================================
    # Helpers
    # =========================================================================

    def _row_to_activity(self, row: dict[str, Any]) -> ActivityLog:
        """Convert database row to ActivityLog entity."""
        return ActivityLog(
            id=row["id"],
            team_id=row["team_id"],
            activity_type=row["activity_type"],
            user_id=row.get("user_id"),
            workspace_id=row.get("workspace_id"),
            resource_type=row.get("resource_type"),
            resource_id=row.get("resource_id"),
            metadata=row.get("metadata", {}),
            started_at=_parse_datetime(row.get("started_at")),
            ended_at=_parse_datetime(row.get("ended_at")),
            duration_seconds=row.get("duration_seconds"),
            created_at=_parse_datetime(row.get("created_at")),
        )


def _parse_datetime(value: str | datetime | None) -> datetime | None:
    """Parse datetime from string or return as-is."""
    if value is None:
        return None
    if isinstance(value, datetime):
        return value
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except (ValueError, AttributeError):
        return None


# =============================================================================
# Factory Function
# =============================================================================


def create_activity_collector(
    url: str | None = None,
    key: str | None = None,
) -> ActivityCollector:
    """Create ActivityCollector instance.

    Args:
        url: Supabase project URL
        key: Supabase anon key

    Returns:
        Configured ActivityCollector instance
    """
    return ActivityCollector(url=url, key=key)
