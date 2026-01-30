"""Integration tests for git hooks in a real git repository."""

import json
import subprocess
from unittest.mock import patch

import pytest

from c4.git_hooks import POST_COMMIT_HOOK, install_hook


class TestPostCommitHookIntegration:
    """Integration tests for post-commit hook in a real git repo."""

    @pytest.fixture
    def git_repo(self, tmp_path):
        """Create a real git repository with C4 structure."""
        repo = tmp_path / "test_repo"
        repo.mkdir()

        # Initialize git repo
        subprocess.run(
            ["git", "init"],
            cwd=repo,
            capture_output=True,
            check=True,
        )

        # Configure git user for commits
        subprocess.run(
            ["git", "config", "user.email", "test@example.com"],
            cwd=repo,
            capture_output=True,
            check=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=repo,
            capture_output=True,
            check=True,
        )

        # Create C4 structure
        c4_dir = repo / ".c4"
        c4_dir.mkdir()
        (c4_dir / "config.yaml").write_text("project_id: test")
        (c4_dir / "state.json").write_text('{"status": "EXECUTE"}')

        return repo

    def test_hook_generates_event_on_commit(self, git_repo):
        """Hook should generate event file after a real commit."""
        # Install the post-commit hook
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            success, _ = install_hook("post-commit", POST_COMMIT_HOOK)
            assert success is True

        # Create a file and commit
        test_file = git_repo / "test.py"
        test_file.write_text("print('hello')")
        subprocess.run(
            ["git", "add", "test.py"],
            cwd=git_repo,
            capture_output=True,
            check=True,
        )
        subprocess.run(
            ["git", "commit", "-m", "[T-001-0] Add test file"],
            cwd=git_repo,
            capture_output=True,
            check=True,
        )

        # Check that event file was created
        events_dir = git_repo / ".c4" / "events"
        event_files = list(events_dir.glob("git-*.json"))
        assert len(event_files) == 1

        # Verify event content
        event = json.loads(event_files[0].read_text())
        assert event["type"] == "git_commit"
        assert len(event["sha"]) == 40  # Full SHA
        assert event["task_id"] == "T-001-0"
        assert "test.py" in event["files"]
        assert event["timestamp"]  # ISO format timestamp

    def test_hook_handles_commit_without_task_id(self, git_repo):
        """Hook should handle commits without task ID."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            install_hook("post-commit", POST_COMMIT_HOOK)

        # Create and commit a file without task ID
        test_file = git_repo / "README.md"
        test_file.write_text("# Test")
        subprocess.run(["git", "add", "README.md"], cwd=git_repo, check=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial commit"],
            cwd=git_repo,
            check=True,
            capture_output=True,
        )

        # Verify event file
        events_dir = git_repo / ".c4" / "events"
        event_files = list(events_dir.glob("git-*.json"))
        assert len(event_files) == 1

        event = json.loads(event_files[0].read_text())
        assert event["task_id"] is None

    def test_hook_handles_multiple_files(self, git_repo):
        """Hook should capture all changed files."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            install_hook("post-commit", POST_COMMIT_HOOK)

        # Create multiple files
        (git_repo / "file1.py").write_text("# file1")
        (git_repo / "file2.py").write_text("# file2")
        (git_repo / "file3.txt").write_text("file3")

        subprocess.run(["git", "add", "."], cwd=git_repo, check=True)
        subprocess.run(
            ["git", "commit", "-m", "[T-002-0] Add multiple files"],
            cwd=git_repo,
            check=True,
            capture_output=True,
        )

        events_dir = git_repo / ".c4" / "events"
        event_files = list(events_dir.glob("git-*.json"))
        event = json.loads(event_files[0].read_text())

        # All user files should be captured (C4 files also included)
        files = event["files"].split(",")
        assert "file1.py" in files
        assert "file2.py" in files
        assert "file3.txt" in files
        assert len(files) >= 3  # At least 3 user files (may include .c4/ files)

    def test_hook_generates_unique_event_per_commit(self, git_repo):
        """Each commit should generate a unique event file."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            install_hook("post-commit", POST_COMMIT_HOOK)

        # First commit
        (git_repo / "a.py").write_text("# a")
        subprocess.run(["git", "add", "."], cwd=git_repo, check=True)
        subprocess.run(
            ["git", "commit", "-m", "First"],
            cwd=git_repo,
            check=True,
            capture_output=True,
        )

        # Second commit
        (git_repo / "b.py").write_text("# b")
        subprocess.run(["git", "add", "."], cwd=git_repo, check=True)
        subprocess.run(
            ["git", "commit", "-m", "Second"],
            cwd=git_repo,
            check=True,
            capture_output=True,
        )

        # Should have 2 event files
        events_dir = git_repo / ".c4" / "events"
        event_files = list(events_dir.glob("git-*.json"))
        assert len(event_files) == 2

        # Each should have different SHA
        shas = {json.loads(f.read_text())["sha"] for f in event_files}
        assert len(shas) == 2

    def test_hook_skips_non_c4_project(self, tmp_path):
        """Hook should skip when not in a C4 project."""
        repo = tmp_path / "non_c4_repo"
        repo.mkdir()

        # Initialize git repo without C4 structure
        subprocess.run(["git", "init"], cwd=repo, check=True, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@example.com"],
            cwd=repo,
            check=True,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=repo,
            check=True,
            capture_output=True,
        )

        with patch("c4.git_hooks.Path.cwd", return_value=repo):
            install_hook("post-commit", POST_COMMIT_HOOK)

        # Commit something
        (repo / "file.txt").write_text("test")
        subprocess.run(["git", "add", "."], cwd=repo, check=True)
        subprocess.run(
            ["git", "commit", "-m", "Test"],
            cwd=repo,
            check=True,
            capture_output=True,
        )

        # No events directory should be created
        events_dir = repo / ".c4" / "events"
        assert not events_dir.exists()
