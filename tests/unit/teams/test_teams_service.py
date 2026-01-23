"""Unit tests for TeamService.

Tests team CRUD operations, member management, invitations,
and permission checks using mocked Supabase client.
"""

from __future__ import annotations

from datetime import datetime, timedelta
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from c4.services.teams import (
    DuplicateMemberError,
    DuplicateSlugError,
    InviteExpiredError,
    InviteNotFoundError,
    InviteStatus,
    MemberNotFoundError,
    Team,
    TeamInvite,
    TeamMember,
    TeamNotFoundError,
    TeamPermissionError,
    TeamPlan,
    TeamRole,
    TeamService,
    create_team_service,
)


# =============================================================================
# Fixtures
# =============================================================================


@pytest.fixture
def mock_supabase_client():
    """Create a mock Supabase client."""
    client = MagicMock()
    return client


@pytest.fixture
def team_service(mock_supabase_client):
    """Create a TeamService with mocked client."""
    service = TeamService(url="http://test", key="test-key")
    service._client = mock_supabase_client
    service._service_client = mock_supabase_client
    return service


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


# =============================================================================
# Entity Conversion Tests
# =============================================================================


class TestEntityConversions:
    """Test database row to entity conversions."""

    def test_row_to_team(self, team_service, sample_team_row):
        """Test converting database row to Team entity."""
        team = team_service._row_to_team(sample_team_row)

        assert team.id == "team-123"
        assert team.name == "Test Team"
        assert team.slug == "test-team-abc123"
        assert team.owner_id == "user-owner"
        assert team.plan == TeamPlan.FREE
        assert team.settings == {"feature_x": True}
        assert team.created_at is not None

    def test_row_to_member(self, team_service, sample_member_row):
        """Test converting database row to TeamMember entity."""
        member = team_service._row_to_member(sample_member_row)

        assert member.id == "member-123"
        assert member.team_id == "team-123"
        assert member.user_id == "user-123"
        assert member.role == TeamRole.ADMIN
        assert member.email == "admin@example.com"

    def test_row_to_invite(self, team_service, sample_invite_row):
        """Test converting database row to TeamInvite entity."""
        invite = team_service._row_to_invite(sample_invite_row)

        assert invite.id == "invite-123"
        assert invite.team_id == "team-123"
        assert invite.email == "new@example.com"
        assert invite.role == TeamRole.MEMBER
        assert invite.status == InviteStatus.PENDING


# =============================================================================
# Team CRUD Tests
# =============================================================================


class TestTeamCRUD:
    """Test team CRUD operations."""

    @pytest.mark.asyncio
    async def test_create_team_success(
        self, team_service, mock_supabase_client, sample_team_row
    ):
        """Test successful team creation."""
        # Setup mocks
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Slug uniqueness check
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            None
        )

        # Use side_effect to return different values for different insert calls
        # First call: team insert, Second call: member insert
        member_row = {
            "id": "member-owner",
            "team_id": "team-123",
            "user_id": "user-owner",
            "role": "owner",
        }

        insert_mock = MagicMock()
        insert_results = iter([[sample_team_row], [member_row]])
        insert_mock.execute.return_value.data = None  # Will be set by side_effect

        def insert_side_effect(*args, **kwargs):
            result = MagicMock()
            result.execute.return_value.data = next(insert_results)
            return result

        mock_table.insert.side_effect = insert_side_effect

        team = await team_service.create_team(
            owner_id="user-owner",
            name="Test Team",
            slug="test-team",
        )

        assert team.name == "Test Team"
        assert team.owner_id == "user-owner"

    @pytest.mark.asyncio
    async def test_create_team_duplicate_slug(
        self, team_service, mock_supabase_client
    ):
        """Test team creation fails with duplicate slug."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Slug exists
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = {
            "id": "existing-team"
        }

        with pytest.raises(DuplicateSlugError) as exc_info:
            await team_service.create_team(
                owner_id="user-owner",
                name="Test Team",
                slug="existing-slug",
            )

        assert exc_info.value.slug == "existing-slug"

    @pytest.mark.asyncio
    async def test_get_team_success(
        self, team_service, mock_supabase_client, sample_team_row
    ):
        """Test successful team retrieval."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            sample_team_row
        )

        team = await team_service.get_team("team-123")

        assert team.id == "team-123"
        assert team.name == "Test Team"

    @pytest.mark.asyncio
    async def test_get_team_not_found(self, team_service, mock_supabase_client):
        """Test team not found raises error."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            None
        )

        with pytest.raises(TeamNotFoundError) as exc_info:
            await team_service.get_team("nonexistent")

        assert exc_info.value.team_id == "nonexistent"

    @pytest.mark.asyncio
    async def test_update_team(
        self, team_service, mock_supabase_client, sample_team_row
    ):
        """Test team update."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Get current team
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            sample_team_row
        )

        # Update response
        updated_row = {**sample_team_row, "name": "Updated Team"}
        mock_table.update.return_value.eq.return_value.execute.return_value.data = [
            updated_row
        ]

        team = await team_service.update_team(
            team_id="team-123",
            name="Updated Team",
        )

        assert team.name == "Updated Team"

    @pytest.mark.asyncio
    async def test_delete_team(self, team_service, mock_supabase_client):
        """Test team deletion."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table
        mock_table.delete.return_value.eq.return_value.execute.return_value.data = [
            {"id": "team-123"}
        ]

        result = await team_service.delete_team("team-123")

        assert result is True

    @pytest.mark.asyncio
    async def test_get_user_teams(self, team_service, mock_supabase_client, sample_team_row):
        """Test getting all teams for a user."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Members query
        mock_table.select.return_value.eq.return_value.execute.return_value.data = [
            {"team_id": "team-123"},
            {"team_id": "team-456"},
        ]

        # Teams query
        mock_table.select.return_value.in_.return_value.execute.return_value.data = [
            sample_team_row,
        ]

        teams = await team_service.get_user_teams("user-123")

        assert len(teams) >= 1


# =============================================================================
# Member Management Tests
# =============================================================================


class TestMemberManagement:
    """Test team member management."""

    @pytest.mark.asyncio
    async def test_get_team_members(
        self, team_service, mock_supabase_client, sample_member_row
    ):
        """Test getting all team members."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table
        mock_table.select.return_value.eq.return_value.execute.return_value.data = [
            sample_member_row,
        ]

        members = await team_service.get_team_members("team-123")

        assert len(members) == 1
        assert members[0].role == TeamRole.ADMIN

    @pytest.mark.asyncio
    async def test_get_member(
        self, team_service, mock_supabase_client, sample_member_row
    ):
        """Test getting a specific member."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            sample_member_row
        )

        member = await team_service.get_member("team-123", "user-123")

        assert member is not None
        assert member.user_id == "user-123"

    @pytest.mark.asyncio
    async def test_update_member_role(
        self, team_service, mock_supabase_client, sample_member_row
    ):
        """Test updating member role."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Actor is owner
        owner_row = {**sample_member_row, "role": "owner", "user_id": "user-owner"}
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            owner_row
        )

        # Target member
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            sample_member_row
        )

        # Update response
        updated_row = {**sample_member_row, "role": "member"}
        mock_table.update.return_value.eq.return_value.execute.return_value.data = [
            updated_row
        ]

        member = await team_service.update_member_role(
            team_id="team-123",
            member_id="member-123",
            new_role=TeamRole.MEMBER,
            actor_id="user-owner",
        )

        assert member.role == TeamRole.MEMBER

    @pytest.mark.asyncio
    async def test_update_member_role_permission_denied(
        self, team_service, mock_supabase_client, sample_member_row
    ):
        """Test role update fails without permission."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Actor is viewer
        viewer_row = {**sample_member_row, "role": "viewer", "user_id": "user-viewer"}
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            viewer_row
        )

        with pytest.raises(TeamPermissionError):
            await team_service.update_member_role(
                team_id="team-123",
                member_id="member-123",
                new_role=TeamRole.ADMIN,
                actor_id="user-viewer",
            )

    @pytest.mark.asyncio
    async def test_remove_member(
        self, team_service, mock_supabase_client, sample_member_row
    ):
        """Test removing a member."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Actor is admin
        admin_row = {**sample_member_row, "role": "admin", "user_id": "user-admin"}
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            admin_row
        )

        # Target member (regular member)
        target_row = {**sample_member_row, "role": "member", "id": "member-target"}
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            target_row
        )

        # Delete response
        mock_table.delete.return_value.eq.return_value.execute.return_value.data = [
            {"id": "member-target"}
        ]

        result = await team_service.remove_member(
            team_id="team-123",
            member_id="member-target",
            actor_id="user-admin",
        )

        assert result is True


# =============================================================================
# Invitation Tests
# =============================================================================


class TestInvitations:
    """Test team invitation management."""

    @pytest.mark.asyncio
    async def test_create_invite(
        self, team_service, mock_supabase_client, sample_member_row, sample_invite_row
    ):
        """Test creating an invitation."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Inviter is admin
        admin_row = {**sample_member_row, "role": "admin"}
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            admin_row
        )

        # Insert response
        mock_table.insert.return_value.execute.return_value.data = [sample_invite_row]

        invite = await team_service.create_invite(
            team_id="team-123",
            email="new@example.com",
            role=TeamRole.MEMBER,
            invited_by="user-admin",
        )

        assert invite.email == "new@example.com"
        assert invite.role == TeamRole.MEMBER
        assert invite.status == InviteStatus.PENDING

    @pytest.mark.asyncio
    async def test_create_invite_permission_denied(
        self, team_service, mock_supabase_client, sample_member_row
    ):
        """Test invitation fails without permission."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Inviter is viewer
        viewer_row = {**sample_member_row, "role": "viewer"}
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            viewer_row
        )

        with pytest.raises(TeamPermissionError):
            await team_service.create_invite(
                team_id="team-123",
                email="new@example.com",
                role=TeamRole.MEMBER,
                invited_by="user-viewer",
            )

    @pytest.mark.asyncio
    async def test_get_invite_by_token(
        self, team_service, mock_supabase_client, sample_invite_row
    ):
        """Test getting invite by token."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            sample_invite_row
        )

        invite = await team_service.get_invite_by_token("abc123token")

        assert invite.token == "abc123token"
        assert invite.status == InviteStatus.PENDING

    @pytest.mark.asyncio
    async def test_get_invite_expired(
        self, team_service, mock_supabase_client, sample_invite_row
    ):
        """Test expired invite raises error."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        expired_row = {
            **sample_invite_row,
            "expires_at": (datetime.now() - timedelta(days=1)).isoformat(),
        }
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            expired_row
        )

        # Update response for marking expired
        mock_table.update.return_value.eq.return_value.execute.return_value.data = []

        with pytest.raises(InviteExpiredError):
            await team_service.get_invite_by_token("expired-token")

    @pytest.mark.asyncio
    async def test_accept_invite(
        self, team_service, mock_supabase_client, sample_invite_row, sample_member_row
    ):
        """Test accepting an invitation."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Get invite
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            sample_invite_row
        )

        # Check not already member
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            None
        )

        # Insert member
        mock_table.insert.return_value.execute.return_value.data = [sample_member_row]

        # Update invite status
        mock_table.update.return_value.eq.return_value.execute.return_value.data = []

        member = await team_service.accept_invite(
            token="abc123token",
            user_id="user-new",
            user_email="new@example.com",
        )

        assert member is not None

    @pytest.mark.asyncio
    async def test_accept_invite_duplicate_member(
        self, team_service, mock_supabase_client, sample_invite_row, sample_member_row
    ):
        """Test accepting invite when already a member."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        # Get invite
        mock_table.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            sample_invite_row
        )

        # Already a member
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            sample_member_row
        )

        with pytest.raises(DuplicateMemberError):
            await team_service.accept_invite(
                token="abc123token",
                user_id="user-existing",
                user_email="new@example.com",
            )

    @pytest.mark.asyncio
    async def test_get_pending_invites(
        self, team_service, mock_supabase_client, sample_invite_row
    ):
        """Test getting pending invites for a team."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table
        mock_table.select.return_value.eq.return_value.eq.return_value.execute.return_value.data = [
            sample_invite_row,
        ]

        invites = await team_service.get_pending_invites("team-123")

        assert len(invites) == 1
        assert invites[0].status == InviteStatus.PENDING

    @pytest.mark.asyncio
    async def test_cancel_invite(self, team_service, mock_supabase_client):
        """Test cancelling an invite."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table
        mock_table.delete.return_value.eq.return_value.eq.return_value.eq.return_value.execute.return_value.data = [
            {"id": "invite-123"}
        ]

        result = await team_service.cancel_invite("team-123", "invite-123")

        assert result is True


# =============================================================================
# Permission Tests
# =============================================================================


class TestPermissions:
    """Test permission checking."""

    @pytest.mark.asyncio
    async def test_check_permission_owner(
        self, team_service, mock_supabase_client, sample_member_row
    ):
        """Test owner has all permissions."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        owner_row = {**sample_member_row, "role": "owner"}
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            owner_row
        )

        assert await team_service.check_permission("team-123", "user-owner", "view")
        assert await team_service.check_permission("team-123", "user-owner", "edit")
        assert await team_service.check_permission(
            "team-123", "user-owner", "manage_members"
        )
        assert await team_service.check_permission(
            "team-123", "user-owner", "delete_team"
        )

    @pytest.mark.asyncio
    async def test_check_permission_viewer(
        self, team_service, mock_supabase_client, sample_member_row
    ):
        """Test viewer has limited permissions."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        viewer_row = {**sample_member_row, "role": "viewer"}
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            viewer_row
        )

        assert await team_service.check_permission("team-123", "user-viewer", "view")
        assert not await team_service.check_permission(
            "team-123", "user-viewer", "edit"
        )
        assert not await team_service.check_permission(
            "team-123", "user-viewer", "manage_members"
        )

    @pytest.mark.asyncio
    async def test_check_permission_non_member(
        self, team_service, mock_supabase_client
    ):
        """Test non-member has no permissions."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            None
        )

        assert not await team_service.check_permission(
            "team-123", "user-nonmember", "view"
        )

    @pytest.mark.asyncio
    async def test_require_permission_success(
        self, team_service, mock_supabase_client, sample_member_row
    ):
        """Test require_permission returns member when permitted."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        admin_row = {**sample_member_row, "role": "admin"}
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            admin_row
        )

        member = await team_service.require_permission(
            "team-123", "user-admin", "manage_members"
        )

        assert member.role == TeamRole.ADMIN

    @pytest.mark.asyncio
    async def test_require_permission_denied(
        self, team_service, mock_supabase_client, sample_member_row
    ):
        """Test require_permission raises error when denied."""
        mock_table = MagicMock()
        mock_supabase_client.table.return_value = mock_table

        viewer_row = {**sample_member_row, "role": "viewer"}
        mock_table.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value.data = (
            viewer_row
        )

        with pytest.raises(TeamPermissionError):
            await team_service.require_permission(
                "team-123", "user-viewer", "manage_members"
            )

    def test_can_modify_role_hierarchy(self, team_service):
        """Test role modification hierarchy.

        Hierarchy: owner (4) > admin (3) > member (2) > viewer (1)
        An actor can only modify roles strictly below their level.
        """
        # Owner can modify all roles below
        assert team_service._can_modify_role(TeamRole.OWNER, TeamRole.ADMIN)
        assert team_service._can_modify_role(TeamRole.OWNER, TeamRole.MEMBER)
        assert team_service._can_modify_role(TeamRole.OWNER, TeamRole.VIEWER)
        # Owner cannot modify owner (same level)
        assert not team_service._can_modify_role(TeamRole.OWNER, TeamRole.OWNER)

        # Admin can modify member and viewer
        assert team_service._can_modify_role(TeamRole.ADMIN, TeamRole.MEMBER)
        assert team_service._can_modify_role(TeamRole.ADMIN, TeamRole.VIEWER)
        # Admin cannot modify admin or owner
        assert not team_service._can_modify_role(TeamRole.ADMIN, TeamRole.ADMIN)
        assert not team_service._can_modify_role(TeamRole.ADMIN, TeamRole.OWNER)

        # Member can modify viewer (below their level)
        assert team_service._can_modify_role(TeamRole.MEMBER, TeamRole.VIEWER)
        # Member cannot modify member or above
        assert not team_service._can_modify_role(TeamRole.MEMBER, TeamRole.MEMBER)
        assert not team_service._can_modify_role(TeamRole.MEMBER, TeamRole.ADMIN)
        assert not team_service._can_modify_role(TeamRole.MEMBER, TeamRole.OWNER)

        # Viewer cannot modify any role (no level below)
        assert not team_service._can_modify_role(TeamRole.VIEWER, TeamRole.VIEWER)


# =============================================================================
# Helper Tests
# =============================================================================


class TestHelpers:
    """Test helper functions."""

    def test_generate_slug(self, team_service):
        """Test slug generation."""
        slug = team_service._generate_slug("My Team Name")

        assert slug.startswith("my-team-name-")
        assert len(slug) <= 50
        assert "-" in slug

    def test_generate_slug_special_chars(self, team_service):
        """Test slug generation with special characters."""
        slug = team_service._generate_slug("Test! @Team# $123%")

        assert "!" not in slug
        assert "@" not in slug
        assert "#" not in slug
        assert "$" not in slug
        assert "%" not in slug


# =============================================================================
# Factory Function Tests
# =============================================================================


class TestFactory:
    """Test factory function."""

    def test_create_team_service(self):
        """Test TeamService factory function."""
        service = create_team_service(
            url="http://test",
            key="test-key",
        )

        assert isinstance(service, TeamService)
        assert service._url == "http://test"
        assert service._key == "test-key"

    def test_create_team_service_no_args(self, monkeypatch):
        """Test factory with environment variables."""
        monkeypatch.setenv("SUPABASE_URL", "http://env-test")
        monkeypatch.setenv("SUPABASE_KEY", "env-key")

        service = create_team_service()

        assert service._url == "http://env-test"
        assert service._key == "env-key"


# =============================================================================
# Exception Tests
# =============================================================================


class TestExceptions:
    """Test custom exception messages."""

    def test_team_not_found_error(self):
        """Test TeamNotFoundError message."""
        error = TeamNotFoundError("team-123")
        assert "team-123" in str(error)
        assert error.team_id == "team-123"

    def test_team_permission_error(self):
        """Test TeamPermissionError message."""
        error = TeamPermissionError("delete_team", "member")
        assert "delete_team" in str(error)
        assert "member" in str(error)

    def test_member_not_found_error(self):
        """Test MemberNotFoundError message."""
        error = MemberNotFoundError("member-123")
        assert "member-123" in str(error)

    def test_invite_not_found_error(self):
        """Test InviteNotFoundError message."""
        error = InviteNotFoundError("token-123")
        assert "token-123" in str(error)

    def test_invite_expired_error(self):
        """Test InviteExpiredError message."""
        error = InviteExpiredError("token-123")
        assert "expired" in str(error).lower()

    def test_duplicate_member_error(self):
        """Test DuplicateMemberError message."""
        error = DuplicateMemberError("test@example.com")
        assert "test@example.com" in str(error)

    def test_duplicate_slug_error(self):
        """Test DuplicateSlugError message."""
        error = DuplicateSlugError("my-team")
        assert "my-team" in str(error)
