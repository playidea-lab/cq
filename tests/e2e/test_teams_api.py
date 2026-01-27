"""E2E tests for Teams API - Full team lifecycle and RBAC.

Tests cover:
- Team CRUD operations
- Member invitation flow (invite → accept)
- Role-based permissions (owner/admin/member/viewer)
- Team settings updates
- Team deletion with cleanup
"""

from __future__ import annotations

import uuid
from datetime import datetime, timezone
from typing import Any
from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi.testclient import TestClient

from c4.api.app import create_app
from c4.api.auth import User
from c4.services.teams import (
    DuplicateMemberError,
    DuplicateSlugError,
    InviteExpiredError,
    InviteNotFoundError,
    Team,
    TeamInvite,
    TeamMember,
    TeamNotFoundError,
    TeamPermissionError,
    TeamPlan,
    TeamRole,
)

# =============================================================================
# Fixtures
# =============================================================================


@pytest.fixture
def app():
    """Create test app with rate limiting disabled."""
    return create_app(enable_rate_limit=False)


@pytest.fixture
def client(app):
    """Create test client."""
    return TestClient(app)


@pytest.fixture
def test_user() -> User:
    """Create a test user (owner)."""
    return User(
        user_id=f"user-{uuid.uuid4().hex[:8]}",
        email="owner@example.com",
        is_api_key_user=False,
    )


@pytest.fixture
def admin_user() -> User:
    """Create an admin test user."""
    return User(
        user_id=f"user-{uuid.uuid4().hex[:8]}",
        email="admin@example.com",
        is_api_key_user=False,
    )


@pytest.fixture
def member_user() -> User:
    """Create a member test user."""
    return User(
        user_id=f"user-{uuid.uuid4().hex[:8]}",
        email="member@example.com",
        is_api_key_user=False,
    )


@pytest.fixture
def viewer_user() -> User:
    """Create a viewer test user."""
    return User(
        user_id=f"user-{uuid.uuid4().hex[:8]}",
        email="viewer@example.com",
        is_api_key_user=False,
    )


@pytest.fixture
def mock_team_service():
    """Create mock TeamService."""
    service = MagicMock()
    # Make all methods async - matching actual TeamService methods
    service.create_team = AsyncMock()
    service.get_team = AsyncMock()
    service.update_team = AsyncMock()
    service.delete_team = AsyncMock()
    service.get_user_teams = AsyncMock()
    service.get_member = AsyncMock()
    service.get_member_by_id = AsyncMock()
    service.get_team_members = AsyncMock()
    service.create_invite = AsyncMock()
    service.accept_invite = AsyncMock()
    service.get_invite_by_token = AsyncMock()
    service.get_pending_invites = AsyncMock()
    service.cancel_invite = AsyncMock()
    service.update_member_role = AsyncMock()
    service.remove_member = AsyncMock()
    service.require_permission = AsyncMock()
    return service


@pytest.fixture
def mock_audit_logger():
    """Create mock AuditLogger."""
    logger = MagicMock()
    logger.log = AsyncMock()
    return logger


@pytest.fixture
def mock_activity_collector():
    """Create mock ActivityCollector."""
    collector = MagicMock()
    collector.log_activity = AsyncMock()
    return collector


def get_auth_override(user: User):
    """Create auth override for a specific user."""
    def override():
        return user
    return override


def create_team_data(suffix: str = "") -> dict[str, Any]:
    """Create team data for testing."""
    unique_id = uuid.uuid4().hex[:6]
    return {
        "name": f"Test Team {suffix or unique_id}",
        "slug": f"test-team-{suffix or unique_id}".lower(),
        "settings": {"theme": "dark", "notifications": True},
    }


# =============================================================================
# Mock Data Helpers
# =============================================================================


def make_team(
    team_id: str,
    owner_id: str,
    name: str = "Test Team",
    slug: str = "test-team",
    settings: dict[str, Any] | None = None,
) -> Team:
    """Create a Team entity for testing."""
    now = datetime.now(timezone.utc)
    return Team(
        id=team_id,
        name=name,
        slug=slug,
        owner_id=owner_id,
        plan=TeamPlan.FREE,
        settings=settings or {},
        created_at=now,
        updated_at=now,
    )


def make_member(
    member_id: str,
    team_id: str,
    user_id: str,
    role: TeamRole = TeamRole.MEMBER,
    email: str = "user@example.com",
) -> TeamMember:
    """Create a TeamMember entity for testing."""
    now = datetime.now(timezone.utc)
    return TeamMember(
        id=member_id,
        team_id=team_id,
        user_id=user_id,
        role=role,
        email=email,
        joined_at=now,
    )


def make_invite(
    invite_id: str,
    team_id: str,
    email: str,
    token: str,
    role: TeamRole = TeamRole.MEMBER,
    invited_by: str = "owner-id",
) -> TeamInvite:
    """Create a TeamInvite entity for testing."""
    from c4.services.teams import InviteStatus
    now = datetime.now(timezone.utc)
    expires = datetime.now(timezone.utc).replace(year=2099)
    return TeamInvite(
        id=invite_id,
        team_id=team_id,
        email=email,
        role=role,
        token=token,
        status=InviteStatus.PENDING,
        expires_at=expires,
        invited_by=invited_by,
        invited_at=now,
    )


def setup_app_overrides(
    app,
    user: User,
    team_service: MagicMock,
    audit_logger: MagicMock,
    activity_collector: MagicMock,
):
    """Set up dependency overrides for the app."""
    from c4.api.auth import get_current_user
    from c4.api.routes.teams import get_activity_collector, get_audit_logger, get_team_service

    app.dependency_overrides[get_current_user] = get_auth_override(user)
    app.dependency_overrides[get_team_service] = lambda: team_service
    app.dependency_overrides[get_audit_logger] = lambda: audit_logger
    app.dependency_overrides[get_activity_collector] = lambda: activity_collector


# =============================================================================
# Team CRUD Tests
# =============================================================================


class TestTeamCRUD:
    """Test team create, read, update, delete operations."""

    def test_create_team_success(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test successful team creation."""
        team_data = create_team_data("create")
        team_id = str(uuid.uuid4())

        # Set up mock response
        mock_team_service.create_team.return_value = make_team(
            team_id, test_user.user_id, team_data["name"], team_data["slug"], team_data["settings"]
        )

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post("/api/teams", json=team_data)

        assert response.status_code == 201
        data = response.json()
        assert data["name"] == team_data["name"]
        assert data["slug"] == team_data["slug"]
        assert data["owner_id"] == test_user.user_id

        app.dependency_overrides.clear()

    def test_create_team_duplicate_slug(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test team creation with duplicate slug fails."""
        team_data = create_team_data("duplicate")

        # Mock duplicate slug error
        mock_team_service.create_team.side_effect = DuplicateSlugError(team_data["slug"])

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post("/api/teams", json=team_data)

        assert response.status_code == 409

        app.dependency_overrides.clear()

    def test_get_team_success(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test getting team details."""
        team_id = str(uuid.uuid4())
        team = make_team(team_id, test_user.user_id)
        member = make_member(
            str(uuid.uuid4()),
            team_id,
            test_user.user_id,
            role=TeamRole.OWNER,
            email=test_user.email,
        )

        mock_team_service.get_member.return_value = member
        mock_team_service.get_team.return_value = team

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.get(f"/api/teams/{team_id}")

        assert response.status_code == 200
        data = response.json()
        assert data["id"] == team_id

        app.dependency_overrides.clear()

    def test_get_team_not_member(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test getting team when not a member returns 403."""
        team_id = str(uuid.uuid4())

        mock_team_service.get_member.return_value = None

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.get(f"/api/teams/{team_id}")

        assert response.status_code == 403

        app.dependency_overrides.clear()

    def test_get_team_not_found(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test getting non-existent team returns 404."""
        team_id = str(uuid.uuid4())
        member = make_member(str(uuid.uuid4()), team_id, test_user.user_id)

        mock_team_service.get_member.return_value = member
        mock_team_service.get_team.side_effect = TeamNotFoundError(team_id)

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.get(f"/api/teams/{team_id}")

        assert response.status_code == 404

        app.dependency_overrides.clear()

    def test_update_team_success(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test successful team update."""
        team_id = str(uuid.uuid4())
        old_team = make_team(team_id, test_user.user_id, "Old Name")
        updated_team = make_team(team_id, test_user.user_id, "New Name")

        mock_team_service.require_permission.return_value = None
        mock_team_service.get_team.return_value = old_team
        mock_team_service.update_team.return_value = updated_team

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.patch(f"/api/teams/{team_id}", json={"name": "New Name"})

        assert response.status_code == 200
        data = response.json()
        assert data["name"] == "New Name"

        app.dependency_overrides.clear()

    def test_delete_team_success(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test successful team deletion by owner."""
        team_id = str(uuid.uuid4())
        team = make_team(team_id, test_user.user_id)

        mock_team_service.get_team.return_value = team
        mock_team_service.delete_team.return_value = True

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.delete(f"/api/teams/{team_id}")

        assert response.status_code == 204

        app.dependency_overrides.clear()

    def test_list_user_teams(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test listing user's teams."""
        teams = [
            make_team(str(uuid.uuid4()), test_user.user_id, f"Team {i}")
            for i in range(3)
        ]

        mock_team_service.get_user_teams.return_value = teams

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.get("/api/teams")

        assert response.status_code == 200
        data = response.json()
        assert len(data) == 3

        app.dependency_overrides.clear()


# =============================================================================
# Member Invitation Flow Tests
# =============================================================================


class TestMemberInvitationFlow:
    """Test complete member invitation workflow."""

    def test_invite_member_success(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test successful member invitation."""
        team_id = str(uuid.uuid4())
        invite_email = "newmember@example.com"
        invite = make_invite(
            str(uuid.uuid4()),
            team_id,
            invite_email,
            "invite-token-123",
            TeamRole.MEMBER,
        )

        mock_team_service.require_permission.return_value = None
        mock_team_service.create_invite.return_value = invite

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post(
            f"/api/teams/{team_id}/members",
            json={"email": invite_email, "role": "member"},
        )

        assert response.status_code == 201

        app.dependency_overrides.clear()

    def test_get_invite_details(
        self,
        app,
        client,
        member_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test getting invite details by token."""
        team_id = str(uuid.uuid4())
        team = make_team(team_id, "owner-id", "Invite Team")
        invite = make_invite(
            str(uuid.uuid4()),
            team_id,
            member_user.email,
            "invite-token-456",
            TeamRole.MEMBER,
        )

        mock_team_service.get_invite_by_token.return_value = invite
        mock_team_service.get_team.return_value = team

        setup_app_overrides(
            app, member_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.get("/api/invites/invite-token-456")

        assert response.status_code == 200
        data = response.json()
        assert data["email"] == member_user.email
        assert data["team_name"] == "Invite Team"

        app.dependency_overrides.clear()

    def test_accept_invite_success(
        self,
        app,
        client,
        member_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test accepting an invite."""
        team_id = str(uuid.uuid4())
        member = make_member(
            str(uuid.uuid4()),
            team_id,
            member_user.user_id,
            TeamRole.MEMBER,
            member_user.email,
        )

        mock_team_service.accept_invite.return_value = member

        setup_app_overrides(
            app, member_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post("/api/invites/valid-token/accept")

        assert response.status_code == 200

        app.dependency_overrides.clear()

    def test_invite_invalid_token_404(
        self,
        app,
        client,
        member_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test invalid invite token returns 404."""
        mock_team_service.get_invite_by_token.side_effect = InviteNotFoundError("invalid-token")

        setup_app_overrides(
            app, member_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.get("/api/invites/invalid-token")

        assert response.status_code == 404

        app.dependency_overrides.clear()


# =============================================================================
# Role-Based Access Control Tests
# =============================================================================


class TestRoleBasedAccessControl:
    """Test RBAC for different team roles."""

    def test_owner_can_delete_team(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test owner can delete team."""
        team_id = str(uuid.uuid4())
        team = make_team(team_id, test_user.user_id)

        mock_team_service.get_team.return_value = team
        mock_team_service.delete_team.return_value = True

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.delete(f"/api/teams/{team_id}")

        assert response.status_code == 204

        app.dependency_overrides.clear()

    def test_non_owner_cannot_delete_team(
        self,
        app,
        client,
        admin_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test non-owner cannot delete team."""
        team_id = str(uuid.uuid4())
        owner_id = "different-owner"
        team = make_team(team_id, owner_id)

        mock_team_service.get_team.return_value = team
        # Simulate permission denied for delete_team action
        mock_team_service.require_permission.side_effect = TeamPermissionError(
            "delete_team"
        )

        setup_app_overrides(
            app, admin_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.delete(f"/api/teams/{team_id}")

        assert response.status_code == 403

        app.dependency_overrides.clear()

    def test_admin_can_invite_members(
        self,
        app,
        client,
        admin_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test admin can invite members."""
        team_id = str(uuid.uuid4())
        invite = make_invite(
            str(uuid.uuid4()),
            team_id,
            "invited@example.com",
            "token-123",
            TeamRole.MEMBER,
        )

        mock_team_service.require_permission.return_value = None
        mock_team_service.create_invite.return_value = invite

        setup_app_overrides(
            app, admin_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post(
            f"/api/teams/{team_id}/members",
            json={"email": "invited@example.com", "role": "member"},
        )

        assert response.status_code == 201

        app.dependency_overrides.clear()

    def test_member_cannot_invite_members(
        self,
        app,
        client,
        member_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test member cannot invite others."""
        team_id = str(uuid.uuid4())

        # create_invite raises TeamPermissionError when member tries to invite
        mock_team_service.create_invite.side_effect = TeamPermissionError(
            "invite members", member_user.user_id
        )

        setup_app_overrides(
            app, member_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post(
            f"/api/teams/{team_id}/members",
            json={"email": "invited@example.com", "role": "member"},
        )

        assert response.status_code == 403

        app.dependency_overrides.clear()

    def test_viewer_cannot_update_team(
        self,
        app,
        client,
        viewer_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test viewer cannot update team."""
        team_id = str(uuid.uuid4())

        mock_team_service.require_permission.side_effect = TeamPermissionError(
            "manage_settings"
        )

        setup_app_overrides(
            app, viewer_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.patch(f"/api/teams/{team_id}", json={"name": "New Name"})

        assert response.status_code == 403

        app.dependency_overrides.clear()

    def test_viewer_can_read_team(
        self,
        app,
        client,
        viewer_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test viewer can read team details."""
        team_id = str(uuid.uuid4())
        team = make_team(team_id, "owner-id")
        member = make_member(
            str(uuid.uuid4()),
            team_id,
            viewer_user.user_id,
            role=TeamRole.VIEWER,
            email=viewer_user.email,
        )

        mock_team_service.get_member.return_value = member
        mock_team_service.get_team.return_value = team

        setup_app_overrides(
            app, viewer_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.get(f"/api/teams/{team_id}")

        assert response.status_code == 200

        app.dependency_overrides.clear()


# =============================================================================
# Member Management Tests
# =============================================================================


class TestMemberManagement:
    """Test team member management operations."""

    def test_list_team_members(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test listing team members."""
        team_id = str(uuid.uuid4())
        members = [
            make_member(str(uuid.uuid4()), team_id, f"user-{i}", email=f"user{i}@example.com")
            for i in range(3)
        ]

        mock_team_service.require_permission.return_value = None
        mock_team_service.get_team_members.return_value = members

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.get(f"/api/teams/{team_id}/members")

        assert response.status_code == 200
        data = response.json()
        assert len(data) == 3

        app.dependency_overrides.clear()

    def test_update_member_role(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test updating member role."""
        team_id = str(uuid.uuid4())
        member_id = str(uuid.uuid4())
        updated_member = make_member(
            member_id,
            team_id,
            "user-123",
            role=TeamRole.ADMIN,
            email="member@example.com",
        )

        mock_team_service.require_permission.return_value = None
        mock_team_service.update_member_role.return_value = updated_member

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.patch(
            f"/api/teams/{team_id}/members/{member_id}",
            json={"role": "admin"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["role"] == "admin"

        app.dependency_overrides.clear()

    def test_remove_member(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test removing a team member."""
        team_id = str(uuid.uuid4())
        member_id = str(uuid.uuid4())

        mock_team_service.require_permission.return_value = None
        mock_team_service.remove_member.return_value = True

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.delete(f"/api/teams/{team_id}/members/{member_id}")

        assert response.status_code == 204

        app.dependency_overrides.clear()

    def test_cannot_remove_owner(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test owner cannot be removed from team."""
        team_id = str(uuid.uuid4())
        owner_member_id = str(uuid.uuid4())

        mock_team_service.require_permission.return_value = None
        mock_team_service.remove_member.side_effect = TeamPermissionError(
            "Cannot remove team owner"
        )

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.delete(f"/api/teams/{team_id}/members/{owner_member_id}")

        assert response.status_code == 403

        app.dependency_overrides.clear()


# =============================================================================
# Edge Cases and Error Handling
# =============================================================================


class TestEdgeCases:
    """Test edge cases and error conditions."""

    def test_unauthenticated_request_fails(self, app, client):
        """Test unauthenticated request returns 401."""
        # No auth override set
        response = client.get("/api/teams")

        # Should return 401 or 403 depending on auth implementation
        assert response.status_code in (401, 403, 422)

    def test_create_team_without_name_fails(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test creating team without name fails validation."""
        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post("/api/teams", json={"slug": "test-slug"})

        assert response.status_code == 422  # Validation error

        app.dependency_overrides.clear()

    def test_invite_to_nonexistent_team(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test inviting to non-existent team fails."""
        team_id = str(uuid.uuid4())

        mock_team_service.create_invite.side_effect = TeamNotFoundError(team_id)

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post(
            f"/api/teams/{team_id}/members",
            json={"email": "new@example.com", "role": "member"},
        )

        assert response.status_code == 404

        app.dependency_overrides.clear()

    def test_invalid_role_in_invite(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test invalid role in invite request fails validation."""
        team_id = str(uuid.uuid4())

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post(
            f"/api/teams/{team_id}/members",
            json={"email": "new@example.com", "role": "superadmin"},  # Invalid role
        )

        assert response.status_code == 422  # Validation error

        app.dependency_overrides.clear()

    def test_duplicate_invite(
        self,
        app,
        client,
        test_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test inviting already invited member fails."""
        team_id = str(uuid.uuid4())

        mock_team_service.require_permission.return_value = None
        mock_team_service.create_invite.side_effect = DuplicateMemberError(
            "already@example.com"
        )

        setup_app_overrides(
            app, test_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post(
            f"/api/teams/{team_id}/members",
            json={"email": "already@example.com", "role": "member"},
        )

        assert response.status_code == 409

        app.dependency_overrides.clear()

    def test_expired_invite(
        self,
        app,
        client,
        member_user,
        mock_team_service,
        mock_audit_logger,
        mock_activity_collector,
    ):
        """Test accepting expired invite fails."""
        mock_team_service.accept_invite.side_effect = InviteExpiredError("expired-token")

        setup_app_overrides(
            app, member_user, mock_team_service, mock_audit_logger, mock_activity_collector
        )

        response = client.post("/api/invites/expired-token/accept")

        assert response.status_code == 410  # Gone

        app.dependency_overrides.clear()
