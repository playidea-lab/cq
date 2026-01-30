"""WorktreeManager for C4 multi-worker isolation.

Provides a dedicated interface for managing Git worktrees,
allowing multiple workers to operate in parallel without file conflicts.
"""

import logging
from dataclasses import dataclass
from pathlib import Path

from c4.daemon.git_ops import GitOperations, GitResult

logger = logging.getLogger(__name__)


@dataclass
class WorktreeInfo:
    """Information about a worktree."""

    worker_id: str
    path: Path
    branch: str | None
    head: str | None
    exists: bool
    has_changes: bool


class WorktreeManager:
    """Manages Git worktrees for C4 multi-worker environments.

    Each worker gets an isolated working directory (worktree) to prevent
    file conflicts when multiple workers execute tasks in parallel.

    Worktrees are stored in .c4/worktrees/{worker_id}/.
    """

    def __init__(self, project_root: Path):
        """Initialize WorktreeManager.

        Args:
            project_root: Root directory of the project (contains .git/)
        """
        self.root = project_root
        self._git_ops = GitOperations(project_root)

    @property
    def worktrees_dir(self) -> Path:
        """Get the directory where worktrees are stored."""
        return self._git_ops.get_worktrees_dir()

    def get_worktree_path(self, worker_id: str) -> Path:
        """Get the worktree path for a specific worker.

        Args:
            worker_id: Worker identifier

        Returns:
            Path to the worker's worktree directory
        """
        return self._git_ops.get_worktree_path(worker_id)

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
        logger.info(f"Creating worktree for worker {worker_id} on branch {branch}")
        return self._git_ops.create_worktree(worker_id, branch, base_branch)

    def remove_worktree(self, worker_id: str, force: bool = False) -> GitResult:
        """Remove a worktree.

        Args:
            worker_id: Worker identifier
            force: Force removal even with uncommitted changes

        Returns:
            GitResult with removal status
        """
        logger.info(f"Removing worktree for worker {worker_id}")
        return self._git_ops.remove_worktree(worker_id, force)

    def list_worktrees(self) -> list[dict[str, str]]:
        """List all git worktrees.

        Returns:
            List of dicts with path, branch, head info
        """
        return self._git_ops.list_worktrees()

    def get_worktree_info(self, worker_id: str) -> WorktreeInfo:
        """Get detailed information about a worker's worktree.

        Args:
            worker_id: Worker identifier

        Returns:
            WorktreeInfo with exists, path, branch, head, has_changes
        """
        status = self._git_ops.get_worktree_status(worker_id)
        return WorktreeInfo(
            worker_id=worker_id,
            path=Path(status["path"]) if status.get("path") else self.get_worktree_path(worker_id),
            branch=status.get("branch") if isinstance(status.get("branch"), str) else None,
            head=status.get("head") if isinstance(status.get("head"), str) else None,
            exists=bool(status.get("exists")),
            has_changes=bool(status.get("has_changes")),
        )

    def commit_in_worktree(self, worker_id: str, message: str) -> GitResult:
        """Create a commit in a worker's worktree.

        Args:
            worker_id: Worker identifier
            message: Commit message

        Returns:
            GitResult with commit SHA
        """
        return self._git_ops.commit_in_worktree(worker_id, message)

    def merge_worktree_to_branch(
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
        return self._git_ops.merge_worktree_branch(worker_id, target_branch, squash)

    def cleanup(self, keep_workers: list[str] | None = None) -> GitResult:
        """Clean up unused worktrees.

        Removes all C4 worktrees except those in keep_workers list.

        Args:
            keep_workers: List of worker_ids to keep (None = remove all)

        Returns:
            GitResult with cleanup summary
        """
        logger.info(f"Cleaning up worktrees, keeping: {keep_workers}")
        return self._git_ops.cleanup_worktrees(keep_workers)

    def get_all_worker_ids(self) -> list[str]:
        """Get list of all worker IDs with existing worktrees.

        Returns:
            List of worker IDs
        """
        if not self.worktrees_dir.exists():
            return []

        return [
            item.name
            for item in self.worktrees_dir.iterdir()
            if item.is_dir()
        ]

    def ensure_worktree(
        self,
        worker_id: str,
        branch: str,
        base_branch: str | None = None,
    ) -> GitResult:
        """Ensure a worktree exists for a worker, creating if necessary.

        If the worktree already exists, returns success without modification.

        Args:
            worker_id: Worker identifier
            branch: Branch name for the worktree
            base_branch: Branch to create from if branch doesn't exist

        Returns:
            GitResult with worktree status
        """
        info = self.get_worktree_info(worker_id)
        if info.exists:
            return GitResult(
                True,
                f"Worktree already exists at {info.path}",
                sha=info.head,
            )
        return self.create_worktree(worker_id, branch, base_branch)
