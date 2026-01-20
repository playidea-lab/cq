"""Tests for Git Operations module."""

import subprocess
from pathlib import Path

import pytest

from c4.daemon.git_ops import GitOperations


class TestGitOperations:
    """Test GitOperations class."""

    @pytest.fixture
    def git_project(self, tmp_path: Path) -> Path:
        """Create a temporary Git repository."""
        project = tmp_path / "git_project"
        project.mkdir()

        # Initialize git
        subprocess.run(["git", "init"], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=project,
            capture_output=True,
        )

        # Create initial file and commit
        (project / "README.md").write_text("# Test Project\n")
        subprocess.run(["git", "add", "."], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial commit"],
            cwd=project,
            capture_output=True,
        )

        return project

    @pytest.fixture
    def git_ops(self, git_project: Path) -> GitOperations:
        """Create GitOperations instance."""
        return GitOperations(git_project)

    def test_is_git_repo(self, git_ops: GitOperations, git_project: Path) -> None:
        """Test Git repository detection."""
        assert git_ops.is_git_repo() is True

        # Test non-repo
        non_repo = git_project.parent / "not_a_repo"
        non_repo.mkdir()
        non_git_ops = GitOperations(non_repo)
        assert non_git_ops.is_git_repo() is False

    def test_is_git_available(self, git_ops: GitOperations) -> None:
        """Test Git command availability check."""
        assert git_ops.is_git_available() is True

    def test_get_current_sha(self, git_ops: GitOperations) -> None:
        """Test getting current commit SHA."""
        sha = git_ops.get_current_sha()
        assert sha is not None
        assert len(sha) >= 7  # Short SHA is at least 7 chars

    def test_has_uncommitted_changes_false(self, git_ops: GitOperations) -> None:
        """Test detecting no uncommitted changes."""
        assert git_ops.has_uncommitted_changes() is False

    def test_has_uncommitted_changes_true(self, git_ops: GitOperations, git_project: Path) -> None:
        """Test detecting uncommitted changes."""
        # Create a new file
        (git_project / "new_file.txt").write_text("content")
        assert git_ops.has_uncommitted_changes() is True

    def test_stage_all(self, git_ops: GitOperations, git_project: Path) -> None:
        """Test staging all changes."""
        (git_project / "staged.txt").write_text("to be staged")
        assert git_ops.stage_all() is True

        # Verify file is staged
        result = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=git_project,
            capture_output=True,
            text=True,
        )
        assert "A  staged.txt" in result.stdout


class TestCommitTaskCompletion:
    """Test commit_task_completion method."""

    @pytest.fixture
    def git_project(self, tmp_path: Path) -> Path:
        """Create a temporary Git repository."""
        project = tmp_path / "git_project"
        project.mkdir()
        subprocess.run(["git", "init"], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=project,
            capture_output=True,
        )
        (project / "README.md").write_text("# Test\n")
        subprocess.run(["git", "add", "."], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=project,
            capture_output=True,
        )
        return project

    @pytest.fixture
    def git_ops(self, git_project: Path) -> GitOperations:
        """Create GitOperations instance."""
        return GitOperations(git_project)

    def test_commit_with_changes(self, git_ops: GitOperations, git_project: Path) -> None:
        """Test committing task completion with changes."""
        # Create changes
        (git_project / "task_output.py").write_text("# Task output\n")

        result = git_ops.commit_task_completion(
            task_id="T-001",
            task_title="Test Task",
            worker_id="worker-test",
        )

        assert result.success is True
        assert result.sha is not None
        assert "T-001" in result.message

        # Verify commit message
        log_result = subprocess.run(
            ["git", "log", "-1", "--format=%s"],
            cwd=git_project,
            capture_output=True,
            text=True,
        )
        assert "[T-001]" in log_result.stdout
        assert "Test Task" in log_result.stdout

    def test_commit_without_changes(self, git_ops: GitOperations) -> None:
        """Test committing when there are no changes."""
        result = git_ops.commit_task_completion(
            task_id="T-002",
            task_title="Empty Task",
        )

        assert result.success is True
        assert "No changes" in result.message
        assert result.sha is not None


class TestCommitRepairCompletion:
    """Test commit_repair_completion method."""

    @pytest.fixture
    def git_project(self, tmp_path: Path) -> Path:
        """Create a temporary Git repository."""
        project = tmp_path / "git_project"
        project.mkdir()
        subprocess.run(["git", "init"], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=project,
            capture_output=True,
        )
        (project / "README.md").write_text("# Test\n")
        subprocess.run(["git", "add", "."], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=project,
            capture_output=True,
        )
        return project

    @pytest.fixture
    def git_ops(self, git_project: Path) -> GitOperations:
        """Create GitOperations instance."""
        return GitOperations(git_project)

    def test_commit_repair(self, git_ops: GitOperations, git_project: Path) -> None:
        """Test committing repair task completion."""
        (git_project / "fixed.py").write_text("# Fixed code\n")

        result = git_ops.commit_repair_completion(
            task_id="REPAIR-T-001-1",
            original_task_id="T-001",
            repair_reason="Test failure",
        )

        assert result.success is True
        assert result.sha is not None

        # Verify commit message
        log_result = subprocess.run(
            ["git", "log", "-1", "--format=%B"],
            cwd=git_project,
            capture_output=True,
            text=True,
        )
        assert "REPAIR-T-001-1" in log_result.stdout
        assert "Original Task: T-001" in log_result.stdout


class TestCheckpointTag:
    """Test checkpoint tag operations."""

    @pytest.fixture
    def git_project(self, tmp_path: Path) -> Path:
        """Create a temporary Git repository."""
        project = tmp_path / "git_project"
        project.mkdir()
        subprocess.run(["git", "init"], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=project,
            capture_output=True,
        )
        (project / "README.md").write_text("# Test\n")
        subprocess.run(["git", "add", "."], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=project,
            capture_output=True,
        )
        return project

    @pytest.fixture
    def git_ops(self, git_project: Path) -> GitOperations:
        """Create GitOperations instance."""
        return GitOperations(git_project)

    def test_create_checkpoint_tag(self, git_ops: GitOperations) -> None:
        """Test creating a checkpoint tag."""
        result = git_ops.create_checkpoint_tag(
            checkpoint_id="CP-001",
            checkpoint_name="First Checkpoint",
        )

        assert result.success is True
        assert result.tag == "c4/CP-001"
        assert "Created tag" in result.message

    def test_create_duplicate_tag(self, git_ops: GitOperations) -> None:
        """Test creating a duplicate tag returns success."""
        # Create first tag
        git_ops.create_checkpoint_tag("CP-002")

        # Try to create same tag
        result = git_ops.create_checkpoint_tag("CP-002")

        assert result.success is True
        assert "already exists" in result.message

    def test_get_checkpoint_tags(self, git_ops: GitOperations, git_project: Path) -> None:
        """Test getting checkpoint tags."""
        # Create some tags
        git_ops.create_checkpoint_tag("CP-001")
        git_ops.create_checkpoint_tag("CP-002")

        tags = git_ops.get_checkpoint_tags()

        assert "c4/CP-001" in tags
        assert "c4/CP-002" in tags


class TestBranchOperations:
    """Test branch-related operations."""

    @pytest.fixture
    def git_project(self, tmp_path: Path) -> Path:
        """Create a temporary Git repository."""
        project = tmp_path / "git_project"
        project.mkdir()
        subprocess.run(["git", "init"], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=project,
            capture_output=True,
        )
        (project / "README.md").write_text("# Test\n")
        subprocess.run(["git", "add", "."], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=project,
            capture_output=True,
        )
        return project

    @pytest.fixture
    def git_ops(self, git_project: Path) -> GitOperations:
        """Create GitOperations instance."""
        return GitOperations(git_project)

    def test_get_branch_name(self, git_ops: GitOperations) -> None:
        """Test getting current branch name."""
        branch = git_ops.get_branch_name()
        # Could be 'main' or 'master' depending on git config
        assert branch in ["main", "master"]

    def test_create_task_branch(self, git_ops: GitOperations) -> None:
        """Test creating a task branch."""
        result = git_ops.create_task_branch("T-001")

        assert result.success is True
        assert git_ops.get_branch_name() == "c4/w-T-001"

    def test_create_existing_branch(self, git_ops: GitOperations) -> None:
        """Test switching to existing task branch."""
        # Create branch first
        git_ops.create_task_branch("T-002")

        # Switch away
        subprocess.run(
            ["git", "checkout", "-"],
            cwd=git_ops.root,
            capture_output=True,
        )

        # Create/switch to same branch
        result = git_ops.create_task_branch("T-002")

        assert result.success is True
        assert "existing branch" in result.message.lower()


class TestRollbackOperations:
    """Test rollback functionality."""

    @pytest.fixture
    def git_project(self, tmp_path: Path) -> Path:
        """Create a temporary Git repository with history."""
        project = tmp_path / "git_project"
        project.mkdir()
        subprocess.run(["git", "init"], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=project,
            capture_output=True,
        )
        (project / "README.md").write_text("# Test\n")
        subprocess.run(["git", "add", "."], cwd=project, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=project,
            capture_output=True,
        )
        return project

    @pytest.fixture
    def git_ops(self, git_project: Path) -> GitOperations:
        """Create GitOperations instance."""
        return GitOperations(git_project)

    def test_get_tag_info(self, git_ops: GitOperations) -> None:
        """Test getting detailed tag information."""
        # Create a checkpoint
        git_ops.create_checkpoint_tag("CP-001", "Test checkpoint")

        info = git_ops.get_tag_info("c4/CP-001")

        assert info is not None
        assert "sha" in info
        assert "date" in info
        assert "message" in info
        assert "Test checkpoint" in info["message"]

    def test_get_tag_info_nonexistent(self, git_ops: GitOperations) -> None:
        """Test getting info for non-existent tag."""
        info = git_ops.get_tag_info("c4/CP-999")
        assert info is None

    def test_list_checkpoint_tags(self, git_ops: GitOperations) -> None:
        """Test listing checkpoint tags with details."""
        # Create checkpoints
        git_ops.create_checkpoint_tag("CP-001", "First")
        git_ops.create_checkpoint_tag("CP-002", "Second")

        tags = git_ops.list_checkpoint_tags()

        assert len(tags) == 2
        assert all("tag" in t for t in tags)
        assert all("sha" in t for t in tags)
        assert all("date" in t for t in tags)

    def test_rollback_to_checkpoint_hard(self, git_ops: GitOperations, git_project: Path) -> None:
        """Test hard rollback to checkpoint."""
        # Create checkpoint
        git_ops.create_checkpoint_tag("CP-001", "Stable")
        stable_sha = git_ops.get_current_sha()

        # Add more work
        (git_project / "new_file.py").write_text("# New\n")
        git_ops.commit_task_completion("T-001", "New feature")

        # Verify file exists
        assert (git_project / "new_file.py").exists()

        # Rollback
        result = git_ops.rollback_to_checkpoint("c4/CP-001", hard=True)

        assert result.success is True
        assert result.sha == stable_sha
        assert not (git_project / "new_file.py").exists()

    def test_rollback_to_checkpoint_soft(self, git_ops: GitOperations, git_project: Path) -> None:
        """Test soft rollback to checkpoint."""
        # Create checkpoint
        git_ops.create_checkpoint_tag("CP-001", "Stable")

        # Add more work
        (git_project / "new_file.py").write_text("# New\n")
        git_ops.commit_task_completion("T-001", "New feature")

        # Soft rollback
        result = git_ops.rollback_to_checkpoint("c4/CP-001", hard=False)

        assert result.success is True
        # File should still exist (soft reset keeps working directory)
        assert (git_project / "new_file.py").exists()

    def test_rollback_nonexistent_tag(self, git_ops: GitOperations) -> None:
        """Test rollback to non-existent tag fails."""
        result = git_ops.rollback_to_checkpoint("c4/CP-999")

        assert result.success is False
        assert "not found" in result.message.lower()

    def test_rollback_returns_correct_message(
        self, git_ops: GitOperations, git_project: Path
    ) -> None:
        """Test rollback message contains useful info."""
        git_ops.create_checkpoint_tag("CP-001")
        (git_project / "file.py").write_text("# File\n")
        git_ops.commit_task_completion("T-001", "Task")

        result = git_ops.rollback_to_checkpoint("c4/CP-001")

        assert result.success is True
        assert "c4/CP-001" in result.message
        assert result.tag == "c4/CP-001"
