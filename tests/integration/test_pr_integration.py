"""Integration tests for PR creation on project completion."""

import subprocess
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
import yaml

from c4.daemon.pr_manager import PRManager
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

        # Create work branch (simulating worktree base_branch)
        subprocess.run(
            ["git", "checkout", "-b", "work"],
            cwd=repo,
            capture_output=True,
        )

        # Create .c4 directory
        (repo / ".c4").mkdir()

        yield repo


def create_config_file(
    repo: Path,
    worktree_enabled: bool = True,
    base_branch: str = "work",
    completion_action: str = "pr",
) -> None:
    """Create config.yaml with worktree settings."""
    config = {
        "project_id": "test-project",
        "default_branch": "main",
        "worktree": {
            "enabled": worktree_enabled,
            "base_branch": base_branch,
            "completion_action": completion_action,
        },
    }
    (repo / ".c4" / "config.yaml").write_text(yaml.dump(config))


class TestCompletionActionPR:
    """Integration tests for completion_action=pr."""

    def test_pr_created_when_base_branch_not_main(self, temp_git_repo):
        """Should create PR when worktree base_branch != main."""
        # Setup config with base_branch=work (not main)
        create_config_file(
            temp_git_repo,
            worktree_enabled=True,
            base_branch="work",
            completion_action="pr",
        )

        daemon = C4Daemon(project_root=temp_git_repo)

        # Mock PRManager methods
        mock_pr_result = MagicMock()
        mock_pr_result.success = True
        mock_pr_result.pr_url = "https://github.com/owner/repo/pull/42"
        mock_pr_result.pr_number = 42
        mock_pr_result.message = "Created PR #42"

        with patch.object(PRManager, "is_gh_available", return_value=True):
            with patch.object(
                PRManager, "create_or_update_pr", return_value=mock_pr_result
            ):
                with patch.object(
                    PRManager, "get_completed_tasks_summary", return_value="## Tasks"
                ):
                    # Mock git push
                    with patch.object(
                        daemon, "_get_completed_tasks", return_value=[]
                    ):
                        from c4.daemon import GitOperations

                        mock_run_result = MagicMock()
                        mock_run_result.returncode = 0
                        with patch.object(
                            GitOperations, "_run_git", return_value=mock_run_result
                        ):
                            result = daemon._perform_completion_action()

        assert result is not None
        assert result["action"] == "pr"
        assert result["status"] == "success"
        assert result["pr_url"] == "https://github.com/owner/repo/pull/42"
        assert result["pr_number"] == 42

    def test_pr_skipped_when_base_branch_is_main(self, temp_git_repo):
        """Should skip PR when worktree base_branch == main."""
        # Setup config with base_branch=main
        create_config_file(
            temp_git_repo,
            worktree_enabled=True,
            base_branch="main",  # Same as default_branch
            completion_action="pr",
        )

        daemon = C4Daemon(project_root=temp_git_repo)
        result = daemon._perform_completion_action()

        assert result is not None
        assert result["action"] == "pr"
        assert result["status"] == "skipped"
        assert "no PR needed" in result["message"]

    def test_pr_skipped_when_gh_not_available(self, temp_git_repo):
        """Should skip PR gracefully when gh CLI not installed."""
        create_config_file(
            temp_git_repo,
            worktree_enabled=True,
            base_branch="work",
            completion_action="pr",
        )

        daemon = C4Daemon(project_root=temp_git_repo)

        with patch.object(PRManager, "is_gh_available", return_value=False):
            result = daemon._perform_completion_action()

        assert result is not None
        assert result["action"] == "pr"
        assert result["status"] == "skipped"
        assert "gh CLI" in result["message"]

    def test_c4state_has_completion_result_field(self, temp_git_repo):
        """Should have completion_result field in C4State for storing PR result."""
        from c4.models import C4State, ProjectStatus

        # Verify C4State has completion_result field
        state = C4State(project_id="test-project", status=ProjectStatus.EXECUTE)
        assert hasattr(state, "completion_result")
        assert state.completion_result is None

        # Verify it can be set
        state.completion_result = {
            "action": "pr",
            "status": "success",
            "pr_url": "https://github.com/owner/repo/pull/123",
            "pr_number": 123,
        }
        assert state.completion_result["action"] == "pr"
        assert state.completion_result["pr_number"] == 123


class TestCompletionActionMerge:
    """Test that merge action still works."""

    def test_merge_action_unchanged(self, temp_git_repo):
        """Should use merge when completion_action=merge."""
        config = {
            "project_id": "test-project",
            "default_branch": "main",
            "completion_action": "merge",
        }
        (temp_git_repo / ".c4" / "config.yaml").write_text(yaml.dump(config))

        daemon = C4Daemon(project_root=temp_git_repo)

        # completion_action is "merge" at top level, not "pr"
        # Should go through merge logic (not PR logic)
        from c4.daemon import GitOperations

        mock_merge_result = MagicMock()
        mock_merge_result.success = True
        mock_merge_result.message = "Merged successfully"

        mock_run_result = MagicMock()
        mock_run_result.returncode = 0

        with patch.object(
            GitOperations, "merge_branch_to_target", return_value=mock_merge_result
        ):
            with patch.object(GitOperations, "_run_git", return_value=mock_run_result):
                result = daemon._perform_completion_action()

        assert result is not None
        assert result["action"] == "merge"
        assert result["status"] == "success"
