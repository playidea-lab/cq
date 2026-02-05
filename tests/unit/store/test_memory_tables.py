"""Tests for memory system tables in SQLite store."""

import json
import sqlite3
import tempfile
from pathlib import Path

import pytest

from c4.store.sqlite import SQLiteTaskStore


class TestMemoryTablesCreation:
    """Tests for memory table schema creation."""

    def test_observations_table_created(self) -> None:
        """c4_observations table should be created on init."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            SQLiteTaskStore(db_path)  # Initialize to create tables

            conn = sqlite3.connect(db_path)
            cursor = conn.execute(
                "SELECT name FROM sqlite_master WHERE type='table' AND name='c4_observations'"
            )
            assert cursor.fetchone() is not None
            conn.close()

    def test_memory_index_table_created(self) -> None:
        """c4_memory_index table should be created on init."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            SQLiteTaskStore(db_path)  # Initialize to create tables

            conn = sqlite3.connect(db_path)
            cursor = conn.execute(
                "SELECT name FROM sqlite_master WHERE type='table' AND name='c4_memory_index'"
            )
            assert cursor.fetchone() is not None
            conn.close()

    def test_observations_indexes_created(self) -> None:
        """Indexes should be created for c4_observations."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            SQLiteTaskStore(db_path)  # Initialize to create tables

            conn = sqlite3.connect(db_path)
            cursor = conn.execute(
                "SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_c4_observations_%'"
            )
            indexes = [row[0] for row in cursor.fetchall()]
            conn.close()

            assert "idx_c4_observations_project" in indexes
            assert "idx_c4_observations_source" in indexes
            assert "idx_c4_observations_importance" in indexes

    def test_memory_index_indexes_created(self) -> None:
        """Indexes should be created for c4_memory_index."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            SQLiteTaskStore(db_path)  # Initialize to create tables

            conn = sqlite3.connect(db_path)
            cursor = conn.execute(
                "SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_c4_memory_index_%'"
            )
            indexes = [row[0] for row in cursor.fetchall()]
            conn.close()

            assert "idx_c4_memory_index_project" in indexes
            assert "idx_c4_memory_index_observation" in indexes
            assert "idx_c4_memory_index_embedding" in indexes


class TestObservationsTableSchema:
    """Tests for c4_observations table schema."""

    def test_insert_observation(self) -> None:
        """Should be able to insert an observation."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            SQLiteTaskStore(db_path)  # Initialize to create tables

            conn = sqlite3.connect(db_path)
            conn.execute(
                """
                INSERT INTO c4_observations (id, project_id, source, content, importance, tags)
                VALUES (?, ?, ?, ?, ?, ?)
                """,
                ("obs-001", "test-project", "conversation", "User prefers Python", 8, '["preference", "language"]')
            )
            conn.commit()

            cursor = conn.execute(
                "SELECT id, project_id, source, content, importance, tags FROM c4_observations"
            )
            row = cursor.fetchone()
            conn.close()

            assert row[0] == "obs-001"
            assert row[1] == "test-project"
            assert row[2] == "conversation"
            assert row[3] == "User prefers Python"
            assert row[4] == 8
            assert json.loads(row[5]) == ["preference", "language"]

    def test_observation_default_importance(self) -> None:
        """Importance should default to 5."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            SQLiteTaskStore(db_path)  # Initialize to create tables

            conn = sqlite3.connect(db_path)
            conn.execute(
                """
                INSERT INTO c4_observations (id, project_id, source, content)
                VALUES (?, ?, ?, ?)
                """,
                ("obs-002", "test-project", "file", "Some content")
            )
            conn.commit()

            cursor = conn.execute(
                "SELECT importance FROM c4_observations WHERE id = ?",
                ("obs-002",)
            )
            row = cursor.fetchone()
            conn.close()

            assert row[0] == 5

    def test_observation_timestamps(self) -> None:
        """Timestamps should be set automatically."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            SQLiteTaskStore(db_path)  # Initialize to create tables

            conn = sqlite3.connect(db_path)
            conn.execute(
                """
                INSERT INTO c4_observations (id, project_id, source, content)
                VALUES (?, ?, ?, ?)
                """,
                ("obs-003", "test-project", "user", "Test content")
            )
            conn.commit()

            cursor = conn.execute(
                "SELECT created_at, updated_at FROM c4_observations WHERE id = ?",
                ("obs-003",)
            )
            row = cursor.fetchone()
            conn.close()

            assert row[0] is not None  # created_at
            assert row[1] is not None  # updated_at


class TestMemoryIndexTableSchema:
    """Tests for c4_memory_index table schema."""

    def test_insert_memory_index(self) -> None:
        """Should be able to insert a memory index entry."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            SQLiteTaskStore(db_path)  # Initialize to create tables

            conn = sqlite3.connect(db_path)

            # First insert observation
            conn.execute(
                """
                INSERT INTO c4_observations (id, project_id, source, content)
                VALUES (?, ?, ?, ?)
                """,
                ("obs-001", "test-project", "file", "Some content")
            )

            # Then insert memory index
            conn.execute(
                """
                INSERT INTO c4_memory_index (id, project_id, observation_id, embedding_id, chunk_index, chunk_text)
                VALUES (?, ?, ?, ?, ?, ?)
                """,
                ("idx-001", "test-project", "obs-001", "vec-001", 0, "Some content")
            )
            conn.commit()

            cursor = conn.execute(
                "SELECT id, project_id, observation_id, embedding_id, chunk_index, chunk_text FROM c4_memory_index"
            )
            row = cursor.fetchone()
            conn.close()

            assert row[0] == "idx-001"
            assert row[1] == "test-project"
            assert row[2] == "obs-001"
            assert row[3] == "vec-001"
            assert row[4] == 0
            assert row[5] == "Some content"

    def test_memory_index_unique_constraint(self) -> None:
        """observation_id + chunk_index should be unique."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            SQLiteTaskStore(db_path)  # Initialize to create tables

            conn = sqlite3.connect(db_path)

            # Insert observation
            conn.execute(
                """
                INSERT INTO c4_observations (id, project_id, source, content)
                VALUES (?, ?, ?, ?)
                """,
                ("obs-001", "test-project", "file", "Content")
            )

            # Insert first chunk
            conn.execute(
                """
                INSERT INTO c4_memory_index (id, project_id, observation_id, embedding_id, chunk_index)
                VALUES (?, ?, ?, ?, ?)
                """,
                ("idx-001", "test-project", "obs-001", "vec-001", 0)
            )
            conn.commit()

            # Try to insert duplicate chunk_index for same observation
            with pytest.raises(sqlite3.IntegrityError):
                conn.execute(
                    """
                    INSERT INTO c4_memory_index (id, project_id, observation_id, embedding_id, chunk_index)
                    VALUES (?, ?, ?, ?, ?)
                    """,
                    ("idx-002", "test-project", "obs-001", "vec-002", 0)
                )
            conn.close()

    def test_memory_index_multiple_chunks(self) -> None:
        """Should support multiple chunks per observation."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            SQLiteTaskStore(db_path)  # Initialize to create tables

            conn = sqlite3.connect(db_path)

            # Insert observation
            conn.execute(
                """
                INSERT INTO c4_observations (id, project_id, source, content)
                VALUES (?, ?, ?, ?)
                """,
                ("obs-001", "test-project", "file", "Long content that was chunked")
            )

            # Insert multiple chunks
            for i in range(3):
                conn.execute(
                    """
                    INSERT INTO c4_memory_index (id, project_id, observation_id, embedding_id, chunk_index, chunk_text)
                    VALUES (?, ?, ?, ?, ?, ?)
                    """,
                    (f"idx-{i}", "test-project", "obs-001", f"vec-{i}", i, f"chunk {i}")
                )
            conn.commit()

            cursor = conn.execute(
                "SELECT COUNT(*) FROM c4_memory_index WHERE observation_id = ?",
                ("obs-001",)
            )
            count = cursor.fetchone()[0]
            conn.close()

            assert count == 3


class TestMigrationIdempotence:
    """Tests for migration/initialization idempotence."""

    def test_multiple_init_is_safe(self) -> None:
        """Multiple store initializations should not cause errors."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            # Initialize multiple times
            SQLiteTaskStore(db_path)
            SQLiteTaskStore(db_path)
            SQLiteTaskStore(db_path)

            # Should not raise and tables should exist
            conn = sqlite3.connect(db_path)
            cursor = conn.execute(
                "SELECT name FROM sqlite_master WHERE type='table' AND name IN ('c4_observations', 'c4_memory_index')"
            )
            tables = [row[0] for row in cursor.fetchall()]
            conn.close()

            assert "c4_observations" in tables
            assert "c4_memory_index" in tables

    def test_existing_data_preserved_on_reinit(self) -> None:
        """Existing data should be preserved when store is re-initialized."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            # First init and insert data
            SQLiteTaskStore(db_path)
            conn = sqlite3.connect(db_path)
            conn.execute(
                """
                INSERT INTO c4_observations (id, project_id, source, content)
                VALUES (?, ?, ?, ?)
                """,
                ("obs-001", "test-project", "test", "Test content")
            )
            conn.commit()
            conn.close()

            # Re-initialize
            SQLiteTaskStore(db_path)

            # Data should still exist
            conn = sqlite3.connect(db_path)
            cursor = conn.execute(
                "SELECT content FROM c4_observations WHERE id = ?",
                ("obs-001",)
            )
            row = cursor.fetchone()
            conn.close()

            assert row is not None
            assert row[0] == "Test content"
