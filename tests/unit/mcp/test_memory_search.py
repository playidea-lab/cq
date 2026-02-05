"""Tests for c4_search_memory MCP tool handler."""

import sqlite3
import tempfile
from datetime import datetime
from pathlib import Path
from unittest.mock import MagicMock, patch

from c4.mcp.handlers.memory import (
    _generate_search_hint,
    _get_memory_searcher,
    handle_search_memory,
)


def _create_test_db(db_path: Path) -> None:
    """Create test database with observations table."""
    conn = sqlite3.connect(db_path)
    conn.execute("""
        CREATE TABLE IF NOT EXISTS c4_observations (
            id TEXT PRIMARY KEY,
            project_id TEXT NOT NULL,
            source TEXT NOT NULL,
            content TEXT NOT NULL,
            importance INTEGER DEFAULT 5,
            tags TEXT,
            metadata TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    """)
    conn.commit()
    conn.close()


def _insert_observation(
    db_path: Path,
    obs_id: str,
    project_id: str,
    source: str,
    content: str,
    importance: int = 5,
    tags: str = "[]",
    created_at: datetime | None = None,
) -> None:
    """Insert a test observation."""
    if created_at is None:
        created_at = datetime.now()
    conn = sqlite3.connect(db_path)
    conn.execute(
        """
        INSERT INTO c4_observations
        (id, project_id, source, content, importance, tags, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        """,
        (obs_id, project_id, source, content, importance, tags, created_at.isoformat()),
    )
    conn.commit()
    conn.close()


class TestHandleSearchMemory:
    """Tests for handle_search_memory handler."""

    def test_query_required(self) -> None:
        """Should return error when query is missing."""
        result = handle_search_memory(None, {})
        assert "error" in result
        assert "query is required" in result["error"]

    def test_empty_query(self) -> None:
        """Should return error for empty query."""
        result = handle_search_memory(None, {"query": ""})
        assert "error" in result
        assert "query is required" in result["error"]

    def test_search_returns_results(self) -> None:
        """Should return matching results."""
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            c4_dir = root / ".c4"
            c4_dir.mkdir()
            db_path = c4_dir / "tasks.db"
            _create_test_db(db_path)

            # Insert test data
            _insert_observation(
                db_path, "obs-001", "test-project", "read_file",
                "User authentication and login flow implementation"
            )
            _insert_observation(
                db_path, "obs-002", "test-project", "read_file",
                "Database connection settings"
            )

            # Mock environment
            with patch.dict("os.environ", {"C4_PROJECT_ROOT": str(root)}):
                with patch("c4.mcp.handlers.memory._get_memory_searcher") as mock_searcher:
                    # Create mock results
                    mock_result = MagicMock()
                    mock_result.id = "obs-001"
                    mock_result.title = "read_file"
                    mock_result.preview = "User authentication..."
                    mock_result.tokens = 50
                    mock_result.score = 0.85
                    mock_result.source = "read_file"
                    mock_result.importance = 6
                    mock_result.created_at = datetime.now()

                    mock_searcher_instance = MagicMock()
                    mock_searcher_instance.search.return_value = [mock_result]
                    mock_searcher.return_value = mock_searcher_instance

                    result = handle_search_memory(None, {"query": "authentication"})

                    assert "error" not in result
                    assert "results" in result
                    assert len(result["results"]) == 1
                    assert result["results"][0]["id"] == "obs-001"
                    assert result["count"] == 1
                    assert "total_tokens_if_expanded" in result
                    assert "hint" in result

    def test_search_with_limit(self) -> None:
        """Should respect limit parameter."""
        with patch("c4.mcp.handlers.memory._get_memory_searcher") as mock_searcher:
            mock_searcher_instance = MagicMock()
            mock_searcher_instance.search.return_value = []
            mock_searcher.return_value = mock_searcher_instance

            handle_search_memory(None, {"query": "test", "limit": 5})

            mock_searcher_instance.search.assert_called_once()
            call_args = mock_searcher_instance.search.call_args
            assert call_args[1]["limit"] == 5

    def test_search_with_filters(self) -> None:
        """Should pass filters to searcher."""
        with patch("c4.mcp.handlers.memory._get_memory_searcher") as mock_searcher:
            mock_searcher_instance = MagicMock()
            mock_searcher_instance.search.return_value = []
            mock_searcher.return_value = mock_searcher_instance

            handle_search_memory(None, {
                "query": "test",
                "filters": {
                    "memory_type": "read_file",
                    "tags": ["important"],
                    "since": "2024-01-01T00:00:00",
                }
            })

            mock_searcher_instance.search.assert_called_once()
            call_args = mock_searcher_instance.search.call_args
            filters = call_args[1]["filters"]
            assert filters is not None
            assert filters.memory_type == "read_file"
            assert filters.tags == ["important"]
            assert filters.since == datetime(2024, 1, 1, 0, 0, 0)

    def test_invalid_since_datetime(self) -> None:
        """Should return error for invalid since datetime."""
        result = handle_search_memory(None, {
            "query": "test",
            "filters": {"since": "not-a-date"}
        })

        assert "error" in result
        assert "Invalid since datetime" in result["error"]

    def test_empty_results(self) -> None:
        """Should handle no results gracefully."""
        with patch("c4.mcp.handlers.memory._get_memory_searcher") as mock_searcher:
            mock_searcher_instance = MagicMock()
            mock_searcher_instance.search.return_value = []
            mock_searcher.return_value = mock_searcher_instance

            result = handle_search_memory(None, {"query": "nonexistent"})

            assert "error" not in result
            assert result["results"] == []
            assert result["count"] == 0
            assert result["total_tokens_if_expanded"] == 0
            assert "No results found" in result["hint"]

    def test_result_format(self) -> None:
        """Should format results correctly."""
        with patch("c4.mcp.handlers.memory._get_memory_searcher") as mock_searcher:
            mock_result = MagicMock()
            mock_result.id = "obs-123"
            mock_result.title = "Test Title"
            mock_result.preview = "Preview text..."
            mock_result.tokens = 100
            mock_result.score = 0.12345
            mock_result.source = "user_message"
            mock_result.importance = 8
            mock_result.created_at = datetime(2024, 6, 15, 12, 30, 0)

            mock_searcher_instance = MagicMock()
            mock_searcher_instance.search.return_value = [mock_result]
            mock_searcher.return_value = mock_searcher_instance

            result = handle_search_memory(None, {"query": "test"})

            assert len(result["results"]) == 1
            item = result["results"][0]
            assert item["id"] == "obs-123"
            assert item["title"] == "Test Title"
            assert item["preview"] == "Preview text..."
            assert item["content_tokens"] == 100
            assert item["score"] == 0.1235  # Rounded to 4 decimal places
            assert item["source"] == "user_message"
            assert item["importance"] == 8
            assert item["created_at"] == "2024-06-15T12:30:00"


class TestGenerateSearchHint:
    """Tests for _generate_search_hint helper."""

    def test_no_results_hint(self) -> None:
        """Should suggest different query when no results."""
        hint = _generate_search_hint(0, 0, 10)
        assert "No results found" in hint
        assert "different query" in hint

    def test_high_token_count_hint(self) -> None:
        """Should warn about context overload."""
        hint = _generate_search_hint(5, 3000, 10)
        assert "3000 tokens" in hint
        assert "context overload" in hint

    def test_limit_reached_hint(self) -> None:
        """Should note when limit was reached."""
        hint = _generate_search_hint(10, 500, 10)
        assert "limit reached" in hint
        assert "more specific query" in hint

    def test_normal_results_hint(self) -> None:
        """Should show summary for normal results."""
        hint = _generate_search_hint(3, 150, 10)
        assert "Found 3 results" in hint
        assert "150 tokens" in hint


class TestGetMemorySearcher:
    """Tests for _get_memory_searcher helper."""

    def test_uses_project_root_env(self) -> None:
        """Should use C4_PROJECT_ROOT environment variable."""
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            c4_dir = root / ".c4"
            c4_dir.mkdir()
            db_path = c4_dir / "tasks.db"
            _create_test_db(db_path)

            with patch.dict("os.environ", {"C4_PROJECT_ROOT": str(root)}):
                searcher = _get_memory_searcher()
                assert searcher.db_path == db_path

    def test_uses_cwd_when_no_env(self) -> None:
        """Should use current directory when C4_PROJECT_ROOT not set."""
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            c4_dir = root / ".c4"
            c4_dir.mkdir()
            db_path = c4_dir / "tasks.db"
            _create_test_db(db_path)

            with patch.dict("os.environ", {}, clear=True):
                with patch("c4.mcp.handlers.memory.Path.cwd", return_value=root):
                    # Remove C4_PROJECT_ROOT if exists
                    import os
                    old_value = os.environ.pop("C4_PROJECT_ROOT", None)
                    try:
                        searcher = _get_memory_searcher()
                        assert searcher.db_path == db_path
                    finally:
                        if old_value:
                            os.environ["C4_PROJECT_ROOT"] = old_value

    def test_uses_directory_name_as_project_id(self) -> None:
        """Should use directory name as project ID by default."""
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            c4_dir = root / ".c4"
            c4_dir.mkdir()
            db_path = c4_dir / "tasks.db"
            _create_test_db(db_path)

            with patch.dict("os.environ", {"C4_PROJECT_ROOT": str(root)}):
                searcher = _get_memory_searcher()
                # Project ID should be the directory name
                assert searcher.project_id == root.name
