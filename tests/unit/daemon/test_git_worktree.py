"""Tests for Git Worktree operations."""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

from c4.daemon.git_ops import GitOperations


class TestGitWorktree:
    """Tests for GitOperations worktree methods."""

    @pytest.fixture
    def git_repo(self, tmp_path: Path) -> Path:
        """Create a temporary git repository."""
        # Initialize git repo with main as default branch
        subprocess.run(
            ["git", "init", "-b", "main"], cwd=tmp_path, capture_output=True
        )
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=tmp_path,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=tmp_path,
            capture_output=True,
        )

        # Create initial commit
        (tmp_path / "README.md").write_text("# Test Project")
        subprocess.run(["git", "add", "."], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial commit"],
            cwd=tmp_path,
            capture_output=True,
        )

        return tmp_path

    def test_get_worktrees_dir(self, git_repo: Path):
        """Should return .c4/worktrees path."""
        ops = GitOperations(git_repo)
        expected = git_repo / ".c4" / "worktrees"
        assert ops.get_worktrees_dir() == expected

    def test_get_worktree_path(self, git_repo: Path):
        """Should return worker-specific worktree path."""
        ops = GitOperations(git_repo)
        path = ops.get_worktree_path("worker-1")
        assert path == git_repo / ".c4" / "worktrees" / "worker-1"

    def test_get_worktree_path_sanitizes_id(self, git_repo: Path):
        """Should sanitize worker_id with slashes."""
        ops = GitOperations(git_repo)
        path = ops.get_worktree_path("worker/1/test")
        assert path == git_repo / ".c4" / "worktrees" / "worker-1-test"

    def test_list_worktrees_initial(self, git_repo: Path):
        """Should list only main worktree initially."""
        ops = GitOperations(git_repo)
        worktrees = ops.list_worktrees()

        # Should have at least the main worktree
        assert len(worktrees) >= 1
        assert any(wt.get("path") == str(git_repo) for wt in worktrees)

    def test_create_worktree_new_branch(self, git_repo: Path):
        """Should create worktree with new branch."""
        ops = GitOperations(git_repo)

        result = ops.create_worktree("worker-1", "c4/w-T-001")

        assert result.success is True
        assert "Created worktree" in result.message

        worktree_path = ops.get_worktree_path("worker-1")
        assert worktree_path.exists()
        assert (worktree_path / "README.md").exists()

    def test_create_worktree_from_base_branch(self, git_repo: Path):
        """Should create worktree from specified base branch."""
        ops = GitOperations(git_repo)

        # Create a feature branch first
        subprocess.run(
            ["git", "checkout", "-b", "feature"],
            cwd=git_repo,
            capture_output=True,
        )
        (git_repo / "feature.txt").write_text("feature content")
        subprocess.run(["git", "add", "."], cwd=git_repo, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Add feature"],
            cwd=git_repo,
            capture_output=True,
        )

        # Go back to main
        subprocess.run(
            ["git", "checkout", "main"],
            cwd=git_repo,
            capture_output=True,
        )

        # Create worktree from feature branch
        result = ops.create_worktree("worker-1", "c4/w-task", base_branch="feature")

        assert result.success is True

        worktree_path = ops.get_worktree_path("worker-1")
        # Should have feature.txt from feature branch
        assert (worktree_path / "feature.txt").exists()

    def test_create_worktree_already_exists(self, git_repo: Path):
        """Should handle existing worktree gracefully."""
        ops = GitOperations(git_repo)

        # Create first time
        ops.create_worktree("worker-1", "c4/w-T-001")

        # Create again - should succeed
        result = ops.create_worktree("worker-1", "c4/w-T-001")

        assert result.success is True
        assert "already exists" in result.message

    def test_remove_worktree(self, git_repo: Path):
        """Should remove worktree."""
        ops = GitOperations(git_repo)

        # Create worktree
        ops.create_worktree("worker-1", "c4/w-T-001")
        worktree_path = ops.get_worktree_path("worker-1")
        assert worktree_path.exists()

        # Remove it
        result = ops.remove_worktree("worker-1")

        assert result.success is True
        assert not worktree_path.exists()

    def test_remove_nonexistent_worktree(self, git_repo: Path):
        """Should handle removing nonexistent worktree."""
        ops = GitOperations(git_repo)

        result = ops.remove_worktree("nonexistent")

        assert result.success is True
        assert "does not exist" in result.message

    def test_get_worktree_status_exists(self, git_repo: Path):
        """Should return status for existing worktree."""
        ops = GitOperations(git_repo)

        ops.create_worktree("worker-1", "c4/w-T-001")

        status = ops.get_worktree_status("worker-1")

        assert status["exists"] is True
        assert status["branch"] == "c4/w-T-001"
        assert status["head"] is not None
        assert status["has_changes"] is False

    def test_get_worktree_status_not_exists(self, git_repo: Path):
        """Should return status for nonexistent worktree."""
        ops = GitOperations(git_repo)

        status = ops.get_worktree_status("nonexistent")

        assert status["exists"] is False
        assert status["branch"] is None

    def test_commit_in_worktree(self, git_repo: Path):
        """Should commit changes in worktree."""
        ops = GitOperations(git_repo)

        ops.create_worktree("worker-1", "c4/w-T-001")
        worktree_path = ops.get_worktree_path("worker-1")

        # Make a change in worktree
        (worktree_path / "new_file.py").write_text("# New file")

        result = ops.commit_in_worktree("worker-1", "Add new file")

        assert result.success is True
        assert result.sha is not None

        # Verify commit
        status = ops.get_worktree_status("worker-1")
        assert status["has_changes"] is False

    def test_commit_in_worktree_no_changes(self, git_repo: Path):
        """Should handle commit with no changes."""
        ops = GitOperations(git_repo)

        ops.create_worktree("worker-1", "c4/w-T-001")

        result = ops.commit_in_worktree("worker-1", "No changes")

        assert result.success is True
        assert "No changes" in result.message

    def test_cleanup_worktrees(self, git_repo: Path):
        """Should cleanup all worktrees."""
        ops = GitOperations(git_repo)

        # Create multiple worktrees
        ops.create_worktree("worker-1", "c4/w-T-001")
        ops.create_worktree("worker-2", "c4/w-T-002")

        result = ops.cleanup_worktrees()

        assert result.success is True
        assert "Cleaned up 2 worktrees" in result.message

        # Verify they're gone
        assert not ops.get_worktree_path("worker-1").exists()
        assert not ops.get_worktree_path("worker-2").exists()

    def test_cleanup_worktrees_with_keep(self, git_repo: Path):
        """Should keep specified worktrees during cleanup."""
        ops = GitOperations(git_repo)

        # Create multiple worktrees
        ops.create_worktree("worker-1", "c4/w-T-001")
        ops.create_worktree("worker-2", "c4/w-T-002")

        result = ops.cleanup_worktrees(keep_workers=["worker-1"])

        assert result.success is True
        assert "Cleaned up 1 worktrees" in result.message

        # Verify correct ones are kept/removed
        assert ops.get_worktree_path("worker-1").exists()
        assert not ops.get_worktree_path("worker-2").exists()

    def test_merge_worktree_branch(self, git_repo: Path):
        """Should merge worktree branch to target."""
        ops = GitOperations(git_repo)

        # Create worktree and make changes
        ops.create_worktree("worker-1", "c4/w-T-001")
        worktree_path = ops.get_worktree_path("worker-1")

        (worktree_path / "task_file.py").write_text("# Task implementation")
        ops.commit_in_worktree("worker-1", "Implement task")

        # Merge back to main
        result = ops.merge_worktree_branch("worker-1", "main")

        assert result.success is True

        # Verify file is in main
        assert (git_repo / "task_file.py").exists()
