"""C4 Git Operations - Automated git operations for task lifecycle."""

import subprocess
from datetime import datetime
from pathlib import Path
from typing import NamedTuple


class GitResult(NamedTuple):
    """Result of a git operation."""

    success: bool
    message: str
    sha: str | None = None
    tag: str | None = None


class GitOperations:
    """Handles automated Git operations for C4 tasks."""

    def __init__(self, project_root: Path):
        """Initialize GitOperations.

        Args:
            project_root: Root directory of the project (contains .git/)
        """
        self.root = project_root
        self._git_available: bool | None = None

    def is_git_repo(self) -> bool:
        """Check if project is a Git repository."""
        return (self.root / ".git").exists()

    def is_git_available(self) -> bool:
        """Check if git command is available."""
        if self._git_available is None:
            try:
                result = subprocess.run(
                    ["git", "--version"],
                    capture_output=True,
                    check=True,
                )
                self._git_available = result.returncode == 0
            except (subprocess.CalledProcessError, FileNotFoundError):
                self._git_available = False
        return self._git_available

    def _run_git(self, *args: str) -> subprocess.CompletedProcess[str]:
        """Run a git command in the project root."""
        return subprocess.run(
            ["git", *args],
            cwd=self.root,
            capture_output=True,
            text=True,
        )

    def get_current_sha(self) -> str | None:
        """Get the current HEAD commit SHA."""
        if not self.is_git_repo():
            return None

        result = self._run_git("rev-parse", "--short", "HEAD")
        if result.returncode == 0:
            return result.stdout.strip()
        return None

    def has_uncommitted_changes(self) -> bool:
        """Check if there are uncommitted changes."""
        if not self.is_git_repo():
            return False

        result = self._run_git("status", "--porcelain")
        return bool(result.stdout.strip())

    def stage_all(self) -> bool:
        """Stage all changes (git add -A)."""
        if not self.is_git_repo():
            return False

        result = self._run_git("add", "-A")
        return result.returncode == 0

    def commit_task_completion(
        self,
        task_id: str,
        task_title: str,
        worker_id: str | None = None,
    ) -> GitResult:
        """Create a commit for task completion.

        Args:
            task_id: Task ID (e.g., "T-001")
            task_title: Task title for commit message
            worker_id: Optional worker ID for reference

        Returns:
            GitResult with commit SHA if successful
        """
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        if not self.is_git_available():
            return GitResult(False, "Git not available")

        if not self.stage_all():
            return GitResult(False, "Failed to stage changes")

        if not self.has_uncommitted_changes():
            sha = self.get_current_sha()
            return GitResult(True, "No changes to commit", sha=sha)

        message_lines = [
            f"[{task_id}] {task_title}",
            "",
            f"Task ID: {task_id}",
        ]
        if worker_id:
            message_lines.append(f"Worker: {worker_id}")
        message_lines.append(f"Completed: {datetime.now().isoformat()}")

        commit_message = "\n".join(message_lines)

        result = self._run_git("commit", "-m", commit_message)
        if result.returncode != 0:
            if "nothing to commit" in result.stdout.lower():
                sha = self.get_current_sha()
                return GitResult(True, "No changes to commit", sha=sha)
            return GitResult(False, f"Commit failed: {result.stderr}")

        sha = self.get_current_sha()
        return GitResult(True, f"Committed {task_id}", sha=sha)

    def commit_repair_completion(
        self,
        task_id: str,
        original_task_id: str,
        repair_reason: str | None = None,
    ) -> GitResult:
        """Create a commit for repair task completion."""
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        if not self.is_git_available():
            return GitResult(False, "Git not available")

        if not self.stage_all():
            return GitResult(False, "Failed to stage changes")

        if not self.has_uncommitted_changes():
            sha = self.get_current_sha()
            return GitResult(True, "No changes to commit", sha=sha)

        message_lines = [
            f"[{task_id}] Repair: {original_task_id}",
            "",
            f"Original Task: {original_task_id}",
            f"Repair ID: {task_id}",
        ]
        if repair_reason:
            message_lines.append(f"Reason: {repair_reason}")
        message_lines.append(f"Completed: {datetime.now().isoformat()}")

        commit_message = "\n".join(message_lines)

        result = self._run_git("commit", "-m", commit_message)
        if result.returncode != 0:
            if "nothing to commit" in result.stdout.lower():
                sha = self.get_current_sha()
                return GitResult(True, "No changes to commit", sha=sha)
            return GitResult(False, f"Commit failed: {result.stderr}")

        sha = self.get_current_sha()
        return GitResult(True, f"Committed repair {task_id}", sha=sha)

    def create_checkpoint_tag(
        self,
        checkpoint_id: str,
        checkpoint_name: str | None = None,
    ) -> GitResult:
        """Create a Git tag for a checkpoint."""
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        if not self.is_git_available():
            return GitResult(False, "Git not available")

        tag_name = f"c4/{checkpoint_id}"
        tag_message = checkpoint_name or f"Checkpoint {checkpoint_id}"

        check_result = self._run_git("tag", "-l", tag_name)
        if check_result.stdout.strip():
            return GitResult(True, f"Tag {tag_name} already exists", tag=tag_name)

        result = self._run_git("tag", "-a", tag_name, "-m", tag_message)

        if result.returncode != 0:
            return GitResult(False, f"Tag creation failed: {result.stderr}")

        sha = self.get_current_sha()
        return GitResult(True, f"Created tag {tag_name}", sha=sha, tag=tag_name)

    def get_checkpoint_tags(self) -> list[str]:
        """Get all C4 checkpoint tags."""
        if not self.is_git_repo():
            return []

        result = self._run_git("tag", "-l", "c4/CP-*")
        if result.returncode != 0:
            return []

        return [t.strip() for t in result.stdout.splitlines() if t.strip()]

    def get_branch_name(self) -> str | None:
        """Get current branch name."""
        if not self.is_git_repo():
            return None

        result = self._run_git("rev-parse", "--abbrev-ref", "HEAD")
        if result.returncode != 0:
            return None

        branch = result.stdout.strip()
        return None if branch == "HEAD" else branch

    def create_task_branch(self, task_id: str) -> GitResult:
        """Create a branch for a task."""
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        branch_name = f"c4/w-{task_id}"

        check_result = self._run_git("branch", "--list", branch_name)
        if check_result.stdout.strip():
            result = self._run_git("checkout", branch_name)
            if result.returncode != 0:
                return GitResult(False, f"Checkout failed: {result.stderr}")
            return GitResult(True, f"Switched to existing branch {branch_name}")

        result = self._run_git("checkout", "-b", branch_name)
        if result.returncode != 0:
            return GitResult(False, f"Branch creation failed: {result.stderr}")

        return GitResult(True, f"Created branch {branch_name}")

    def get_tag_info(self, tag: str) -> dict[str, str] | None:
        """Get detailed information about a tag."""
        if not self.is_git_repo():
            return None

        sha_result = self._run_git("rev-list", "-1", tag)
        if sha_result.returncode != 0:
            return None

        sha = sha_result.stdout.strip()[:7]

        date_result = self._run_git("log", "-1", "--format=%ci", tag)
        date = date_result.stdout.strip() if date_result.returncode == 0 else "unknown"

        msg_result = self._run_git("tag", "-l", "-n1", tag)
        message = ""
        if msg_result.returncode == 0 and msg_result.stdout.strip():
            parts = msg_result.stdout.strip().split(maxsplit=1)
            message = parts[1] if len(parts) > 1 else ""

        return {"sha": sha, "date": date, "message": message}

    def list_checkpoint_tags(self) -> list[dict[str, str]]:
        """List all C4 checkpoint tags with details."""
        tags = self.get_checkpoint_tags()
        result = []

        for tag in sorted(tags, reverse=True):
            info = self.get_tag_info(tag)
            if info:
                result.append({
                    "tag": tag,
                    "sha": info["sha"],
                    "date": info["date"],
                    "message": info["message"],
                })

        return result

    def rollback_to_checkpoint(
        self,
        checkpoint_tag: str,
        hard: bool = True,
    ) -> GitResult:
        """Rollback to a specific checkpoint tag."""
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        if not self.is_git_available():
            return GitResult(False, "Git not available")

        check_result = self._run_git("tag", "-l", checkpoint_tag)
        if not check_result.stdout.strip():
            return GitResult(False, f"Tag '{checkpoint_tag}' not found")

        current_sha = self.get_current_sha()

        reset_mode = "--hard" if hard else "--soft"
        result = self._run_git("reset", reset_mode, checkpoint_tag)

        if result.returncode != 0:
            return GitResult(False, f"Rollback failed: {result.stderr}")

        new_sha = self.get_current_sha()
        return GitResult(
            True,
            f"Rolled back from {current_sha} to {new_sha} ({checkpoint_tag})",
            sha=new_sha,
            tag=checkpoint_tag,
        )

    def get_commits_since_tag(self, tag: str) -> list[dict[str, str]]:
        """Get commits since a specific tag."""
        if not self.is_git_repo():
            return []

        result = self._run_git("log", f"{tag}..HEAD", "--format=%h|%s|%ci")

        if result.returncode != 0:
            return []

        commits = []
        for line in result.stdout.splitlines():
            parts = line.split("|", 2)
            if len(parts) >= 3:
                commits.append({
                    "sha": parts[0],
                    "message": parts[1],
                    "date": parts[2],
                })

        return commits
