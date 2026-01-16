"""C4 GitHub Integration - GitHub API operations for permission management."""

from __future__ import annotations

import json
import logging
import os
import subprocess
from dataclasses import dataclass
from enum import Enum
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

logger = logging.getLogger(__name__)


class MembershipRole(Enum):
    """GitHub organization membership roles."""

    MEMBER = "member"
    ADMIN = "admin"
    NONE = "none"


class CollaboratorPermission(Enum):
    """GitHub repository collaborator permissions."""

    PULL = "pull"
    PUSH = "push"
    ADMIN = "admin"
    MAINTAIN = "maintain"
    TRIAGE = "triage"


@dataclass
class GitHubResult:
    """Result of a GitHub API operation."""

    success: bool
    message: str
    data: dict[str, Any] | None = None


class GitHubClient:
    """GitHub API client for permission management.

    Supports both `gh` CLI and direct API calls.
    """

    API_BASE = "https://api.github.com"

    def __init__(self, token: str | None = None):
        """Initialize GitHub client.

        Args:
            token: GitHub personal access token. If not provided,
                   uses GITHUB_TOKEN environment variable.
        """
        self.token = token or os.environ.get("GITHUB_TOKEN")
        self._gh_available: bool | None = None

    def is_gh_available(self) -> bool:
        """Check if GitHub CLI is available."""
        if self._gh_available is None:
            try:
                result = subprocess.run(
                    ["gh", "--version"],
                    capture_output=True,
                    check=True,
                )
                self._gh_available = result.returncode == 0
            except (subprocess.CalledProcessError, FileNotFoundError):
                self._gh_available = False
        return self._gh_available

    def _run_gh(self, *args: str) -> subprocess.CompletedProcess[str]:
        """Run a gh CLI command.

        Args:
            *args: Command arguments

        Returns:
            CompletedProcess with stdout/stderr as text
        """
        return subprocess.run(
            ["gh", *args],
            capture_output=True,
            text=True,
        )

    def _api_request(
        self,
        method: str,
        endpoint: str,
        data: dict[str, Any] | None = None,
    ) -> tuple[int, dict[str, Any] | None]:
        """Make a direct API request to GitHub.

        Args:
            method: HTTP method (GET, POST, PUT, DELETE)
            endpoint: API endpoint (e.g., "/orgs/{org}/members/{username}")
            data: Request body data

        Returns:
            Tuple of (status_code, response_data)
        """
        if not self.token:
            return 401, {"message": "No GitHub token provided"}

        url = f"{self.API_BASE}{endpoint}"
        headers = {
            "Authorization": f"Bearer {self.token}",
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
        }

        body = json.dumps(data).encode() if data else None
        if body:
            headers["Content-Type"] = "application/json"

        req = Request(url, data=body, headers=headers, method=method)

        try:
            with urlopen(req) as response:
                status = response.status
                response_data = json.loads(response.read().decode())
                return status, response_data
        except HTTPError as e:
            try:
                error_body = json.loads(e.read().decode())
            except Exception:
                error_body = {"message": str(e)}
            return e.code, error_body
        except URLError as e:
            return 0, {"message": f"Network error: {e.reason}"}

    def check_org_membership(
        self,
        org: str,
        username: str,
    ) -> GitHubResult:
        """Check if a user is a member of an organization.

        Args:
            org: Organization name
            username: GitHub username to check

        Returns:
            GitHubResult with membership info
        """
        # Try gh CLI first
        if self.is_gh_available():
            result = self._run_gh(
                "api",
                f"/orgs/{org}/members/{username}",
                "--silent",
            )
            if result.returncode == 0:
                return GitHubResult(
                    success=True,
                    message=f"{username} is a member of {org}",
                    data={"is_member": True, "role": MembershipRole.MEMBER.value},
                )
            elif "404" in result.stderr or result.returncode == 1:
                return GitHubResult(
                    success=True,
                    message=f"{username} is not a member of {org}",
                    data={"is_member": False, "role": MembershipRole.NONE.value},
                )

        # Fallback to direct API
        status, data = self._api_request("GET", f"/orgs/{org}/members/{username}")

        if status == 204:  # No content means user is a member
            return GitHubResult(
                success=True,
                message=f"{username} is a member of {org}",
                data={"is_member": True, "role": MembershipRole.MEMBER.value},
            )
        elif status == 404:
            return GitHubResult(
                success=True,
                message=f"{username} is not a member of {org}",
                data={"is_member": False, "role": MembershipRole.NONE.value},
            )
        elif status == 401:
            return GitHubResult(
                success=False,
                message="Authentication failed",
                data=data,
            )
        else:
            return GitHubResult(
                success=False,
                message=f"Failed to check membership: {data}",
                data=data,
            )

    def get_org_membership(
        self,
        org: str,
        username: str,
    ) -> GitHubResult:
        """Get detailed membership info for a user in an organization.

        Args:
            org: Organization name
            username: GitHub username

        Returns:
            GitHubResult with detailed membership info (role, state)
        """
        # Try gh CLI first
        if self.is_gh_available():
            result = self._run_gh(
                "api",
                f"/orgs/{org}/memberships/{username}",
            )
            if result.returncode == 0:
                try:
                    data = json.loads(result.stdout)
                    return GitHubResult(
                        success=True,
                        message=f"Got membership for {username}",
                        data={
                            "role": data.get("role", "member"),
                            "state": data.get("state", "active"),
                            "is_member": True,
                        },
                    )
                except json.JSONDecodeError:
                    pass

        # Fallback to direct API
        status, data = self._api_request(
            "GET", f"/orgs/{org}/memberships/{username}"
        )

        if status == 200 and data:
            return GitHubResult(
                success=True,
                message=f"Got membership for {username}",
                data={
                    "role": data.get("role", "member"),
                    "state": data.get("state", "active"),
                    "is_member": True,
                },
            )
        elif status == 404:
            return GitHubResult(
                success=True,
                message=f"{username} is not a member of {org}",
                data={"is_member": False, "role": MembershipRole.NONE.value},
            )
        else:
            return GitHubResult(
                success=False,
                message=f"Failed to get membership: {data}",
                data=data,
            )

    def invite_collaborator(
        self,
        owner: str,
        repo: str,
        username: str,
        permission: CollaboratorPermission = CollaboratorPermission.PUSH,
    ) -> GitHubResult:
        """Invite a user as a collaborator to a repository.

        Args:
            owner: Repository owner
            repo: Repository name
            username: GitHub username to invite
            permission: Permission level to grant

        Returns:
            GitHubResult with invitation status
        """
        # Try gh CLI first
        if self.is_gh_available():
            result = self._run_gh(
                "api",
                f"/repos/{owner}/{repo}/collaborators/{username}",
                "-X", "PUT",
                "-f", f"permission={permission.value}",
            )
            if result.returncode == 0:
                return GitHubResult(
                    success=True,
                    message=f"Invited {username} to {owner}/{repo}",
                    data={"invited": True, "permission": permission.value},
                )
            elif "204" in result.stdout or result.returncode == 0:
                return GitHubResult(
                    success=True,
                    message=f"{username} is already a collaborator",
                    data={"invited": False, "already_collaborator": True},
                )

        # Fallback to direct API
        status, data = self._api_request(
            "PUT",
            f"/repos/{owner}/{repo}/collaborators/{username}",
            {"permission": permission.value},
        )

        if status == 201:  # Created - invitation sent
            return GitHubResult(
                success=True,
                message=f"Invited {username} to {owner}/{repo}",
                data={
                    "invited": True,
                    "permission": permission.value,
                    "invitation": data,
                },
            )
        elif status == 204:  # No content - already a collaborator
            return GitHubResult(
                success=True,
                message=f"{username} is already a collaborator",
                data={"invited": False, "already_collaborator": True},
            )
        elif status == 422:  # Unprocessable - user doesn't exist or can't be invited
            return GitHubResult(
                success=False,
                message=f"Cannot invite {username}: {data}",
                data=data,
            )
        else:
            return GitHubResult(
                success=False,
                message=f"Failed to invite collaborator: {data}",
                data=data,
            )

    def remove_collaborator(
        self,
        owner: str,
        repo: str,
        username: str,
    ) -> GitHubResult:
        """Remove a collaborator from a repository.

        Args:
            owner: Repository owner
            repo: Repository name
            username: GitHub username to remove

        Returns:
            GitHubResult with removal status
        """
        # Try gh CLI first
        if self.is_gh_available():
            result = self._run_gh(
                "api",
                f"/repos/{owner}/{repo}/collaborators/{username}",
                "-X", "DELETE",
            )
            if result.returncode == 0:
                return GitHubResult(
                    success=True,
                    message=f"Removed {username} from {owner}/{repo}",
                    data={"removed": True},
                )

        # Fallback to direct API
        status, data = self._api_request(
            "DELETE",
            f"/repos/{owner}/{repo}/collaborators/{username}",
        )

        if status == 204:  # No content - successfully removed
            return GitHubResult(
                success=True,
                message=f"Removed {username} from {owner}/{repo}",
                data={"removed": True},
            )
        elif status == 404:
            return GitHubResult(
                success=True,
                message=f"{username} was not a collaborator",
                data={"removed": False, "was_collaborator": False},
            )
        else:
            return GitHubResult(
                success=False,
                message=f"Failed to remove collaborator: {data}",
                data=data,
            )

    def list_collaborators(
        self,
        owner: str,
        repo: str,
    ) -> GitHubResult:
        """List all collaborators of a repository.

        Args:
            owner: Repository owner
            repo: Repository name

        Returns:
            GitHubResult with list of collaborators
        """
        # Try gh CLI first
        if self.is_gh_available():
            result = self._run_gh(
                "api",
                f"/repos/{owner}/{repo}/collaborators",
                "--paginate",
            )
            if result.returncode == 0:
                try:
                    collaborators = json.loads(result.stdout)
                    return GitHubResult(
                        success=True,
                        message=f"Found {len(collaborators)} collaborators",
                        data={
                            "collaborators": [
                                {
                                    "username": c["login"],
                                    "permissions": c.get("permissions", {}),
                                }
                                for c in collaborators
                            ]
                        },
                    )
                except json.JSONDecodeError:
                    pass

        # Fallback to direct API
        status, data = self._api_request(
            "GET",
            f"/repos/{owner}/{repo}/collaborators",
        )

        if status == 200 and isinstance(data, list):
            return GitHubResult(
                success=True,
                message=f"Found {len(data)} collaborators",
                data={
                    "collaborators": [
                        {
                            "username": c["login"],
                            "permissions": c.get("permissions", {}),
                        }
                        for c in data
                    ]
                },
            )
        else:
            return GitHubResult(
                success=False,
                message=f"Failed to list collaborators: {data}",
                data=data,
            )

    def check_collaborator(
        self,
        owner: str,
        repo: str,
        username: str,
    ) -> GitHubResult:
        """Check if a user is a collaborator of a repository.

        Args:
            owner: Repository owner
            repo: Repository name
            username: GitHub username to check

        Returns:
            GitHubResult with collaborator status
        """
        # Try gh CLI first
        if self.is_gh_available():
            result = self._run_gh(
                "api",
                f"/repos/{owner}/{repo}/collaborators/{username}",
                "--silent",
            )
            if result.returncode == 0:
                return GitHubResult(
                    success=True,
                    message=f"{username} is a collaborator",
                    data={"is_collaborator": True},
                )
            elif "404" in result.stderr:
                return GitHubResult(
                    success=True,
                    message=f"{username} is not a collaborator",
                    data={"is_collaborator": False},
                )

        # Fallback to direct API
        status, data = self._api_request(
            "GET",
            f"/repos/{owner}/{repo}/collaborators/{username}",
        )

        if status == 204:  # No content means user is a collaborator
            return GitHubResult(
                success=True,
                message=f"{username} is a collaborator",
                data={"is_collaborator": True},
            )
        elif status == 404:
            return GitHubResult(
                success=True,
                message=f"{username} is not a collaborator",
                data={"is_collaborator": False},
            )
        else:
            return GitHubResult(
                success=False,
                message=f"Failed to check collaborator: {data}",
                data=data,
            )

    def auto_invite_team_members(
        self,
        owner: str,
        repo: str,
        org: str,
        permission: CollaboratorPermission = CollaboratorPermission.PUSH,
    ) -> GitHubResult:
        """Automatically invite all organization members as collaborators.

        Args:
            owner: Repository owner
            repo: Repository name
            org: Organization to get members from
            permission: Permission level to grant

        Returns:
            GitHubResult with invitation results
        """
        # Get org members
        if self.is_gh_available():
            result = self._run_gh(
                "api",
                f"/orgs/{org}/members",
                "--paginate",
            )
            if result.returncode != 0:
                return GitHubResult(
                    success=False,
                    message="Failed to get organization members",
                    data={"error": result.stderr},
                )
            try:
                members = json.loads(result.stdout)
            except json.JSONDecodeError:
                return GitHubResult(
                    success=False,
                    message="Failed to parse members response",
                    data={},
                )
        else:
            status, data = self._api_request("GET", f"/orgs/{org}/members")
            if status != 200:
                return GitHubResult(
                    success=False,
                    message=f"Failed to get organization members: {data}",
                    data=data,
                )
            members = data if isinstance(data, list) else []

        # Invite each member
        invited = []
        already_collaborators = []
        failed = []

        for member in members:
            username = member.get("login")
            if not username:
                continue

            invite_result = self.invite_collaborator(
                owner, repo, username, permission
            )
            if invite_result.success:
                if invite_result.data and invite_result.data.get("invited"):
                    invited.append(username)
                else:
                    already_collaborators.append(username)
            else:
                failed.append({"username": username, "error": invite_result.message})

        return GitHubResult(
            success=len(failed) == 0,
            message=(
                f"Invited {len(invited)} members, "
                f"{len(already_collaborators)} already collaborators, "
                f"{len(failed)} failed"
            ),
            data={
                "invited": invited,
                "already_collaborators": already_collaborators,
                "failed": failed,
            },
        )
