"""Tests for WorktreeManager."""

import subprocess
from pathlib import Path

import pytest

from c4.daemon.worktree_manager import WorktreeInfo, WorktreeManager


@pytest.fixture
def git_repo(tmp_path: Path) -> Path:
    """Create a temporary git repository."""
    # Initialize git repo
    subprocess.run(["git", "init"], cwd=tmp_path, capture_output=True, check=True)
    subprocess.run(
        ["git", "config", "user.email", "test@example.com"],
        cwd=tmp_path,
        capture_output=True,
        check=True,
    )
    subprocess.run(
        ["git", "config", "user.name", "Test User"],
        cwd=tmp_path,
        capture_output=True,
        check=True,
    )

    # Create initial commit
    (tmp_path / "README.md").write_text("# Test Project")
    subprocess.run(["git", "add", "README.md"], cwd=tmp_path, capture_output=True, check=True)
    subprocess.run(
        ["git", "commit", "-m", "Initial commit"],
        cwd=tmp_path,
        capture_output=True,
        check=True,
    )

    # Create .c4 directory
    (tmp_path / ".c4").mkdir()

    return tmp_path


@pytest.fixture
def manager(git_repo: Path) -> WorktreeManager:
    """Create WorktreeManager instance."""
    return WorktreeManager(git_repo)


class TestWorktreeManagerInit:
    """Tests for WorktreeManager initialization."""

    def test_init_sets_project_root(self, git_repo: Path) -> None:
        """Should set project root correctly."""
        manager = WorktreeManager(git_repo)
        assert manager.root == git_repo

    def test_worktrees_dir_path(self, manager: WorktreeManager, git_repo: Path) -> None:
        """Should return correct worktrees directory path."""
        expected = git_repo / ".c4" / "worktrees"
        assert manager.worktrees_dir == expected


class TestGetWorktreePath:
    """Tests for get_worktree_path method."""

    def test_returns_correct_path(self, manager: WorktreeManager, git_repo: Path) -> None:
        """Should return correct path for worker ID."""
        path = manager.get_worktree_path("worker-1")
        expected = git_repo / ".c4" / "worktrees" / "worker-1"
        assert path == expected

    def test_sanitizes_worker_id(self, manager: WorktreeManager, git_repo: Path) -> None:
        """Should sanitize worker ID with slashes."""
        path = manager.get_worktree_path("worker/test/1")
        expected = git_repo / ".c4" / "worktrees" / "worker-test-1"
        assert path == expected


class TestCreateWorktree:
    """Tests for create_worktree method."""

    def test_creates_new_worktree(self, manager: WorktreeManager, git_repo: Path) -> None:
        """Should create new worktree for worker."""
        result = manager.create_worktree("worker-1", "c4/w-T-001-0")

        assert result.success is True
        assert "Created worktree" in result.message or "already exists" in result.message

        worktree_path = manager.get_worktree_path("worker-1")
        assert worktree_path.exists()

    def test_creates_worktree_with_base_branch(self, manager: WorktreeManager, git_repo: Path) -> None:
        """Should create worktree from specified base branch."""
        # Create a feature branch
        subprocess.run(
            ["git", "checkout", "-b", "feature-base"],
            cwd=git_repo,
            capture_output=True,
            check=True,
        )
        (git_repo / "feature.txt").write_text("feature content")
        subprocess.run(["git", "add", "."], cwd=git_repo, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Feature commit"],
            cwd=git_repo,
            capture_output=True,
        )
        subprocess.run(["git", "checkout", "main"], cwd=git_repo, capture_output=True)

        result = manager.create_worktree("worker-2", "c4/w-T-002-0", base_branch="feature-base")

        assert result.success is True

        worktree_path = manager.get_worktree_path("worker-2")
        # The feature file should exist in the worktree since it's based on feature-base
        assert (worktree_path / "feature.txt").exists()

    def test_existing_worktree_returns_success(self, manager: WorktreeManager) -> None:
        """Should return success for already existing worktree."""
        # Create first time
        manager.create_worktree("worker-3", "c4/w-T-003-0")

        # Try to create again
        result = manager.create_worktree("worker-3", "c4/w-T-003-0")

        assert result.success is True
        assert "already exists" in result.message


class TestRemoveWorktree:
    """Tests for remove_worktree method."""

    def test_removes_existing_worktree(self, manager: WorktreeManager) -> None:
        """Should remove existing worktree."""
        # Create worktree first
        manager.create_worktree("worker-remove", "c4/w-remove")
        worktree_path = manager.get_worktree_path("worker-remove")
        assert worktree_path.exists()

        # Remove it
        result = manager.remove_worktree("worker-remove")

        assert result.success is True
        assert not worktree_path.exists()

    def test_remove_nonexistent_returns_success(self, manager: WorktreeManager) -> None:
        """Should return success when removing non-existent worktree."""
        result = manager.remove_worktree("nonexistent-worker")

        assert result.success is True
        assert "does not exist" in result.message


class TestListWorktrees:
    """Tests for list_worktrees method."""

    def test_list_empty_initially(self, manager: WorktreeManager) -> None:
        """Should list only main worktree initially."""
        worktrees = manager.list_worktrees()
        # At least the main worktree exists
        assert len(worktrees) >= 1

    def test_list_includes_created_worktrees(self, manager: WorktreeManager) -> None:
        """Should include created worktrees in list."""
        manager.create_worktree("list-test-worker", "c4/w-list-test")

        worktrees = manager.list_worktrees()
        paths = [wt.get("path", "") for wt in worktrees]

        worktree_path = str(manager.get_worktree_path("list-test-worker"))
        assert any(worktree_path in p for p in paths)


class TestGetWorktreeInfo:
    """Tests for get_worktree_info method."""

    def test_info_for_nonexistent_worktree(self, manager: WorktreeManager) -> None:
        """Should return exists=False for non-existent worktree."""
        info = manager.get_worktree_info("nonexistent")

        assert isinstance(info, WorktreeInfo)
        assert info.exists is False
        assert info.worker_id == "nonexistent"

    def test_info_for_existing_worktree(self, manager: WorktreeManager) -> None:
        """Should return full info for existing worktree."""
        manager.create_worktree("info-test", "c4/w-info-test")

        info = manager.get_worktree_info("info-test")

        assert info.exists is True
        assert info.worker_id == "info-test"
        assert info.branch == "c4/w-info-test"
        assert info.head is not None  # Should have a HEAD SHA
        assert info.has_changes is False  # Fresh worktree has no changes


class TestCommitInWorktree:
    """Tests for commit_in_worktree method."""

    def test_commit_with_changes(self, manager: WorktreeManager) -> None:
        """Should commit changes in worktree."""
        manager.create_worktree("commit-test", "c4/w-commit-test")
        worktree_path = manager.get_worktree_path("commit-test")

        # Create a file in the worktree
        (worktree_path / "test.txt").write_text("test content")

        result = manager.commit_in_worktree("commit-test", "[T-001] Test commit")

        assert result.success is True
        assert result.sha is not None

    def test_commit_with_no_changes(self, manager: WorktreeManager) -> None:
        """Should handle no changes gracefully."""
        manager.create_worktree("no-changes", "c4/w-no-changes")

        result = manager.commit_in_worktree("no-changes", "[T-002] No changes")

        assert result.success is True
        assert "No changes" in result.message


class TestCleanup:
    """Tests for cleanup method."""

    def test_cleanup_all_worktrees(self, manager: WorktreeManager) -> None:
        """Should remove all worktrees when no keep list."""
        manager.create_worktree("cleanup-1", "c4/w-cleanup-1")
        manager.create_worktree("cleanup-2", "c4/w-cleanup-2")

        result = manager.cleanup()

        assert result.success is True
        assert not manager.get_worktree_path("cleanup-1").exists()
        assert not manager.get_worktree_path("cleanup-2").exists()

    def test_cleanup_keeps_specified_workers(self, manager: WorktreeManager) -> None:
        """Should keep worktrees in keep_workers list."""
        manager.create_worktree("keep-this", "c4/w-keep")
        manager.create_worktree("remove-this", "c4/w-remove")

        result = manager.cleanup(keep_workers=["keep-this"])

        assert result.success is True
        assert manager.get_worktree_path("keep-this").exists()
        assert not manager.get_worktree_path("remove-this").exists()


class TestGetAllWorkerIds:
    """Tests for get_all_worker_ids method."""

    def test_empty_initially(self, manager: WorktreeManager) -> None:
        """Should return empty list when no worktrees."""
        worker_ids = manager.get_all_worker_ids()
        assert worker_ids == []

    def test_returns_all_worker_ids(self, manager: WorktreeManager) -> None:
        """Should return all worker IDs with worktrees."""
        manager.create_worktree("worker-a", "c4/w-a")
        manager.create_worktree("worker-b", "c4/w-b")

        worker_ids = manager.get_all_worker_ids()

        assert set(worker_ids) == {"worker-a", "worker-b"}


class TestEnsureWorktree:
    """Tests for ensure_worktree method."""

    def test_creates_if_not_exists(self, manager: WorktreeManager) -> None:
        """Should create worktree if it doesn't exist."""
        result = manager.ensure_worktree("ensure-new", "c4/w-ensure-new")

        assert result.success is True
        assert manager.get_worktree_path("ensure-new").exists()

    def test_returns_success_if_exists(self, manager: WorktreeManager) -> None:
        """Should return success without modification if exists."""
        manager.create_worktree("ensure-exists", "c4/w-ensure-exists")

        result = manager.ensure_worktree("ensure-exists", "c4/w-ensure-exists")

        assert result.success is True
        assert "already exists" in result.message
