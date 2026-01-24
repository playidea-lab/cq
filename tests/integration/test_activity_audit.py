"""Activity and Audit Integration Tests.

Tests for ActivityCollector and AuditLogger services with real Supabase database.

Requirements:
- SUPABASE_URL and SUPABASE_KEY environment variables set
- Test database with activity_logs and audit_logs tables
"""

from __future__ import annotations

import asyncio
import os
import uuid
from datetime import date, datetime, timedelta, UTC
from typing import AsyncGenerator

import pytest

from c4.services.activity import (
    ActivityCollector,
    ActivityLog,
    UsageReport,
    create_activity_collector,
)
from c4.services.audit import (
    ActorType,
    AuditAction,
    AuditFilter,
    AuditLog,
    AuditLogger,
    create_audit_logger,
)


# =============================================================================
# Skip if no Supabase credentials
# =============================================================================

SKIP_INTEGRATION = not (
    os.environ.get("SUPABASE_URL") and os.environ.get("SUPABASE_KEY")
)
SKIP_REASON = "SUPABASE_URL and SUPABASE_KEY required for integration tests"


# =============================================================================
# Fixtures
# =============================================================================


@pytest.fixture
def team_id() -> str:
    """Generate unique team ID for test isolation."""
    return f"test-team-{uuid.uuid4().hex[:8]}"


@pytest.fixture
def user_id() -> str:
    """Generate unique user ID for test isolation."""
    return f"test-user-{uuid.uuid4().hex[:8]}"


@pytest.fixture
def workspace_id() -> str:
    """Generate unique workspace ID for test isolation."""
    return f"test-workspace-{uuid.uuid4().hex[:8]}"


@pytest.fixture
def activity_collector() -> ActivityCollector:
    """Create ActivityCollector instance."""
    return create_activity_collector()


@pytest.fixture
def audit_logger() -> AuditLogger:
    """Create AuditLogger instance."""
    return create_audit_logger()


# =============================================================================
# ActivityCollector Tests
# =============================================================================


@pytest.mark.skipif(SKIP_INTEGRATION, reason=SKIP_REASON)
class TestActivityCollectorIntegration:
    """Integration tests for ActivityCollector."""

    @pytest.mark.asyncio
    async def test_log_activity_creates_entry(
        self,
        activity_collector: ActivityCollector,
        team_id: str,
        user_id: str,
    ):
        """log_activity should create and return an activity log entry."""
        # Arrange
        activity_type = "task_started"

        # Act
        log = await activity_collector.log_activity(
            team_id=team_id,
            activity_type=activity_type,
            user_id=user_id,
            resource_type="task",
            resource_id="T-001",
            metadata={"title": "Test Task"},
        )

        # Assert
        assert log is not None
        assert log.id is not None
        assert log.team_id == team_id
        assert log.user_id == user_id
        assert log.activity_type == activity_type
        assert log.resource_type == "task"
        assert log.resource_id == "T-001"
        assert log.metadata == {"title": "Test Task"}

    @pytest.mark.asyncio
    async def test_log_activity_with_duration(
        self,
        activity_collector: ActivityCollector,
        team_id: str,
    ):
        """log_activity should record duration when start and end times provided."""
        # Arrange
        start = datetime.now(UTC)
        end = start + timedelta(minutes=30)

        # Act
        log = await activity_collector.log_activity(
            team_id=team_id,
            activity_type="task_completed",
            started_at=start,
            ended_at=end,
        )

        # Assert
        assert log is not None
        assert log.started_at is not None
        assert log.ended_at is not None
        # Duration should be approximately 30 minutes (1800 seconds)
        if log.duration_seconds is not None:
            assert 1790 <= log.duration_seconds <= 1810

    @pytest.mark.asyncio
    async def test_track_activity_context_manager(
        self,
        activity_collector: ActivityCollector,
        team_id: str,
    ):
        """track_activity context manager should auto-record duration."""
        # Act
        async with activity_collector.track_activity(
            team_id=team_id,
            activity_type="command_executed",
            resource_type="command",
            resource_id="test-cmd",
        ):
            # Simulate some work
            await asyncio.sleep(0.1)

        # Assert - verify by querying
        activities = await activity_collector.get_team_activities(
            team_id=team_id,
            activity_type="command_executed",
            limit=1,
        )
        assert len(activities) >= 1
        latest = activities[0]
        assert latest.resource_id == "test-cmd"
        assert latest.duration_seconds is not None
        assert latest.duration_seconds >= 0.1

    @pytest.mark.asyncio
    async def test_get_team_activities_with_filters(
        self,
        activity_collector: ActivityCollector,
        team_id: str,
        user_id: str,
    ):
        """get_team_activities should filter by various criteria."""
        # Arrange - create multiple activities
        for i in range(3):
            await activity_collector.log_activity(
                team_id=team_id,
                activity_type="task_started",
                user_id=user_id,
                resource_id=f"task-{i}",
            )

        await activity_collector.log_activity(
            team_id=team_id,
            activity_type="pr_created",
            user_id=user_id,
            resource_id="pr-1",
        )

        # Act - filter by activity type
        task_activities = await activity_collector.get_team_activities(
            team_id=team_id,
            activity_type="task_started",
        )

        # Assert
        assert len(task_activities) >= 3
        for activity in task_activities:
            assert activity.activity_type == "task_started"

    @pytest.mark.asyncio
    async def test_get_team_activities_date_range(
        self,
        activity_collector: ActivityCollector,
        team_id: str,
    ):
        """get_team_activities should filter by date range."""
        # Arrange
        await activity_collector.log_activity(
            team_id=team_id,
            activity_type="task_completed",
        )

        # Act
        today = date.today()
        yesterday = today - timedelta(days=1)
        activities = await activity_collector.get_team_activities(
            team_id=team_id,
            start_date=yesterday,
            end_date=today,
        )

        # Assert
        assert len(activities) >= 1

    @pytest.mark.asyncio
    async def test_get_team_usage_report(
        self,
        activity_collector: ActivityCollector,
        team_id: str,
        user_id: str,
    ):
        """get_team_usage should return aggregated usage report."""
        # Arrange - create activities
        await activity_collector.log_activity(
            team_id=team_id,
            activity_type="task_started",
            user_id=user_id,
            started_at=datetime.now(UTC),
            ended_at=datetime.now(UTC) + timedelta(hours=1),
        )
        await activity_collector.log_activity(
            team_id=team_id,
            activity_type="task_completed",
            user_id=user_id,
        )

        # Act
        today = date.today()
        start_date = today - timedelta(days=7)
        report = await activity_collector.get_team_usage(
            team_id=team_id,
            start_date=start_date,
            end_date=today,
        )

        # Assert
        assert report is not None
        assert isinstance(report, UsageReport)
        assert report.team_id == team_id
        assert report.total_activities >= 2

    @pytest.mark.asyncio
    async def test_get_team_activities_pagination(
        self,
        activity_collector: ActivityCollector,
        team_id: str,
    ):
        """get_team_activities should support limit parameter."""
        # Arrange - create 5 activities
        for i in range(5):
            await activity_collector.log_activity(
                team_id=team_id,
                activity_type="command_executed",
                resource_id=f"cmd-{i}",
            )

        # Act - get only 2
        activities = await activity_collector.get_team_activities(
            team_id=team_id,
            limit=2,
        )

        # Assert
        assert len(activities) == 2


# =============================================================================
# AuditLogger Tests
# =============================================================================


@pytest.mark.skipif(SKIP_INTEGRATION, reason=SKIP_REASON)
class TestAuditLoggerIntegration:
    """Integration tests for AuditLogger."""

    @pytest.mark.asyncio
    async def test_log_creates_audit_entry(
        self,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """log should create and return an audit log entry."""
        # Act
        log = await audit_logger.log(
            team_id=team_id,
            action=AuditAction.TEAM_CREATED,
            resource_type="team",
            resource_id=team_id,
            actor_type=ActorType.USER,
            actor_id=user_id,
            actor_email="test@example.com",
            new_value={"name": "Test Team"},
        )

        # Assert
        assert log is not None
        assert log.id is not None
        assert log.team_id == team_id
        assert log.action == AuditAction.TEAM_CREATED.value
        assert log.actor_id == user_id
        assert log.actor_email == "test@example.com"
        assert log.new_value == {"name": "Test Team"}

    @pytest.mark.asyncio
    async def test_log_with_old_and_new_values(
        self,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """log should record old and new values for changes."""
        # Act
        log = await audit_logger.log(
            team_id=team_id,
            action=AuditAction.MEMBER_ROLE_CHANGED,
            resource_type="member",
            resource_id="member-123",
            actor_id=user_id,
            old_value={"role": "member"},
            new_value={"role": "admin"},
        )

        # Assert
        assert log.old_value == {"role": "member"}
        assert log.new_value == {"role": "admin"}

    @pytest.mark.asyncio
    async def test_get_logs_returns_team_logs(
        self,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """get_logs should return audit logs for a team."""
        # Arrange - create multiple logs
        await audit_logger.log(
            team_id=team_id,
            action=AuditAction.TEAM_CREATED,
            resource_type="team",
            resource_id=team_id,
            actor_id=user_id,
        )
        await audit_logger.log(
            team_id=team_id,
            action=AuditAction.MEMBER_INVITED,
            resource_type="member",
            resource_id="member-1",
            actor_id=user_id,
        )

        # Act
        logs = await audit_logger.get_logs(team_id=team_id)

        # Assert
        assert len(logs) >= 2

    @pytest.mark.asyncio
    async def test_get_logs_with_filter(
        self,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """get_logs should filter by action and resource type."""
        # Arrange
        await audit_logger.log(
            team_id=team_id,
            action=AuditAction.MEMBER_INVITED,
            resource_type="member",
            resource_id="m-1",
            actor_id=user_id,
        )
        await audit_logger.log(
            team_id=team_id,
            action=AuditAction.SETTINGS_UPDATED,
            resource_type="settings",
            resource_id="s-1",
            actor_id=user_id,
        )

        # Act
        filter = AuditFilter(
            action=AuditAction.MEMBER_INVITED.value,
            resource_type="member",
        )
        logs = await audit_logger.get_logs(team_id=team_id, filter=filter)

        # Assert
        assert len(logs) >= 1
        for log in logs:
            assert log.action == AuditAction.MEMBER_INVITED.value
            assert log.resource_type == "member"

    @pytest.mark.asyncio
    async def test_get_log_count(
        self,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """get_log_count should return count of matching logs."""
        # Arrange
        for i in range(3):
            await audit_logger.log(
                team_id=team_id,
                action=AuditAction.API_KEY_CREATED,
                resource_type="api_key",
                resource_id=f"key-{i}",
                actor_id=user_id,
            )

        # Act
        count = await audit_logger.get_log_count(
            team_id=team_id,
            filter=AuditFilter(action=AuditAction.API_KEY_CREATED.value),
        )

        # Assert
        assert count >= 3

    @pytest.mark.asyncio
    async def test_export_logs_csv(
        self,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """export_logs should return CSV formatted data."""
        # Arrange
        await audit_logger.log(
            team_id=team_id,
            action=AuditAction.LOGIN_SUCCESS,
            resource_type="auth",
            resource_id=user_id,
            actor_id=user_id,
            actor_email="test@example.com",
        )

        # Act
        csv_data = await audit_logger.export_logs(
            team_id=team_id,
            format="csv",
        )

        # Assert
        assert csv_data is not None
        assert isinstance(csv_data, bytes)
        csv_str = csv_data.decode("utf-8")
        assert "id" in csv_str
        assert "action" in csv_str
        assert "auth.login_success" in csv_str

    @pytest.mark.asyncio
    async def test_export_logs_json(
        self,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """export_logs should return JSON formatted data."""
        # Arrange
        await audit_logger.log(
            team_id=team_id,
            action=AuditAction.LOGOUT,
            resource_type="auth",
            resource_id=user_id,
            actor_id=user_id,
        )

        # Act
        json_data = await audit_logger.export_logs(
            team_id=team_id,
            format="json",
        )

        # Assert
        assert json_data is not None
        assert isinstance(json_data, bytes)
        import json
        logs = json.loads(json_data.decode("utf-8"))
        assert isinstance(logs, list)
        assert len(logs) >= 1

    @pytest.mark.asyncio
    async def test_get_logs_date_filter(
        self,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """get_logs should filter by date range."""
        # Arrange
        await audit_logger.log(
            team_id=team_id,
            action=AuditAction.WORKSPACE_CREATED,
            resource_type="workspace",
            resource_id="ws-1",
            actor_id=user_id,
        )

        # Act
        today = date.today()
        yesterday = today - timedelta(days=1)
        filter = AuditFilter(start_date=yesterday, end_date=today)
        logs = await audit_logger.get_logs(team_id=team_id, filter=filter)

        # Assert
        assert len(logs) >= 1

    @pytest.mark.asyncio
    async def test_get_logs_pagination(
        self,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """get_logs should support limit and offset."""
        # Arrange
        for i in range(5):
            await audit_logger.log(
                team_id=team_id,
                action=AuditAction.INTEGRATION_CONNECTED,
                resource_type="integration",
                resource_id=f"int-{i}",
                actor_id=user_id,
            )

        # Act
        filter = AuditFilter(limit=2, offset=0)
        page1 = await audit_logger.get_logs(team_id=team_id, filter=filter)

        filter = AuditFilter(limit=2, offset=2)
        page2 = await audit_logger.get_logs(team_id=team_id, filter=filter)

        # Assert
        assert len(page1) == 2
        assert len(page2) == 2
        # Pages should have different entries
        page1_ids = {log.id for log in page1}
        page2_ids = {log.id for log in page2}
        assert page1_ids.isdisjoint(page2_ids)


# =============================================================================
# Cross-Service Tests
# =============================================================================


@pytest.mark.skipif(SKIP_INTEGRATION, reason=SKIP_REASON)
class TestCrossServiceIntegration:
    """Integration tests for Activity and Audit services working together."""

    @pytest.mark.asyncio
    async def test_activity_and_audit_consistency(
        self,
        activity_collector: ActivityCollector,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """Both services should record events for same team consistently."""
        # Act - log related events
        await activity_collector.log_activity(
            team_id=team_id,
            activity_type="task_started",
            user_id=user_id,
            resource_type="task",
            resource_id="T-100",
        )

        await audit_logger.log(
            team_id=team_id,
            action=AuditAction.WORKSPACE_CREATED,
            resource_type="workspace",
            resource_id="ws-100",
            actor_id=user_id,
        )

        # Assert - both services have data for same team
        activities = await activity_collector.get_team_activities(team_id=team_id)
        audit_logs = await audit_logger.get_logs(team_id=team_id)

        assert len(activities) >= 1
        assert len(audit_logs) >= 1

    @pytest.mark.asyncio
    async def test_full_workflow_create_query_export(
        self,
        activity_collector: ActivityCollector,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """Full workflow: create → query → export should work end-to-end."""
        # Step 1: Create data
        await activity_collector.log_activity(
            team_id=team_id,
            activity_type="pr_created",
            user_id=user_id,
            resource_id="PR-1",
        )

        await audit_logger.log(
            team_id=team_id,
            action=AuditAction.MEMBER_JOINED,
            resource_type="member",
            resource_id=user_id,
            actor_id=user_id,
        )

        # Step 2: Query data
        activities = await activity_collector.get_team_activities(team_id=team_id)
        audit_logs = await audit_logger.get_logs(team_id=team_id)

        assert any(a.resource_id == "PR-1" for a in activities)
        assert any(l.action == "member.joined" for l in audit_logs)

        # Step 3: Get usage report
        today = date.today()
        usage = await activity_collector.get_team_usage(
            team_id=team_id,
            start_date=today - timedelta(days=1),
            end_date=today,
        )
        assert usage.total_activities >= 1

        # Step 4: Export audit logs
        csv_export = await audit_logger.export_logs(team_id=team_id, format="csv")
        json_export = await audit_logger.export_logs(team_id=team_id, format="json")

        assert b"member.joined" in csv_export
        assert b"member.joined" in json_export


# =============================================================================
# Actor Type Tests
# =============================================================================


@pytest.mark.skipif(SKIP_INTEGRATION, reason=SKIP_REASON)
class TestActorTypes:
    """Tests for different actor types in audit logging."""

    @pytest.mark.asyncio
    async def test_user_actor(
        self,
        audit_logger: AuditLogger,
        team_id: str,
        user_id: str,
    ):
        """Audit log with USER actor type."""
        log = await audit_logger.log(
            team_id=team_id,
            action=AuditAction.SETTINGS_UPDATED,
            resource_type="settings",
            resource_id="s-1",
            actor_type=ActorType.USER,
            actor_id=user_id,
        )
        assert log.actor_type == ActorType.USER

    @pytest.mark.asyncio
    async def test_api_key_actor(
        self,
        audit_logger: AuditLogger,
        team_id: str,
    ):
        """Audit log with API_KEY actor type."""
        log = await audit_logger.log(
            team_id=team_id,
            action=AuditAction.API_KEY_CREATED,
            resource_type="api_key",
            resource_id="key-1",
            actor_type=ActorType.API_KEY,
            actor_id="ak_test123",
        )
        assert log.actor_type == ActorType.API_KEY

    @pytest.mark.asyncio
    async def test_system_actor(
        self,
        audit_logger: AuditLogger,
        team_id: str,
    ):
        """Audit log with SYSTEM actor type."""
        log = await audit_logger.log(
            team_id=team_id,
            action=AuditAction.API_KEY_REVOKED,
            resource_type="api_key",
            resource_id="key-expired",
            actor_type=ActorType.SYSTEM,
            actor_id="system",
        )
        assert log.actor_type == ActorType.SYSTEM

    @pytest.mark.asyncio
    async def test_worker_actor(
        self,
        audit_logger: AuditLogger,
        team_id: str,
    ):
        """Audit log with WORKER actor type."""
        log = await audit_logger.log(
            team_id=team_id,
            action=AuditAction.WORKSPACE_UPDATED,
            resource_type="workspace",
            resource_id="ws-1",
            actor_type=ActorType.WORKER,
            actor_id="worker-001",
        )
        assert log.actor_type == ActorType.WORKER
