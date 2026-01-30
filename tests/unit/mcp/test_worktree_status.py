"""Tests for c4_worktree_status MCP tool."""

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


class TestWorktreeStatusDisabled:
    """Test c4_worktree_status when worktree is disabled."""

    def test_returns_error_when_disabled(self, daemon_with_worktree_disabled):
        """Should return error when worktree feature is disabled."""
        result = daemon_with_worktree_disabled.c4_worktree_status()

        assert result["success"] is False
        assert "disabled" in result["error"]
        assert "hint" in result


class TestWorktreeStatusAllWorktrees:
    """Test c4_worktree_status with no worker_id (list all)."""

    def test_returns_empty_list_initially(self, daemon_with_worktree_enabled):
        """Should return empty list when no worktrees exist."""
        result = daemon_with_worktree_enabled.c4_worktree_status()

        assert result["success"] is True
        assert result["worker_count"] == 0
        assert result["worker_ids"] == []
        assert "worktrees_dir" in result

    def test_returns_all_worktrees(self, daemon_with_worktree_enabled, temp_git_repo):
        """Should return all worktrees when multiple exist."""
        # Create some worktrees
        manager = WorktreeManager(temp_git_repo)
        manager.create_worktree("worker-1", "c4/w-T-001-0")
        manager.create_worktree("worker-2", "c4/w-T-002-0")

        result = daemon_with_worktree_enabled.c4_worktree_status()

        assert result["success"] is True
        assert result["worker_count"] == 2
        assert set(result["worker_ids"]) == {"worker-1", "worker-2"}


class TestWorktreeStatusSpecificWorker:
    """Test c4_worktree_status with specific worker_id."""

    def test_returns_info_for_nonexistent_worker(self, daemon_with_worktree_enabled):
        """Should return exists=False for nonexistent worker."""
        result = daemon_with_worktree_enabled.c4_worktree_status(
            worker_id="nonexistent"
        )

        assert result["success"] is True
        assert result["worker_id"] == "nonexistent"
        assert result["exists"] is False

    def test_returns_detailed_info_for_existing_worker(
        self, daemon_with_worktree_enabled, temp_git_repo
    ):
        """Should return detailed info for existing worker."""
        # Create a worktree
        manager = WorktreeManager(temp_git_repo)
        manager.create_worktree("worker-test", "c4/w-T-test-0")

        result = daemon_with_worktree_enabled.c4_worktree_status(
            worker_id="worker-test"
        )

        assert result["success"] is True
        assert result["worker_id"] == "worker-test"
        assert result["exists"] is True
        assert result["branch"] == "c4/w-T-test-0"
        assert result["head"] is not None
        assert result["has_changes"] is False
        assert "path" in result


class TestWorktreeStatusNotGitRepo:
    """Test c4_worktree_status when not a git repository."""

    def test_returns_error_for_non_git_repo(self):
        """Should return error when not in a git repository."""
        with tempfile.TemporaryDirectory() as tmpdir:
            repo = Path(tmpdir)
            (repo / ".c4").mkdir()
            create_config_file(repo, worktree_enabled=True)

            daemon = C4Daemon(project_root=repo)
            result = daemon.c4_worktree_status()

            assert result["success"] is False
            assert "git repository" in result["error"].lower()
