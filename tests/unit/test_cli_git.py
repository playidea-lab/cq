"""Tests for CLI Git automation features."""

import subprocess
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest


class TestInitGitRepo:
    """Test _init_git_repo function."""

    @pytest.fixture
    def temp_project(self, tmp_path: Path) -> Path:
        """Create a temporary project directory."""
        project = tmp_path / "test_project"
        project.mkdir()
        return project

    def test_git_init_creates_repo(self, temp_project: Path) -> None:
        """Test that git init is called for new projects."""
        from c4.cli import _init_git_repo

        result = _init_git_repo(temp_project)

        assert result["git_init"] is True
        assert (temp_project / ".git").exists()

    def test_git_init_skips_existing_repo(self, temp_project: Path) -> None:
        """Test that git init is skipped if .git exists."""
        from c4.cli import _init_git_repo

        # Initialize git first
        subprocess.run(["git", "init"], cwd=temp_project, capture_output=True)
        
        result = _init_git_repo(temp_project)

        assert result["git_init"] is True

    def test_gitignore_created(self, temp_project: Path) -> None:
        """Test that .gitignore is created with C4 patterns."""
        from c4.cli import _init_git_repo

        result = _init_git_repo(temp_project)

        assert result["gitignore"] is True
        gitignore = temp_project / ".gitignore"
        assert gitignore.exists()
        content = gitignore.read_text()
        assert ".c4/locks/" in content
        assert ".c4/daemon.pid" in content

    def test_gitignore_appends_to_existing(self, temp_project: Path) -> None:
        """Test that C4 patterns are appended to existing .gitignore."""
        from c4.cli import _init_git_repo

        # Create existing .gitignore
        gitignore = temp_project / ".gitignore"
        gitignore.write_text("*.log\n__pycache__/\n")

        result = _init_git_repo(temp_project)

        assert result["gitignore"] is True
        content = gitignore.read_text()
        assert "*.log" in content  # Original content preserved
        assert ".c4/locks/" in content  # C4 patterns added

    def test_gitignore_skips_if_c4_patterns_exist(self, temp_project: Path) -> None:
        """Test that C4 patterns are not duplicated."""
        from c4.cli import _init_git_repo

        # Create .gitignore with C4 patterns already
        gitignore = temp_project / ".gitignore"
        gitignore.write_text(".c4/locks/\n.c4/daemon.pid\n")
        original_content = gitignore.read_text()

        result = _init_git_repo(temp_project)

        assert result["gitignore"] is True
        # Content should not have duplicates
        content = gitignore.read_text()
        assert content.count(".c4/locks/") == 1

    def test_initial_commit_created(self, temp_project: Path) -> None:
        """Test that initial commit is created for new repos."""
        from c4.cli import _init_git_repo

        result = _init_git_repo(temp_project)

        assert result["initial_commit"] is True
        
        # Verify commit exists
        log_result = subprocess.run(
            ["git", "log", "--oneline", "-1"],
            cwd=temp_project,
            capture_output=True,
            text=True,
        )
        assert log_result.returncode == 0
        assert "Initial commit" in log_result.stdout or "C4" in log_result.stdout

    def test_skips_commit_if_commits_exist(self, temp_project: Path) -> None:
        """Test that initial commit is skipped if repo has commits."""
        from c4.cli import _init_git_repo

        # Initialize and make a commit
        subprocess.run(["git", "init"], cwd=temp_project, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=temp_project,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test"],
            cwd=temp_project,
            capture_output=True,
        )
        (temp_project / "README.md").write_text("# Test")
        subprocess.run(["git", "add", "."], cwd=temp_project, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Existing commit"],
            cwd=temp_project,
            capture_output=True,
        )

        result = _init_git_repo(temp_project)

        # Should report success but not create new initial commit
        assert result["initial_commit"] is True

    @patch("c4.cli.subprocess.run")
    def test_handles_git_not_found(
        self, mock_run: MagicMock, temp_project: Path
    ) -> None:
        """Test graceful handling when git is not installed."""
        from c4.cli import _init_git_repo

        mock_run.side_effect = FileNotFoundError("git not found")

        result = _init_git_repo(temp_project)

        assert result["git_init"] is False
        assert result["gitignore"] is False
        assert result["initial_commit"] is False


class TestGitignorePatterns:
    """Test .gitignore content."""

    @pytest.fixture
    def temp_project(self, tmp_path: Path) -> Path:
        """Create a temporary project directory."""
        project = tmp_path / "test_project"
        project.mkdir()
        return project

    def test_c4_patterns_present(self, temp_project: Path) -> None:
        """Test that all required C4 patterns are in .gitignore."""
        from c4.cli import _init_git_repo

        _init_git_repo(temp_project)
        content = (temp_project / ".gitignore").read_text()

        required_patterns = [
            ".c4/locks/",
            ".c4/daemon.pid",
            ".c4/daemon.log",
            ".c4/workers/",
        ]
        for pattern in required_patterns:
            assert pattern in content, f"Missing pattern: {pattern}"

    def test_common_patterns_present(self, temp_project: Path) -> None:
        """Test that common ignore patterns are included."""
        from c4.cli import _init_git_repo

        _init_git_repo(temp_project)
        content = (temp_project / ".gitignore").read_text()

        common_patterns = [
            "__pycache__/",
            "*.pyc",
            ".venv/",
            ".env",
        ]
        for pattern in common_patterns:
            assert pattern in content, f"Missing common pattern: {pattern}"
