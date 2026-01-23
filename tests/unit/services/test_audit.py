"""Tests for AuditLogger service."""

from __future__ import annotations

from datetime import datetime
from unittest.mock import MagicMock, patch

import pytest

from c4.services.audit import (
    ActorType,
    AuditAction,
    AuditError,
    AuditFilter,
    AuditLog,
    AuditLogger,
    create_audit_logger,
)


class TestActorType:
    """Tests for ActorType enum."""

    def test_values(self) -> None:
        """Test ActorType values."""
        assert ActorType.USER.value == "user"
        assert ActorType.API_KEY.value == "api_key"
        assert ActorType.SYSTEM.value == "system"
        assert ActorType.WORKER.value == "worker"


class TestAuditAction:
    """Tests for AuditAction enum."""

    def test_team_actions(self) -> None:
        """Test team action values."""
        assert AuditAction.TEAM_CREATED.value == "team.created"
        assert AuditAction.TEAM_UPDATED.value == "team.updated"
        assert AuditAction.TEAM_DELETED.value == "team.deleted"

    def test_member_actions(self) -> None:
        """Test member action values."""
        assert AuditAction.MEMBER_INVITED.value == "member.invited"
        assert AuditAction.MEMBER_ROLE_CHANGED.value == "member.role_changed"
        assert AuditAction.MEMBER_REMOVED.value == "member.removed"

    def test_integration_actions(self) -> None:
        """Test integration action values."""
        assert AuditAction.INTEGRATION_CONNECTED.value == "integration.connected"
        assert AuditAction.INTEGRATION_DISCONNECTED.value == "integration.disconnected"


class TestAuditLogger:
    """Tests for AuditLogger class."""

    @pytest.fixture
    def logger(self) -> AuditLogger:
        """Create AuditLogger with mocked client."""
        with patch.dict("os.environ", {"SUPABASE_URL": "http://test", "SUPABASE_KEY": "test"}):
            audit_logger = AuditLogger(url="http://test", key="test")
            audit_logger._client = MagicMock()
            return audit_logger

    def test_init_with_env_vars(self) -> None:
        """Test initialization from environment variables."""
        with patch.dict("os.environ", {"SUPABASE_URL": "http://env", "SUPABASE_KEY": "key"}):
            audit_logger = AuditLogger()
            assert audit_logger._url == "http://env"
            assert audit_logger._key == "key"

    @pytest.mark.asyncio
    async def test_log_success(self, logger: AuditLogger) -> None:
        """Test successful audit logging."""
        mock_response = MagicMock()
        mock_response.data = [{
            "id": "audit-1",
            "team_id": "team-1",
            "actor_type": "user",
            "actor_id": "user-1",
            "actor_email": "user@example.com",
            "action": "team.created",
            "resource_type": "team",
            "resource_id": "team-1",
            "old_value": None,
            "new_value": {"name": "New Team"},
            "ip_address": "127.0.0.1",
            "user_agent": "Test/1.0",
            "request_id": "req-123",
            "created_at": "2025-01-21T10:00:00+00:00",
            "hash": "abc123",
        }]

        logger._client.table.return_value.insert.return_value.execute.return_value = mock_response

        result = await logger.log(
            team_id="team-1",
            action=AuditAction.TEAM_CREATED,
            resource_type="team",
            resource_id="team-1",
            actor_id="user-1",
            actor_email="user@example.com",
            new_value={"name": "New Team"},
        )

        assert result.id == "audit-1"
        assert result.action == "team.created"
        assert result.resource_type == "team"

    @pytest.mark.asyncio
    async def test_log_failure(self, logger: AuditLogger) -> None:
        """Test audit logging failure."""
        mock_response = MagicMock()
        mock_response.data = []

        logger._client.table.return_value.insert.return_value.execute.return_value = mock_response

        with pytest.raises(AuditError, match="Failed to log audit event"):
            await logger.log(
                team_id="team-1",
                action="test.action",
                resource_type="test",
                resource_id="test-1",
            )

    @pytest.mark.asyncio
    async def test_get_logs(self, logger: AuditLogger) -> None:
        """Test getting audit logs."""
        mock_response = MagicMock()
        mock_response.data = [
            {
                "id": "audit-1",
                "team_id": "team-1",
                "actor_type": "user",
                "actor_id": "user-1",
                "actor_email": "user@example.com",
                "action": "team.created",
                "resource_type": "team",
                "resource_id": "team-1",
                "old_value": None,
                "new_value": None,
                "ip_address": None,
                "user_agent": None,
                "request_id": None,
                "created_at": "2025-01-21T10:00:00+00:00",
                "hash": "abc123",
            },
        ]

        query = MagicMock()
        query.select.return_value = query
        query.eq.return_value = query
        query.order.return_value = query
        query.limit.return_value = query
        query.offset.return_value = query
        query.gte.return_value = query
        query.lte.return_value = query
        query.execute.return_value = mock_response

        logger._client.table.return_value = query

        logs = await logger.get_logs("team-1")

        assert len(logs) == 1
        assert logs[0].id == "audit-1"
        assert logs[0].action == "team.created"

    @pytest.mark.asyncio
    async def test_get_logs_with_filter(self, logger: AuditLogger) -> None:
        """Test getting audit logs with filter."""
        mock_response = MagicMock()
        mock_response.data = []

        query = MagicMock()
        query.select.return_value = query
        query.eq.return_value = query
        query.order.return_value = query
        query.limit.return_value = query
        query.offset.return_value = query
        query.execute.return_value = mock_response

        logger._client.table.return_value = query

        audit_filter = AuditFilter(
            actor_id="user-1",
            action="team.created",
            resource_type="team",
            limit=50,
            offset=10,
        )

        logs = await logger.get_logs("team-1", audit_filter)

        assert len(logs) == 0
        # Verify filter was applied
        query.eq.assert_any_call("team_id", "team-1")

    @pytest.mark.asyncio
    async def test_get_log_count(self, logger: AuditLogger) -> None:
        """Test getting audit log count."""
        mock_response = MagicMock()
        mock_response.count = 42

        query = MagicMock()
        query.select.return_value = query
        query.eq.return_value = query
        query.execute.return_value = mock_response

        logger._client.table.return_value = query

        count = await logger.get_log_count("team-1")

        assert count == 42

    @pytest.mark.asyncio
    async def test_export_logs_csv(self, logger: AuditLogger) -> None:
        """Test exporting logs as CSV."""
        mock_response = MagicMock()
        mock_response.data = [
            {
                "id": "audit-1",
                "team_id": "team-1",
                "actor_type": "user",
                "actor_id": "user-1",
                "actor_email": "user@example.com",
                "action": "team.created",
                "resource_type": "team",
                "resource_id": "team-1",
                "old_value": None,
                "new_value": None,
                "ip_address": None,
                "user_agent": None,
                "request_id": None,
                "created_at": "2025-01-21T10:00:00+00:00",
                "hash": "abc123",
            },
        ]

        query = MagicMock()
        query.select.return_value = query
        query.eq.return_value = query
        query.order.return_value = query
        query.limit.return_value = query
        query.offset.return_value = query
        query.execute.return_value = mock_response

        logger._client.table.return_value = query

        data = await logger.export_logs("team-1", format="csv")

        assert b"id,created_at,actor_type" in data
        assert b"audit-1" in data

    @pytest.mark.asyncio
    async def test_export_logs_json(self, logger: AuditLogger) -> None:
        """Test exporting logs as JSON."""
        mock_response = MagicMock()
        mock_response.data = [
            {
                "id": "audit-1",
                "team_id": "team-1",
                "actor_type": "user",
                "actor_id": "user-1",
                "actor_email": None,
                "action": "team.created",
                "resource_type": "team",
                "resource_id": "team-1",
                "old_value": None,
                "new_value": None,
                "ip_address": None,
                "user_agent": None,
                "request_id": None,
                "created_at": "2025-01-21T10:00:00+00:00",
                "hash": "abc123",
            },
        ]

        query = MagicMock()
        query.select.return_value = query
        query.eq.return_value = query
        query.order.return_value = query
        query.limit.return_value = query
        query.offset.return_value = query
        query.execute.return_value = mock_response

        logger._client.table.return_value = query

        data = await logger.export_logs("team-1", format="json")

        import json
        parsed = json.loads(data)
        assert len(parsed) == 1
        assert parsed[0]["id"] == "audit-1"

    def test_verify_hash_valid(self, logger: AuditLogger) -> None:
        """Test hash verification with valid hash."""
        import hashlib
        created_at = datetime(2025, 1, 21, 10, 0, 0)
        expected_hash = hashlib.sha256(
            f"audit-1user-1team.createdteam-1{created_at.isoformat()}".encode()
        ).hexdigest()

        log = AuditLog(
            id="audit-1",
            team_id="team-1",
            actor_type=ActorType.USER,
            actor_id="user-1",
            action="team.created",
            resource_type="team",
            resource_id="team-1",
            created_at=created_at,
            hash=expected_hash,
        )

        assert logger.verify_hash(log) is True

    def test_verify_hash_invalid(self, logger: AuditLogger) -> None:
        """Test hash verification with invalid hash."""
        log = AuditLog(
            id="audit-1",
            team_id="team-1",
            actor_type=ActorType.USER,
            actor_id="user-1",
            action="team.created",
            resource_type="team",
            resource_id="team-1",
            created_at=datetime(2025, 1, 21, 10, 0, 0),
            hash="invalid-hash",
        )

        assert logger.verify_hash(log) is False

    def test_verify_hash_missing(self, logger: AuditLogger) -> None:
        """Test hash verification with missing hash."""
        log = AuditLog(
            id="audit-1",
            team_id="team-1",
            actor_type=ActorType.USER,
            actor_id="user-1",
            action="team.created",
            resource_type="team",
            resource_id="team-1",
        )

        assert logger.verify_hash(log) is False


class TestCreateAuditLogger:
    """Tests for create_audit_logger factory."""

    def test_create_with_args(self) -> None:
        """Test factory with explicit arguments."""
        audit_logger = create_audit_logger(url="http://test", key="test")
        assert audit_logger._url == "http://test"
        assert audit_logger._key == "test"
