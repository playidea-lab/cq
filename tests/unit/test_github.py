"""Tests for C4 GitHub Integration."""

import json
from unittest.mock import MagicMock, patch

import pytest

from c4.integrations.github import (
    CollaboratorPermission,
    GitHubClient,
    GitHubResult,
    MembershipRole,
)


class TestGitHubClient:
    """Test GitHubClient initialization."""

    def test_init_with_token(self):
        """Test client initialization with token."""
        client = GitHubClient(token="test-token")
        assert client.token == "test-token"

    def test_init_from_env(self):
        """Test client initialization from environment."""
        with patch.dict("os.environ", {"GITHUB_TOKEN": "env-token"}):
            client = GitHubClient()
            assert client.token == "env-token"

    def test_init_no_token(self):
        """Test client initialization without token."""
        with patch.dict("os.environ", {}, clear=True):
            client = GitHubClient()
            assert client.token is None


class TestGhCliDetection:
    """Test GitHub CLI detection."""

    def test_gh_available(self):
        """Test gh CLI is available."""
        client = GitHubClient(token="test")

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0)
            assert client.is_gh_available() is True

    def test_gh_not_available(self):
        """Test gh CLI is not available."""
        client = GitHubClient(token="test")

        with patch("subprocess.run") as mock_run:
            mock_run.side_effect = FileNotFoundError
            assert client.is_gh_available() is False


class TestOrgMembership:
    """Test organization membership operations."""

    @pytest.fixture
    def client(self):
        """Create a GitHub client with mocked gh CLI."""
        client = GitHubClient(token="test-token")
        client._gh_available = False  # Force API fallback
        return client

    def test_check_org_membership_is_member(self, client):
        """Test checking membership when user is a member."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (204, None)

            result = client.check_org_membership("test-org", "test-user")

            assert result.success is True
            assert result.data["is_member"] is True
            mock_api.assert_called_once_with("GET", "/orgs/test-org/members/test-user")

    def test_check_org_membership_not_member(self, client):
        """Test checking membership when user is not a member."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (404, {"message": "Not Found"})

            result = client.check_org_membership("test-org", "test-user")

            assert result.success is True
            assert result.data["is_member"] is False

    def test_check_org_membership_auth_failure(self, client):
        """Test checking membership with auth failure."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (401, {"message": "Bad credentials"})

            result = client.check_org_membership("test-org", "test-user")

            assert result.success is False
            assert "Authentication failed" in result.message

    def test_get_org_membership_details(self, client):
        """Test getting detailed membership info."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (
                200,
                {"role": "admin", "state": "active"},
            )

            result = client.get_org_membership("test-org", "test-user")

            assert result.success is True
            assert result.data["role"] == "admin"
            assert result.data["state"] == "active"


class TestCollaboratorManagement:
    """Test collaborator management operations."""

    @pytest.fixture
    def client(self):
        """Create a GitHub client with mocked gh CLI."""
        client = GitHubClient(token="test-token")
        client._gh_available = False
        return client

    def test_invite_collaborator_success(self, client):
        """Test inviting a new collaborator."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (
                201,
                {"id": 123, "invitee": {"login": "new-user"}},
            )

            result = client.invite_collaborator(
                "owner", "repo", "new-user", CollaboratorPermission.PUSH
            )

            assert result.success is True
            assert result.data["invited"] is True
            mock_api.assert_called_once_with(
                "PUT",
                "/repos/owner/repo/collaborators/new-user",
                {"permission": "push"},
            )

    def test_invite_collaborator_already_exists(self, client):
        """Test inviting an existing collaborator."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (204, None)

            result = client.invite_collaborator("owner", "repo", "existing-user")

            assert result.success is True
            assert result.data["already_collaborator"] is True

    def test_invite_collaborator_invalid_user(self, client):
        """Test inviting an invalid user."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (
                422,
                {"message": "User does not exist"},
            )

            result = client.invite_collaborator("owner", "repo", "invalid-user")

            assert result.success is False
            assert "Cannot invite" in result.message

    def test_remove_collaborator_success(self, client):
        """Test removing a collaborator."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (204, None)

            result = client.remove_collaborator("owner", "repo", "user")

            assert result.success is True
            assert result.data["removed"] is True

    def test_remove_collaborator_not_found(self, client):
        """Test removing a non-collaborator."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (404, {"message": "Not Found"})

            result = client.remove_collaborator("owner", "repo", "non-user")

            assert result.success is True
            assert result.data["was_collaborator"] is False

    def test_list_collaborators(self, client):
        """Test listing collaborators."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (
                200,
                [
                    {"login": "user1", "permissions": {"push": True}},
                    {"login": "user2", "permissions": {"admin": True}},
                ],
            )

            result = client.list_collaborators("owner", "repo")

            assert result.success is True
            assert len(result.data["collaborators"]) == 2
            assert result.data["collaborators"][0]["username"] == "user1"

    def test_check_collaborator_is_collaborator(self, client):
        """Test checking if user is a collaborator."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (204, None)

            result = client.check_collaborator("owner", "repo", "user")

            assert result.success is True
            assert result.data["is_collaborator"] is True

    def test_check_collaborator_not_collaborator(self, client):
        """Test checking if user is not a collaborator."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (404, {"message": "Not Found"})

            result = client.check_collaborator("owner", "repo", "user")

            assert result.success is True
            assert result.data["is_collaborator"] is False


class TestAutoInviteTeamMembers:
    """Test auto-invite team members functionality."""

    @pytest.fixture
    def client(self):
        """Create a GitHub client with mocked gh CLI."""
        client = GitHubClient(token="test-token")
        client._gh_available = False
        return client

    def test_auto_invite_all_members(self, client):
        """Test auto-inviting all organization members."""
        with patch.object(client, "_api_request") as mock_api:
            # First call: get members
            # Second call onwards: invite each member
            mock_api.side_effect = [
                (200, [{"login": "user1"}, {"login": "user2"}]),  # get members
                (201, {"id": 1}),  # invite user1
                (201, {"id": 2}),  # invite user2
            ]

            result = client.auto_invite_team_members("owner", "repo", "test-org")

            assert result.success is True
            assert len(result.data["invited"]) == 2
            assert "user1" in result.data["invited"]
            assert "user2" in result.data["invited"]

    def test_auto_invite_some_already_collaborators(self, client):
        """Test auto-invite with some existing collaborators."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.side_effect = [
                (200, [{"login": "user1"}, {"login": "user2"}]),  # get members
                (201, {"id": 1}),  # invite user1 (new)
                (204, None),  # user2 already collaborator
            ]

            result = client.auto_invite_team_members("owner", "repo", "test-org")

            assert result.success is True
            assert len(result.data["invited"]) == 1
            assert len(result.data["already_collaborators"]) == 1

    def test_auto_invite_failed_to_get_members(self, client):
        """Test auto-invite when failed to get members."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (401, {"message": "Bad credentials"})

            result = client.auto_invite_team_members("owner", "repo", "test-org")

            assert result.success is False
            assert "Failed to get organization members" in result.message


class TestGhCliUsage:
    """Test operations using gh CLI."""

    def test_check_membership_via_gh(self):
        """Test checking membership via gh CLI."""
        client = GitHubClient(token="test")
        client._gh_available = True

        with patch.object(client, "_run_gh") as mock_gh:
            mock_gh.return_value = MagicMock(returncode=0, stderr="")

            result = client.check_org_membership("test-org", "test-user")

            assert result.success is True
            assert result.data["is_member"] is True
            mock_gh.assert_called_once()

    def test_invite_collaborator_via_gh(self):
        """Test inviting collaborator via gh CLI."""
        client = GitHubClient(token="test")
        client._gh_available = True

        with patch.object(client, "_run_gh") as mock_gh:
            mock_gh.return_value = MagicMock(returncode=0, stdout="")

            result = client.invite_collaborator("owner", "repo", "user")

            assert result.success is True
            mock_gh.assert_called_once()

    def test_list_collaborators_via_gh(self):
        """Test listing collaborators via gh CLI."""
        client = GitHubClient(token="test")
        client._gh_available = True

        with patch.object(client, "_run_gh") as mock_gh:
            mock_gh.return_value = MagicMock(
                returncode=0,
                stdout=json.dumps(
                    [
                        {"login": "user1", "permissions": {}},
                        {"login": "user2", "permissions": {}},
                    ]
                ),
            )

            result = client.list_collaborators("owner", "repo")

            assert result.success is True
            assert len(result.data["collaborators"]) == 2


class TestEnums:
    """Test enum values."""

    def test_membership_role_values(self):
        """Test MembershipRole enum values."""
        assert MembershipRole.MEMBER.value == "member"
        assert MembershipRole.ADMIN.value == "admin"
        assert MembershipRole.NONE.value == "none"

    def test_collaborator_permission_values(self):
        """Test CollaboratorPermission enum values."""
        assert CollaboratorPermission.PULL.value == "pull"
        assert CollaboratorPermission.PUSH.value == "push"
        assert CollaboratorPermission.ADMIN.value == "admin"
        assert CollaboratorPermission.MAINTAIN.value == "maintain"
        assert CollaboratorPermission.TRIAGE.value == "triage"


class TestGitHubResult:
    """Test GitHubResult dataclass."""

    def test_result_success(self):
        """Test successful result."""
        result = GitHubResult(
            success=True,
            message="Operation completed",
            data={"key": "value"},
        )
        assert result.success is True
        assert result.message == "Operation completed"
        assert result.data == {"key": "value"}

    def test_result_failure(self):
        """Test failure result."""
        result = GitHubResult(
            success=False,
            message="Operation failed",
        )
        assert result.success is False
        assert result.data is None
