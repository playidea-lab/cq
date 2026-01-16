"""Tests for GitHub Integration."""

from unittest.mock import MagicMock, patch

import pytest

from c4.integrations.github import (
    GitHubClient,
    GitHubPermissionManager,
    MembershipStatus,
    OrgMembership,
    PermissionLevel,
    RepoCollaborator,
)


class TestEnums:
    """Test enum values."""

    def test_membership_status(self) -> None:
        """Test MembershipStatus values."""
        assert MembershipStatus.ACTIVE.value == "active"
        assert MembershipStatus.PENDING.value == "pending"
        assert MembershipStatus.NOT_MEMBER.value == "not_member"

    def test_permission_level(self) -> None:
        """Test PermissionLevel values."""
        assert PermissionLevel.ADMIN.value == "admin"
        assert PermissionLevel.WRITE.value == "push"
        assert PermissionLevel.READ.value == "pull"


class TestDataclasses:
    """Test dataclass structures."""

    def test_org_membership(self) -> None:
        """Test OrgMembership dataclass."""
        membership = OrgMembership(
            org="my-org",
            username="user1",
            status=MembershipStatus.ACTIVE,
            role="member",
        )

        assert membership.org == "my-org"
        assert membership.username == "user1"
        assert membership.status == MembershipStatus.ACTIVE

    def test_repo_collaborator(self) -> None:
        """Test RepoCollaborator dataclass."""
        collab = RepoCollaborator(
            repo="owner/repo",
            username="user1",
            permission=PermissionLevel.WRITE,
        )

        assert collab.repo == "owner/repo"
        assert collab.permission == PermissionLevel.WRITE


class TestGitHubClientInit:
    """Test GitHubClient initialization."""

    def test_init_with_token(self) -> None:
        """Test initialization with explicit token."""
        client = GitHubClient(token="ghp_test123")
        assert client._token == "ghp_test123"

    def test_init_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test initialization from environment."""
        monkeypatch.setenv("GITHUB_TOKEN", "ghp_env_token")
        client = GitHubClient()
        assert client._token == "ghp_env_token"

    def test_context_manager(self) -> None:
        """Test context manager protocol."""
        with GitHubClient(token="test") as client:
            assert client is not None


class TestOrgMembership:
    """Test organization membership methods."""

    @pytest.fixture
    def client(self) -> GitHubClient:
        """Create client with mock HTTP client."""
        client = GitHubClient(token="test")
        client._client = MagicMock()
        return client

    def test_check_membership_active(self, client: GitHubClient) -> None:
        """Test checking active membership."""
        mock_response = MagicMock()
        mock_response.status_code = 204
        client._client.get.return_value = mock_response

        result = client.check_org_membership("my-org", "user1")

        assert result.status == MembershipStatus.ACTIVE
        assert result.role == "member"

    def test_check_membership_not_member(self, client: GitHubClient) -> None:
        """Test checking non-member."""
        mock_response = MagicMock()
        mock_response.status_code = 404
        client._client.get.return_value = mock_response

        result = client.check_org_membership("my-org", "user1")

        assert result.status == MembershipStatus.NOT_MEMBER

    def test_get_org_membership_details(self, client: GitHubClient) -> None:
        """Test getting detailed membership info."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "role": "admin",
            "state": "active",
        }
        client._client.get.return_value = mock_response

        result = client.get_org_membership("my-org", "user1")

        assert result.status == MembershipStatus.ACTIVE
        assert result.role == "admin"

    def test_invite_to_org(self, client: GitHubClient) -> None:
        """Test inviting user to organization."""
        # Mock user lookup
        user_response = MagicMock()
        user_response.status_code = 200
        user_response.json.return_value = {"id": 12345}

        # Mock invitation
        invite_response = MagicMock()
        invite_response.status_code = 201

        client._client.get.return_value = user_response
        client._client.post.return_value = invite_response

        result = client.invite_to_org("my-org", username="user1")

        assert result is True


class TestRepoCollaborators:
    """Test repository collaborator methods."""

    @pytest.fixture
    def client(self) -> GitHubClient:
        """Create client with mock HTTP client."""
        client = GitHubClient(token="test")
        client._client = MagicMock()
        return client

    def test_check_collaborator(self, client: GitHubClient) -> None:
        """Test checking collaborator status."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"permission": "push"}
        client._client.get.return_value = mock_response

        result = client.check_repo_collaborator("owner", "repo", "user1")

        assert result is not None
        assert result.permission == PermissionLevel.WRITE

    def test_check_collaborator_not_found(self, client: GitHubClient) -> None:
        """Test checking non-collaborator."""
        mock_response = MagicMock()
        mock_response.status_code = 404
        client._client.get.return_value = mock_response

        result = client.check_repo_collaborator("owner", "repo", "user1")

        assert result is None

    def test_add_collaborator_new(self, client: GitHubClient) -> None:
        """Test adding new collaborator."""
        mock_response = MagicMock()
        mock_response.status_code = 201
        mock_response.json.return_value = {"id": 99999}
        client._client.put.return_value = mock_response

        result = client.add_repo_collaborator(
            "owner", "repo", "user1", PermissionLevel.WRITE
        )

        assert result is not None
        assert result.invitation_id == 99999

    def test_add_collaborator_existing(self, client: GitHubClient) -> None:
        """Test adding existing collaborator."""
        mock_response = MagicMock()
        mock_response.status_code = 204
        client._client.put.return_value = mock_response

        result = client.add_repo_collaborator("owner", "repo", "user1")

        assert result is not None
        assert result.invitation_id is None

    def test_remove_collaborator(self, client: GitHubClient) -> None:
        """Test removing collaborator."""
        mock_response = MagicMock()
        mock_response.status_code = 204
        client._client.delete.return_value = mock_response

        result = client.remove_repo_collaborator("owner", "repo", "user1")

        assert result is True

    def test_list_collaborators(self, client: GitHubClient) -> None:
        """Test listing collaborators."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = [
            {"login": "user1", "permissions": {"push": True, "pull": True}},
            {"login": "user2", "permissions": {"admin": True}},
        ]
        client._client.get.return_value = mock_response

        result = client.list_repo_collaborators("owner", "repo")

        assert len(result) == 2
        assert result[0].username == "user1"
        assert result[0].permission == PermissionLevel.WRITE
        assert result[1].permission == PermissionLevel.ADMIN


class TestPermissionManager:
    """Test GitHubPermissionManager."""

    @pytest.fixture
    def mock_client(self) -> MagicMock:
        """Create mock GitHub client."""
        return MagicMock(spec=GitHubClient)

    @pytest.fixture
    def manager(self, mock_client: MagicMock) -> GitHubPermissionManager:
        """Create permission manager with mock client."""
        return GitHubPermissionManager(client=mock_client)

    def test_check_org_members(
        self, manager: GitHubPermissionManager, mock_client: MagicMock
    ) -> None:
        """Test checking multiple org members."""
        mock_client.check_org_membership.side_effect = [
            OrgMembership("org", "user1", MembershipStatus.ACTIVE),
            OrgMembership("org", "user2", MembershipStatus.NOT_MEMBER),
        ]

        result = manager.check_org_members("org", ["user1", "user2"])

        assert result["user1"] == MembershipStatus.ACTIVE
        assert result["user2"] == MembershipStatus.NOT_MEMBER

    def test_invite_to_org(
        self, manager: GitHubPermissionManager, mock_client: MagicMock
    ) -> None:
        """Test inviting multiple users."""
        mock_client.check_org_membership.return_value = OrgMembership(
            "org", "user1", MembershipStatus.NOT_MEMBER
        )
        mock_client.invite_to_org.return_value = True

        result = manager.invite_to_org("org", ["user1"])

        assert result["user1"] is True
        mock_client.invite_to_org.assert_called()

    def test_invite_skip_existing(
        self, manager: GitHubPermissionManager, mock_client: MagicMock
    ) -> None:
        """Test skipping existing members when inviting."""
        mock_client.check_org_membership.return_value = OrgMembership(
            "org", "user1", MembershipStatus.ACTIVE
        )

        result = manager.invite_to_org("org", ["user1"], skip_existing=True)

        assert result["user1"] is True
        mock_client.invite_to_org.assert_not_called()

    def test_ensure_repo_access_existing(
        self, manager: GitHubPermissionManager, mock_client: MagicMock
    ) -> None:
        """Test ensuring access for existing collaborator."""
        mock_client.check_repo_collaborator.return_value = RepoCollaborator(
            repo="owner/repo",
            username="user1",
            permission=PermissionLevel.WRITE,
        )

        result = manager.ensure_repo_access(
            "owner", "repo", ["user1"], PermissionLevel.WRITE
        )

        assert result["user1"] is True
        mock_client.add_repo_collaborator.assert_not_called()

    def test_ensure_repo_access_new(
        self, manager: GitHubPermissionManager, mock_client: MagicMock
    ) -> None:
        """Test ensuring access for new user."""
        mock_client.check_repo_collaborator.return_value = None
        mock_client.add_repo_collaborator.return_value = RepoCollaborator(
            repo="owner/repo",
            username="user1",
            permission=PermissionLevel.WRITE,
        )

        result = manager.ensure_repo_access("owner", "repo", ["user1"])

        assert result["user1"] is True
        mock_client.add_repo_collaborator.assert_called()

    def test_verify_team_access(
        self, manager: GitHubPermissionManager, mock_client: MagicMock
    ) -> None:
        """Test verifying team member access."""
        mock_client.check_repo_collaborator.side_effect = [
            RepoCollaborator("repo", "user1", PermissionLevel.WRITE),
            RepoCollaborator("repo", "user2", PermissionLevel.READ),
            None,
        ]

        result = manager.verify_team_access(
            "owner", "repo", ["user1", "user2", "user3"], PermissionLevel.WRITE
        )

        assert result["user1"] is True  # Has write
        assert result["user2"] is False  # Only has read
        assert result["user3"] is False  # Not a collaborator

    def test_permission_comparison(
        self, manager: GitHubPermissionManager
    ) -> None:
        """Test permission level comparison."""
        assert manager._has_sufficient_permission(
            PermissionLevel.ADMIN, PermissionLevel.WRITE
        )
        assert manager._has_sufficient_permission(
            PermissionLevel.WRITE, PermissionLevel.WRITE
        )
        assert not manager._has_sufficient_permission(
            PermissionLevel.READ, PermissionLevel.WRITE
        )

    def test_context_manager(self, mock_client: MagicMock) -> None:
        """Test context manager protocol."""
        with GitHubPermissionManager(client=mock_client) as manager:
            assert manager is not None
