"""Tests for the hybrid search module."""

import sqlite3
import tempfile
from datetime import datetime, timedelta
from pathlib import Path
from unittest.mock import MagicMock

from c4.memory.search import (
    RRF_K,
    MemorySearcher,
    MemorySearchResult,
    SearchFilters,
    get_memory_searcher,
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


class TestMemorySearchResult:
    """Tests for MemorySearchResult dataclass."""

    def test_create_result(self) -> None:
        """Should create result with all fields."""
        result = MemorySearchResult(
            id="obs-001",
            title="Test Title",
            preview="Preview text...",
            tokens=50,
            score=0.85,
            source="read_file",
            importance=7,
            created_at=datetime(2024, 1, 1, 12, 0, 0),
        )

        assert result.id == "obs-001"
        assert result.title == "Test Title"
        assert result.preview == "Preview text..."
        assert result.tokens == 50
        assert result.score == 0.85
        assert result.source == "read_file"
        assert result.importance == 7

    def test_to_dict(self) -> None:
        """Should convert to dictionary."""
        result = MemorySearchResult(
            id="obs-001",
            title="Test",
            preview="Preview",
            tokens=10,
            score=0.5,
        )

        data = result.to_dict()

        assert data["id"] == "obs-001"
        assert data["score"] == 0.5
        assert "created_at" in data


class TestSearchFilters:
    """Tests for SearchFilters dataclass."""

    def test_create_empty_filters(self) -> None:
        """Should create filters with defaults."""
        filters = SearchFilters()

        assert filters.memory_type is None
        assert filters.tags is None
        assert filters.since is None
        assert filters.min_importance is None

    def test_create_with_values(self) -> None:
        """Should create filters with specified values."""
        since = datetime(2024, 1, 1)
        filters = SearchFilters(
            memory_type="read_file",
            tags=["important", "code"],
            since=since,
            min_importance=7,
        )

        assert filters.memory_type == "read_file"
        assert filters.tags == ["important", "code"]
        assert filters.since == since
        assert filters.min_importance == 7


class TestMemorySearcherKeywordSearch:
    """Tests for keyword search functionality."""

    def test_keyword_search_finds_match(self) -> None:
        """Should find observations matching query."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            # Insert test observations
            _insert_observation(
                db_path, "obs-001", "test-project", "read_file",
                "This is a test about authentication flow"
            )
            _insert_observation(
                db_path, "obs-002", "test-project", "read_file",
                "Database connection settings"
            )

            searcher = MemorySearcher("test-project", db_path)
            results = searcher._keyword_search("authentication", limit=10)

            assert len(results) == 1
            assert results[0][0] == "obs-001"

    def test_keyword_search_no_match(self) -> None:
        """Should return empty list when no match."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            _insert_observation(
                db_path, "obs-001", "test-project", "read_file", "Hello world"
            )

            searcher = MemorySearcher("test-project", db_path)
            results = searcher._keyword_search("nonexistent", limit=10)

            assert len(results) == 0

    def test_keyword_search_respects_limit(self) -> None:
        """Should respect limit parameter."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            # Insert multiple matching observations
            for i in range(10):
                _insert_observation(
                    db_path, f"obs-{i:03d}", "test-project", "read_file",
                    f"Test content {i} with keyword"
                )

            searcher = MemorySearcher("test-project", db_path)
            results = searcher._keyword_search("keyword", limit=3)

            assert len(results) == 3

    def test_keyword_search_filters_by_source(self) -> None:
        """Should filter by memory_type in filters."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            _insert_observation(
                db_path, "obs-001", "test-project", "read_file",
                "Code content"
            )
            _insert_observation(
                db_path, "obs-002", "test-project", "user_message",
                "Code question"
            )

            searcher = MemorySearcher("test-project", db_path)
            filters = SearchFilters(memory_type="read_file")
            results = searcher._keyword_search("Code", limit=10, filters=filters)

            assert len(results) == 1
            assert results[0][0] == "obs-001"


class TestMemorySearcherVectorSearch:
    """Tests for vector search functionality."""

    def test_vector_search_without_provider_returns_empty(self) -> None:
        """Should return empty list when no vector provider."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            searcher = MemorySearcher("test-project", db_path)
            results = searcher._vector_search("test query", limit=10)

            assert results == []

    def test_vector_search_with_mocked_provider(self) -> None:
        """Should perform vector search with provider."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            # Mock embedding provider
            mock_provider = MagicMock()
            mock_provider.embed.return_value = [0.1, 0.2, 0.3]

            # Mock vector store
            mock_vector_store = MagicMock()
            mock_result = MagicMock()
            mock_result.id = "emb-obs-001"
            mock_result.score = 0.95
            mock_vector_store.search.return_value = [mock_result]

            searcher = MemorySearcher(
                "test-project", db_path,
                embedding_provider=mock_provider,
                vector_store=mock_vector_store
            )
            results = searcher._vector_search("test query", limit=10)

            assert len(results) == 1
            assert results[0][0] == "obs-001"
            assert results[0][1] == 0.95


class TestRRFMerge:
    """Tests for Reciprocal Rank Fusion."""

    def test_rrf_merge_combines_results(self) -> None:
        """Should combine results from both searches."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            # Insert observations
            _insert_observation(
                db_path, "obs-001", "test-project", "source1", "Content 1"
            )
            _insert_observation(
                db_path, "obs-002", "test-project", "source2", "Content 2"
            )
            _insert_observation(
                db_path, "obs-003", "test-project", "source3", "Content 3"
            )

            searcher = MemorySearcher("test-project", db_path)

            vector_results = [("obs-001", 0.9), ("obs-002", 0.7)]
            keyword_results = [("obs-002", 0.8), ("obs-003", 0.6)]

            merged = searcher._rrf_merge(vector_results, keyword_results)

            # obs-002 should be first (appears in both)
            assert len(merged) == 3
            assert merged[0].id == "obs-002"  # Highest RRF score (in both lists)

    def test_rrf_merge_empty_inputs(self) -> None:
        """Should handle empty input lists."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            searcher = MemorySearcher("test-project", db_path)
            merged = searcher._rrf_merge([], [])

            assert merged == []

    def test_rrf_constant_value(self) -> None:
        """RRF_K should be 60."""
        assert RRF_K == 60


class TestSearch:
    """Tests for the main search method."""

    def test_search_empty_query_returns_empty(self) -> None:
        """Should return empty list for empty query."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            searcher = MemorySearcher("test-project", db_path)
            results = searcher.search("")

            assert results == []

    def test_search_returns_results(self) -> None:
        """Should return matching results."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            _insert_observation(
                db_path, "obs-001", "test-project", "read_file",
                "User authentication and login flow implementation"
            )

            searcher = MemorySearcher("test-project", db_path)
            results = searcher.search("authentication", limit=10)

            assert len(results) == 1
            assert results[0].id == "obs-001"

    def test_search_respects_limit(self) -> None:
        """Should respect limit parameter."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            for i in range(10):
                _insert_observation(
                    db_path, f"obs-{i:03d}", "test-project", "source",
                    f"Content with test keyword {i}"
                )

            searcher = MemorySearcher("test-project", db_path)
            results = searcher.search("test", limit=3)

            assert len(results) <= 3

    def test_search_with_filters(self) -> None:
        """Should apply filters to results."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            _insert_observation(
                db_path, "obs-001", "test-project", "read_file",
                "Code content", importance=8
            )
            _insert_observation(
                db_path, "obs-002", "test-project", "user_message",
                "Code question", importance=3
            )

            searcher = MemorySearcher("test-project", db_path)
            filters = SearchFilters(min_importance=5)
            results = searcher.search("Code", limit=10, filters=filters)

            assert len(results) == 1
            assert results[0].id == "obs-001"


class TestSearchByTags:
    """Tests for tag-based search."""

    def test_search_by_tags(self) -> None:
        """Should find observations with matching tags."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            _insert_observation(
                db_path, "obs-001", "test-project", "source",
                "Content 1", tags='["important", "code"]'
            )
            _insert_observation(
                db_path, "obs-002", "test-project", "source",
                "Content 2", tags='["documentation"]'
            )

            searcher = MemorySearcher("test-project", db_path)
            results = searcher.search_by_tags(["important"], limit=10)

            assert len(results) == 1
            assert results[0].id == "obs-001"

    def test_search_by_tags_empty_list(self) -> None:
        """Should return empty for empty tags list."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            searcher = MemorySearcher("test-project", db_path)
            results = searcher.search_by_tags([])

            assert results == []


class TestGetRecent:
    """Tests for getting recent observations."""

    def test_get_recent_returns_newest_first(self) -> None:
        """Should return observations ordered by created_at desc."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            now = datetime.now()
            _insert_observation(
                db_path, "obs-old", "test-project", "source",
                "Old content", created_at=now - timedelta(hours=2)
            )
            _insert_observation(
                db_path, "obs-new", "test-project", "source",
                "New content", created_at=now
            )

            searcher = MemorySearcher("test-project", db_path)
            results = searcher.get_recent(limit=10)

            assert len(results) == 2
            assert results[0].id == "obs-new"
            assert results[1].id == "obs-old"

    def test_get_recent_with_source_filter(self) -> None:
        """Should filter by source."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            _create_test_db(db_path)

            _insert_observation(
                db_path, "obs-001", "test-project", "read_file", "Content 1"
            )
            _insert_observation(
                db_path, "obs-002", "test-project", "user_message", "Content 2"
            )

            searcher = MemorySearcher("test-project", db_path)
            results = searcher.get_recent(source="read_file")

            assert len(results) == 1
            assert results[0].source == "read_file"


class TestGetMemorySearcher:
    """Tests for factory function."""

    def test_create_searcher(self) -> None:
        """Should create searcher without vector search."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            searcher = get_memory_searcher("test-project", db_path)

            assert searcher.project_id == "test-project"
            assert searcher._embedding_provider is None
            assert searcher._vector_store is None

    def test_create_searcher_vector_search_fallback(self) -> None:
        """Should fallback gracefully when vector search init fails."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            # enable_vector_search=True but no API key
            searcher = get_memory_searcher(
                "test-project",
                db_path,
                enable_vector_search=True,
            )

            assert searcher.project_id == "test-project"
