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

    # =========================================================================
    # Enhanced PR Description
    # =========================================================================

    def generate_pr_description_from_commits(
        self,
        source_branch: str,
        target_branch: str | None = None,
        include_diffs: bool = False,
    ) -> str:
        """Generate PR description from commit history.

        Args:
            source_branch: Source branch name
            target_branch: Target branch (defaults to config.base_branch)
            include_diffs: Include file diffs summary

        Returns:
            Formatted PR description
        """
        target = target_branch or self.config.base_branch

        # Get commits between branches
        commits = self._get_commits_between(target, source_branch)

        # Get file changes summary
        file_changes = self._get_file_changes_summary(target, source_branch)

        # Build description
        lines = ["## Summary", ""]

        if commits:
            lines.extend(["### Changes", ""])
            for commit in commits:
                sha = commit.get("sha", "")[:7]
                subject = commit.get("subject", "")
                author = commit.get("author", "")
                lines.append(f"- `{sha}` {subject} (@{author})")
            lines.append("")

        if file_changes:
            lines.extend(["### Files Changed", ""])
            for change in file_changes[:20]:  # Limit to 20 files
                status = change.get("status", "M")
                file_path = change.get("file", "")
                symbol = {"A": "+", "D": "-", "M": "~"}.get(status, "?")
                lines.append(f"- {symbol} `{file_path}`")

            if len(file_changes) > 20:
                lines.append(f"- ... and {len(file_changes) - 20} more files")
            lines.append("")

        # Add statistics
        if file_changes:
            added = sum(1 for f in file_changes if f.get("status") == "A")
            deleted = sum(1 for f in file_changes if f.get("status") == "D")
            modified = len(file_changes) - added - deleted

            lines.extend([
                "### Statistics",
                f"- **{len(commits)}** commits",
                f"- **{len(file_changes)}** files ({added} added, {deleted} deleted, {modified} modified)",
                "",
            ])

        lines.extend([
            "---",
            f"🤖 Generated by C4 at {datetime.now().isoformat()}",
        ])

        return "\n".join(lines)

    def _get_commits_between(
        self, base: str, head: str
    ) -> list[dict[str, str]]:
        """Get commits between two branches."""
        result = self._run_git(
            "log",
            f"{base}..{head}",
            "--pretty=format:%h|%s|%an",
            "--no-merges",
        )

        if result.returncode != 0 or not result.stdout.strip():
            return []

        commits = []
        for line in result.stdout.strip().split("\n"):
            parts = line.split("|", 2)
            if len(parts) >= 3:
                commits.append({
                    "sha": parts[0],
                    "subject": parts[1],
                    "author": parts[2],
                })

        return commits

    def _get_file_changes_summary(
        self, base: str, head: str
    ) -> list[dict[str, str]]:
        """Get summary of file changes between branches."""
        result = self._run_git(
            "diff",
            f"{base}..{head}",
            "--name-status",
        )

        if result.returncode != 0 or not result.stdout.strip():
            return []

        changes = []
        for line in result.stdout.strip().split("\n"):
            parts = line.split("\t", 1)
            if len(parts) >= 2:
                changes.append({
                    "status": parts[0][0],  # First char (A, D, M, R, etc.)
                    "file": parts[1],
                })

        return changes

    # =========================================================================
    # CODEOWNERS Integration
    # =========================================================================

    def get_codeowners_reviewers(
        self,
        files: list[str],
    ) -> list[str]:
        """Get reviewers from CODEOWNERS for given files.

        Args:
            files: List of file paths to check

        Returns:
            List of GitHub usernames/teams to request review from
        """
        codeowners = self._parse_codeowners()
        if not codeowners:
            return []

        reviewers: set[str] = set()

        for file_path in files:
            # Check each pattern (last match wins per GitHub behavior)
            for pattern, owners in codeowners:
                if self._matches_codeowners_pattern(file_path, pattern):
                    reviewers.update(owners)

        return list(reviewers)

    def _parse_codeowners(self) -> list[tuple[str, list[str]]]:
        """Parse CODEOWNERS file.

        Returns:
            List of (pattern, [owners]) tuples
        """
        # Check common locations for CODEOWNERS
        locations = [
            self.repo_path / "CODEOWNERS",
            self.repo_path / ".github" / "CODEOWNERS",
            self.repo_path / "docs" / "CODEOWNERS",
        ]

        codeowners_path = None
        for loc in locations:
            if loc.exists():
                codeowners_path = loc
                break

        if not codeowners_path:
            return []

        try:
            content = codeowners_path.read_text()
        except Exception as e:
            logger.warning(f"Failed to read CODEOWNERS: {e}")
            return []

        patterns: list[tuple[str, list[str]]] = []

        for line in content.split("\n"):
            line = line.strip()

            # Skip comments and empty lines
            if not line or line.startswith("#"):
                continue

            parts = line.split()
            if len(parts) < 2:
                continue

            pattern = parts[0]
            owners = [o.lstrip("@") for o in parts[1:] if o.startswith("@")]

            if owners:
                patterns.append((pattern, owners))

        return patterns

    def _matches_codeowners_pattern(self, file_path: str, pattern: str) -> bool:
        """Check if a file path matches a CODEOWNERS pattern.

        Args:
            file_path: File path to check
            pattern: CODEOWNERS pattern (supports *, **, /)

        Returns:
            True if pattern matches
        """
        import fnmatch

        # Normalize paths
        file_path = file_path.lstrip("/")
        pattern = pattern.lstrip("/")

        # Handle directory patterns (ending with /)
        if pattern.endswith("/"):
            return file_path.startswith(pattern[:-1])

        # Handle ** patterns (match any depth)
        if "**" in pattern:
            # Convert ** to regex-like behavior
            pattern_parts = pattern.split("**")
            if len(pattern_parts) == 2:
                prefix, suffix = pattern_parts
                prefix = prefix.rstrip("/")
                suffix = suffix.lstrip("/")

                if prefix and not file_path.startswith(prefix):
                    return False
                if suffix and not file_path.endswith(suffix):
                    return False
                return True

        # Standard fnmatch
        return fnmatch.fnmatch(file_path, pattern)

    def auto_pr_with_codeowners(
        self,
        project_id: str,
        title: str,
        tasks_completed: list[dict[str, str]],
        source_branch: str,
        target_branch: str | None = None,
    ) -> PRResult:
        """Create PR with CODEOWNERS-based reviewer assignment.

        Args:
            project_id: Project identifier
            title: PR title
            tasks_completed: List of completed tasks
            source_branch: Source branch
            target_branch: Target branch (defaults to config.base_branch)

        Returns:
            PRResult with PR details
        """
        target = target_branch or self.config.base_branch

        # Get changed files
        file_changes = self._get_file_changes_summary(target, source_branch)
        changed_files = [f["file"] for f in file_changes]

        # Get reviewers from CODEOWNERS
        codeowners_reviewers = self.get_codeowners_reviewers(changed_files)

        # Merge with configured reviewers
        all_reviewers = list(set(self.config.reviewers + codeowners_reviewers))

        # Generate enhanced PR body
        body = self.generate_pr_description_from_commits(source_branch, target)

        # Add tasks section
        tasks_section = "\n\n## Completed Tasks\n"
        for task in tasks_completed:
            task_id = task.get("id", "Unknown")
            task_title = task.get("title", "No title")
            tasks_section += f"- [x] **{task_id}**: {task_title}\n"

        body += tasks_section

        # Create PR with reviewers
        try:
            if self.client.is_gh_available():
                return self._create_pr_with_reviewers(
                    title=title,
                    body=body,
                    source_branch=source_branch,
                    target_branch=target,
                    reviewers=all_reviewers,
                )
            else:
                # Fallback to regular auto_pr
                return self.auto_pr(
                    project_id=project_id,
                    title=title,
                    tasks_completed=tasks_completed,
                    source_branch=source_branch,
                    target_branch=target,
                )

        except Exception as e:
            logger.error(f"Auto-PR with CODEOWNERS failed: {e}")
            return PRResult(
                success=False,
                message=f"Error: {e}",
            )

    def _create_pr_with_reviewers(
        self,
        title: str,
        body: str,
        source_branch: str,
        target_branch: str,
        reviewers: list[str],
    ) -> PRResult:
        """Create PR with specific reviewers using gh CLI."""
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

        # Add reviewers (both from config and CODEOWNERS)
        for reviewer in reviewers:
            # Skip team reviewers for now (they need different handling)
            if "/" not in reviewer:
                args.extend(["--reviewer", reviewer])

        # Add labels
        for label in self.config.labels:
            args.extend(["--label", label])

        if self.config.draft:
            args.append("--draft")

        result = subprocess.run(
            ["gh", *args],
            capture_output=True,
            text=True,
            cwd=self.repo_path,
        )

        if result.returncode != 0:
            if "already exists" in result.stderr.lower():
                return PRResult(
                    success=True,
                    message="PR already exists",
                )
            return PRResult(
                success=False,
                message=f"gh pr create failed: {result.stderr}",
            )

        pr_url = result.stdout.strip()
        pr_number = None

        if pr_url:
            try:
                pr_number = int(pr_url.split("/")[-1])
            except (ValueError, IndexError):
                pass

        logger.info(f"Auto-PR created with CODEOWNERS reviewers: {pr_url}")

        return PRResult(
            success=True,
            message=f"PR created with {len(reviewers)} reviewers",
            pr_number=pr_number,
            pr_url=pr_url,
        )


# =============================================================================
# Webhook Handler
# =============================================================================


@dataclass
class WebhookEvent:
    """Parsed webhook event from GitHub."""

    event_type: str
    action: str | None
    repository: dict[str, Any]
    sender: dict[str, Any]
    payload: dict[str, Any]

    @property
    def repo_full_name(self) -> str:
        """Get full repository name (owner/repo)."""
        return self.repository.get("full_name", "")

    @property
    def sender_login(self) -> str:
        """Get sender username."""
        return self.sender.get("login", "")


class WebhookHandler:
    """Handle GitHub webhook events for C4 integration.

    Supported events:
    - push: Trigger reindexing, update task status
    - pull_request: Update checkpoint status
    - check_run: Track CI status
    - issue_comment: Handle review comments

    Example:
        handler = WebhookHandler(daemon)
        event = handler.parse_event(headers, body)
        await handler.handle(event)
    """

    def __init__(self, daemon: Any | None = None, secret: str | None = None):
        """Initialize webhook handler.

        Args:
            daemon: C4Daemon instance for state updates
            secret: Webhook secret for signature verification
        """
        self.daemon = daemon
        self.secret = secret or os.environ.get("GITHUB_WEBHOOK_SECRET")

    def verify_signature(self, payload: bytes, signature: str) -> bool:
        """Verify webhook signature.

        Args:
            payload: Raw request body
            signature: X-Hub-Signature-256 header value

        Returns:
            True if signature is valid
        """
        if not self.secret:
            logger.warning("No webhook secret configured, skipping verification")
            return True

        import hashlib
        import hmac

        expected = hmac.new(
            self.secret.encode(),
            payload,
            hashlib.sha256,
        ).hexdigest()

        # Compare with timing-safe function
        return hmac.compare_digest(f"sha256={expected}", signature)

    def parse_event(
        self,
        headers: dict[str, str],
        body: bytes | str,
    ) -> WebhookEvent | None:
        """Parse webhook event from request.

        Args:
            headers: Request headers
            body: Request body

        Returns:
            WebhookEvent or None if parsing failed
        """
        event_type = headers.get("X-GitHub-Event", headers.get("x-github-event", ""))
        signature = headers.get(
            "X-Hub-Signature-256", headers.get("x-hub-signature-256", "")
        )

        if isinstance(body, str):
            body = body.encode()

        # Verify signature
        if signature and not self.verify_signature(body, signature):
            logger.warning("Invalid webhook signature")
            return None

        try:
            payload = json.loads(body.decode())
        except json.JSONDecodeError as e:
            logger.error(f"Failed to parse webhook payload: {e}")
            return None

        return WebhookEvent(
            event_type=event_type,
            action=payload.get("action"),
            repository=payload.get("repository", {}),
            sender=payload.get("sender", {}),
            payload=payload,
        )

    async def handle(self, event: WebhookEvent) -> dict[str, Any]:
        """Handle a webhook event.

        Args:
            event: Parsed webhook event

        Returns:
            Result dictionary with handling status
        """
        handler_name = f"_handle_{event.event_type}"
        handler = getattr(self, handler_name, None)

        if handler is None:
            logger.debug(f"No handler for event: {event.event_type}")
            return {"handled": False, "reason": "no_handler"}

        try:
            return await handler(event)
        except Exception as e:
            logger.error(f"Error handling {event.event_type}: {e}")
            return {"handled": False, "error": str(e)}

    async def _handle_push(self, event: WebhookEvent) -> dict[str, Any]:
        """Handle push event - update tracking and trigger reindex."""
        commits = event.payload.get("commits", [])
        ref = event.payload.get("ref", "")

        logger.info(
            f"Push event: {len(commits)} commits to {ref} "
            f"by {event.sender_login} in {event.repo_full_name}"
        )

        # Extract task IDs from commit messages
        task_ids = []
        for commit in commits:
            message = commit.get("message", "")
            # Look for patterns like "T-001", "R-001", etc.
            import re
            matches = re.findall(r"[TR]-\d{3}", message)
            task_ids.extend(matches)

        result = {
            "handled": True,
            "commits": len(commits),
            "task_ids": list(set(task_ids)),
            "branch": ref.replace("refs/heads/", ""),
        }

        # Trigger daemon updates if available
        if self.daemon and task_ids:
            for task_id in set(task_ids):
                logger.info(f"Push affects task: {task_id}")

        return result

    async def _handle_pull_request(self, event: WebhookEvent) -> dict[str, Any]:
        """Handle pull_request event - track PR status."""
        action = event.action
        pr = event.payload.get("pull_request", {})
        pr_number = pr.get("number")
        pr_state = pr.get("state")
        merged = pr.get("merged", False)

        logger.info(
            f"PR event: #{pr_number} {action} (state={pr_state}, merged={merged}) "
            f"in {event.repo_full_name}"
        )

        result = {
            "handled": True,
            "pr_number": pr_number,
            "action": action,
            "state": pr_state,
            "merged": merged,
        }

        # Check for C4 project ID in PR body
        body = pr.get("body", "") or ""
        if "C4 project:" in body:
            import re
            match = re.search(r"C4 project:\s*`?([^`\s]+)`?", body)
            if match:
                result["project_id"] = match.group(1)

        return result

    async def _handle_check_run(self, event: WebhookEvent) -> dict[str, Any]:
        """Handle check_run event - track CI status."""
        check_run = event.payload.get("check_run", {})
        name = check_run.get("name")
        status = check_run.get("status")
        conclusion = check_run.get("conclusion")

        logger.info(
            f"Check run: {name} status={status} conclusion={conclusion} "
            f"in {event.repo_full_name}"
        )

        return {
            "handled": True,
            "check_name": name,
            "status": status,
            "conclusion": conclusion,
        }

    async def _handle_issue_comment(self, event: WebhookEvent) -> dict[str, Any]:
        """Handle issue_comment event - check for C4 commands."""
        action = event.action
        comment = event.payload.get("comment", {})
        issue = event.payload.get("issue", {})
        body = comment.get("body", "")

        # Check for C4 commands in comment
        c4_commands = []
        if body.startswith("/c4"):
            parts = body.split()
            if len(parts) >= 1:
                c4_commands.append(parts[0])

        logger.info(
            f"Issue comment: {action} on #{issue.get('number')} "
            f"by {event.sender_login}"
        )

        return {
            "handled": True,
            "action": action,
            "issue_number": issue.get("number"),
            "c4_commands": c4_commands,
        }
