"""Tests for Git Worktree Allocation in C4.

Tests verify:
1. Worktree creation during task assignment
2. Multiple workers with isolated worktrees
3. Same file modification without conflicts
4. Worktree cleanup on task completion
"""

import subprocess
import tempfile
from pathlib import Path

import pytest

from c4.daemon.git_ops import GitOperations


@pytest.fixture
def temp_git_repo():
    """Create a temporary git repository with initial commit."""
    with tempfile.TemporaryDirectory() as tmpdir:
        repo = Path(tmpdir)
        # Initialize git repo with main as default branch
        subprocess.run(
            ["git", "init", "-b", "main"], cwd=repo, capture_output=True
        )
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=repo,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=repo,
            capture_output=True,
        )

        # Create initial commit
        (repo / "README.md").write_text("# Test Project")
        subprocess.run(["git", "add", "."], cwd=repo, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial commit"],
            cwd=repo,
            capture_output=True,
        )

        # Create .c4 directory for worktrees
        (repo / ".c4").mkdir()

        yield repo


@pytest.fixture
def git_ops(temp_git_repo):
    """Create GitOperations instance for test repo."""
    return GitOperations(temp_git_repo)


class TestWorktreeCreation:
    """Test worktree creation functionality."""

    def test_create_worktree_success(self, git_ops, temp_git_repo):
        """Create a new worktree successfully."""
        worker_id = "worker-1"
        branch = "c4/w-T-001-0"

        result = git_ops.create_worktree(worker_id, branch, base_branch="main")

        assert result.success is True
        worktree_path = git_ops.get_worktree_path(worker_id)
        assert worktree_path.exists()
        assert (worktree_path / ".git").exists()

    def test_create_worktree_with_new_branch(self, git_ops, temp_git_repo):
        """Create worktree with new branch from base."""
        worker_id = "worker-2"
        branch = "c4/w-T-002-0"

        result = git_ops.create_worktree(worker_id, branch, base_branch="main")

        assert result.success is True
        worktree_path = git_ops.get_worktree_path(worker_id)
        assert worktree_path.exists()

        # Verify branch was created
        branches = subprocess.run(
            ["git", "branch", "--list", branch],
            cwd=temp_git_repo,
            capture_output=True,
            text=True,
        )
        assert branch in branches.stdout

    def test_create_worktree_existing_returns_success(self, git_ops):
        """Creating worktree that already exists returns success."""
        worker_id = "worker-3"
        branch = "c4/w-T-003-0"

        # Create worktree first time
        result1 = git_ops.create_worktree(worker_id, branch, base_branch="main")
        assert result1.success is True

        # Try to create again - should return success
        result = git_ops.create_worktree(worker_id, branch)

        assert result.success is True
        # Second call should either say "already exists" or succeed normally
        # The key is that it doesn't fail


class TestWorktreeList:
    """Test worktree listing functionality."""

    def test_list_worktrees_includes_main(self, git_ops, temp_git_repo):
        """List worktrees includes main working directory."""
        worktrees = git_ops.list_worktrees()

        assert len(worktrees) >= 1
        # Main repo should be in the list
        paths = [wt["path"] for wt in worktrees]
        assert any(str(temp_git_repo) in p for p in paths)

    def test_list_worktrees_after_create(self, git_ops):
        """List worktrees includes newly created worktree."""
        worker_id = "worker-4"
        branch = "c4/w-T-004-0"

        git_ops.create_worktree(worker_id, branch, base_branch="main")

        worktrees = git_ops.list_worktrees()

        # Should have at least 2 worktrees now (main + new)
        assert len(worktrees) >= 2


class TestWorktreeRemoval:
    """Test worktree removal functionality."""

    def test_remove_worktree(self, git_ops):
        """Remove an existing worktree."""
        worker_id = "worker-5"
        branch = "c4/w-T-005-0"

        git_ops.create_worktree(worker_id, branch, base_branch="main")
        worktree_path = git_ops.get_worktree_path(worker_id)
        assert worktree_path.exists()

        result = git_ops.remove_worktree(worker_id)

        assert result.success is True
        assert not worktree_path.exists()

    def test_remove_nonexistent_worktree_returns_success(self, git_ops):
        """Removing nonexistent worktree returns success."""
        result = git_ops.remove_worktree("nonexistent-worker")

        assert result.success is True


class TestWorktreeStatus:
    """Test worktree status functionality."""

    def test_get_worktree_status_exists(self, git_ops):
        """Get status for existing worktree."""
        worker_id = "worker-6"
        branch = "c4/w-T-006-0"

        git_ops.create_worktree(worker_id, branch, base_branch="main")

        status = git_ops.get_worktree_status(worker_id)

        assert status["exists"] is True
        assert status["branch"] == branch
        assert status["has_changes"] is False

    def test_get_worktree_status_nonexistent(self, git_ops):
        """Get status for nonexistent worktree."""
        status = git_ops.get_worktree_status("nonexistent-worker")

        assert status["exists"] is False
        assert status["branch"] is None


class TestMultipleWorkersIsolation:
    """Test multiple workers working in isolated worktrees."""

    def test_two_workers_independent_worktrees(self, git_ops):
        """Two workers can create independent worktrees."""
        worker1 = "worker-7"
        worker2 = "worker-8"
        branch1 = "c4/w-T-007-0"
        branch2 = "c4/w-T-008-0"

        result1 = git_ops.create_worktree(worker1, branch1, base_branch="main")
        result2 = git_ops.create_worktree(worker2, branch2, base_branch="main")

        assert result1.success is True
        assert result2.success is True

        path1 = git_ops.get_worktree_path(worker1)
        path2 = git_ops.get_worktree_path(worker2)

        assert path1.exists()
        assert path2.exists()
        assert path1 != path2

    def test_workers_modify_same_file_no_conflict(self, git_ops):
        """Workers can modify same file in different worktrees without conflict."""
        worker1 = "worker-9"
        worker2 = "worker-10"
        branch1 = "c4/w-T-009-0"
        branch2 = "c4/w-T-010-0"

        git_ops.create_worktree(worker1, branch1, base_branch="main")
        git_ops.create_worktree(worker2, branch2, base_branch="main")

        path1 = git_ops.get_worktree_path(worker1)
        path2 = git_ops.get_worktree_path(worker2)

        # Worker 1 modifies README.md
        readme1 = path1 / "README.md"
        readme1.write_text("# Modified by Worker 1")

        # Worker 2 modifies README.md (different content)
        readme2 = path2 / "README.md"
        readme2.write_text("# Modified by Worker 2")

        # Both files should have different content
        assert readme1.read_text() != readme2.read_text()
        assert "Worker 1" in readme1.read_text()
        assert "Worker 2" in readme2.read_text()

    def test_workers_commit_independently(self, git_ops):
        """Workers can commit independently in their worktrees."""
        worker1 = "worker-11"
        worker2 = "worker-12"
        branch1 = "c4/w-T-011-0"
        branch2 = "c4/w-T-012-0"

        git_ops.create_worktree(worker1, branch1, base_branch="main")
        git_ops.create_worktree(worker2, branch2, base_branch="main")

        path1 = git_ops.get_worktree_path(worker1)
        path2 = git_ops.get_worktree_path(worker2)

        # Worker 1 commits
        (path1 / "file1.txt").write_text("Worker 1 file")
        subprocess.run(["git", "add", "."], cwd=path1, capture_output=True)
        result1 = subprocess.run(
            ["git", "commit", "-m", "Worker 1 commit"],
            cwd=path1,
            capture_output=True,
        )

        # Worker 2 commits
        (path2 / "file2.txt").write_text("Worker 2 file")
        subprocess.run(["git", "add", "."], cwd=path2, capture_output=True)
        result2 = subprocess.run(
            ["git", "commit", "-m", "Worker 2 commit"],
            cwd=path2,
            capture_output=True,
        )

        assert result1.returncode == 0
        assert result2.returncode == 0

        # Verify commits are on different branches
        log1 = subprocess.run(
            ["git", "log", "--oneline", "-1"],
            cwd=path1,
            capture_output=True,
            text=True,
        )
        log2 = subprocess.run(
            ["git", "log", "--oneline", "-1"],
            cwd=path2,
            capture_output=True,
            text=True,
        )

        assert "Worker 1 commit" in log1.stdout
        assert "Worker 2 commit" in log2.stdout


class TestWorktreeCommit:
    """Test committing in worktrees."""

    def test_commit_in_worktree(self, git_ops):
        """Commit changes in a worktree."""
        worker_id = "worker-13"
        branch = "c4/w-T-013-0"

        git_ops.create_worktree(worker_id, branch, base_branch="main")

        # Make changes
        path = git_ops.get_worktree_path(worker_id)
        (path / "new_file.txt").write_text("New content")

        result = git_ops.commit_in_worktree(worker_id, "Add new file")

        assert result.success is True
        assert result.sha is not None

    def test_commit_no_changes(self, git_ops):
        """Commit with no changes returns success."""
        worker_id = "worker-14"
        branch = "c4/w-T-014-0"

        git_ops.create_worktree(worker_id, branch, base_branch="main")

        result = git_ops.commit_in_worktree(worker_id, "Empty commit")

        assert result.success is True
        assert "no changes" in result.message.lower()


class TestWorktreeCleanup:
    """Test worktree cleanup functionality."""

    def test_cleanup_worktrees(self, git_ops):
        """Cleanup removes all worktrees except kept ones."""
        # Create multiple worktrees
        git_ops.create_worktree("worker-15", "branch-15", base_branch="main")
        git_ops.create_worktree("worker-16", "branch-16", base_branch="main")
        git_ops.create_worktree("worker-17", "branch-17", base_branch="main")

        # Cleanup all except worker-16
        result = git_ops.cleanup_worktrees(keep_workers=["worker-16"])

        assert result.success is True

        # worker-16 should exist, others should not
        assert git_ops.get_worktree_path("worker-16").exists()
        assert not git_ops.get_worktree_path("worker-15").exists()
        assert not git_ops.get_worktree_path("worker-17").exists()

    def test_cleanup_all_worktrees(self, git_ops):
        """Cleanup removes all worktrees when keep_workers is None."""
        git_ops.create_worktree("worker-18", "branch-18", base_branch="main")
        git_ops.create_worktree("worker-19", "branch-19", base_branch="main")

        result = git_ops.cleanup_worktrees(keep_workers=None)

        assert result.success is True
        assert not git_ops.get_worktree_path("worker-18").exists()
        assert not git_ops.get_worktree_path("worker-19").exists()


class TestWorktreePath:
    """Test worktree path generation."""

    def test_get_worktree_path(self, git_ops, temp_git_repo):
        """Get worktree path for a worker."""
        worker_id = "my-worker"
        expected = temp_git_repo / ".c4" / "worktrees" / "my-worker"

        result = git_ops.get_worktree_path(worker_id)

        assert result == expected

    def test_get_worktree_path_sanitizes_slashes(self, git_ops, temp_git_repo):
        """Slashes in worker_id are converted to dashes."""
        worker_id = "swarm/worker-1"
        expected = temp_git_repo / ".c4" / "worktrees" / "swarm-worker-1"

        result = git_ops.get_worktree_path(worker_id)

        assert result == expected
