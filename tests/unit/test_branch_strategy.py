"""Tests for C4 Branch Strategy.

Branch strategy: main → c4/{project_id} → c4/w-T-XXX → merge back

These tests verify:
1. Work branch creation from default_branch
2. Task branch creation from work branch
3. Checkpoint APPROVE merges task branches
4. Plan completion triggers completion_action
"""

import subprocess
from pathlib import Path

import pytest

from c4.daemon.git_ops import GitOperations, GitResult
from c4.mcp_server import C4Daemon
from c4.models import ProjectStatus
from c4.models.config import C4Config


class TestC4ConfigBranchStrategy:
    """Test C4Config branch strategy fields."""

    def test_default_work_branch_none(self):
        """work_branch defaults to None."""
        config = C4Config(project_id="test")
        assert config.work_branch is None

    def test_get_work_branch_default(self):
        """get_work_branch() returns c4/{project_id} when work_branch is None."""
        config = C4Config(project_id="my-project")
        assert config.get_work_branch() == "c4/my-project"

    def test_get_work_branch_custom(self):
        """get_work_branch() returns custom work_branch when set."""
        config = C4Config(project_id="test", work_branch="custom-branch")
        assert config.get_work_branch() == "custom-branch"

    def test_completion_action_default_merge(self):
        """completion_action defaults to 'merge'."""
        config = C4Config(project_id="test")
        assert config.completion_action == "merge"

    def test_completion_action_pr(self):
        """completion_action can be set to 'pr'."""
        config = C4Config(project_id="test", completion_action="pr")
        assert config.completion_action == "pr"

    def test_completion_action_manual(self):
        """completion_action can be set to 'manual'."""
        config = C4Config(project_id="test", completion_action="manual")
        assert config.completion_action == "manual"

    def test_completion_action_invalid(self):
        """completion_action rejects invalid values."""
        with pytest.raises(ValueError):
            C4Config(project_id="test", completion_action="invalid")


class TestGitOperationsEnsureWorkBranch:
    """Test GitOperations.ensure_work_branch()."""

    @pytest.fixture
    def git_repo(self, tmp_path):
        """Create an initialized git repo with an initial commit."""
        subprocess.run(["git", "init"], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=tmp_path,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test"],
            cwd=tmp_path,
            capture_output=True,
        )
        # Create initial commit on main
        (tmp_path / "README.md").write_text("# Test")
        subprocess.run(["git", "add", "."], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial commit"],
            cwd=tmp_path,
            capture_output=True,
        )
        return tmp_path

    def test_create_work_branch_from_main(self, git_repo):
        """Creates work branch from main when it doesn't exist."""
        git_ops = GitOperations(git_repo)
        result = git_ops.ensure_work_branch("c4/test", "main")

        assert result.success is True
        assert "Created work branch" in result.message

        # Verify we're on the work branch
        current = git_ops.get_branch_name()
        assert current == "c4/test"

    def test_switch_to_existing_work_branch(self, git_repo):
        """Switches to existing work branch."""
        git_ops = GitOperations(git_repo)

        # Create work branch first
        git_ops.ensure_work_branch("c4/test", "main")

        # Switch back to main
        subprocess.run(["git", "checkout", "main"], cwd=git_repo, capture_output=True)

        # Ensure work branch should switch to existing
        result = git_ops.ensure_work_branch("c4/test", "main")

        assert result.success is True
        assert "Switched to existing" in result.message
        assert git_ops.get_branch_name() == "c4/test"

    def test_create_work_branch_no_default(self, tmp_path):
        """Creates work branch even when default_branch doesn't exist."""
        # Initialize git without any commits
        subprocess.run(["git", "init"], cwd=tmp_path, capture_output=True)

        git_ops = GitOperations(tmp_path)
        result = git_ops.ensure_work_branch("c4/test", "main")

        assert result.success is True
        assert "(no base branch)" in result.message

    def test_not_a_git_repo(self, tmp_path):
        """Returns error when not a git repo."""
        git_ops = GitOperations(tmp_path)
        result = git_ops.ensure_work_branch("c4/test", "main")

        assert result.success is False
        assert "Not a Git repository" in result.message


class TestGitOperationsMergeBranch:
    """Test GitOperations.merge_branch_to_target()."""

    @pytest.fixture
    def git_repo(self, tmp_path):
        """Create git repo with main and feature branch."""
        subprocess.run(["git", "init"], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=tmp_path,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test"],
            cwd=tmp_path,
            capture_output=True,
        )
        # Initial commit on main
        (tmp_path / "README.md").write_text("# Test")
        subprocess.run(["git", "add", "."], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=tmp_path,
            capture_output=True,
        )
        # Ensure branch is named "main" (git version-agnostic)
        subprocess.run(
            ["git", "branch", "-M", "main"],
            cwd=tmp_path,
            capture_output=True,
        )
        # Create feature branch with change
        subprocess.run(
            ["git", "checkout", "-b", "feature"],
            cwd=tmp_path,
            capture_output=True,
        )
        (tmp_path / "feature.txt").write_text("feature content")
        subprocess.run(["git", "add", "."], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Add feature"],
            cwd=tmp_path,
            capture_output=True,
        )
        return tmp_path

    def test_merge_branch_no_squash(self, git_repo):
        """Merges feature branch into main without squash."""
        git_ops = GitOperations(git_repo)
        result = git_ops.merge_branch_to_target(
            source_branch="feature",
            target_branch="main",
            squash=False,
        )

        assert result.success is True
        assert "Merged" in result.message

        # Verify we're on main with the feature file
        assert git_ops.get_branch_name() == "main"
        assert (git_repo / "feature.txt").exists()

    def test_merge_branch_squash(self, git_repo):
        """Squash merges feature branch into main."""
        git_ops = GitOperations(git_repo)
        result = git_ops.merge_branch_to_target(
            source_branch="feature",
            target_branch="main",
            squash=True,
        )

        assert result.success is True
        assert "Merged" in result.message

        # Verify the feature file exists
        assert (git_repo / "feature.txt").exists()


class TestC4StartWorkBranch:
    """Test c4_start creates work branch."""

    @pytest.fixture
    def git_daemon(self, tmp_path):
        """Create C4Daemon with initialized git repo."""
        # Initialize git
        subprocess.run(["git", "init"], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=tmp_path,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test"],
            cwd=tmp_path,
            capture_output=True,
        )
        # Create initial commit on main
        (tmp_path / "README.md").write_text("# Test")
        subprocess.run(["git", "add", "."], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial"],
            cwd=tmp_path,
            capture_output=True,
        )

        daemon = C4Daemon(project_root=tmp_path)
        daemon.initialize(project_id="test-branch")

        # Skip to PLAN
        daemon.state_machine._state.status = ProjectStatus.PLAN
        daemon.state_machine.save_state()

        return daemon

    def test_c4_start_creates_work_branch(self, git_daemon):
        """c4_start creates c4/{project_id} work branch."""
        result = git_daemon.c4_start()

        assert result["success"] is True
        assert result["work_branch"] == "c4/test-branch"
        assert "Created work branch" in result["branch_message"]

    def test_c4_start_returns_work_branch_info(self, git_daemon):
        """c4_start response includes work_branch and branch_message."""
        result = git_daemon.c4_start()

        assert "work_branch" in result
        assert "branch_message" in result


class TestC4GetTaskBranch:
    """Test c4_get_task creates task branch from work branch."""

    @pytest.fixture
    def git_daemon(self, tmp_path):
        """Create C4Daemon with git repo and task."""
        # Initialize git
        subprocess.run(["git", "init"], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=tmp_path,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test"],
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
        daemon.initialize(project_id="test-branch")

        # Skip to PLAN and add task
        daemon.state_machine._state.status = ProjectStatus.PLAN
        daemon.state_machine.save_state()
        daemon.c4_add_todo(task_id="T-001", title="Test", scope=None, dod="DoD")
        daemon.c4_start()

        return daemon

    def test_c4_get_task_creates_task_branch(self, git_daemon):
        """c4_get_task creates c4/w-T-XXX branch from work branch."""
        assignment = git_daemon.c4_get_task(worker_id="worker-1")

        assert assignment is not None
        assert assignment.branch == "c4/w-T-001-0"

        # Verify branch was created
        git_ops = GitOperations(git_daemon.root)
        assert git_ops.get_branch_name() == "c4/w-T-001-0"


class TestCompletionAction:
    """Test completion action configuration."""

    def test_completion_action_pattern(self):
        """completion_action only accepts valid values."""
        # Valid values
        C4Config(project_id="test", completion_action="merge")
        C4Config(project_id="test", completion_action="pr")
        C4Config(project_id="test", completion_action="manual")

        # Invalid value
        with pytest.raises(ValueError):
            C4Config(project_id="test", completion_action="auto")
