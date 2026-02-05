"""Tests for VectorStore with sqlite-vec."""

import tempfile
from pathlib import Path

import pytest

from c4.memory.vector_store import SearchResult, VectorStore


class TestVectorStoreInit:
    """Tests for VectorStore initialization."""

    def test_init_in_memory(self) -> None:
        """In-memory database should work."""
        store = VectorStore(":memory:", dimension=4)
        assert store.dimension == 4
        assert store.db_path == ":memory:"
        store.close()

    def test_init_file_database(self) -> None:
        """File-based database should be created."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            store = VectorStore(db_path, dimension=8)
            store.add("test", [0.1] * 8)
            store.close()

            # Database file should exist
            assert db_path.exists()

    def test_init_custom_table_name(self) -> None:
        """Custom table name should be used."""
        store = VectorStore(":memory:", dimension=4, table_name="custom_vectors")
        assert store.table_name == "custom_vectors"
        store.close()


class TestVectorStoreAdd:
    """Tests for adding embeddings."""

    def test_add_single_embedding(self) -> None:
        """Adding a single embedding should succeed."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("doc1", [1.0, 0.0, 0.0, 0.0])
            assert store.count() == 1

    def test_add_multiple_embeddings(self) -> None:
        """Adding multiple embeddings should succeed."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("doc1", [1.0, 0.0, 0.0, 0.0])
            store.add("doc2", [0.0, 1.0, 0.0, 0.0])
            store.add("doc3", [0.0, 0.0, 1.0, 0.0])
            assert store.count() == 3

    def test_add_wrong_dimension_raises_error(self) -> None:
        """Wrong dimension should raise ValueError."""
        with VectorStore(":memory:", dimension=4) as store:
            with pytest.raises(ValueError, match="dimension"):
                store.add("doc1", [1.0, 0.0, 0.0])  # 3 instead of 4

    def test_add_duplicate_id_raises_error(self) -> None:
        """Duplicate ID should raise IntegrityError."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("doc1", [1.0, 0.0, 0.0, 0.0])
            with pytest.raises(Exception):  # sqlite3.IntegrityError
                store.add("doc1", [0.0, 1.0, 0.0, 0.0])

    def test_add_batch(self) -> None:
        """Batch add should insert multiple embeddings."""
        with VectorStore(":memory:", dimension=4) as store:
            items = [
                ("doc1", [1.0, 0.0, 0.0, 0.0]),
                ("doc2", [0.0, 1.0, 0.0, 0.0]),
                ("doc3", [0.0, 0.0, 1.0, 0.0]),
            ]
            store.add_batch(items)
            assert store.count() == 3


class TestVectorStoreSearch:
    """Tests for similarity search."""

    def test_search_empty_store(self) -> None:
        """Search on empty store should return empty list."""
        with VectorStore(":memory:", dimension=4) as store:
            results = store.search([1.0, 0.0, 0.0, 0.0], limit=10)
            assert results == []

    def test_search_returns_most_similar(self) -> None:
        """Search should return most similar embedding first."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("similar", [0.9, 0.1, 0.0, 0.0])
            store.add("different", [0.0, 0.0, 1.0, 0.0])

            results = store.search([1.0, 0.0, 0.0, 0.0], limit=2)

            assert len(results) == 2
            assert results[0].id == "similar"

    def test_search_respects_limit(self) -> None:
        """Search should respect limit parameter."""
        with VectorStore(":memory:", dimension=4) as store:
            for i in range(10):
                store.add(f"doc{i}", [float(i % 4 == j) for j in range(4)])

            results = store.search([1.0, 0.0, 0.0, 0.0], limit=3)
            assert len(results) == 3

    def test_search_with_threshold(self) -> None:
        """Search with threshold should filter results."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("very_similar", [1.0, 0.0, 0.0, 0.0])
            store.add("somewhat_similar", [0.7, 0.3, 0.0, 0.0])
            store.add("different", [0.0, 0.0, 1.0, 0.0])

            # Very low threshold should only match exact or near-exact
            results = store.search([1.0, 0.0, 0.0, 0.0], limit=10, threshold=0.1)

            # Should filter out the different one
            ids = [r.id for r in results]
            assert "different" not in ids or len(results) < 3

    def test_search_result_has_score(self) -> None:
        """Search results should include similarity score."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("doc1", [1.0, 0.0, 0.0, 0.0])

            results = store.search([1.0, 0.0, 0.0, 0.0], limit=1)

            assert len(results) == 1
            assert results[0].score > 0
            assert results[0].distance >= 0

    def test_search_wrong_dimension_raises_error(self) -> None:
        """Query with wrong dimension should raise ValueError."""
        with VectorStore(":memory:", dimension=4) as store:
            with pytest.raises(ValueError, match="dimension"):
                store.search([1.0, 0.0, 0.0], limit=10)  # 3 instead of 4


class TestVectorStoreDelete:
    """Tests for deleting embeddings."""

    def test_delete_existing(self) -> None:
        """Deleting existing embedding should return True."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("doc1", [1.0, 0.0, 0.0, 0.0])
            assert store.delete("doc1") is True
            assert store.count() == 0

    def test_delete_nonexistent(self) -> None:
        """Deleting non-existent embedding should return False."""
        with VectorStore(":memory:", dimension=4) as store:
            assert store.delete("nonexistent") is False

    def test_delete_removes_from_search(self) -> None:
        """Deleted embedding should not appear in search results."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("doc1", [1.0, 0.0, 0.0, 0.0])
            store.add("doc2", [0.0, 1.0, 0.0, 0.0])

            store.delete("doc1")

            results = store.search([1.0, 0.0, 0.0, 0.0], limit=10)
            ids = [r.id for r in results]
            assert "doc1" not in ids


class TestVectorStoreHelpers:
    """Tests for helper methods."""

    def test_count_empty(self) -> None:
        """Empty store should have count 0."""
        with VectorStore(":memory:", dimension=4) as store:
            assert store.count() == 0

    def test_count_after_operations(self) -> None:
        """Count should reflect add/delete operations."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("doc1", [1.0, 0.0, 0.0, 0.0])
            assert store.count() == 1

            store.add("doc2", [0.0, 1.0, 0.0, 0.0])
            assert store.count() == 2

            store.delete("doc1")
            assert store.count() == 1

    def test_exists_true(self) -> None:
        """exists() should return True for existing ID."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("doc1", [1.0, 0.0, 0.0, 0.0])
            assert store.exists("doc1") is True

    def test_exists_false(self) -> None:
        """exists() should return False for non-existent ID."""
        with VectorStore(":memory:", dimension=4) as store:
            assert store.exists("doc1") is False


class TestSearchResult:
    """Tests for SearchResult dataclass."""

    def test_search_result_creation(self) -> None:
        """SearchResult should be creatable with all fields."""
        result = SearchResult(id="doc1", distance=0.5, score=0.67)
        assert result.id == "doc1"
        assert result.distance == 0.5
        assert result.score == 0.67


class TestContextManager:
    """Tests for context manager usage."""

    def test_context_manager(self) -> None:
        """VectorStore should work as context manager."""
        with VectorStore(":memory:", dimension=4) as store:
            store.add("doc1", [1.0, 0.0, 0.0, 0.0])
            assert store.count() == 1
        # Connection should be closed after exiting context

    def test_persistence_across_connections(self) -> None:
        """Data should persist across connections for file database."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            # Add data in first connection
            with VectorStore(db_path, dimension=4) as store:
                store.add("doc1", [1.0, 0.0, 0.0, 0.0])

            # Verify data in second connection
            with VectorStore(db_path, dimension=4) as store:
                assert store.count() == 1
                assert store.exists("doc1") is True
