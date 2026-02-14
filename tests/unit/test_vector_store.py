"""Tests for the C4 Knowledge VectorStore (sqlite-vec based).

Covers initialization, CRUD operations, search, batch inserts,
threshold filtering, context manager usage, and error handling.

Follows TDD: RED phase defines expectations, GREEN makes them pass.
"""

import sqlite3
from pathlib import Path

import pytest

from c4.knowledge.vector_store import SearchResult, VectorStore

# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

DIM = 4  # Small dimension for fast tests


@pytest.fixture
def store():
    """In-memory VectorStore for fast, isolated tests."""
    with VectorStore(":memory:", dimension=DIM) as s:
        yield s


@pytest.fixture
def store_on_disk(tmp_path: Path):
    """Disk-backed VectorStore to verify persistence."""
    db = tmp_path / "test_vec.db"
    with VectorStore(db_path=db, dimension=DIM) as s:
        yield s, db


def _vec(x: float, y: float = 0.0, z: float = 0.0, w: float = 0.0) -> list[float]:
    """Helper to build a 4-dim vector with readable params."""
    return [x, y, z, w]


# ---------------------------------------------------------------------------
# 1. Store Initialization
# ---------------------------------------------------------------------------

class TestInitialization:
    """Verify store creation and default configuration."""

    def test_in_memory_store_creates_successfully(self, store: VectorStore):
        assert store is not None
        assert store.dimension == DIM
        assert store.count() == 0

    def test_default_dimension_is_384(self):
        with VectorStore(":memory:") as s:
            assert s.dimension == 384

    def test_custom_table_name(self):
        with VectorStore(":memory:", dimension=DIM, table_name="my_vecs") as s:
            assert s.table_name == "my_vecs"
            # Should work without errors
            s.add("t1", _vec(1.0))
            assert s.count() == 1

    def test_disk_backed_store_creates_db_file(self, tmp_path: Path):
        db = tmp_path / "vec.db"
        assert not db.exists()
        with VectorStore(db_path=db, dimension=DIM):
            pass
        assert db.exists()


# ---------------------------------------------------------------------------
# 2. Add Document (Insert)
# ---------------------------------------------------------------------------

class TestAdd:
    """Verify embedding insertion."""

    def test_add_single_embedding(self, store: VectorStore):
        store.add("doc1", _vec(1.0))
        assert store.count() == 1

    def test_add_multiple_embeddings(self, store: VectorStore):
        store.add("a", _vec(1.0))
        store.add("b", _vec(0.0, 1.0))
        store.add("c", _vec(0.0, 0.0, 1.0))
        assert store.count() == 3

    def test_add_duplicate_id_raises(self, store: VectorStore):
        store.add("dup", _vec(1.0))
        with pytest.raises(sqlite3.IntegrityError):
            store.add("dup", _vec(0.0, 1.0))

    def test_add_wrong_dimension_raises(self, store: VectorStore):
        with pytest.raises(ValueError, match="dimension"):
            store.add("bad", [1.0, 2.0])  # only 2 dims, need 4


# ---------------------------------------------------------------------------
# 3. Get / Exists by ID
# ---------------------------------------------------------------------------

class TestExists:
    """Verify existence checks."""

    def test_exists_returns_true_for_known_id(self, store: VectorStore):
        store.add("doc1", _vec(1.0))
        assert store.exists("doc1") is True

    def test_exists_returns_false_for_unknown_id(self, store: VectorStore):
        assert store.exists("ghost") is False

    def test_exists_after_delete_returns_false(self, store: VectorStore):
        store.add("doc1", _vec(1.0))
        store.delete("doc1")
        assert store.exists("doc1") is False


# ---------------------------------------------------------------------------
# 4. Search by Embedding (KNN)
# ---------------------------------------------------------------------------

class TestSearch:
    """Verify KNN similarity search."""

    def test_search_returns_nearest(self, store: VectorStore):
        store.add("x_axis", _vec(1.0, 0.0))
        store.add("y_axis", _vec(0.0, 1.0))
        # Query close to x_axis
        results = store.search(_vec(0.9, 0.1), limit=2)
        assert len(results) >= 1
        assert results[0].id == "x_axis"

    def test_search_result_has_correct_fields(self, store: VectorStore):
        store.add("doc1", _vec(1.0))
        results = store.search(_vec(1.0), limit=1)
        r = results[0]
        assert isinstance(r, SearchResult)
        assert isinstance(r.id, str)
        assert isinstance(r.distance, float)
        assert isinstance(r.score, float)

    def test_search_identical_vector_has_high_score(self, store: VectorStore):
        store.add("exact", _vec(1.0, 0.0, 0.0, 0.0))
        results = store.search(_vec(1.0, 0.0, 0.0, 0.0), limit=1)
        # Identical vector -> distance ~ 0.0, score ~ 1.0
        assert results[0].distance == pytest.approx(0.0, abs=1e-5)
        assert results[0].score == pytest.approx(1.0, abs=1e-3)

    def test_search_respects_limit(self, store: VectorStore):
        for i in range(5):
            store.add(f"d{i}", _vec(float(i), float(i)))
        results = store.search(_vec(1.0, 1.0), limit=3)
        assert len(results) <= 3

    def test_search_wrong_dimension_raises(self, store: VectorStore):
        store.add("doc1", _vec(1.0))
        with pytest.raises(ValueError, match="dimension"):
            store.search([1.0, 2.0], limit=1)

    def test_search_with_threshold_filters(self, store: VectorStore):
        store.add("near", _vec(1.0, 0.0, 0.0, 0.0))
        store.add("far", _vec(0.0, 0.0, 0.0, 1.0))
        # Use a tight threshold to exclude far vectors
        results = store.search(_vec(1.0, 0.0, 0.0, 0.0), limit=10, threshold=0.5)
        ids = [r.id for r in results]
        assert "near" in ids
        # 'far' should be excluded by threshold (Euclidean dist ~ sqrt(2) > 0.5)
        assert "far" not in ids


# ---------------------------------------------------------------------------
# 5. Delete
# ---------------------------------------------------------------------------

class TestDelete:
    """Verify embedding deletion."""

    def test_delete_existing_returns_true(self, store: VectorStore):
        store.add("doc1", _vec(1.0))
        assert store.delete("doc1") is True

    def test_delete_nonexistent_returns_false(self, store: VectorStore):
        assert store.delete("nope") is False

    def test_delete_reduces_count(self, store: VectorStore):
        store.add("a", _vec(1.0))
        store.add("b", _vec(0.0, 1.0))
        assert store.count() == 2
        store.delete("a")
        assert store.count() == 1

    def test_deleted_embedding_not_in_search(self, store: VectorStore):
        store.add("alive", _vec(1.0))
        store.add("dead", _vec(0.9, 0.1))
        store.delete("dead")
        results = store.search(_vec(1.0), limit=10)
        ids = [r.id for r in results]
        assert "dead" not in ids
        assert "alive" in ids


# ---------------------------------------------------------------------------
# 6. Batch Insert
# ---------------------------------------------------------------------------

class TestAddBatch:
    """Verify batch insertion."""

    def test_batch_adds_all(self, store: VectorStore):
        items = [
            ("b1", _vec(1.0)),
            ("b2", _vec(0.0, 1.0)),
            ("b3", _vec(0.0, 0.0, 1.0)),
        ]
        store.add_batch(items)
        assert store.count() == 3

    def test_batch_wrong_dimension_raises(self, store: VectorStore):
        items = [
            ("ok", _vec(1.0)),
            ("bad", [1.0, 2.0]),  # wrong dim
        ]
        with pytest.raises(ValueError, match="dimension"):
            store.add_batch(items)

    def test_batch_items_searchable(self, store: VectorStore):
        items = [
            ("b1", _vec(1.0, 0.0, 0.0, 0.0)),
            ("b2", _vec(0.0, 1.0, 0.0, 0.0)),
        ]
        store.add_batch(items)
        results = store.search(_vec(1.0, 0.0, 0.0, 0.0), limit=1)
        assert results[0].id == "b1"


# ---------------------------------------------------------------------------
# 7. Search with No Results (Empty Store)
# ---------------------------------------------------------------------------

class TestSearchEmpty:
    """Verify search behavior on empty store."""

    def test_search_empty_store_returns_empty_list(self, store: VectorStore):
        results = store.search(_vec(1.0), limit=10)
        assert results == []

    def test_count_empty_store_is_zero(self, store: VectorStore):
        assert store.count() == 0


# ---------------------------------------------------------------------------
# 8. Persistence (Disk-backed)
# ---------------------------------------------------------------------------

class TestPersistence:
    """Verify data survives close/reopen on disk."""

    def test_data_persists_after_reopen(self, tmp_path: Path):
        db = tmp_path / "persist.db"
        # Write
        with VectorStore(db_path=db, dimension=DIM) as s:
            s.add("persistent", _vec(1.0))
            assert s.count() == 1

        # Reopen and verify
        with VectorStore(db_path=db, dimension=DIM) as s2:
            assert s2.count() == 1
            assert s2.exists("persistent") is True


# ---------------------------------------------------------------------------
# 9. Context Manager
# ---------------------------------------------------------------------------

class TestContextManager:
    """Verify context manager protocol."""

    def test_context_manager_returns_store(self):
        with VectorStore(":memory:", dimension=DIM) as s:
            assert isinstance(s, VectorStore)

    def test_connection_closed_after_exit(self):
        store = VectorStore(":memory:", dimension=DIM)
        store.close()
        assert store._conn is None


# ---------------------------------------------------------------------------
# 10. SearchResult Dataclass
# ---------------------------------------------------------------------------

class TestSearchResult:
    """Verify the SearchResult dataclass."""

    def test_fields_are_accessible(self):
        r = SearchResult(id="x", distance=0.5, score=0.67)
        assert r.id == "x"
        assert r.distance == 0.5
        assert r.score == 0.67

    def test_equality(self):
        a = SearchResult(id="x", distance=0.5, score=0.67)
        b = SearchResult(id="x", distance=0.5, score=0.67)
        assert a == b
