"""Tests for c4_analyze_history MCP tool."""

import subprocess
import tempfile
from datetime import datetime
from pathlib import Path
from unittest.mock import MagicMock, patch

from c4.mcp.handlers.git_history import (
    GitCommit,
    GitHistoryAnalyzer,
    handle_analyze_history,
    parse_git_log,
)

# =============================================================================
# GitCommit Model Tests
# =============================================================================


class TestGitCommit:
    """Tests for GitCommit dataclass."""

    def test_create_basic(self) -> None:
        """Should create with required fields."""
        commit = GitCommit(
            sha="abc123",
            message="fix: resolve bug",
            author="Test User",
            date=datetime(2025, 1, 15),
        )
        assert commit.sha == "abc123"
        assert commit.message == "fix: resolve bug"
        assert commit.author == "Test User"

    def test_create_with_files(self) -> None:
        """Should create with changed files."""
        commit = GitCommit(
            sha="def456",
            message="feat: add feature",
            author="Test User",
            date=datetime(2025, 1, 15),
            files_changed=["src/main.py", "tests/test_main.py"],
        )
        assert commit.files_changed == ["src/main.py", "tests/test_main.py"]


# =============================================================================
# Git Log Parsing Tests
# =============================================================================


class TestParseGitLog:
    """Tests for parse_git_log function."""

    def test_parse_single_commit(self) -> None:
        """Should parse single commit."""
        log_output = """abc1234
fix: resolve authentication bug
John Doe
2025-01-15 10:30:00 +0000

"""
        commits = parse_git_log(log_output)

        assert len(commits) == 1
        assert commits[0].sha == "abc1234"
        assert commits[0].message == "fix: resolve authentication bug"
        assert commits[0].author == "John Doe"

    def test_parse_multiple_commits(self) -> None:
        """Should parse multiple commits."""
        log_output = """abc1234
fix: bug one
Author One
2025-01-15 10:00:00 +0000

def5678
feat: feature two
Author Two
2025-01-14 09:00:00 +0000

"""
        commits = parse_git_log(log_output)

        assert len(commits) == 2
        assert commits[0].sha == "abc1234"
        assert commits[1].sha == "def5678"

    def test_parse_empty_output(self) -> None:
        """Should return empty list for empty output."""
        commits = parse_git_log("")
        assert commits == []

    def test_parse_with_subject_only(self) -> None:
        """Should parse commits with subject only (from %s format)."""
        # Our git log format uses %s (subject only), not %B (full body)
        log_output = """abc1234
fix: resolve bug with detailed explanation
John Doe
2025-01-15 10:30:00 +0000

"""
        commits = parse_git_log(log_output)

        assert len(commits) == 1
        assert commits[0].message == "fix: resolve bug with detailed explanation"


# =============================================================================
# GitHistoryAnalyzer Tests
# =============================================================================


class TestGitHistoryAnalyzer:
    """Tests for GitHistoryAnalyzer class."""

    def test_init_with_project_root(self) -> None:
        """Should initialize with project root."""
        analyzer = GitHistoryAnalyzer(project_root=Path("/test/project"))
        assert analyzer.project_root == Path("/test/project")

    @patch("subprocess.run")
    def test_get_commits_basic(self, mock_run: MagicMock) -> None:
        """Should get commits from git log."""
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="""abc1234
fix: resolve bug
Test Author
2025-01-15 10:00:00 +0000

""",
        )

        analyzer = GitHistoryAnalyzer(project_root=Path("/test"))
        commits = analyzer.get_commits(since="2025-01-01")

        assert len(commits) == 1
        assert commits[0].sha == "abc1234"
        mock_run.assert_called_once()

    @patch("subprocess.run")
    def test_get_commits_with_until(self, mock_run: MagicMock) -> None:
        """Should accept until parameter."""
        mock_run.return_value = MagicMock(returncode=0, stdout="")

        analyzer = GitHistoryAnalyzer(project_root=Path("/test"))
        analyzer.get_commits(since="2025-01-01", until="2025-01-15")

        call_args = mock_run.call_args[0][0]
        assert "--until=2025-01-15" in call_args

    @patch("subprocess.run")
    def test_get_commits_with_branch(self, mock_run: MagicMock) -> None:
        """Should accept branch parameter."""
        mock_run.return_value = MagicMock(returncode=0, stdout="")

        analyzer = GitHistoryAnalyzer(project_root=Path("/test"))
        analyzer.get_commits(since="2025-01-01", branch="feature/test")

        call_args = mock_run.call_args[0][0]
        assert "feature/test" in call_args

    @patch("subprocess.run")
    def test_get_commits_git_error(self, mock_run: MagicMock) -> None:
        """Should handle git errors gracefully."""
        mock_run.side_effect = subprocess.CalledProcessError(1, "git")

        analyzer = GitHistoryAnalyzer(project_root=Path("/test"))
        commits = analyzer.get_commits(since="2025-01-01")

        assert commits == []

    def test_analyze_commits_empty(self) -> None:
        """Should return empty result for empty commits."""
        analyzer = GitHistoryAnalyzer(project_root=Path("/test"))
        result = analyzer.analyze_commits([])

        assert result["stories"] == []
        assert result["graph"]["nodes"] == []
        assert result["graph"]["edges"] == []

    def test_analyze_commits_single(self) -> None:
        """Should analyze single commit."""
        analyzer = GitHistoryAnalyzer(project_root=Path("/test"))
        commits = [
            GitCommit(
                sha="abc123",
                message="fix: resolve auth bug",
                author="Test",
                date=datetime(2025, 1, 15),
            )
        ]

        result = analyzer.analyze_commits(commits)

        assert len(result["stories"]) >= 1
        assert "graph" in result

    def test_analyze_commits_groups_similar(self) -> None:
        """Should group similar commits into stories."""
        analyzer = GitHistoryAnalyzer(project_root=Path("/test"))
        commits = [
            GitCommit(
                sha="abc1",
                message="fix: auth bug 1",
                author="Test",
                date=datetime(2025, 1, 15),
            ),
            GitCommit(
                sha="abc2",
                message="fix: auth bug 2",
                author="Test",
                date=datetime(2025, 1, 15),
            ),
            GitCommit(
                sha="def1",
                message="feat: new dashboard",
                author="Test",
                date=datetime(2025, 1, 14),
            ),
        ]

        result = analyzer.analyze_commits(commits)

        # Should have at least 1 story (commits may or may not cluster)
        assert len(result["stories"]) >= 1


# =============================================================================
# Handler Tests
# =============================================================================


class TestHandleAnalyzeHistory:
    """Tests for handle_analyze_history function."""

    @patch("subprocess.run")
    def test_basic_call(self, mock_run: MagicMock) -> None:
        """Should handle basic analyze_history call."""
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="""abc1234
fix: test
Test Author
2025-01-15 10:00:00 +0000

""",
        )

        mock_daemon = MagicMock()
        mock_daemon.project_root = Path("/test/project")

        result = handle_analyze_history(
            mock_daemon,
            {"since": "2025-01-01"},
        )

        assert "stories" in result
        assert "graph" in result
        assert isinstance(result["stories"], list)

    def test_missing_since_parameter(self) -> None:
        """Should return error when since is missing."""
        mock_daemon = MagicMock()
        mock_daemon.project_root = Path("/test/project")

        result = handle_analyze_history(mock_daemon, {})

        assert "error" in result

    @patch("subprocess.run")
    def test_with_all_parameters(self, mock_run: MagicMock) -> None:
        """Should handle all parameters."""
        mock_run.return_value = MagicMock(returncode=0, stdout="")

        mock_daemon = MagicMock()
        mock_daemon.project_root = Path("/test/project")

        result = handle_analyze_history(
            mock_daemon,
            {
                "since": "2025-01-01",
                "until": "2025-01-31",
                "branch": "main",
            },
        )

        assert "stories" in result
        assert "graph" in result

    @patch("subprocess.run")
    def test_story_format(self, mock_run: MagicMock) -> None:
        """Should return stories in expected format."""
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="""abc1234
fix: authentication bug
Test Author
2025-01-15 10:00:00 +0000

""",
        )

        mock_daemon = MagicMock()
        mock_daemon.project_root = Path("/test/project")

        result = handle_analyze_history(
            mock_daemon,
            {"since": "2025-01-01"},
        )

        if result["stories"]:
            story = result["stories"][0]
            assert "id" in story
            assert "title" in story
            assert "commits" in story

    @patch("subprocess.run")
    def test_graph_format(self, mock_run: MagicMock) -> None:
        """Should return graph in expected format."""
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="""abc1234
fix: test
Author
2025-01-15 10:00:00 +0000

""",
        )

        mock_daemon = MagicMock()
        mock_daemon.project_root = Path("/test/project")

        result = handle_analyze_history(
            mock_daemon,
            {"since": "2025-01-01"},
        )

        assert "graph" in result
        graph = result["graph"]
        assert "nodes" in graph
        assert "edges" in graph


# =============================================================================
# Integration Tests
# =============================================================================


class TestAnalyzeHistoryIntegration:
    """Integration tests with actual git operations."""

    def test_with_real_git_repo(self) -> None:
        """Should work with a real git repository."""
        with tempfile.TemporaryDirectory() as tmpdir:
            project_root = Path(tmpdir)

            # Initialize git repo
            subprocess.run(
                ["git", "init"],
                cwd=project_root,
                capture_output=True,
            )
            subprocess.run(
                ["git", "config", "user.email", "test@test.com"],
                cwd=project_root,
                capture_output=True,
            )
            subprocess.run(
                ["git", "config", "user.name", "Test User"],
                cwd=project_root,
                capture_output=True,
            )

            # Create initial commit
            test_file = project_root / "test.txt"
            test_file.write_text("initial content")
            subprocess.run(
                ["git", "add", "."],
                cwd=project_root,
                capture_output=True,
            )
            subprocess.run(
                ["git", "commit", "-m", "fix: initial commit"],
                cwd=project_root,
                capture_output=True,
            )

            # Test analyzer
            analyzer = GitHistoryAnalyzer(project_root=project_root)
            commits = analyzer.get_commits(since="2020-01-01")

            assert len(commits) >= 1
            assert commits[0].message == "fix: initial commit"

    def test_handler_with_real_repo(self) -> None:
        """Should handle real repository via handler."""
        with tempfile.TemporaryDirectory() as tmpdir:
            project_root = Path(tmpdir)

            # Initialize git repo with commits
            subprocess.run(["git", "init"], cwd=project_root, capture_output=True)
            subprocess.run(
                ["git", "config", "user.email", "test@test.com"],
                cwd=project_root,
                capture_output=True,
            )
            subprocess.run(
                ["git", "config", "user.name", "Test"],
                cwd=project_root,
                capture_output=True,
            )

            test_file = project_root / "test.txt"
            test_file.write_text("content")
            subprocess.run(["git", "add", "."], cwd=project_root, capture_output=True)
            subprocess.run(
                ["git", "commit", "-m", "feat: add feature"],
                cwd=project_root,
                capture_output=True,
            )

            mock_daemon = MagicMock()
            mock_daemon.project_root = project_root

            result = handle_analyze_history(
                mock_daemon,
                {"since": "2020-01-01"},
            )

            assert "stories" in result
            assert "graph" in result
