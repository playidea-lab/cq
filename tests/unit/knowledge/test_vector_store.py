"""Tests for VectorStore.recreate_for_dimension — 768-dim migration.

Covers:
  - TestVectorStoreRecreateDim768: recreate_for_dimension(768) then upsert succeeds
  - TestVectorStoreRecreateDeletesExisting: existing rows are deleted
  - TestMigration00025Exists: SQL migration file for 768-dim exists
"""

from pathlib import Path

import pytest

from c4.knowledge.vector_store import VectorStore

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _vec768(val: float = 1.0) -> list[float]:
    """Create a 768-dim vector with first element set, rest zeros."""
    v = [0.0] * 768
    v[0] = val
    return v


def _vec4(val: float = 1.0) -> list[float]:
    """Create a 4-dim vector."""
    return [val, 0.0, 0.0, 0.0]


# ---------------------------------------------------------------------------
# TestVectorStoreRecreateDim768
# ---------------------------------------------------------------------------

class TestVectorStoreRecreateDim768:
    """recreate_for_dimension(768) succeeds and allows 768-dim upsert."""

    def test_recreate_sets_new_dimension(self):
        with VectorStore(":memory:", dimension=4) as store:
            store.recreate_for_dimension(768)
            assert store.dimension == 768

    def test_recreate_allows_768dim_insert(self):
        with VectorStore(":memory:", dimension=4) as store:
            store.recreate_for_dimension(768)
            # Should not raise
            store.add("doc768", _vec768(1.0))
            assert store.count() == 1

    def test_recreate_rejects_wrong_dim_after(self):
        with VectorStore(":memory:", dimension=4) as store:
            store.recreate_for_dimension(768)
            with pytest.raises(ValueError, match="dimension"):
                store.add("bad", _vec4(1.0))

    def test_recreate_search_works_after(self):
        with VectorStore(":memory:", dimension=4) as store:
            store.recreate_for_dimension(768)
            store.add("a", _vec768(1.0))
            store.add("b", _vec768(0.5))
            results = store.search(_vec768(1.0), limit=1)
            assert len(results) == 1
            assert results[0].id == "a"


# ---------------------------------------------------------------------------
# TestVectorStoreRecreateDeletesExisting
# ---------------------------------------------------------------------------

class TestVectorStoreRecreateDeletesExisting:
    """Existing rows are deleted by recreate_for_dimension."""

    def test_existing_rows_deleted_after_recreate(self):
        with VectorStore(":memory:", dimension=4) as store:
            store.add("old1", _vec4(1.0))
            store.add("old2", _vec4(0.5))
            assert store.count() == 2

            store.recreate_for_dimension(768)
            assert store.count() == 0

    def test_old_ids_not_found_after_recreate(self):
        with VectorStore(":memory:", dimension=4) as store:
            store.add("old_doc", _vec4(1.0))
            assert store.exists("old_doc") is True

            store.recreate_for_dimension(768)
            assert store.exists("old_doc") is False

    def test_recreate_empty_store_is_noop(self):
        with VectorStore(":memory:", dimension=4) as store:
            assert store.count() == 0
            # Should not raise on empty store
            store.recreate_for_dimension(768)
            assert store.count() == 0

    def test_new_inserts_work_after_recreate_with_same_id(self):
        with VectorStore(":memory:", dimension=4) as store:
            store.add("reuse_id", _vec4(1.0))
            store.recreate_for_dimension(768)
            # Old row deleted, same ID should be reusable
            store.add("reuse_id", _vec768(1.0))
            assert store.count() == 1
            assert store.exists("reuse_id") is True


# ---------------------------------------------------------------------------
# TestMigration00025Exists
# ---------------------------------------------------------------------------

class TestMigration00025Exists:
    """The Supabase migration SQL file for 768-dim exists."""

    def test_migration_file_exists(self):
        # Locate repo root relative to this test file
        # tests/unit/knowledge/ -> tests/unit/ -> tests/ -> repo root
        repo_root = Path(__file__).parent.parent.parent.parent
        migrations_dir = repo_root / "infra" / "supabase" / "migrations"

        # Find migration file matching the 768-dim pattern
        matches = list(migrations_dir.glob("*knowledge*768*")) + list(
            migrations_dir.glob("*768*knowledge*")
        )
        assert len(matches) >= 1, (
            f"No 768-dim knowledge migration found in {migrations_dir}. "
            f"Expected a file matching '*knowledge*768*' or '*768*knowledge*'."
        )

    def test_migration_file_contains_768(self):
        repo_root = Path(__file__).parent.parent.parent.parent
        migrations_dir = repo_root / "infra" / "supabase" / "migrations"

        matches = list(migrations_dir.glob("*knowledge*768*")) + list(
            migrations_dir.glob("*768*knowledge*")
        )
        assert len(matches) >= 1, "Migration file not found"

        content = matches[0].read_text()
        assert "768" in content, "Migration file should reference 768 dimensions"
