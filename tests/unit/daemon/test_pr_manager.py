"""Tests for PRManager."""

import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.daemon.pr_manager import PRManager


@pytest.fixture
def temp_dir():
    """Create a temporary directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def pr_manager(temp_dir):
    """Create PRManager instance."""
    return PRManager(temp_dir)


class TestIsGhAvailable:
    """Tests for is_gh_available method."""

    def test_gh_available_when_installed(self, pr_manager):
        """Should return True when gh is installed."""
        with patch("shutil.which", return_value="/usr/local/bin/gh"):
            result = pr_manager.is_gh_available()
            assert result is True

    def test_gh_not_available_when_not_installed(self, pr_manager):
        """Should return False when gh is not installed."""
        with patch("shutil.which", return_value=None):
            # Reset cached value
            pr_manager._gh_available = None
            result = pr_manager.is_gh_available()
            assert result is False


class TestGetCompletedTasksSummary:
    """Tests for get_completed_tasks_summary method."""

    def test_empty_tasks_list(self, pr_manager):
        """Should return no tasks message for empty list."""
        result = pr_manager.get_completed_tasks_summary([])
        assert "No tasks completed" in result

    def test_formats_tasks_as_markdown(self, pr_manager):
        """Should format tasks as markdown list."""
        tasks = [
            {"task_id": "T-001-0", "title": "Implement feature A"},
            {"task_id": "T-002-0", "title": "Implement feature B"},
        ]
        result = pr_manager.get_completed_tasks_summary(tasks)

        assert "## Completed Tasks" in result
        assert "**T-001-0**" in result
        assert "Implement feature A" in result
        assert "**T-002-0**" in result
        assert "Implement feature B" in result

    def test_includes_dod_when_requested(self, pr_manager):
        """Should include DoD when include_dod=True."""
        tasks = [
            {
                "task_id": "T-001-0",
                "title": "Implement feature",
                "dod": "- Create file\n- Add tests",
            },
        ]
        result = pr_manager.get_completed_tasks_summary(tasks, include_dod=True)

        assert "Create file" in result
        assert "Add tests" in result


class TestCreateOrUpdatePR:
    """Tests for create_or_update_pr method."""

    def test_returns_error_when_gh_not_available(self, pr_manager):
        """Should return error when gh CLI is not installed."""
        with patch.object(pr_manager, "is_gh_available", return_value=False):
            result = pr_manager.create_or_update_pr(
                branch="feature-branch",
                title="Test PR",
                body="Test body",
            )

            assert result.success is False
            assert "gh CLI is not installed" in result.message

    def test_creates_new_pr_when_none_exists(self, pr_manager):
        """Should create new PR when no existing PR."""
        mock_run_result = MagicMock()
        mock_run_result.returncode = 0
        mock_run_result.stdout = "https://github.com/owner/repo/pull/123\n"

        with patch.object(pr_manager, "is_gh_available", return_value=True):
            with patch.object(pr_manager, "get_existing_pr", return_value=None):
                with patch.object(pr_manager, "_run_gh", return_value=mock_run_result):
                    result = pr_manager.create_or_update_pr(
                        branch="feature-branch",
                        title="Test PR",
                        body="Test body",
                    )

                    assert result.success is True
                    assert result.pr_url == "https://github.com/owner/repo/pull/123"
                    assert result.pr_number == 123

    def test_updates_existing_pr(self, pr_manager):
        """Should update existing PR instead of creating new one."""
        existing_pr = {
            "number": 42,
            "url": "https://github.com/owner/repo/pull/42",
            "state": "open",
        }

        mock_run_result = MagicMock()
        mock_run_result.returncode = 0

        with patch.object(pr_manager, "is_gh_available", return_value=True):
            with patch.object(pr_manager, "get_existing_pr", return_value=existing_pr):
                with patch.object(pr_manager, "_run_gh", return_value=mock_run_result):
                    result = pr_manager.create_or_update_pr(
                        branch="feature-branch",
                        title="Updated PR",
                        body="Updated body",
                    )

                    assert result.success is True
                    assert "Updated PR #42" in result.message
                    assert result.pr_number == 42

    def test_handles_pr_creation_failure(self, pr_manager):
        """Should handle PR creation failure gracefully."""
        mock_run_result = MagicMock()
        mock_run_result.returncode = 1
        mock_run_result.stderr = "Error: branch already has a PR"

        with patch.object(pr_manager, "is_gh_available", return_value=True):
            with patch.object(pr_manager, "get_existing_pr", return_value=None):
                with patch.object(pr_manager, "_run_gh", return_value=mock_run_result):
                    result = pr_manager.create_or_update_pr(
                        branch="feature-branch",
                        title="Test PR",
                        body="Test body",
                    )

                    assert result.success is False
                    assert "Failed to create PR" in result.message


class TestGetExistingPR:
    """Tests for get_existing_pr method."""

    def test_returns_none_when_gh_not_available(self, pr_manager):
        """Should return None when gh CLI is not available."""
        with patch.object(pr_manager, "is_gh_available", return_value=False):
            result = pr_manager.get_existing_pr("feature-branch")
            assert result is None

    def test_returns_none_when_no_pr_exists(self, pr_manager):
        """Should return None when no PR exists for branch."""
        mock_run_result = MagicMock()
        mock_run_result.returncode = 1

        with patch.object(pr_manager, "is_gh_available", return_value=True):
            with patch.object(pr_manager, "_run_gh", return_value=mock_run_result):
                result = pr_manager.get_existing_pr("feature-branch")
                assert result is None

    def test_returns_pr_info_when_exists(self, pr_manager):
        """Should return PR info when PR exists."""
        mock_run_result = MagicMock()
        mock_run_result.returncode = 0
        mock_run_result.stdout = '{"number": 42, "url": "https://github.com/owner/repo/pull/42", "state": "open"}'

        with patch.object(pr_manager, "is_gh_available", return_value=True):
            with patch.object(pr_manager, "_run_gh", return_value=mock_run_result):
                result = pr_manager.get_existing_pr("feature-branch")

                assert result is not None
                assert result["number"] == 42
                assert result["state"] == "open"
