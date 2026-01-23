"""Tests for Reports API routes."""

from __future__ import annotations

from datetime import date, datetime
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi.testclient import TestClient

from c4.api.server import create_app
from c4.services.activity import ActivityLog, UsageReport, UsageSummary
from c4.services.audit import ActorType, AuditLog


@pytest.fixture
def app():
    """Create test application."""
    return create_app()


@pytest.fixture
def client(app):
    """Create test client."""
    return TestClient(app)


class TestUsageReportEndpoint:
    """Tests for GET /reports/teams/{team_id}/usage endpoint."""

    def test_get_usage_report_success(self, client: TestClient) -> None:
        """Test successful usage report retrieval."""
        mock_report = UsageReport(
            team_id="team-1",
            start_date=date(2025, 1, 1),
            end_date=date(2025, 1, 31),
            total_activities=100,
            total_duration_seconds=36000,
            by_type={"task_completed": 50, "pr_created": 50},
            by_date=[
                UsageSummary(
                    team_id="team-1",
                    date=date(2025, 1, 1),
                    activity_type="all",
                    activity_count=10,
                    total_seconds=3600,
                    unique_users=5,
                )
            ],
        )

        with patch("c4.api.routes.reports.create_activity_collector") as mock_factory:
            mock_collector = MagicMock()
            mock_collector.get_team_usage = AsyncMock(return_value=mock_report)
            mock_factory.return_value = mock_collector

            response = client.get(
                "/api/reports/teams/team-1/usage",
                params={"start_date": "2025-01-01", "end_date": "2025-01-31"},
            )

        assert response.status_code == 200
        data = response.json()
        assert data["team_id"] == "team-1"
        assert data["total_activities"] == 100
        assert data["total_duration_seconds"] == 36000
        assert data["by_type"]["task_completed"] == 50

    def test_get_usage_report_missing_dates(self, client: TestClient) -> None:
        """Test usage report with missing required dates."""
        response = client.get("/api/reports/teams/team-1/usage")
        assert response.status_code == 422  # Validation error


class TestActivitiesEndpoint:
    """Tests for GET /reports/teams/{team_id}/activities endpoint."""

    def test_get_activities_success(self, client: TestClient) -> None:
        """Test successful activities retrieval."""
        mock_activities = [
            ActivityLog(
                id="activity-1",
                team_id="team-1",
                activity_type="task_completed",
                user_id="user-1",
                workspace_id=None,
                resource_type="task",
                resource_id="T-001",
                metadata={},
                started_at=datetime(2025, 1, 21, 10, 0, 0),
                ended_at=datetime(2025, 1, 21, 10, 30, 0),
                duration_seconds=1800,
                created_at=datetime(2025, 1, 21, 10, 0, 0),
            ),
        ]

        with patch("c4.api.routes.reports.create_activity_collector") as mock_factory:
            mock_collector = MagicMock()
            mock_collector.get_team_activities = AsyncMock(return_value=mock_activities)
            mock_factory.return_value = mock_collector

            response = client.get("/api/reports/teams/team-1/activities")

        assert response.status_code == 200
        data = response.json()
        assert len(data) == 1
        assert data[0]["id"] == "activity-1"
        assert data[0]["activity_type"] == "task_completed"

    def test_get_activities_with_filters(self, client: TestClient) -> None:
        """Test activities with filters."""
        with patch("c4.api.routes.reports.create_activity_collector") as mock_factory:
            mock_collector = MagicMock()
            mock_collector.get_team_activities = AsyncMock(return_value=[])
            mock_factory.return_value = mock_collector

            response = client.get(
                "/api/reports/teams/team-1/activities",
                params={
                    "start_date": "2025-01-01",
                    "end_date": "2025-01-31",
                    "activity_type": "task_completed",
                    "user_id": "user-1",
                    "limit": 50,
                },
            )

        assert response.status_code == 200
        mock_collector.get_team_activities.assert_called_once()


class TestAuditLogsEndpoint:
    """Tests for GET /reports/teams/{team_id}/audit endpoint."""

    def test_get_audit_logs_success(self, client: TestClient) -> None:
        """Test successful audit logs retrieval."""
        mock_logs = [
            AuditLog(
                id="audit-1",
                team_id="team-1",
                actor_type=ActorType.USER,
                actor_id="user-1",
                actor_email="user@example.com",
                action="team.created",
                resource_type="team",
                resource_id="team-1",
                old_value=None,
                new_value={"name": "New Team"},
                ip_address=None,
                user_agent=None,
                request_id=None,
                created_at=datetime(2025, 1, 21, 10, 0, 0),
                hash="abc123",
            ),
        ]

        with patch("c4.api.routes.reports.create_audit_logger") as mock_factory:
            mock_logger = MagicMock()
            mock_logger.get_logs = AsyncMock(return_value=mock_logs)
            mock_logger.get_log_count = AsyncMock(return_value=1)
            mock_factory.return_value = mock_logger

            response = client.get("/api/reports/teams/team-1/audit")

        assert response.status_code == 200
        data = response.json()
        assert data["team_id"] == "team-1"
        assert data["total"] == 1
        assert len(data["logs"]) == 1
        assert data["logs"][0]["id"] == "audit-1"
        assert data["logs"][0]["action"] == "team.created"

    def test_get_audit_logs_with_filters(self, client: TestClient) -> None:
        """Test audit logs with filters."""
        with patch("c4.api.routes.reports.create_audit_logger") as mock_factory:
            mock_logger = MagicMock()
            mock_logger.get_logs = AsyncMock(return_value=[])
            mock_logger.get_log_count = AsyncMock(return_value=0)
            mock_factory.return_value = mock_logger

            response = client.get(
                "/api/reports/teams/team-1/audit",
                params={
                    "actor_id": "user-1",
                    "action": "team.created",
                    "resource_type": "team",
                    "limit": 50,
                    "offset": 10,
                },
            )

        assert response.status_code == 200
        data = response.json()
        assert data["limit"] == 50
        assert data["offset"] == 10


class TestAuditExportEndpoint:
    """Tests for GET /reports/teams/{team_id}/audit/export endpoint."""

    def test_export_csv(self, client: TestClient) -> None:
        """Test CSV export."""
        csv_data = b"id,created_at,actor_type\naudit-1,2025-01-21,user"

        with patch("c4.api.routes.reports.create_audit_logger") as mock_factory:
            mock_logger = MagicMock()
            mock_logger.export_logs = AsyncMock(return_value=csv_data)
            mock_factory.return_value = mock_logger

            response = client.get(
                "/api/reports/teams/team-1/audit/export",
                params={"format": "csv"},
            )

        assert response.status_code == 200
        assert response.headers["content-type"] == "text/csv; charset=utf-8"
        assert "attachment" in response.headers["content-disposition"]
        assert response.content == csv_data

    def test_export_json(self, client: TestClient) -> None:
        """Test JSON export."""
        json_data = b'[{"id": "audit-1"}]'

        with patch("c4.api.routes.reports.create_audit_logger") as mock_factory:
            mock_logger = MagicMock()
            mock_logger.export_logs = AsyncMock(return_value=json_data)
            mock_factory.return_value = mock_logger

            response = client.get(
                "/api/reports/teams/team-1/audit/export",
                params={"format": "json"},
            )

        assert response.status_code == 200
        assert response.headers["content-type"] == "application/json"

    def test_export_invalid_format(self, client: TestClient) -> None:
        """Test invalid format returns error."""
        response = client.get(
            "/api/reports/teams/team-1/audit/export",
            params={"format": "xml"},
        )

        assert response.status_code == 400
        assert "csv" in response.json()["detail"] or "json" in response.json()["detail"]
