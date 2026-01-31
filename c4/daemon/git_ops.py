"""C4 Git Operations - Automated git operations for task lifecycle."""

import logging
import subprocess
from datetime import datetime
from pathlib import Path
from typing import NamedTuple

logger = logging.getLogger(__name__)


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
        """Run a git command in the project root.

        Args:
            *args: Git command arguments

        Returns:
            CompletedProcess with stdout/stderr as text
        """
        return subprocess.run(
            ["git", *args],
            cwd=self.root,
            capture_output=True,
            text=True,
        )

    def get_current_sha(self) -> str | None:
        """Get the current HEAD commit SHA.

        Returns:
            Short SHA or None if not available
        """
        if not self.is_git_repo():
            return None

        result = self._run_git("rev-parse", "--short", "HEAD")
        if result.returncode == 0:
            return result.stdout.strip()
        return None

    def has_uncommitted_changes(self) -> bool:
        """Check if there are uncommitted changes.

        Returns:
            True if there are staged or unstaged changes
        """
        if not self.is_git_repo():
            return False

        result = self._run_git("status", "--porcelain")
        return bool(result.stdout.strip())

    def stage_all(self) -> bool:
        """Stage all changes (git add -A).

        Returns:
            True if successful
        """
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

        # Stage all changes
        if not self.stage_all():
            return GitResult(False, "Failed to stage changes")

        # Check if there are changes to commit
        if not self.has_uncommitted_changes():
            # No changes, return current SHA
            sha = self.get_current_sha()
            return GitResult(True, "No changes to commit", sha=sha)

        # Create commit message
        message_lines = [
            f"[{task_id}] {task_title}",
            "",
            f"Task ID: {task_id}",
        ]
        if worker_id:
            message_lines.append(f"Worker: {worker_id}")
        message_lines.append(f"Completed: {datetime.now().isoformat()}")

        commit_message = "\n".join(message_lines)

        # Commit
        result = self._run_git("commit", "-m", commit_message)
        if result.returncode != 0:
            # Check if it's "nothing to commit"
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
        """Create a commit for repair task completion.

        Args:
            task_id: Repair task ID (e.g., "REPAIR-T-001-1")
            original_task_id: Original task that was repaired
            repair_reason: Reason for the repair

        Returns:
            GitResult with commit SHA if successful
        """
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        if not self.is_git_available():
            return GitResult(False, "Git not available")

        # Stage all changes
        if not self.stage_all():
            return GitResult(False, "Failed to stage changes")

        if not self.has_uncommitted_changes():
            sha = self.get_current_sha()
            return GitResult(True, "No changes to commit", sha=sha)

        # Create commit message
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
        """Create a Git tag for a checkpoint.

        Args:
            checkpoint_id: Checkpoint ID (e.g., "CP-001")
            checkpoint_name: Optional descriptive name

        Returns:
            GitResult with tag name if successful
        """
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        if not self.is_git_available():
            return GitResult(False, "Git not available")

        # Generate tag name
        tag_name = f"c4/{checkpoint_id}"
        tag_message = checkpoint_name or f"Checkpoint {checkpoint_id}"

        # Check if tag already exists
        check_result = self._run_git("tag", "-l", tag_name)
        if check_result.stdout.strip():
            return GitResult(True, f"Tag {tag_name} already exists", tag=tag_name)

        # Create annotated tag
        result = self._run_git(
            "tag",
            "-a",
            tag_name,
            "-m",
            tag_message,
        )

        if result.returncode != 0:
            return GitResult(False, f"Tag creation failed: {result.stderr}")

        sha = self.get_current_sha()
        return GitResult(True, f"Created tag {tag_name}", sha=sha, tag=tag_name)

    def get_checkpoint_tags(self) -> list[str]:
        """Get all C4 checkpoint tags.

        Returns:
            List of checkpoint tag names
        """
        if not self.is_git_repo():
            return []

        result = self._run_git("tag", "-l", "c4/CP-*")
        if result.returncode != 0:
            return []

        return [t.strip() for t in result.stdout.splitlines() if t.strip()]

    def get_commits_since_tag(self, tag: str) -> list[dict[str, str]]:
        """Get commits since a specific tag.

        Args:
            tag: Tag name to start from

        Returns:
            List of commit info dicts with sha, message, date
        """
        if not self.is_git_repo():
            return []

        result = self._run_git(
            "log",
            f"{tag}..HEAD",
            "--format=%h|%s|%ci",
        )

        if result.returncode != 0:
            return []

        commits = []
        for line in result.stdout.splitlines():
            parts = line.split("|", 2)
            if len(parts) >= 3:
                commits.append(
                    {
                        "sha": parts[0],
                        "message": parts[1],
                        "date": parts[2],
                    }
                )

        return commits

    def get_branch_name(self) -> str | None:
        """Get current branch name.

        Returns:
            Branch name or None if detached HEAD
        """
        if not self.is_git_repo():
            return None

        result = self._run_git("rev-parse", "--abbrev-ref", "HEAD")
        if result.returncode != 0:
            return None

        branch = result.stdout.strip()
        return None if branch == "HEAD" else branch

    def is_branch_merged(self, branch: str, target: str = "main") -> bool:
        """Check if a branch has been merged into target branch.

        Args:
            branch: Branch to check (e.g., 'c4/w-T-001-0')
            target: Target branch to check against (default: 'main')

        Returns:
            True if branch is merged into target, False otherwise
        """
        if not self.is_git_repo():
            return False

        # List all branches merged into target
        result = self._run_git("branch", "--merged", target)
        if result.returncode != 0:
            return False

        merged_branches = [b.strip().lstrip("* ") for b in result.stdout.strip().split("\n")]
        return branch in merged_branches

    def get_merged_task_branches(self, target: str = "main") -> list[str]:
        """Get all c4/w-* branches that have been merged into target.

        Args:
            target: Target branch to check against (default: 'main')

        Returns:
            List of merged task branch names
        """
        if not self.is_git_repo():
            return []

        result = self._run_git("branch", "--merged", target)
        if result.returncode != 0:
            return []

        merged = []
        for line in result.stdout.strip().split("\n"):
            branch = line.strip().lstrip("* ")
            if branch.startswith("c4/w-"):
                merged.append(branch)
        return merged

    def create_task_branch(self, task_id: str) -> GitResult:
        """Create a branch for a task.

        Args:
            task_id: Task ID for branch name

        Returns:
            GitResult with branch creation status
        """
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        branch_name = f"c4/w-{task_id}"

        # Check if branch exists
        check_result = self._run_git("branch", "--list", branch_name)
        if check_result.stdout.strip():
            # Branch exists, checkout to it
            result = self._run_git("checkout", branch_name)
            if result.returncode != 0:
                return GitResult(False, f"Checkout failed: {result.stderr}")
            return GitResult(True, f"Switched to existing branch {branch_name}")

        # Create and checkout new branch
        result = self._run_git("checkout", "-b", branch_name)
        if result.returncode != 0:
            return GitResult(False, f"Branch creation failed: {result.stderr}")

        return GitResult(True, f"Created branch {branch_name}")

    def ensure_work_branch(self, work_branch: str, default_branch: str = "main") -> GitResult:
        """Ensure work branch exists and checkout to it.

        Creates the work branch from default_branch if it doesn't exist.
        This is called at c4_start to set up the C4 working environment.

        Args:
            work_branch: Name of the work branch (e.g., 'c4/my-project')
            default_branch: Branch to create work branch from (default: 'main')

        Returns:
            GitResult with branch setup status
        """
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        # Check if work branch exists
        check_result = self._run_git("branch", "--list", work_branch)
        if check_result.stdout.strip():
            # Branch exists, checkout to it
            result = self._run_git("checkout", work_branch)
            if result.returncode != 0:
                return GitResult(False, f"Checkout failed: {result.stderr}")
            return GitResult(True, f"Switched to existing work branch {work_branch}")

        # Check if default_branch exists
        default_check = self._run_git("branch", "--list", default_branch)
        if not default_check.stdout.strip():
            # Default branch doesn't exist (fresh repo with no commits)
            # Create work branch from current HEAD (or create initial commit if needed)
            result = self._run_git("checkout", "-b", work_branch)
            if result.returncode != 0:
                return GitResult(False, f"Branch creation failed: {result.stderr}")
            return GitResult(True, f"Created work branch {work_branch} (no base branch)")

        # Need to create work branch from default_branch
        # First, ensure we're on default_branch
        current_branch = self.get_branch_name()
        if current_branch != default_branch:
            checkout_result = self._run_git("checkout", default_branch)
            if checkout_result.returncode != 0:
                return GitResult(
                    False,
                    f"Cannot checkout {default_branch}: {checkout_result.stderr}",
                )

        # Create and checkout work branch
        result = self._run_git("checkout", "-b", work_branch)
        if result.returncode != 0:
            return GitResult(False, f"Branch creation failed: {result.stderr}")

        return GitResult(True, f"Created work branch {work_branch} from {default_branch}")

    def merge_branch_to_target(
        self, source_branch: str, target_branch: str, squash: bool = False
    ) -> GitResult:
        """Merge source branch into target branch.

        Used for merging task branches into work branch (on checkpoint APPROVE)
        and for merging work branch into default branch (on plan completion).

        Args:
            source_branch: Branch to merge from (e.g., 'c4/w-T-001-0')
            target_branch: Branch to merge into (e.g., 'c4/my-project')
            squash: If True, squash merge (combines all commits into one)

        Returns:
            GitResult with merge status
        """
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        # Checkout target branch
        checkout_result = self._run_git("checkout", target_branch)
        if checkout_result.returncode != 0:
            return GitResult(False, f"Cannot checkout {target_branch}: {checkout_result.stderr}")

        # Merge source branch
        if squash:
            merge_result = self._run_git("merge", "--squash", source_branch)
            if merge_result.returncode != 0:
                # Abort merge on conflict
                self._run_git("merge", "--abort")
                return GitResult(False, f"Squash merge failed: {merge_result.stderr}")
            # Commit the squashed changes
            commit_result = self._run_git("commit", "-m", f"Squash merge {source_branch}")
            if commit_result.returncode != 0:
                return GitResult(False, f"Commit failed: {commit_result.stderr}")
        else:
            merge_result = self._run_git(
                "merge", "--no-ff", source_branch, "-m", f"Merge {source_branch}"
            )
            if merge_result.returncode != 0:
                # Abort merge on conflict
                self._run_git("merge", "--abort")
                return GitResult(False, f"Merge failed: {merge_result.stderr}")

        return GitResult(True, f"Merged {source_branch} into {target_branch}")

    def get_tag_info(self, tag: str) -> dict[str, str] | None:
        """Get detailed information about a tag.

        Args:
            tag: Tag name

        Returns:
            Dict with sha, date, message or None if tag doesn't exist
        """
        if not self.is_git_repo():
            return None

        # Get tag target SHA
        sha_result = self._run_git("rev-list", "-1", tag)
        if sha_result.returncode != 0:
            return None

        sha = sha_result.stdout.strip()[:7]

        # Get tag date and message
        date_result = self._run_git("log", "-1", "--format=%ci", tag)
        date = date_result.stdout.strip() if date_result.returncode == 0 else "unknown"

        msg_result = self._run_git("tag", "-l", "-n1", tag)
        # Output format: "tag_name    message"
        message = ""
        if msg_result.returncode == 0 and msg_result.stdout.strip():
            parts = msg_result.stdout.strip().split(maxsplit=1)
            message = parts[1] if len(parts) > 1 else ""

        return {"sha": sha, "date": date, "message": message}

    def list_checkpoint_tags(self) -> list[dict[str, str]]:
        """List all C4 checkpoint tags with details.

        Returns:
            List of dicts with tag, sha, date, message
        """
        tags = self.get_checkpoint_tags()
        result = []

        for tag in sorted(tags, reverse=True):  # Most recent first
            info = self.get_tag_info(tag)
            if info:
                result.append(
                    {
                        "tag": tag,
                        "sha": info["sha"],
                        "date": info["date"],
                        "message": info["message"],
                    }
                )

        return result

    def rollback_to_checkpoint(
        self,
        checkpoint_tag: str,
        hard: bool = True,
    ) -> GitResult:
        """Rollback to a specific checkpoint tag.

        Args:
            checkpoint_tag: Tag name to rollback to (e.g., "c4/CP-001")
            hard: If True, discard all changes (git reset --hard)

        Returns:
            GitResult with success status
        """
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        if not self.is_git_available():
            return GitResult(False, "Git not available")

        # Verify the tag exists
        check_result = self._run_git("tag", "-l", checkpoint_tag)
        if not check_result.stdout.strip():
            return GitResult(False, f"Tag '{checkpoint_tag}' not found")

        # Get current SHA before rollback for reference
        current_sha = self.get_current_sha()

        # Perform rollback
        reset_mode = "--hard" if hard else "--soft"
        result = self._run_git("reset", reset_mode, checkpoint_tag)

        if result.returncode != 0:
            return GitResult(
                False,
                f"Rollback failed: {result.stderr}",
            )

        new_sha = self.get_current_sha()
        return GitResult(
            True,
            f"Rolled back from {current_sha} to {new_sha} ({checkpoint_tag})",
            sha=new_sha,
            tag=checkpoint_tag,
        )

    # ========== Git Worktree Operations ==========
    # For multi-worker isolation - each worker gets an independent working directory

    def get_worktrees_dir(self) -> Path:
        """Get the directory where worktrees are stored.

        Returns:
            Path to .c4/worktrees/ directory
        """
        return self.root / ".c4" / "worktrees"

    def get_worktree_path(self, worker_id: str) -> Path:
        """Get the worktree path for a specific worker.

        Args:
            worker_id: Worker identifier

        Returns:
            Path to the worker's worktree directory
        """
        # Sanitize worker_id for filesystem
        safe_id = worker_id.replace("/", "-").replace("\\", "-")
        return self.get_worktrees_dir() / safe_id

    def list_worktrees(self) -> list[dict[str, str]]:
        """List all git worktrees.

        Returns:
            List of dicts with path, branch, head info
        """
        if not self.is_git_repo():
            return []

        result = self._run_git("worktree", "list", "--porcelain")
        if result.returncode != 0:
            return []

        worktrees: list[dict[str, str]] = []
        current: dict[str, str] = {}

        for line in result.stdout.splitlines():
            if line.startswith("worktree "):
                if current:
                    worktrees.append(current)
                current = {"path": line[9:]}
            elif line.startswith("HEAD "):
                current["head"] = line[5:]
            elif line.startswith("branch "):
                current["branch"] = line[7:]
            elif line == "bare":
                current["bare"] = "true"
            elif line == "detached":
                current["detached"] = "true"

        if current:
            worktrees.append(current)

        return worktrees

    def create_worktree(
        self,
        worker_id: str,
        branch: str,
        base_branch: str | None = None,
    ) -> GitResult:
        """Create a worktree for a worker.

        Creates a new worktree at .c4/worktrees/{worker_id} with the specified branch.
        If the branch doesn't exist, creates it from base_branch (or current HEAD).

        Args:
            worker_id: Worker identifier for the worktree directory
            branch: Branch name for the worktree (e.g., 'c4/w-T-001-0')
            base_branch: Branch to create from if branch doesn't exist

        Returns:
            GitResult with worktree path on success
        """
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        # Prune stale worktree entries before creating
        # This cleans up entries where the directory was removed but git still tracks it
        self._run_git("worktree", "prune")

        worktree_path = self.get_worktree_path(worker_id)

        # Check if worktree already exists
        if worktree_path.exists():
            # Verify it's a valid worktree
            # Resolve paths to handle symlinks (e.g., /var -> /private/var on macOS)
            resolved_worktree = worktree_path.resolve()
            existing = self.list_worktrees()
            for wt in existing:
                wt_path = Path(wt.get("path", ""))
                if wt_path.exists() and wt_path.resolve() == resolved_worktree:
                    return GitResult(
                        True,
                        f"Worktree already exists at {worktree_path}",
                        sha=wt.get("head", "")[:7] if wt.get("head") else None,
                    )
            # Directory exists but not a worktree - remove it
            import shutil

            shutil.rmtree(worktree_path)

        # Ensure worktrees directory exists
        self.get_worktrees_dir().mkdir(parents=True, exist_ok=True)

        # Check if branch exists
        branch_check = self._run_git("branch", "--list", branch)
        branch_exists = bool(branch_check.stdout.strip())

        if branch_exists:
            # Branch exists - create worktree with existing branch
            result = self._run_git("worktree", "add", str(worktree_path), branch)
        else:
            # Branch doesn't exist - create new branch in worktree
            if base_branch:
                result = self._run_git(
                    "worktree", "add", "-b", branch, str(worktree_path), base_branch
                )
            else:
                result = self._run_git(
                    "worktree", "add", "-b", branch, str(worktree_path)
                )

        if result.returncode != 0:
            return GitResult(False, f"Worktree creation failed: {result.stderr}")

        return GitResult(
            True,
            f"Created worktree at {worktree_path}",
            sha=self._get_worktree_head(worktree_path),
        )

    def _get_worktree_head(self, worktree_path: Path) -> str | None:
        """Get HEAD SHA of a worktree.

        Args:
            worktree_path: Path to the worktree

        Returns:
            Short SHA or None
        """
        result = subprocess.run(
            ["git", "rev-parse", "--short", "HEAD"],
            cwd=worktree_path,
            capture_output=True,
            text=True,
        )
        return result.stdout.strip() if result.returncode == 0 else None

    def remove_worktree(self, worker_id: str, force: bool = False) -> GitResult:
        """Remove a worktree.

        Args:
            worker_id: Worker identifier
            force: Force removal even with uncommitted changes

        Returns:
            GitResult with removal status
        """
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        worktree_path = self.get_worktree_path(worker_id)

        if not worktree_path.exists():
            return GitResult(True, f"Worktree {worker_id} does not exist")

        # Remove worktree
        args = ["worktree", "remove", str(worktree_path)]
        if force:
            args.append("--force")

        result = self._run_git(*args)
        if result.returncode != 0:
            return GitResult(False, f"Worktree removal failed: {result.stderr}")

        # Prune stale worktree references
        self._run_git("worktree", "prune")

        return GitResult(True, f"Removed worktree {worker_id}")

    def get_worktree_status(self, worker_id: str) -> dict[str, str | bool | None]:
        """Get status of a worker's worktree.

        Args:
            worker_id: Worker identifier

        Returns:
            Dict with exists, path, branch, head, has_changes
        """
        worktree_path = self.get_worktree_path(worker_id)

        if not worktree_path.exists():
            return {
                "exists": False,
                "path": str(worktree_path),
                "branch": None,
                "head": None,
                "has_changes": None,
            }

        # Get branch and HEAD
        branch_result = subprocess.run(
            ["git", "rev-parse", "--abbrev-ref", "HEAD"],
            cwd=worktree_path,
            capture_output=True,
            text=True,
        )
        branch = branch_result.stdout.strip() if branch_result.returncode == 0 else None

        head = self._get_worktree_head(worktree_path)

        # Check for uncommitted changes
        status_result = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=worktree_path,
            capture_output=True,
            text=True,
        )
        has_changes = bool(status_result.stdout.strip())

        return {
            "exists": True,
            "path": str(worktree_path),
            "branch": branch,
            "head": head,
            "has_changes": has_changes,
        }

    def commit_in_worktree(
        self,
        worker_id: str,
        message: str,
    ) -> GitResult:
        """Create a commit in a worker's worktree.

        Args:
            worker_id: Worker identifier
            message: Commit message

        Returns:
            GitResult with commit SHA
        """
        worktree_path = self.get_worktree_path(worker_id)

        if not worktree_path.exists():
            return GitResult(False, f"Worktree {worker_id} does not exist")

        # Stage all changes
        stage_result = subprocess.run(
            ["git", "add", "-A"],
            cwd=worktree_path,
            capture_output=True,
            text=True,
        )
        if stage_result.returncode != 0:
            return GitResult(False, f"Failed to stage changes: {stage_result.stderr}")

        # Check if there are changes
        status_result = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=worktree_path,
            capture_output=True,
            text=True,
        )
        if not status_result.stdout.strip():
            head = self._get_worktree_head(worktree_path)
            return GitResult(True, "No changes to commit", sha=head)

        # Commit
        commit_result = subprocess.run(
            ["git", "commit", "-m", message],
            cwd=worktree_path,
            capture_output=True,
            text=True,
        )
        if commit_result.returncode != 0:
            return GitResult(False, f"Commit failed: {commit_result.stderr}")

        head = self._get_worktree_head(worktree_path)
        return GitResult(True, f"Committed in worktree {worker_id}", sha=head)

    def merge_worktree_branch(
        self,
        worker_id: str,
        target_branch: str,
        squash: bool = False,
    ) -> GitResult:
        """Merge a worktree's branch into target branch.

        Performed from the main repository (not the worktree).

        Args:
            worker_id: Worker identifier (to get branch name)
            target_branch: Branch to merge into
            squash: If True, squash merge

        Returns:
            GitResult with merge status
        """
        status = self.get_worktree_status(worker_id)
        if not status.get("exists"):
            return GitResult(False, f"Worktree {worker_id} does not exist")

        source_branch = status.get("branch")
        if not source_branch:
            return GitResult(False, f"Cannot determine branch for worktree {worker_id}")

        # source_branch is str | bool | None, but we checked it's truthy and is a branch
        return self.merge_branch_to_target(str(source_branch), target_branch, squash=squash)

    def cleanup_worktrees(self, keep_workers: list[str] | None = None) -> GitResult:
        """Clean up unused worktrees.

        Removes all C4 worktrees except those in keep_workers list.

        Args:
            keep_workers: List of worker_ids to keep (None = remove all)

        Returns:
            GitResult with cleanup summary
        """
        if not self.is_git_repo():
            return GitResult(False, "Not a Git repository")

        worktrees_dir = self.get_worktrees_dir()
        if not worktrees_dir.exists():
            return GitResult(True, "No worktrees directory")

        keep_set = set(keep_workers) if keep_workers else set()
        removed = []
        failed = []

        for item in worktrees_dir.iterdir():
            if item.is_dir() and item.name not in keep_set:
                result = self.remove_worktree(item.name, force=True)
                if result.success:
                    removed.append(item.name)
                else:
                    failed.append(f"{item.name}: {result.message}")

        # Prune worktree references
        self._run_git("worktree", "prune")

        if failed:
            return GitResult(
                False,
                f"Removed {len(removed)}, failed {len(failed)}: {', '.join(failed)}",
            )

        return GitResult(True, f"Cleaned up {len(removed)} worktrees")
