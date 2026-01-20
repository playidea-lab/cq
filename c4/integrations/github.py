"""C4 GitHub Integration - GitHub API operations and automation.

Features:
- Permission management (collaborators, org membership)
- Auto commit on task completion
- Auto PR creation on project completion
"""

from __future__ import annotations

import json
import logging
import os
import subprocess
from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from pathlib import Path
from typing import TYPE_CHECKING, Any
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

if TYPE_CHECKING:
    from c4.models.config import GitHubConfig

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
        status, data = self._api_request("GET", f"/orgs/{org}/memberships/{username}")

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
                "-X",
                "PUT",
                "-f",
                f"permission={permission.value}",
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
                "-X",
                "DELETE",
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

            invite_result = self.invite_collaborator(owner, repo, username, permission)
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


# =============================================================================
# GitHub Automation
# =============================================================================


@dataclass
class CommitResult:
    """Result of a git commit operation."""

    success: bool
    message: str
    commit_sha: str | None = None
    files_changed: int = 0


@dataclass
class PRResult:
    """Result of a PR creation operation."""

    success: bool
    message: str
    pr_number: int | None = None
    pr_url: str | None = None


class GitHubAutomation:
    """Automates GitHub operations for C4 workflow.

    Features:
    - Auto commit on task completion
    - Auto PR creation on project completion
    - Commit message formatting
    - PR body generation

    Example:
        automation = GitHubAutomation(config, repo_path)
        result = automation.auto_commit(
            task_id="T-001",
            title="Add user authentication",
            files=["src/auth.py", "tests/test_auth.py"],
        )

        if result.success:
            print(f"Committed: {result.commit_sha}")
    """

    def __init__(
        self,
        config: "GitHubConfig",
        repo_path: Path | str,
        client: GitHubClient | None = None,
    ):
        """Initialize automation.

        Args:
            config: GitHub configuration
            repo_path: Path to git repository
            client: Optional GitHubClient for API operations
        """
        self.config = config
        self.repo_path = Path(repo_path)
        self.client = client or GitHubClient()
        self._repo_info: dict[str, str] | None = None

    @property
    def repo_info(self) -> dict[str, str]:
        """Get repository owner and name from git remote."""
        if self._repo_info is None:
            self._repo_info = self._get_repo_info()
        return self._repo_info

    def _get_repo_info(self) -> dict[str, str]:
        """Parse repository info from git remote URL."""
        try:
            result = subprocess.run(
                ["git", "remote", "get-url", "origin"],
                capture_output=True,
                text=True,
                cwd=self.repo_path,
            )
            if result.returncode != 0:
                return {"owner": "", "repo": ""}

            url = result.stdout.strip()

            # Parse git@github.com:owner/repo.git or https://github.com/owner/repo.git
            if url.startswith("git@"):
                # git@github.com:owner/repo.git
                parts = url.split(":")[-1].replace(".git", "").split("/")
            else:
                # https://github.com/owner/repo.git
                parts = url.replace(".git", "").split("/")[-2:]

            if len(parts) >= 2:
                return {"owner": parts[-2], "repo": parts[-1]}

        except Exception as e:
            logger.warning(f"Failed to get repo info: {e}")

        return {"owner": "", "repo": ""}

    def _run_git(self, *args: str) -> subprocess.CompletedProcess[str]:
        """Run a git command in the repository."""
        return subprocess.run(
            ["git", *args],
            capture_output=True,
            text=True,
            cwd=self.repo_path,
        )

    # =========================================================================
    # Auto Commit
    # =========================================================================

    def auto_commit(
        self,
        task_id: str,
        title: str,
        files: list[str] | None = None,
        body: str | None = None,
    ) -> CommitResult:
        """Create an auto-commit for a completed task.

        Args:
            task_id: Task identifier (e.g., "T-001")
            title: Task title
            files: Specific files to commit, or None for all changes
            body: Optional commit body

        Returns:
            CommitResult with commit details
        """
        if not self.config.enabled:
            return CommitResult(
                success=False,
                message="GitHub integration disabled",
            )

        if not self.config.auto_commit:
            return CommitResult(
                success=False,
                message="Auto-commit disabled",
            )

        try:
            # Stage files
            if files:
                for f in files:
                    result = self._run_git("add", f)
                    if result.returncode != 0:
                        logger.warning(f"Failed to stage {f}: {result.stderr}")
            else:
                # Stage all changes
                self._run_git("add", "-A")

            # Check if there are changes to commit
            status = self._run_git("status", "--porcelain")
            if not status.stdout.strip():
                return CommitResult(
                    success=True,
                    message="No changes to commit",
                    files_changed=0,
                )

            # Count staged files
            diff_stat = self._run_git("diff", "--cached", "--stat")
            files_changed = len(
                [line for line in diff_stat.stdout.split("\n") if line.strip() and "|" in line]
            )

            # Build commit message
            prefix = self.config.commit_prefix
            commit_msg = f"{prefix} {task_id}: {title}"

            if body:
                commit_msg += f"\n\n{body}"

            # Add timestamp
            commit_msg += f"\n\nGenerated by C4 at {datetime.now().isoformat()}"

            # Create commit
            result = self._run_git("commit", "-m", commit_msg)

            if result.returncode != 0:
                return CommitResult(
                    success=False,
                    message=f"Commit failed: {result.stderr}",
                )

            # Get commit SHA
            sha_result = self._run_git("rev-parse", "HEAD")
            commit_sha = sha_result.stdout.strip() if sha_result.returncode == 0 else None

            logger.info(f"Auto-commit created: {commit_sha} for {task_id}")

            return CommitResult(
                success=True,
                message=f"Committed {files_changed} files",
                commit_sha=commit_sha,
                files_changed=files_changed,
            )

        except Exception as e:
            logger.error(f"Auto-commit failed: {e}")
            return CommitResult(
                success=False,
                message=f"Error: {e}",
            )

    # =========================================================================
    # Auto PR
    # =========================================================================

    def auto_pr(
        self,
        project_id: str,
        title: str,
        tasks_completed: list[dict[str, str]],
        source_branch: str,
        target_branch: str | None = None,
    ) -> PRResult:
        """Create an auto-PR for completed project.

        Args:
            project_id: Project identifier
            title: PR title
            tasks_completed: List of completed tasks with id and title
            source_branch: Branch to create PR from
            target_branch: Target branch for PR (defaults to config.base_branch)

        Returns:
            PRResult with PR details
        """
        if not self.config.enabled:
            return PRResult(
                success=False,
                message="GitHub integration disabled",
            )

        if not self.config.auto_pr:
            return PRResult(
                success=False,
                message="Auto-PR disabled",
            )

        target = target_branch or self.config.base_branch

        # Build PR body
        body = self._build_pr_body(project_id, tasks_completed)

        try:
            # Use gh CLI for PR creation (most reliable)
            if self.client.is_gh_available():
                return self._create_pr_with_gh(
                    title=title,
                    body=body,
                    source_branch=source_branch,
                    target_branch=target,
                )
            else:
                # Fallback to API
                return self._create_pr_with_api(
                    title=title,
                    body=body,
                    source_branch=source_branch,
                    target_branch=target,
                )

        except Exception as e:
            logger.error(f"Auto-PR failed: {e}")
            return PRResult(
                success=False,
                message=f"Error: {e}",
            )

    def _build_pr_body(
        self,
        project_id: str,
        tasks_completed: list[dict[str, str]],
    ) -> str:
        """Build PR body from completed tasks."""
        lines = [
            "## Summary",
            f"Automated PR for C4 project: `{project_id}`",
            "",
            "## Completed Tasks",
        ]

        for task in tasks_completed:
            task_id = task.get("id", "Unknown")
            task_title = task.get("title", "No title")
            lines.append(f"- [x] **{task_id}**: {task_title}")

        lines.extend(
            [
                "",
                "## Verification",
                "- [ ] All validations passed",
                "- [ ] Code reviewed",
                "",
                "---",
                f"🤖 Generated by C4 at {datetime.now().isoformat()}",
            ]
        )

        return "\n".join(lines)

    def _create_pr_with_gh(
        self,
        title: str,
        body: str,
        source_branch: str,
        target_branch: str,
    ) -> PRResult:
        """Create PR using gh CLI."""
        args = [
            "pr",
            "create",
            "--title",
            title,
            "--body",
            body,
            "--base",
            target_branch,
            "--head",
            source_branch,
        ]

        # Add reviewers
        for reviewer in self.config.reviewers:
            args.extend(["--reviewer", reviewer])

        # Add labels
        for label in self.config.labels:
            args.extend(["--label", label])

        # Draft PR
        if self.config.draft:
            args.append("--draft")

        result = subprocess.run(
            ["gh", *args],
            capture_output=True,
            text=True,
            cwd=self.repo_path,
        )

        if result.returncode != 0:
            # Check if PR already exists
            if "already exists" in result.stderr.lower():
                return PRResult(
                    success=True,
                    message="PR already exists",
                )
            return PRResult(
                success=False,
                message=f"gh pr create failed: {result.stderr}",
            )

        # Parse PR URL from output
        pr_url = result.stdout.strip()
        pr_number = None

        if pr_url:
            # Extract PR number from URL
            try:
                pr_number = int(pr_url.split("/")[-1])
            except (ValueError, IndexError):
                pass

        logger.info(f"Auto-PR created: {pr_url}")

        return PRResult(
            success=True,
            message="PR created successfully",
            pr_number=pr_number,
            pr_url=pr_url,
        )

    def _create_pr_with_api(
        self,
        title: str,
        body: str,
        source_branch: str,
        target_branch: str,
    ) -> PRResult:
        """Create PR using GitHub API."""
        owner = self.repo_info["owner"]
        repo = self.repo_info["repo"]

        if not owner or not repo:
            return PRResult(
                success=False,
                message="Could not determine repository owner/name",
            )

        # Push branch first
        push_result = self._run_git("push", "-u", "origin", source_branch)
        if push_result.returncode != 0:
            logger.warning(f"Push may have failed: {push_result.stderr}")

        # Create PR via API
        data = {
            "title": title,
            "body": body,
            "head": source_branch,
            "base": target_branch,
            "draft": self.config.draft,
        }

        status, response = self.client._api_request(
            "POST",
            f"/repos/{owner}/{repo}/pulls",
            data,
        )

        if status == 201 and response:
            pr_number = response.get("number")
            pr_url = response.get("html_url")

            # Add reviewers if configured
            if self.config.reviewers and pr_number:
                self.client._api_request(
                    "POST",
                    f"/repos/{owner}/{repo}/pulls/{pr_number}/requested_reviewers",
                    {"reviewers": self.config.reviewers},
                )

            # Add labels if configured
            if self.config.labels and pr_number:
                self.client._api_request(
                    "POST",
                    f"/repos/{owner}/{repo}/issues/{pr_number}/labels",
                    {"labels": self.config.labels},
                )

            logger.info(f"Auto-PR created via API: {pr_url}")

            return PRResult(
                success=True,
                message="PR created successfully",
                pr_number=pr_number,
                pr_url=pr_url,
            )
        elif status == 422 and response:
            # PR might already exist
            errors = response.get("errors", [])
            for err in errors:
                if "pull request already exists" in str(err).lower():
                    return PRResult(
                        success=True,
                        message="PR already exists",
                    )

        return PRResult(
            success=False,
            message=f"API error {status}: {response}",
        )

    # =========================================================================
    # Utility
    # =========================================================================

    def push_branch(self, branch: str) -> bool:
        """Push a branch to origin.

        Args:
            branch: Branch name to push

        Returns:
            True if successful
        """
        result = self._run_git("push", "-u", "origin", branch)
        return result.returncode == 0

    def get_current_branch(self) -> str | None:
        """Get current git branch name."""
        result = self._run_git("rev-parse", "--abbrev-ref", "HEAD")
        if result.returncode == 0:
            return result.stdout.strip()
        return None

    def has_uncommitted_changes(self) -> bool:
        """Check if there are uncommitted changes."""
        result = self._run_git("status", "--porcelain")
        return bool(result.stdout.strip())
