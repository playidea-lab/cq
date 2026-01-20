"""Integration tests for Git operations in C4 workflow.

Tests the complete flow of:
- Automatic commits on task completion
- Checkpoint tag creation
- Rollback functionality
"""

import subprocess
from pathlib import Path

import pytest

from c4.daemon.git_ops import GitOperations


class TestAutoCommitWorkflow:
    """Test automatic commit workflow during task lifecycle."""

    @pytest.fixture
    def project_with_c4(self, tmp_path: Path) -> Path:
        """Create a project with Git and C4 initialized."""
        project = tmp_path / "c4_project"
        project.mkdir()

        # Initialize git
        subprocess.run(["git", "init"], cwd=project, capture_output=True, check=True)
        subprocess.run(
            ["git", "config", "user.email", "test@c4.local"],
            cwd=project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "C4 Test"],
            cwd=project,
            capture_output=True,
        )

        # Create initial structure
        (project / "README.md").write_text("# C4 Test Project\n")
        (project / ".c4").mkdir()
        (project / ".c4" / "config.yaml").write_text("project_id: test\n")

        subprocess.run(["git", "add", "."], cwd=project, capture_output=True, check=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial commit"],
            cwd=project,
            capture_output=True,
            check=True,
        )

        return project

    def test_task_completion_creates_commit(self, project_with_c4: Path) -> None:
        """Test that completing a task creates a proper commit."""
        git_ops = GitOperations(project_with_c4)

        # Simulate task work
        (project_with_c4 / "src").mkdir()
        (project_with_c4 / "src" / "feature.py").write_text("# New feature\n")

        # Complete task
        result = git_ops.commit_task_completion(
            task_id="T-001",
            task_title="Implement feature",
            worker_id="worker-test",
        )

        assert result.success is True
        assert result.sha is not None

        # Verify commit in history
        log = subprocess.run(
            ["git", "log", "--oneline", "-1"],
            cwd=project_with_c4,
            capture_output=True,
            text=True,
        )
        assert "[T-001]" in log.stdout
        assert "Implement feature" in log.stdout

    def test_multiple_tasks_create_separate_commits(self, project_with_c4: Path) -> None:
        """Test that multiple tasks create separate commits."""
        git_ops = GitOperations(project_with_c4)

        # Task 1
        (project_with_c4 / "task1.py").write_text("# Task 1\n")
        git_ops.commit_task_completion("T-001", "First task")

        # Task 2
        (project_with_c4 / "task2.py").write_text("# Task 2\n")
        git_ops.commit_task_completion("T-002", "Second task")

        # Verify both commits exist
        log = subprocess.run(
            ["git", "log", "--oneline", "-3"],
            cwd=project_with_c4,
            capture_output=True,
            text=True,
        )
        assert "[T-001]" in log.stdout
        assert "[T-002]" in log.stdout

    def test_repair_task_creates_commit(self, project_with_c4: Path) -> None:
        """Test that repair tasks create proper commits."""
        git_ops = GitOperations(project_with_c4)

        # Original task
        (project_with_c4 / "buggy.py").write_text("# Buggy code\n")
        git_ops.commit_task_completion("T-001", "Initial implementation")

        # Repair task
        (project_with_c4 / "buggy.py").write_text("# Fixed code\n")
        result = git_ops.commit_repair_completion(
            task_id="REPAIR-T-001-1",
            original_task_id="T-001",
            repair_reason="Test failure",
        )

        assert result.success is True

        # Verify repair commit
        log = subprocess.run(
            ["git", "log", "--oneline", "-1"],
            cwd=project_with_c4,
            capture_output=True,
            text=True,
        )
        assert "REPAIR-T-001-1" in log.stdout


class TestCheckpointTagWorkflow:
    """Test checkpoint tag creation and management."""

    @pytest.fixture
    def project_with_tasks(self, tmp_path: Path) -> tuple[Path, GitOperations]:
        """Create a project with some completed tasks."""
        project = tmp_path / "c4_project"
        project.mkdir()

        subprocess.run(["git", "init"], cwd=project, capture_output=True, check=True)
        subprocess.run(
            ["git", "config", "user.email", "test@c4.local"],
            cwd=project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "C4 Test"],
            cwd=project,
            capture_output=True,
        )

        (project / "README.md").write_text("# Project\n")
        subprocess.run(["git", "add", "."], cwd=project, capture_output=True, check=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=project,
            capture_output=True,
            check=True,
        )

        git_ops = GitOperations(project)

        # Complete some tasks
        (project / "t1.py").write_text("# T1\n")
        git_ops.commit_task_completion("T-001", "Task 1")

        (project / "t2.py").write_text("# T2\n")
        git_ops.commit_task_completion("T-002", "Task 2")

        return project, git_ops

    def test_checkpoint_creates_tag(self, project_with_tasks: tuple[Path, GitOperations]) -> None:
        """Test that checkpoint creates an annotated tag."""
        project, git_ops = project_with_tasks

        result = git_ops.create_checkpoint_tag(
            checkpoint_id="CP-001",
            checkpoint_name="First checkpoint",
        )

        assert result.success is True
        assert result.tag == "c4/CP-001"

        # Verify tag exists
        tags = subprocess.run(
            ["git", "tag", "-l", "c4/*"],
            cwd=project,
            capture_output=True,
            text=True,
        )
        assert "c4/CP-001" in tags.stdout

    def test_multiple_checkpoints(self, project_with_tasks: tuple[Path, GitOperations]) -> None:
        """Test creating multiple checkpoints."""
        project, git_ops = project_with_tasks

        # First checkpoint
        git_ops.create_checkpoint_tag("CP-001", "Phase 1")

        # Add more work
        (project / "t3.py").write_text("# T3\n")
        git_ops.commit_task_completion("T-003", "Task 3")

        # Second checkpoint
        git_ops.create_checkpoint_tag("CP-002", "Phase 2")

        # Verify both tags
        tags = git_ops.get_checkpoint_tags()
        assert "c4/CP-001" in tags
        assert "c4/CP-002" in tags

    def test_commits_between_checkpoints(
        self, project_with_tasks: tuple[Path, GitOperations]
    ) -> None:
        """Test getting commits between checkpoints."""
        project, git_ops = project_with_tasks

        # First checkpoint
        git_ops.create_checkpoint_tag("CP-001", "Phase 1")

        # Add more work
        (project / "t3.py").write_text("# T3\n")
        git_ops.commit_task_completion("T-003", "Task 3")

        (project / "t4.py").write_text("# T4\n")
        git_ops.commit_task_completion("T-004", "Task 4")

        # Get commits since checkpoint
        commits = git_ops.get_commits_since_tag("c4/CP-001")

        assert len(commits) == 2
        messages = [c["message"] for c in commits]
        assert any("T-003" in m for m in messages)
        assert any("T-004" in m for m in messages)


class TestRollbackWorkflow:
    """Test rollback functionality using Git."""

    @pytest.fixture
    def project_with_history(self, tmp_path: Path) -> tuple[Path, GitOperations]:
        """Create a project with commit history and checkpoint."""
        project = tmp_path / "c4_project"
        project.mkdir()

        subprocess.run(["git", "init"], cwd=project, capture_output=True, check=True)
        subprocess.run(
            ["git", "config", "user.email", "test@c4.local"],
            cwd=project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "C4 Test"],
            cwd=project,
            capture_output=True,
        )

        (project / "README.md").write_text("# Project\n")
        subprocess.run(["git", "add", "."], cwd=project, capture_output=True, check=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=project,
            capture_output=True,
            check=True,
        )

        git_ops = GitOperations(project)
        return project, git_ops

    def test_can_view_checkpoint_history(
        self, project_with_history: tuple[Path, GitOperations]
    ) -> None:
        """Test that we can view history from checkpoints."""
        project, git_ops = project_with_history

        # Create work and checkpoint
        (project / "v1.py").write_text("# Version 1\n")
        git_ops.commit_task_completion("T-001", "Version 1")
        git_ops.create_checkpoint_tag("CP-001")

        # More work after checkpoint
        (project / "v2.py").write_text("# Version 2\n")
        git_ops.commit_task_completion("T-002", "Version 2")

        # Can see commits since checkpoint
        commits = git_ops.get_commits_since_tag("c4/CP-001")
        assert len(commits) == 1
        assert "T-002" in commits[0]["message"]

    def test_rollback_to_checkpoint(self, project_with_history: tuple[Path, GitOperations]) -> None:
        """Test rolling back to a checkpoint using git reset."""
        project, git_ops = project_with_history

        # Create work and checkpoint
        (project / "stable.py").write_text("# Stable\n")
        git_ops.commit_task_completion("T-001", "Stable feature")
        git_ops.create_checkpoint_tag("CP-001")
        stable_sha = git_ops.get_current_sha()

        # More work that we want to rollback
        (project / "broken.py").write_text("# Broken\n")
        git_ops.commit_task_completion("T-002", "Broken feature")

        # Verify broken file exists
        assert (project / "broken.py").exists()

        # Rollback to checkpoint (hard reset)
        result = subprocess.run(
            ["git", "reset", "--hard", "c4/CP-001"],
            cwd=project,
            capture_output=True,
        )
        assert result.returncode == 0

        # Verify rollback
        assert not (project / "broken.py").exists()
        assert (project / "stable.py").exists()
        assert git_ops.get_current_sha() == stable_sha

    def test_soft_rollback_preserves_changes(
        self, project_with_history: tuple[Path, GitOperations]
    ) -> None:
        """Test soft rollback that keeps working directory changes."""
        project, git_ops = project_with_history

        # Create work and checkpoint
        (project / "v1.py").write_text("# V1\n")
        git_ops.commit_task_completion("T-001", "V1")
        git_ops.create_checkpoint_tag("CP-001")

        # More work
        (project / "v2.py").write_text("# V2\n")
        git_ops.commit_task_completion("T-002", "V2")

        # Soft reset (keeps files, unstages commits)
        result = subprocess.run(
            ["git", "reset", "--soft", "c4/CP-001"],
            cwd=project,
            capture_output=True,
        )
        assert result.returncode == 0

        # Files still exist
        assert (project / "v2.py").exists()

        # But commits are unstaged
        status = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=project,
            capture_output=True,
            text=True,
        )
        assert "v2.py" in status.stdout  # Shows as staged


class TestTaskBranchWorkflow:
    """Test task branch creation and management."""

    @pytest.fixture
    def git_project(self, tmp_path: Path) -> tuple[Path, GitOperations]:
        """Create a basic git project."""
        project = tmp_path / "c4_project"
        project.mkdir()

        subprocess.run(["git", "init"], cwd=project, capture_output=True, check=True)
        subprocess.run(
            ["git", "config", "user.email", "test@c4.local"],
            cwd=project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "C4 Test"],
            cwd=project,
            capture_output=True,
        )

        (project / "README.md").write_text("# Project\n")
        subprocess.run(["git", "add", "."], cwd=project, capture_output=True, check=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=project,
            capture_output=True,
            check=True,
        )

        return project, GitOperations(project)

    def test_create_task_branch(self, git_project: tuple[Path, GitOperations]) -> None:
        """Test creating a branch for a task."""
        project, git_ops = git_project

        result = git_ops.create_task_branch("T-001")

        assert result.success is True
        assert git_ops.get_branch_name() == "c4/w-T-001"

    def test_switch_between_task_branches(self, git_project: tuple[Path, GitOperations]) -> None:
        """Test switching between multiple task branches."""
        project, git_ops = git_project

        # Get default branch name (could be main or master)
        default_branch = git_ops.get_branch_name()

        # Create and work on T-001
        git_ops.create_task_branch("T-001")
        (project / "t1.py").write_text("# T1\n")
        git_ops.commit_task_completion("T-001", "Task 1")

        # Create and work on T-002 from default branch
        subprocess.run(
            ["git", "checkout", default_branch],
            cwd=project,
            capture_output=True,
        )
        git_ops.create_task_branch("T-002")
        (project / "t2.py").write_text("# T2\n")
        git_ops.commit_task_completion("T-002", "Task 2")

        # T-001 branch should have t1.py but not t2.py
        subprocess.run(
            ["git", "checkout", "c4/w-T-001"],
            cwd=project,
            capture_output=True,
        )
        assert (project / "t1.py").exists()
        assert not (project / "t2.py").exists()

        # T-002 branch should have t2.py but not t1.py
        subprocess.run(
            ["git", "checkout", "c4/w-T-002"],
            cwd=project,
            capture_output=True,
        )
        assert (project / "t2.py").exists()
        assert not (project / "t1.py").exists()
