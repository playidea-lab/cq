"""Tests for ActivityCollector service."""

from __future__ import annotations

from datetime import date
from unittest.mock import MagicMock, patch

import pytest

from c4.services.activity import (
    ActivityCollector,
    ActivityError,
    create_activity_collector,
)


class TestActivityCollector:
    """Tests for ActivityCollector class."""

    @pytest.fixture
    def collector(self) -> ActivityCollector:
        """Create ActivityCollector with mocked client."""
        with patch.dict("os.environ", {"SUPABASE_URL": "http://test", "SUPABASE_KEY": "test"}):
            collector = ActivityCollector(url="http://test", key="test")
            collector._client = MagicMock()
            return collector

    def test_init_with_env_vars(self) -> None:
        """Test initialization from environment variables."""
        with patch.dict("os.environ", {"SUPABASE_URL": "http://env", "SUPABASE_KEY": "key"}):
            collector = ActivityCollector()
            assert collector._url == "http://env"
            assert collector._key == "key"

    def test_init_with_args(self) -> None:
        """Test initialization with explicit arguments."""
        collector = ActivityCollector(url="http://explicit", key="explicit_key")
        assert collector._url == "http://explicit"
        assert collector._key == "explicit_key"

    @pytest.mark.asyncio
    async def test_log_activity_success(self, collector: ActivityCollector) -> None:
        """Test successful activity logging."""
        mock_response = MagicMock()
        mock_response.data = [{
            "id": "test-id",
            "team_id": "team-1",
            "activity_type": "task_completed",
            "user_id": "user-1",
            "workspace_id": None,
            "resource_type": "task",
            "resource_id": "T-001",
            "metadata": {},
            "started_at": "2025-01-21T10:00:00+00:00",
            "ended_at": None,
            "duration_seconds": None,
            "created_at": "2025-01-21T10:00:00+00:00",
        }]

        collector._client.table.return_value.insert.return_value.execute.return_value = mock_response

        result = await collector.log_activity(
            team_id="team-1",
            activity_type="task_completed",
            user_id="user-1",
            resource_type="task",
            resource_id="T-001",
        )

        assert result.id == "test-id"
        assert result.team_id == "team-1"
        assert result.activity_type == "task_completed"

    @pytest.mark.asyncio
    async def test_log_activity_failure(self, collector: ActivityCollector) -> None:
        """Test activity logging failure."""
        mock_response = MagicMock()
        mock_response.data = []

        collector._client.table.return_value.insert.return_value.execute.return_value = mock_response

        with pytest.raises(ActivityError, match="Failed to log activity"):
            await collector.log_activity(
                team_id="team-1",
                activity_type="test",
            )

    @pytest.mark.asyncio
    async def test_get_team_activities(self, collector: ActivityCollector) -> None:
        """Test getting team activities."""
        mock_response = MagicMock()
        mock_response.data = [
            {
                "id": "activity-1",
                "team_id": "team-1",
                "activity_type": "task_completed",
                "user_id": "user-1",
                "workspace_id": None,
                "resource_type": "task",
                "resource_id": "T-001",
                "metadata": {},
                "started_at": "2025-01-21T10:00:00+00:00",
                "ended_at": "2025-01-21T10:30:00+00:00",
                "duration_seconds": 1800,
                "created_at": "2025-01-21T10:00:00+00:00",
            },
        ]

        query = MagicMock()
        query.select.return_value = query
        query.eq.return_value = query
        query.order.return_value = query
        query.limit.return_value = query
        query.gte.return_value = query
        query.lte.return_value = query
        query.execute.return_value = mock_response

        collector._client.table.return_value = query

        activities = await collector.get_team_activities("team-1", limit=10)

        assert len(activities) == 1
        assert activities[0].id == "activity-1"
        assert activities[0].duration_seconds == 1800

    @pytest.mark.asyncio
    async def test_get_team_usage(self, collector: ActivityCollector) -> None:
        """Test getting team usage report."""
        mock_response = MagicMock()
        mock_response.data = [
            {
                "id": "activity-1",
                "team_id": "team-1",
                "activity_type": "task_completed",
                "user_id": "user-1",
                "workspace_id": None,
                "resource_type": "task",
                "resource_id": "T-001",
                "metadata": {},
                "started_at": "2025-01-21T10:00:00+00:00",
                "ended_at": "2025-01-21T10:30:00+00:00",
                "duration_seconds": 1800,
                "created_at": "2025-01-21T10:00:00+00:00",
            },
            {
                "id": "activity-2",
                "team_id": "team-1",
                "activity_type": "pr_created",
                "user_id": "user-2",
                "workspace_id": None,
                "resource_type": "pr",
                "resource_id": "PR-001",
                "metadata": {},
                "started_at": "2025-01-21T11:00:00+00:00",
                "ended_at": "2025-01-21T11:15:00+00:00",
                "duration_seconds": 900,
                "created_at": "2025-01-21T11:00:00+00:00",
            },
        ]

        query = MagicMock()
        query.select.return_value = query
        query.eq.return_value = query
        query.gte.return_value = query
        query.lte.return_value = query
        query.execute.return_value = mock_response

        collector._client.table.return_value = query

        report = await collector.get_team_usage(
            team_id="team-1",
            start_date=date(2025, 1, 21),
            end_date=date(2025, 1, 21),
        )

        assert report.team_id == "team-1"
        assert report.total_activities == 2
        assert report.total_duration_seconds == 2700
        assert report.by_type["task_completed"] == 1
        assert report.by_type["pr_created"] == 1


class TestCreateActivityCollector:
    """Tests for create_activity_collector factory."""

    def test_create_with_args(self) -> None:
        """Test factory with explicit arguments."""
        collector = create_activity_collector(url="http://test", key="test")
        assert collector._url == "http://test"
        assert collector._key == "test"

    def test_create_with_env_vars(self) -> None:
        """Test factory with environment variables."""
        with patch.dict("os.environ", {"SUPABASE_URL": "http://env", "SUPABASE_KEY": "key"}):
            collector = create_activity_collector()
            assert collector._url == "http://env"
            assert collector._key == "key"
