"""Tests for worktree auto-allocation in c4_get_task."""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

from c4.mcp_server import C4Daemon
from c4.models import ProjectStatus
from tests.conftest import WORKER_1, WORKER_2, WORKER_3


class TestWorktreeAutoAllocation:
    """Tests for worktree auto-allocation when assigning tasks."""

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

    @pytest.fixture
    def daemon(self, git_repo: Path) -> C4Daemon:
        """Create C4Daemon with git repo and worktree enabled."""
        daemon = C4Daemon(project_root=git_repo)
        daemon.initialize(project_id="test-worktree")

        # Enable worktree for parallel execution
        daemon.config.worktree.enabled = True

        # Skip to PLAN and start
        daemon.state_machine._state.status = ProjectStatus.PLAN
        daemon.state_machine.save_state()

        return daemon

    def test_task_assignment_creates_worktree(self, daemon: C4Daemon, git_repo: Path):
        """c4_get_task should create worktree for assigned task."""
        # Add a task
        daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="Implement feature",
        )
        daemon.c4_start()

        # Get task - should create worktree
        result = daemon.c4_get_task(worker_id=WORKER_1)

        assert result is not None
        assert result.task_id == "T-001-0"
        assert result.worktree_path is not None

        # Verify worktree was created
        worktree_path = Path(result.worktree_path)
        assert worktree_path.exists()
        assert worktree_path.name == WORKER_1
        assert (worktree_path / "README.md").exists()

    def test_task_assignment_worktree_path_format(
        self, daemon: C4Daemon, git_repo: Path
    ):
        """Worktree path should be under .c4/worktrees/{worker_id}."""
        daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="Implement feature",
        )
        daemon.c4_start()

        result = daemon.c4_get_task(worker_id=WORKER_3)

        assert result is not None
        assert result.worktree_path is not None
        assert f".c4/worktrees/{WORKER_3}" in result.worktree_path

    def test_resumed_task_gets_existing_worktree(
        self, daemon: C4Daemon, git_repo: Path
    ):
        """Resuming a task should return existing worktree path."""
        daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="Implement feature",
        )
        daemon.c4_start()

        # First call - creates worktree
        result1 = daemon.c4_get_task(worker_id=WORKER_1)
        assert result1 is not None
        worktree_path1 = result1.worktree_path

        # Second call (resume) - should return same worktree
        result2 = daemon.c4_get_task(worker_id=WORKER_1)
        assert result2 is not None
        assert result2.worktree_path == worktree_path1

    def test_different_workers_get_different_worktrees(
        self, daemon: C4Daemon, git_repo: Path
    ):
        """Different workers should get separate worktrees."""
        daemon.c4_add_todo(
            task_id="T-001",
            title="Task 1",
            scope=None,
            dod="Implement feature 1",
        )
        daemon.c4_add_todo(
            task_id="T-002",
            title="Task 2",
            scope=None,
            dod="Implement feature 2",
        )
        daemon.c4_start()

        # Worker 1 gets task
        result1 = daemon.c4_get_task(worker_id=WORKER_1)
        assert result1 is not None

        # Worker 2 gets different task
        result2 = daemon.c4_get_task(worker_id=WORKER_2)
        assert result2 is not None

        # Different worktrees
        assert result1.worktree_path != result2.worktree_path
        assert WORKER_1 in result1.worktree_path
        assert WORKER_2 in result2.worktree_path


class TestWorktreeAutoAllocationEdgeCases:
    """Edge case tests for worktree auto-allocation."""

    def test_worktree_failure_does_not_block_task_assignment(self, tmp_path: Path):
        """Task assignment should succeed even if worktree creation fails."""
        # Create a non-git directory
        (tmp_path / ".c4").mkdir(exist_ok=True)

        daemon = C4Daemon(project_root=tmp_path)
        daemon.initialize(project_id="test-no-git")

        # Enable worktree (will fail for non-git repo)
        daemon.config.worktree.enabled = True

        # Skip to PLAN
        daemon.state_machine._state.status = ProjectStatus.PLAN
        daemon.state_machine.save_state()

        daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="Implement feature",
        )
        daemon.c4_start()

        # Should still get task (without worktree)
        result = daemon.c4_get_task(worker_id=WORKER_1)
        assert result is not None
        assert result.task_id == "T-001-0"
        # No worktree for non-git repo
        assert result.worktree_path is None

    @pytest.mark.skip(reason="Worker ID validation now enforces format 'worker-[a-f0-9]{8}'")
    def test_worker_id_with_special_characters(self, tmp_path: Path):
        """Worker IDs with slashes should be sanitized in worktree path."""
        # Initialize git repo
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
        (tmp_path / "README.md").write_text("# Test")
        subprocess.run(["git", "add", "."], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=tmp_path,
            capture_output=True,
        )

        daemon = C4Daemon(project_root=tmp_path)
        daemon.initialize(project_id="test-special")

        daemon.state_machine._state.status = ProjectStatus.PLAN
        daemon.state_machine.save_state()

        daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="Implement feature",
        )
        daemon.c4_start()

        # Worker ID with slashes
        result = daemon.c4_get_task(worker_id="worker/1/test")
        assert result is not None
        # Worktree path should have sanitized worker ID
        if result.worktree_path:
            assert "worker-1-test" in result.worktree_path
