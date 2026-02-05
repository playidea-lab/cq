"""Tests for c4_get_memory_detail MCP tool handler."""

import sqlite3
import tempfile
from datetime import datetime
from pathlib import Path
from unittest.mock import MagicMock, patch

from c4.mcp.handlers.memory import (
    _estimate_tokens,
    _find_related_memories,
    _get_auto_capture_handler,
    _update_access_stats,
    handle_get_memory_detail,
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
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            access_count INTEGER DEFAULT 0,
            accessed_at TIMESTAMP
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
    metadata: str = "{}",
    created_at: datetime | None = None,
) -> None:
    """Insert a test observation."""
    if created_at is None:
        created_at = datetime.now()
    conn = sqlite3.connect(db_path)
    conn.execute(
        """
        INSERT INTO c4_observations
        (id, project_id, source, content, importance, tags, metadata, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        """,
        (obs_id, project_id, source, content, importance, tags, metadata, created_at.isoformat()),
    )
    conn.commit()
    conn.close()


class TestHandleGetMemoryDetail:
    """Tests for handle_get_memory_detail handler."""

    def test_memory_id_required(self) -> None:
        """Should return error when memory_id is missing."""
        result = handle_get_memory_detail(None, {})
        assert "error" in result
        assert "memory_id is required" in result["error"]

    def test_empty_memory_id(self) -> None:
        """Should return error for empty memory_id."""
        result = handle_get_memory_detail(None, {"memory_id": ""})
        assert "error" in result
        assert "memory_id is required" in result["error"]

    def test_memory_not_found(self) -> None:
        """Should return not found for non-existent memory."""
        with patch("c4.mcp.handlers.memory._get_auto_capture_handler") as mock_handler:
            mock_instance = MagicMock()
            mock_instance.get_observation.return_value = None
            mock_handler.return_value = mock_instance

            result = handle_get_memory_detail(None, {"memory_id": "obs-nonexistent"})

            assert result["found"] is False
            assert result["memory_id"] == "obs-nonexistent"
            assert "not found" in result["message"]

    def test_get_memory_detail_success(self) -> None:
        """Should return full memory details."""
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            c4_dir = root / ".c4"
            c4_dir.mkdir()
            db_path = c4_dir / "tasks.db"
            _create_test_db(db_path)

            with patch.dict("os.environ", {"C4_PROJECT_ROOT": str(root)}):
                with patch("c4.mcp.handlers.memory._get_auto_capture_handler") as mock_handler:
                    # Create mock observation
                    mock_obs = MagicMock()
                    mock_obs.id = "obs-001"
                    mock_obs.source = "read_file"
                    mock_obs.content = "User authentication flow implementation code"
                    mock_obs.importance = 7
                    mock_obs.tags = ["auth", "code"]
                    mock_obs.metadata = {"path": "/src/auth.py"}
                    mock_obs.created_at = datetime(2024, 6, 15, 12, 0, 0)

                    mock_instance = MagicMock()
                    mock_instance.get_observation.return_value = mock_obs
                    mock_handler.return_value = mock_instance

                    # Insert observation for access stats update
                    _insert_observation(db_path, "obs-001", "test", "read_file", "content")

                    result = handle_get_memory_detail(None, {"memory_id": "obs-001"})

                    assert result["found"] is True
                    assert result["id"] == "obs-001"
                    assert result["title"] == "read_file"
                    assert result["content"] == "User authentication flow implementation code"
                    assert result["content_tokens"] > 0
                    assert "metadata" in result
                    assert result["metadata"]["source"] == "read_file"
                    assert result["metadata"]["importance"] == 7
                    assert result["metadata"]["path"] == "/src/auth.py"

    def test_get_memory_detail_with_related(self) -> None:
        """Should include related memories when requested."""
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            c4_dir = root / ".c4"
            c4_dir.mkdir()
            db_path = c4_dir / "tasks.db"
            _create_test_db(db_path)

            with patch.dict("os.environ", {"C4_PROJECT_ROOT": str(root)}):
                with patch("c4.mcp.handlers.memory._get_auto_capture_handler") as mock_handler:
                    with patch("c4.mcp.handlers.memory._find_related_memories") as mock_related:
                        # Create mock observation
                        mock_obs = MagicMock()
                        mock_obs.id = "obs-001"
                        mock_obs.source = "read_file"
                        mock_obs.content = "Test content"
                        mock_obs.importance = 5
                        mock_obs.tags = []
                        mock_obs.metadata = {}
                        mock_obs.created_at = datetime.now()

                        mock_instance = MagicMock()
                        mock_instance.get_observation.return_value = mock_obs
                        mock_handler.return_value = mock_instance

                        # Mock related memories
                        mock_related.return_value = [
                            {"id": "obs-002", "title": "related", "preview": "...", "score": 0.5}
                        ]

                        # Insert observation for access stats update
                        _insert_observation(db_path, "obs-001", "test", "read_file", "content")

                        result = handle_get_memory_detail(None, {
                            "memory_id": "obs-001",
                            "include_related": True,
                        })

                        assert result["found"] is True
                        assert "related" in result
                        assert len(result["related"]) == 1
                        assert result["related"][0]["id"] == "obs-002"


class TestUpdateAccessStats:
    """Tests for _update_access_stats helper."""

    def test_updates_access_count(self) -> None:
        """Should increment access_count."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)
            _insert_observation(db_path, "obs-001", "test", "source", "content")

            # Update access stats
            _update_access_stats(db_path, "obs-001")

            # Check updated values
            conn = sqlite3.connect(db_path)
            cursor = conn.execute(
                "SELECT access_count, accessed_at FROM c4_observations WHERE id = ?",
                ("obs-001",),
            )
            row = cursor.fetchone()
            conn.close()

            assert row[0] == 1  # access_count
            assert row[1] is not None  # accessed_at

    def test_increments_existing_count(self) -> None:
        """Should increment existing access_count."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)
            _insert_observation(db_path, "obs-001", "test", "source", "content")

            # Update multiple times
            _update_access_stats(db_path, "obs-001")
            _update_access_stats(db_path, "obs-001")
            _update_access_stats(db_path, "obs-001")

            # Check count
            conn = sqlite3.connect(db_path)
            cursor = conn.execute(
                "SELECT access_count FROM c4_observations WHERE id = ?",
                ("obs-001",),
            )
            row = cursor.fetchone()
            conn.close()

            assert row[0] == 3

    def test_adds_columns_if_missing(self) -> None:
        """Should add access_count and accessed_at columns if missing."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            # Create table without access columns
            conn = sqlite3.connect(db_path)
            conn.execute("""
                CREATE TABLE c4_observations (
                    id TEXT PRIMARY KEY,
                    project_id TEXT NOT NULL,
                    source TEXT NOT NULL,
                    content TEXT NOT NULL
                )
            """)
            conn.execute(
                "INSERT INTO c4_observations (id, project_id, source, content) VALUES (?, ?, ?, ?)",
                ("obs-001", "test", "source", "content"),
            )
            conn.commit()
            conn.close()

            # Should not raise error
            _update_access_stats(db_path, "obs-001")

            # Check columns were added
            conn = sqlite3.connect(db_path)
            cursor = conn.execute("PRAGMA table_info(c4_observations)")
            columns = [row[1] for row in cursor.fetchall()]
            conn.close()

            assert "access_count" in columns
            assert "accessed_at" in columns


class TestEstimateTokens:
    """Tests for _estimate_tokens helper."""

    def test_empty_string(self) -> None:
        """Should return 0 for empty string."""
        assert _estimate_tokens("") == 0

    def test_short_text(self) -> None:
        """Should return at least 1 for non-empty text."""
        assert _estimate_tokens("hi") >= 1

    def test_longer_text(self) -> None:
        """Should estimate ~4 chars per token."""
        text = "a" * 400  # 100 tokens approximately
        result = _estimate_tokens(text)
        assert 90 <= result <= 110


class TestFindRelatedMemories:
    """Tests for _find_related_memories helper."""

    def test_excludes_self(self) -> None:
        """Should exclude the source memory from results."""
        with patch("c4.mcp.handlers.memory._get_memory_searcher") as mock_searcher:
            # Create mock results including self
            mock_result_self = MagicMock()
            mock_result_self.id = "obs-001"
            mock_result_self.title = "self"
            mock_result_self.preview = "..."
            mock_result_self.score = 1.0

            mock_result_other = MagicMock()
            mock_result_other.id = "obs-002"
            mock_result_other.title = "other"
            mock_result_other.preview = "..."
            mock_result_other.score = 0.5

            mock_instance = MagicMock()
            mock_instance.search.return_value = [mock_result_self, mock_result_other]
            mock_searcher.return_value = mock_instance

            result = _find_related_memories("obs-001", "test content", limit=5)

            assert len(result) == 1
            assert result[0]["id"] == "obs-002"

    def test_respects_limit(self) -> None:
        """Should respect the limit parameter."""
        with patch("c4.mcp.handlers.memory._get_memory_searcher") as mock_searcher:
            # Create many mock results
            mock_results = []
            for i in range(10):
                mock_r = MagicMock()
                mock_r.id = f"obs-{i:03d}"
                mock_r.title = f"title-{i}"
                mock_r.preview = "..."
                mock_r.score = 0.9 - i * 0.05
                mock_results.append(mock_r)

            mock_instance = MagicMock()
            mock_instance.search.return_value = mock_results
            mock_searcher.return_value = mock_instance

            result = _find_related_memories("obs-self", "test content", limit=3)

            assert len(result) == 3

    def test_empty_content(self) -> None:
        """Should return empty list for empty content."""
        result = _find_related_memories("obs-001", "", limit=5)
        assert result == []

    def test_handles_search_error(self) -> None:
        """Should return empty list on search error."""
        with patch("c4.mcp.handlers.memory._get_memory_searcher") as mock_searcher:
            mock_searcher.side_effect = Exception("Search failed")

            result = _find_related_memories("obs-001", "test content", limit=5)

            assert result == []


class TestGetAutoCaptureHandler:
    """Tests for _get_auto_capture_handler helper."""

    def test_uses_project_root_env(self) -> None:
        """Should use C4_PROJECT_ROOT environment variable."""
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            c4_dir = root / ".c4"
            c4_dir.mkdir()
            db_path = c4_dir / "tasks.db"
            _create_test_db(db_path)

            with patch.dict("os.environ", {"C4_PROJECT_ROOT": str(root)}):
                handler = _get_auto_capture_handler()
                assert handler.db_path == db_path

    def test_uses_directory_name_as_project_id(self) -> None:
        """Should use directory name as project ID by default."""
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            c4_dir = root / ".c4"
            c4_dir.mkdir()
            db_path = c4_dir / "tasks.db"
            _create_test_db(db_path)

            with patch.dict("os.environ", {"C4_PROJECT_ROOT": str(root)}):
                handler = _get_auto_capture_handler()
                assert handler.project_id == root.name
