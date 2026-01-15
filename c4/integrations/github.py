"""GitHub Integration - Organization membership and collaborator management."""

from __future__ import annotations

import os
from dataclasses import dataclass
from enum import Enum
from typing import Any


class MembershipStatus(str, Enum):
    """Organization membership status."""

    ACTIVE = "active"
    PENDING = "pending"
    NOT_MEMBER = "not_member"
    UNKNOWN = "unknown"


class PermissionLevel(str, Enum):
    """Repository permission levels."""

    ADMIN = "admin"
    MAINTAIN = "maintain"
    WRITE = "push"
    TRIAGE = "triage"
    READ = "pull"


@dataclass
class OrgMembership:
    """Organization membership info."""

    org: str
    username: str
    status: MembershipStatus
    role: str | None = None  # "admin", "member"
    state: str | None = None  # "active", "pending"


@dataclass
class RepoCollaborator:
    """Repository collaborator info."""

    repo: str
    username: str
    permission: PermissionLevel
    invitation_id: int | None = None


class GitHubClient:
    """
    GitHub API client for organization and repository operations.

    Requires a GitHub token with appropriate scopes:
    - read:org - for organization membership checks
    - admin:org - for inviting members
    - repo - for collaborator management

    Example:
        client = GitHubClient()  # Uses GITHUB_TOKEN env
        status = client.check_org_membership("my-org", "username")
    """

    API_BASE = "https://api.github.com"

    def __init__(self, token: str | None = None):
        """Initialize GitHub client.

        Args:
            token: GitHub token (or GITHUB_TOKEN env)
        """
        self._token = token or os.environ.get("GITHUB_TOKEN")
        self._client: Any = None

    @property
    def client(self) -> Any:
        """Get HTTP client (lazy init)."""
        if self._client is None:
            import httpx

            headers = {
                "Accept": "application/vnd.github.v3+json",
                "X-GitHub-Api-Version": "2022-11-28",
            }
            if self._token:
                headers["Authorization"] = f"Bearer {self._token}"

            self._client = httpx.Client(
                base_url=self.API_BASE,
                headers=headers,
                timeout=30.0,
            )
        return self._client

    def close(self) -> None:
        """Close HTTP client."""
        if self._client:
            self._client.close()
            self._client = None

    def __enter__(self) -> "GitHubClient":
        return self

    def __exit__(self, *args: Any) -> None:
        self.close()

    # =========================================================================
    # Organization Membership
    # =========================================================================

    def check_org_membership(self, org: str, username: str) -> OrgMembership:
        """Check if a user is a member of an organization.

        Args:
            org: Organization name
            username: GitHub username

        Returns:
            OrgMembership with status details
        """
        try:
            response = self.client.get(f"/orgs/{org}/members/{username}")

            if response.status_code == 204:
                return OrgMembership(
                    org=org,
                    username=username,
                    status=MembershipStatus.ACTIVE,
                    role="member",
                    state="active",
                )
            elif response.status_code == 404:
                return OrgMembership(
                    org=org,
                    username=username,
                    status=MembershipStatus.NOT_MEMBER,
                )
            elif response.status_code == 302:
                # Redirect means pending invite
                return OrgMembership(
                    org=org,
                    username=username,
                    status=MembershipStatus.PENDING,
                    state="pending",
                )
            else:
                return OrgMembership(
                    org=org,
                    username=username,
                    status=MembershipStatus.UNKNOWN,
                )
        except Exception:
            return OrgMembership(
                org=org,
                username=username,
                status=MembershipStatus.UNKNOWN,
            )

    def get_org_membership(self, org: str, username: str) -> OrgMembership:
        """Get detailed membership info for a user.

        Args:
            org: Organization name
            username: GitHub username

        Returns:
            OrgMembership with role and state
        """
        try:
            response = self.client.get(f"/orgs/{org}/memberships/{username}")

            if response.status_code == 200:
                data = response.json()
                return OrgMembership(
                    org=org,
                    username=username,
                    status=MembershipStatus.ACTIVE
                    if data.get("state") == "active"
                    else MembershipStatus.PENDING,
                    role=data.get("role"),
                    state=data.get("state"),
                )
            else:
                return OrgMembership(
                    org=org,
                    username=username,
                    status=MembershipStatus.NOT_MEMBER,
                )
        except Exception:
            return OrgMembership(
                org=org,
                username=username,
                status=MembershipStatus.UNKNOWN,
            )

    def invite_to_org(
        self,
        org: str,
        username: str | None = None,
        email: str | None = None,
        role: str = "direct_member",
        team_ids: list[int] | None = None,
    ) -> bool:
        """Invite a user to an organization.

        Args:
            org: Organization name
            username: GitHub username (or email)
            email: Email address (if username not provided)
            role: Role to assign ("admin", "direct_member", "billing_manager")
            team_ids: Optional team IDs to add user to

        Returns:
            True if invited successfully
        """
        if not username and not email:
            return False

        try:
            payload: dict[str, Any] = {"role": role}
            if username:
                # Get user ID first
                user_response = self.client.get(f"/users/{username}")
                if user_response.status_code == 200:
                    payload["invitee_id"] = user_response.json()["id"]
                else:
                    return False
            else:
                payload["email"] = email

            if team_ids:
                payload["team_ids"] = team_ids

            response = self.client.post(f"/orgs/{org}/invitations", json=payload)
            return response.status_code in (201, 422)  # 422 = already invited

        except Exception:
            return False

    # =========================================================================
    # Repository Collaborators
    # =========================================================================

    def check_repo_collaborator(
        self, owner: str, repo: str, username: str
    ) -> RepoCollaborator | None:
        """Check if a user is a collaborator on a repository.

        Args:
            owner: Repository owner
            repo: Repository name
            username: GitHub username

        Returns:
            RepoCollaborator if found, None otherwise
        """
        try:
            response = self.client.get(
                f"/repos/{owner}/{repo}/collaborators/{username}/permission"
            )

            if response.status_code == 200:
                data = response.json()
                permission = data.get("permission", "read")
                return RepoCollaborator(
                    repo=f"{owner}/{repo}",
                    username=username,
                    permission=PermissionLevel(permission),
                )
            return None
        except Exception:
            return None

    def add_repo_collaborator(
        self,
        owner: str,
        repo: str,
        username: str,
        permission: PermissionLevel = PermissionLevel.WRITE,
    ) -> RepoCollaborator | None:
        """Add a collaborator to a repository.

        Args:
            owner: Repository owner
            repo: Repository name
            username: GitHub username
            permission: Permission level to grant

        Returns:
            RepoCollaborator with invitation details, None on failure
        """
        try:
            response = self.client.put(
                f"/repos/{owner}/{repo}/collaborators/{username}",
                json={"permission": permission.value},
            )

            if response.status_code == 201:
                # Invitation sent
                data = response.json()
                return RepoCollaborator(
                    repo=f"{owner}/{repo}",
                    username=username,
                    permission=permission,
                    invitation_id=data.get("id"),
                )
            elif response.status_code == 204:
                # Already a collaborator
                return RepoCollaborator(
                    repo=f"{owner}/{repo}",
                    username=username,
                    permission=permission,
                )
            return None
        except Exception:
            return None

    def remove_repo_collaborator(
        self, owner: str, repo: str, username: str
    ) -> bool:
        """Remove a collaborator from a repository.

        Args:
            owner: Repository owner
            repo: Repository name
            username: GitHub username

        Returns:
            True if removed successfully
        """
        try:
            response = self.client.delete(
                f"/repos/{owner}/{repo}/collaborators/{username}"
            )
            return response.status_code == 204
        except Exception:
            return False

    def list_repo_collaborators(
        self, owner: str, repo: str
    ) -> list[RepoCollaborator]:
        """List all collaborators on a repository.

        Args:
            owner: Repository owner
            repo: Repository name

        Returns:
            List of collaborators
        """
        try:
            response = self.client.get(
                f"/repos/{owner}/{repo}/collaborators",
                params={"per_page": 100},
            )

            if response.status_code != 200:
                return []

            collaborators = []
            for collab in response.json():
                perm = collab.get("permissions", {})
                if perm.get("admin"):
                    level = PermissionLevel.ADMIN
                elif perm.get("maintain"):
                    level = PermissionLevel.MAINTAIN
                elif perm.get("push"):
                    level = PermissionLevel.WRITE
                elif perm.get("triage"):
                    level = PermissionLevel.TRIAGE
                else:
                    level = PermissionLevel.READ

                collaborators.append(
                    RepoCollaborator(
                        repo=f"{owner}/{repo}",
                        username=collab["login"],
                        permission=level,
                    )
                )
            return collaborators
        except Exception:
            return []


class GitHubPermissionManager:
    """
    Manages GitHub permissions for C4 teams.

    Features:
    - Auto-invite team members to organization
    - Auto-add collaborators to project repositories
    - Verify required permissions before task assignment

    Example:
        manager = GitHubPermissionManager(client)
        manager.ensure_repo_access("owner/repo", ["user1", "user2"])
    """

    def __init__(self, client: GitHubClient | None = None):
        """Initialize permission manager.

        Args:
            client: GitHub client (creates one if not provided)
        """
        self._client = client or GitHubClient()
        self._owns_client = client is None

    def close(self) -> None:
        """Close client if we own it."""
        if self._owns_client:
            self._client.close()

    def __enter__(self) -> "GitHubPermissionManager":
        return self

    def __exit__(self, *args: Any) -> None:
        self.close()

    # =========================================================================
    # Organization Management
    # =========================================================================

    def check_org_members(
        self, org: str, usernames: list[str]
    ) -> dict[str, MembershipStatus]:
        """Check membership status for multiple users.

        Args:
            org: Organization name
            usernames: List of usernames to check

        Returns:
            Dict mapping username to membership status
        """
        results = {}
        for username in usernames:
            membership = self._client.check_org_membership(org, username)
            results[username] = membership.status
        return results

    def invite_to_org(
        self,
        org: str,
        usernames: list[str],
        skip_existing: bool = True,
    ) -> dict[str, bool]:
        """Invite multiple users to an organization.

        Args:
            org: Organization name
            usernames: Users to invite
            skip_existing: Skip users who are already members

        Returns:
            Dict mapping username to invite success
        """
        results = {}
        for username in usernames:
            if skip_existing:
                membership = self._client.check_org_membership(org, username)
                if membership.status == MembershipStatus.ACTIVE:
                    results[username] = True
                    continue

            results[username] = self._client.invite_to_org(org, username=username)
        return results

    # =========================================================================
    # Repository Management
    # =========================================================================

    def ensure_repo_access(
        self,
        owner: str,
        repo: str,
        usernames: list[str],
        permission: PermissionLevel = PermissionLevel.WRITE,
    ) -> dict[str, bool]:
        """Ensure users have access to a repository.

        Adds users as collaborators if they don't have access.

        Args:
            owner: Repository owner
            repo: Repository name
            usernames: Users who need access
            permission: Minimum permission level

        Returns:
            Dict mapping username to success status
        """
        results = {}
        for username in usernames:
            # Check existing access
            collab = self._client.check_repo_collaborator(owner, repo, username)
            if collab and self._has_sufficient_permission(collab.permission, permission):
                results[username] = True
                continue

            # Add as collaborator
            new_collab = self._client.add_repo_collaborator(
                owner, repo, username, permission
            )
            results[username] = new_collab is not None
        return results

    def verify_team_access(
        self,
        owner: str,
        repo: str,
        team_members: list[str],
        required_permission: PermissionLevel = PermissionLevel.WRITE,
    ) -> dict[str, bool]:
        """Verify all team members have required access.

        Args:
            owner: Repository owner
            repo: Repository name
            team_members: Team member usernames
            required_permission: Required permission level

        Returns:
            Dict mapping username to has-access boolean
        """
        results = {}
        for username in team_members:
            collab = self._client.check_repo_collaborator(owner, repo, username)
            if collab:
                results[username] = self._has_sufficient_permission(
                    collab.permission, required_permission
                )
            else:
                results[username] = False
        return results

    def _has_sufficient_permission(
        self, current: PermissionLevel, required: PermissionLevel
    ) -> bool:
        """Check if current permission meets required level."""
        permission_order = [
            PermissionLevel.READ,
            PermissionLevel.TRIAGE,
            PermissionLevel.WRITE,
            PermissionLevel.MAINTAIN,
            PermissionLevel.ADMIN,
        ]
        current_idx = permission_order.index(current)
        required_idx = permission_order.index(required)
        return current_idx >= required_idx
