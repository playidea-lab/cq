"""Tests for c4_worktree_cleanup MCP tool."""

import subprocess
import tempfile
from pathlib import Path

import pytest
import yaml

from c4.daemon.worktree_manager import WorktreeManager
from c4.mcp_server import C4Daemon


@pytest.fixture
def temp_git_repo():
    """Create a temporary git repository with initial commit."""
    with tempfile.TemporaryDirectory() as tmpdir:
        repo = Path(tmpdir)
        # Initialize git repo with main as default branch
        subprocess.run(["git", "init", "-b", "main"], cwd=repo, capture_output=True)
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


def create_config_file(repo: Path, worktree_enabled: bool = True) -> None:
    """Create config.yaml with worktree settings."""
    config = {
        "project_id": "test-project",
        "worktree": {
            "enabled": worktree_enabled,
        },
    }
    (repo / ".c4" / "config.yaml").write_text(yaml.dump(config))


@pytest.fixture
def daemon_with_worktree_enabled(temp_git_repo):
    """Create C4Daemon with worktree enabled."""
    create_config_file(temp_git_repo, worktree_enabled=True)
    daemon = C4Daemon(project_root=temp_git_repo)
    return daemon


@pytest.fixture
def daemon_with_worktree_disabled(temp_git_repo):
    """Create C4Daemon with worktree disabled."""
    create_config_file(temp_git_repo, worktree_enabled=False)
    daemon = C4Daemon(project_root=temp_git_repo)
    return daemon


class TestWorktreeCleanupDisabled:
    """Test c4_worktree_cleanup when worktree is disabled."""

    def test_returns_error_when_disabled(self, daemon_with_worktree_disabled):
        """Should return error when worktree feature is disabled."""
        result = daemon_with_worktree_disabled.c4_worktree_cleanup()

        assert result["success"] is False
        assert "disabled" in result["error"]
        assert "hint" in result


class TestWorktreeCleanupNoWorktrees:
    """Test c4_worktree_cleanup when no worktrees exist."""

    def test_cleanup_with_no_worktrees(self, daemon_with_worktree_enabled):
        """Should return success with zero deleted when no worktrees."""
        result = daemon_with_worktree_enabled.c4_worktree_cleanup()

        assert result["success"] is True
        assert result["deleted_count"] == 0
        assert result["kept_count"] == 0


class TestWorktreeCleanupKeepActiveFalse:
    """Test c4_worktree_cleanup with keep_active=False."""

    def test_removes_all_worktrees(self, daemon_with_worktree_enabled, temp_git_repo):
        """Should remove all worktrees when keep_active=False."""
        # Create some worktrees
        manager = WorktreeManager(temp_git_repo)
        manager.create_worktree("worker-1", "c4/w-T-001-0")
        manager.create_worktree("worker-2", "c4/w-T-002-0")

        # Verify worktrees exist
        assert len(manager.get_all_worker_ids()) == 2

        # Cleanup with keep_active=False
        result = daemon_with_worktree_enabled.c4_worktree_cleanup(keep_active=False)

        assert result["success"] is True
        assert result["deleted_count"] == 2
        assert result["kept_count"] == 0
        assert result["kept_workers"] == []

        # Verify worktrees are removed
        assert len(manager.get_all_worker_ids()) == 0


class TestWorktreeCleanupKeepActiveTrue:
    """Test c4_worktree_cleanup with keep_active=True (default)."""

    def test_removes_all_when_no_active_workers(
        self, daemon_with_worktree_enabled, temp_git_repo
    ):
        """Should remove all worktrees when no workers have active tasks."""
        # Create some worktrees
        manager = WorktreeManager(temp_git_repo)
        manager.create_worktree("worker-1", "c4/w-T-001-0")
        manager.create_worktree("worker-2", "c4/w-T-002-0")

        # Cleanup with keep_active=True (default)
        # Since there's no state machine initialized, no active workers
        result = daemon_with_worktree_enabled.c4_worktree_cleanup(keep_active=True)

        assert result["success"] is True
        # Without state machine, all worktrees should be removed
        assert result["deleted_count"] == 2


class TestWorktreeCleanupNotGitRepo:
    """Test c4_worktree_cleanup when not a git repository."""

    def test_returns_error_for_non_git_repo(self):
        """Should return error when not in a git repository."""
        with tempfile.TemporaryDirectory() as tmpdir:
            repo = Path(tmpdir)
            (repo / ".c4").mkdir()
            create_config_file(repo, worktree_enabled=True)

            daemon = C4Daemon(project_root=repo)
            result = daemon.c4_worktree_cleanup()

            assert result["success"] is False
            assert "git repository" in result["error"].lower()
