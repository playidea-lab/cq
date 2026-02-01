"""Unit tests for CommitAnalyzer."""

from unittest.mock import patch

import pytest

from c4.daemon.commit_analyzer import CommitAnalyzer, CommitInfo
from c4.models import Task, TaskStatus


@pytest.fixture
def analyzer(tmp_path):
    """Create a CommitAnalyzer with a temporary project root."""
    return CommitAnalyzer(tmp_path)


@pytest.fixture
def sample_tasks():
    """Create sample C4 tasks for matching."""
    return {
        "T-001-0": Task(
            id="T-001-0",
            title="Implement user authentication",
            scope="src/auth",
            status=TaskStatus.PENDING,
            dod="- [ ] Implement login endpoint",
        ),
        "T-002-0": Task(
            id="T-002-0",
            title="Add database migration",
            scope="db/migrations",
            status=TaskStatus.PENDING,
            dod="- [ ] Add user table migration",
        ),
        "T-003-0": Task(
            id="T-003-0",
            title="Fix login button styling",
            scope="src/components",
            status=TaskStatus.DONE,
            dod="- [x] Fix button color",
        ),
    }


class TestExtractTaskIds:
    """Test task ID extraction from commit messages."""

    def test_extract_simple_task_id(self, analyzer):
        """Test extracting T-XXX-N format."""
        message = "[T-001-0] feat: implement login"
        ids = analyzer.extract_task_ids_from_message(message)
        assert "T-001-0" == ids[0]

    def test_extract_multiple_task_ids(self, analyzer):
        """Test extracting multiple task IDs."""
        message = "[T-001-0] and [T-002-0] refactor auth"
        ids = analyzer.extract_task_ids_from_message(message)
        assert len(ids) == 2
        assert "T-001-0" in ids
        assert "T-002-0" in ids

    def test_extract_review_task_id(self, analyzer):
        """Test extracting R-XXX-N format."""
        message = "[R-001-0] code review for auth"
        ids = analyzer.extract_task_ids_from_message(message)
        assert "R-001-0" in ids

    def test_extract_inline_task_id(self, analyzer):
        """Test extracting task ID without brackets."""
        message = "fix: T-001-0 login issue"
        ids = analyzer.extract_task_ids_from_message(message)
        assert "T-001-0" in ids

    def test_no_task_id(self, analyzer):
        """Test message without task ID."""
        message = "feat: add new feature"
        ids = analyzer.extract_task_ids_from_message(message)
        assert len(ids) == 0


class TestPathMatching:
    """Test file path to scope matching."""

    def test_exact_match(self, analyzer):
        """Test exact path match."""
        assert analyzer._path_matches_scope("src/auth/login.py", ["src", "auth"])

    def test_nested_match(self, analyzer):
        """Test nested file within scope."""
        assert analyzer._path_matches_scope("src/auth/utils/helper.py", ["src", "auth"])

    def test_no_match(self, analyzer):
        """Test path outside scope."""
        assert not analyzer._path_matches_scope("tests/test_auth.py", ["src", "auth"])

    def test_partial_match_fails(self, analyzer):
        """Test partial directory name doesn't match."""
        assert not analyzer._path_matches_scope("src/authorization/user.py", ["src", "auth"])


class TestMatchCommitToTasks:
    """Test commit to task matching logic."""

    def test_match_by_task_id_in_message(self, analyzer, sample_tasks):
        """Test matching when task ID is in commit message."""
        commit = CommitInfo(
            sha="abc123",
            message="[T-001-0] implement login endpoint",
            files_added=["src/api/login.py"],
            files_modified=[],
            files_deleted=[],
        )
        matches = analyzer.match_commit_to_tasks(commit, sample_tasks)

        assert len(matches) == 1
        assert matches[0].task_id == "T-001-0"
        assert matches[0].confidence == 1.0

    def test_match_by_scope(self, analyzer, sample_tasks):
        """Test matching by file scope."""
        commit = CommitInfo(
            sha="abc123",
            message="feat: add password hashing",
            files_added=[],
            files_modified=["src/auth/password.py"],
            files_deleted=[],
        )
        matches = analyzer.match_commit_to_tasks(commit, sample_tasks)

        # Should match T-001-0 by scope (src/auth)
        scope_matches = [m for m in matches if m.task_id == "T-001-0"]
        assert len(scope_matches) == 1
        assert scope_matches[0].confidence == 0.7

    def test_skip_done_tasks(self, analyzer, sample_tasks):
        """Test that already done tasks are not matched."""
        commit = CommitInfo(
            sha="abc123",
            message="fix: button color",
            files_added=[],
            files_modified=["src/components/Button.tsx"],
            files_deleted=[],
        )
        matches = analyzer.match_commit_to_tasks(commit, sample_tasks)

        # T-003-0 is done, should not be in matches
        task_ids = [m.task_id for m in matches]
        assert "T-003-0" not in task_ids

    def test_multiple_strategies(self, analyzer, sample_tasks):
        """Test that task ID match takes priority over scope match."""
        commit = CommitInfo(
            sha="abc123",
            message="[T-002-0] add user migration",
            files_added=["src/auth/user.py"],  # Matches T-001-0 scope
            files_modified=[],
            files_deleted=[],
        )
        matches = analyzer.match_commit_to_tasks(commit, sample_tasks)

        # Should match T-002-0 by message (high confidence)
        # and T-001-0 by scope (lower confidence)
        assert any(m.task_id == "T-002-0" and m.confidence == 1.0 for m in matches)
        assert any(m.task_id == "T-001-0" and m.confidence < 1.0 for m in matches)


class TestCommitInfo:
    """Test CommitInfo dataclass."""

    def test_all_files_property(self):
        """Test that all_files combines added, modified, and deleted."""
        commit = CommitInfo(
            sha="abc123",
            message="test",
            files_added=["a.py"],
            files_modified=["b.py"],
            files_deleted=["c.py"],
        )
        assert commit.all_files == ["a.py", "b.py", "c.py"]


class TestAnalyzeAndSuggest:
    """Test the high-level analyze_and_suggest method."""

    @patch.object(CommitAnalyzer, "get_commit_info")
    def test_filters_by_confidence(self, mock_get_commit, analyzer, sample_tasks):
        """Test that results are filtered by minimum confidence."""
        mock_get_commit.return_value = CommitInfo(
            sha="abc123",
            message="feat: some changes",
            files_added=["src/auth/login.py"],
            files_modified=[],
            files_deleted=[],
        )

        # With high threshold, should filter out low-confidence matches
        matches = analyzer.analyze_and_suggest("abc123", sample_tasks, min_confidence=0.9)
        assert all(m.confidence >= 0.9 for m in matches)

    @patch.object(CommitAnalyzer, "get_commit_info")
    def test_returns_empty_on_no_commit(self, mock_get_commit, analyzer, sample_tasks):
        """Test handling of missing commit."""
        mock_get_commit.return_value = None
        matches = analyzer.analyze_and_suggest("invalid", sample_tasks)
        assert matches == []
