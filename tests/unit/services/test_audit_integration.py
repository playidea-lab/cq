"""Integration tests for AuditLogger with TeamService and IntegrationService.

These tests verify that services properly call AuditLogger when
performing auditable operations like creating teams, updating settings,
and connecting integrations.
"""

from __future__ import annotations

from datetime import datetime, timedelta
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from c4.services.audit import AuditLogger, AuditAction
from c4.services.integrations import IntegrationService, IntegrationStatus
from c4.services.teams import TeamService, TeamRole


# =============================================================================
# Fixtures
# =============================================================================


@pytest.fixture
def mock_supabase_client():
    """Create a mock Supabase client."""
    client = MagicMock()
    return client


@pytest.fixture
def mock_audit_logger():
    """Create a mock AuditLogger."""
    audit_logger = MagicMock(spec=AuditLogger)
    audit_logger.log = AsyncMock()
    return audit_logger


@pytest.fixture
def sample_team_row():
    """Sample team database row."""
    return {
        "id": "team-123",
        "name": "Test Team",
        "slug": "test-team-abc123",
        "owner_id": "user-owner",
        "plan": "free",
        "settings": {"feature_x": True},
        "stripe_customer_id": None,
        "created_at": "2025-01-01T00:00:00Z",
        "updated_at": "2025-01-01T00:00:00Z",
    }


@pytest.fixture
def sample_member_row():
    """Sample team member database row."""
    return {
        "id": "member-123",
        "team_id": "team-123",
        "user_id": "user-123",
        "role": "admin",
        "email": "admin@example.com",
        "joined_at": "2025-01-01T00:00:00Z",
    }


@pytest.fixture
def sample_invite_row():
    """Sample team invite database row."""
    return {
        "id": "invite-123",
        "team_id": "team-123",
        "email": "new@example.com",
        "role": "member",
        "status": "pending",
        "token": "abc123token",
        "invited_by": "user-admin",
        "invited_at": "2025-01-01T00:00:00Z",
        "expires_at": (datetime.now() + timedelta(days=7)).isoformat(),
    }


@pytest.fixture
def sample_integration_row():
    """Sample integration database row."""
    return {
        "id": "int-123",
        "team_id": "team-123",
        "provider_id": "github",
        "external_id": "12345",
        "external_name": "my-org",
        "credentials": {"access_token": "***"},
        "settings": {"auto_assign": True},
        "status": "active",
        "connected_by": "user-admin",
        "connected_at": "2025-01-01T00:00:00Z",
        "last_used_at": None,
    }


# =============================================================================
# TeamService Audit Integration Tests
# =============================================================================


class TestTeamServiceAuditIntegration:
    """Tests that TeamService properly calls AuditLogger."""

    @pytest.fixture
    def team_service_with_audit(self, mock_supabase_client, mock_audit_logger):
        """Create a TeamService with mocked client and audit logger."""
        service = TeamService(
            url="http://test",
            key="test-key",
            audit_logger=mock_audit_logger,
        )
        service._client = mock_supabase_client
        service._service_client = mock_supabase_client
        return service

    @pytest.mark.asyncio
    async def test_create_team_logs_audit(
        self,
        team_service_with_audit,
        mock_supabase_client,
        mock_audit_logger,
        sample_team_row,
        sample_member_row,
    ):
        """Test that create_team calls audit logger."""
        # Setup mock for slug uniqueness check (returns None = no duplicate)
        slug_check_response = MagicMock()
        slug_check_response.data = None

        mock_supabase_client.table.return_value.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value = slug_check_response

        # Setup mock responses for insert
        team_response = MagicMock()
        team_response.data = [sample_team_row]

        member_response = MagicMock()
        member_response.data = [sample_member_row]

        mock_supabase_client.table.return_value.insert.return_value.execute.side_effect = [
            team_response,
            member_response,
        ]

        # Create team
        team = await team_service_with_audit.create_team(
            name="Test Team",
            owner_id="user-owner",
        )

        # Verify audit logger was called
        mock_audit_logger.log.assert_called_once()
        call_kwargs = mock_audit_logger.log.call_args[1]

        assert call_kwargs["team_id"] == "team-123"
        assert call_kwargs["action"] == "team.created"
        assert call_kwargs["resource_type"] == "team"
        assert call_kwargs["resource_id"] == "team-123"
        assert call_kwargs["actor_id"] == "user-owner"
        assert "name" in call_kwargs["new_value"]

    @pytest.mark.asyncio
    async def test_update_team_logs_audit(
        self,
        team_service_with_audit,
        mock_supabase_client,
        mock_audit_logger,
        sample_team_row,
    ):
        """Test that update_team calls audit logger."""
        # Setup mock for get_team (to get old value)
        get_response = MagicMock()
        get_response.data = sample_team_row

        # Setup mock for update
        updated_row = {**sample_team_row, "name": "Updated Team"}
        update_response = MagicMock()
        update_response.data = [updated_row]

        mock_supabase_client.table.return_value.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value = get_response
        mock_supabase_client.table.return_value.update.return_value.eq.return_value.execute.return_value = update_response

        # Update team
        team = await team_service_with_audit.update_team(
            team_id="team-123",
            name="Updated Team",
            actor_id="user-admin",
        )

        # Verify audit logger was called
        mock_audit_logger.log.assert_called_once()
        call_kwargs = mock_audit_logger.log.call_args[1]

        assert call_kwargs["team_id"] == "team-123"
        assert call_kwargs["action"] == "team.updated"
        assert call_kwargs["actor_id"] == "user-admin"
        assert "old_value" in call_kwargs
        assert "new_value" in call_kwargs

    @pytest.mark.asyncio
    async def test_create_invite_logs_audit(
        self,
        team_service_with_audit,
        mock_supabase_client,
        mock_audit_logger,
        sample_invite_row,
        sample_member_row,
    ):
        """Test that create_invite calls audit logger."""
        # Setup mock for get_member (inviter permission check) - returns inviter as admin
        inviter_response = MagicMock()
        inviter_response.data = sample_member_row  # Inviter is admin

        # Setup mock for member existence check (returns None = invitee is not a member)
        member_check_response = MagicMock()
        member_check_response.data = None

        # Calls happen in order: get_member(inviter), get_member_by_email(invitee)
        mock_supabase_client.table.return_value.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.side_effect = [
            inviter_response,     # get_member(team_id, invited_by) - inviter check
            member_check_response,  # get_member_by_email - invitee not a member
        ]

        # Setup mock for insert
        invite_response = MagicMock()
        invite_response.data = [sample_invite_row]

        mock_supabase_client.table.return_value.insert.return_value.execute.return_value = invite_response

        # Create invite
        invite = await team_service_with_audit.create_invite(
            team_id="team-123",
            email="new@example.com",
            role=TeamRole.MEMBER,
            invited_by="user-admin",
        )

        # Verify audit logger was called
        mock_audit_logger.log.assert_called_once()
        call_kwargs = mock_audit_logger.log.call_args[1]

        assert call_kwargs["team_id"] == "team-123"
        assert call_kwargs["action"] == "member.invited"
        assert call_kwargs["resource_type"] == "team_invite"
        assert call_kwargs["actor_id"] == "user-admin"
        assert call_kwargs["new_value"]["email"] == "new@example.com"


# =============================================================================
# IntegrationService Audit Integration Tests
# =============================================================================


class TestIntegrationServiceAuditIntegration:
    """Tests that IntegrationService properly calls AuditLogger."""

    @pytest.fixture
    def integration_service_with_audit(self, mock_supabase_client, mock_audit_logger):
        """Create an IntegrationService with mocked client and audit logger."""
        service = IntegrationService(
            url="http://test",
            key="test-key",
            audit_logger=mock_audit_logger,
        )
        service._client = mock_supabase_client
        return service

    @pytest.mark.asyncio
    async def test_save_integration_logs_audit(
        self,
        integration_service_with_audit,
        mock_supabase_client,
        mock_audit_logger,
        sample_integration_row,
    ):
        """Test that save_integration calls audit logger."""
        # Setup mock response
        insert_response = MagicMock()
        insert_response.data = [sample_integration_row]

        mock_supabase_client.table.return_value.insert.return_value.execute.return_value = insert_response

        # Save integration
        integration = await integration_service_with_audit.save_integration(
            team_id="team-123",
            provider_id="github",
            external_id="12345",
            external_name="my-org",
            credentials={"access_token": "***"},
            connected_by="user-admin",
        )

        # Verify audit logger was called
        mock_audit_logger.log.assert_called_once()
        call_kwargs = mock_audit_logger.log.call_args[1]

        assert call_kwargs["team_id"] == "team-123"
        assert call_kwargs["action"] == "integration.connected"
        assert call_kwargs["resource_type"] == "integration"
        assert call_kwargs["resource_id"] == "int-123"
        assert call_kwargs["actor_id"] == "user-admin"
        assert call_kwargs["new_value"]["provider_id"] == "github"

    @pytest.mark.asyncio
    async def test_update_integration_settings_logs_audit(
        self,
        integration_service_with_audit,
        mock_supabase_client,
        mock_audit_logger,
        sample_integration_row,
    ):
        """Test that update_integration_settings calls audit logger."""
        # Setup mock for get_integration (to get old value)
        get_response = MagicMock()
        get_response.data = sample_integration_row

        # Setup mock for update
        updated_row = {**sample_integration_row, "settings": {"auto_assign": False}}
        update_response = MagicMock()
        update_response.data = [updated_row]

        mock_supabase_client.table.return_value.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value = get_response
        mock_supabase_client.table.return_value.update.return_value.eq.return_value.eq.return_value.execute.return_value = update_response

        # Update settings
        integration = await integration_service_with_audit.update_integration_settings(
            team_id="team-123",
            integration_id="int-123",
            settings={"auto_assign": False},
            actor_id="user-admin",
        )

        # Verify audit logger was called
        mock_audit_logger.log.assert_called_once()
        call_kwargs = mock_audit_logger.log.call_args[1]

        assert call_kwargs["team_id"] == "team-123"
        assert call_kwargs["action"] == "integration.updated"
        assert call_kwargs["resource_id"] == "int-123"
        assert call_kwargs["actor_id"] == "user-admin"
        assert "old_value" in call_kwargs
        assert "new_value" in call_kwargs

    @pytest.mark.asyncio
    async def test_delete_integration_logs_audit(
        self,
        integration_service_with_audit,
        mock_supabase_client,
        mock_audit_logger,
        sample_integration_row,
    ):
        """Test that delete_integration calls audit logger."""
        # Setup mock for get_integration (to get old value)
        get_response = MagicMock()
        get_response.data = sample_integration_row

        # Setup mock for delete
        delete_response = MagicMock()
        delete_response.data = [sample_integration_row]

        mock_supabase_client.table.return_value.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value = get_response
        mock_supabase_client.table.return_value.delete.return_value.eq.return_value.eq.return_value.execute.return_value = delete_response

        # Delete integration
        success = await integration_service_with_audit.delete_integration(
            team_id="team-123",
            integration_id="int-123",
            actor_id="user-admin",
        )

        # Verify audit logger was called
        mock_audit_logger.log.assert_called_once()
        call_kwargs = mock_audit_logger.log.call_args[1]

        assert call_kwargs["team_id"] == "team-123"
        assert call_kwargs["action"] == "integration.disconnected"
        assert call_kwargs["resource_id"] == "int-123"
        assert call_kwargs["actor_id"] == "user-admin"
        assert call_kwargs["old_value"]["provider_id"] == "github"


# =============================================================================
# Service Without AuditLogger Tests
# =============================================================================


class TestServicesWithoutAuditLogger:
    """Tests that services work correctly without audit logger (optional)."""

    @pytest.mark.asyncio
    async def test_team_service_works_without_audit_logger(
        self,
        mock_supabase_client,
        sample_team_row,
        sample_member_row,
    ):
        """Test TeamService works when audit logger is not provided."""
        service = TeamService(url="http://test", key="test-key")
        service._client = mock_supabase_client
        service._service_client = mock_supabase_client

        # Setup mock for slug uniqueness check
        slug_check_response = MagicMock()
        slug_check_response.data = None

        mock_supabase_client.table.return_value.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value = slug_check_response

        # Setup mock responses
        team_response = MagicMock()
        team_response.data = [sample_team_row]

        member_response = MagicMock()
        member_response.data = [sample_member_row]

        mock_supabase_client.table.return_value.insert.return_value.execute.side_effect = [
            team_response,
            member_response,
        ]

        # Create team should work without audit logger
        team = await service.create_team(
            name="Test Team",
            owner_id="user-owner",
        )

        assert team.id == "team-123"

    @pytest.mark.asyncio
    async def test_integration_service_works_without_audit_logger(
        self,
        mock_supabase_client,
        sample_integration_row,
    ):
        """Test IntegrationService works when audit logger is not provided."""
        service = IntegrationService(url="http://test", key="test-key")
        service._client = mock_supabase_client

        # Setup mock response
        insert_response = MagicMock()
        insert_response.data = [sample_integration_row]

        mock_supabase_client.table.return_value.insert.return_value.execute.return_value = insert_response

        # Save integration should work without audit logger
        integration = await service.save_integration(
            team_id="team-123",
            provider_id="github",
            external_id="12345",
            connected_by="user-admin",
        )

        assert integration.id == "int-123"
