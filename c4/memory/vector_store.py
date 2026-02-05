"""Vector store implementation using sqlite-vec for embedding storage.

This module provides efficient vector storage and similarity search
using SQLite with the sqlite-vec extension.

Usage:
    from c4.memory.vector_store import VectorStore

    store = VectorStore(db_path="memory.db", dimension=384)
    store.add("doc1", [0.1, 0.2, ...])  # Add embedding
    results = store.search(query_embedding, limit=10)  # KNN search
    store.delete("doc1")  # Remove embedding
"""

import json
import sqlite3
from dataclasses import dataclass
from pathlib import Path

import sqlite_vec


@dataclass
class SearchResult:
    """Result from a vector similarity search.

    Attributes:
        id: Unique identifier for the stored embedding
        distance: Distance from query (lower is more similar)
        score: Similarity score (higher is more similar, 1.0 = identical)
    """

    id: str
    distance: float
    score: float


class VectorStore:
    """Vector store using sqlite-vec for efficient similarity search.

    Stores embeddings in a SQLite database with the sqlite-vec extension,
    enabling fast approximate nearest neighbor search.

    Attributes:
        db_path: Path to the SQLite database
        dimension: Dimension of embeddings to store
        table_name: Name of the vector table (default: c4_embeddings)

    Example:
        >>> store = VectorStore(":memory:", dimension=4)
        >>> store.add("doc1", [1.0, 0.0, 0.0, 0.0])
        >>> store.add("doc2", [0.0, 1.0, 0.0, 0.0])
        >>> results = store.search([0.9, 0.1, 0.0, 0.0], limit=1)
        >>> results[0].id
        'doc1'
    """

    def __init__(
        self,
        db_path: str | Path = ":memory:",
        dimension: int = 384,
        table_name: str = "c4_embeddings",
    ) -> None:
        """Initialize the vector store.

        Args:
            db_path: Path to SQLite database. Use ":memory:" for in-memory.
            dimension: Dimension of embedding vectors.
            table_name: Name for the vector table.
        """
        self.db_path = str(db_path)
        self.dimension = dimension
        self.table_name = table_name
        self._conn = None

        # Initialize database
        self._setup_database()

    def _get_connection(self) -> sqlite3.Connection:
        """Get or create database connection with sqlite-vec loaded."""
        if self._conn is None:
            self._conn = sqlite3.connect(self.db_path)
            self._conn.enable_load_extension(True)
            sqlite_vec.load(self._conn)
            self._conn.enable_load_extension(False)
        return self._conn

    def _setup_database(self) -> None:
        """Create the vector table if it doesn't exist."""
        conn = self._get_connection()

        # Create ID mapping table
        conn.execute(
            """
            CREATE TABLE IF NOT EXISTS c4_embedding_ids (
                rowid INTEGER PRIMARY KEY AUTOINCREMENT,
                external_id TEXT UNIQUE NOT NULL,
                created_at TEXT DEFAULT CURRENT_TIMESTAMP
            )
            """
        )

        # Create vector table using vec0
        conn.execute(
            f"""
            CREATE VIRTUAL TABLE IF NOT EXISTS {self.table_name}
            USING vec0(embedding float[{self.dimension}])
            """
        )

        conn.commit()

    def add(self, id: str, embedding: list[float]) -> None:
        """Add an embedding to the store.

        Args:
            id: Unique identifier for this embedding.
            embedding: Vector of floats with length matching store dimension.

        Raises:
            ValueError: If embedding dimension doesn't match store dimension.
            sqlite3.IntegrityError: If id already exists.

        Example:
            >>> store = VectorStore(":memory:", dimension=3)
            >>> store.add("doc1", [1.0, 0.0, 0.0])
        """
        if len(embedding) != self.dimension:
            raise ValueError(
                f"Embedding dimension {len(embedding)} doesn't match "
                f"store dimension {self.dimension}"
            )

        conn = self._get_connection()
        cursor = conn.cursor()

        # Insert ID mapping
        cursor.execute(
            "INSERT INTO c4_embedding_ids (external_id) VALUES (?)",
            (id,)
        )
        rowid = cursor.lastrowid

        # Insert embedding
        embedding_json = json.dumps(embedding)
        cursor.execute(
            f"INSERT INTO {self.table_name} (rowid, embedding) VALUES (?, ?)",
            (rowid, embedding_json)
        )

        conn.commit()

    def add_batch(self, items: list[tuple[str, list[float]]]) -> None:
        """Add multiple embeddings in a batch.

        Args:
            items: List of (id, embedding) tuples.

        Raises:
            ValueError: If any embedding dimension doesn't match.
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        for id, embedding in items:
            if len(embedding) != self.dimension:
                raise ValueError(
                    f"Embedding dimension {len(embedding)} doesn't match "
                    f"store dimension {self.dimension}"
                )

            cursor.execute(
                "INSERT INTO c4_embedding_ids (external_id) VALUES (?)",
                (id,)
            )
            rowid = cursor.lastrowid

            embedding_json = json.dumps(embedding)
            cursor.execute(
                f"INSERT INTO {self.table_name} (rowid, embedding) VALUES (?, ?)",
                (rowid, embedding_json)
            )

        conn.commit()

    def search(
        self,
        query_embedding: list[float],
        limit: int = 10,
        threshold: float | None = None,
    ) -> list[SearchResult]:
        """Search for similar embeddings using KNN.

        Args:
            query_embedding: Query vector to find similar embeddings.
            limit: Maximum number of results to return.
            threshold: Optional distance threshold (only return results
                with distance <= threshold).

        Returns:
            List of SearchResult ordered by similarity (most similar first).

        Example:
            >>> store = VectorStore(":memory:", dimension=3)
            >>> store.add("doc1", [1.0, 0.0, 0.0])
            >>> store.add("doc2", [0.0, 1.0, 0.0])
            >>> results = store.search([0.9, 0.1, 0.0], limit=2)
            >>> [r.id for r in results]
            ['doc1', 'doc2']
        """
        if len(query_embedding) != self.dimension:
            raise ValueError(
                f"Query dimension {len(query_embedding)} doesn't match "
                f"store dimension {self.dimension}"
            )

        conn = self._get_connection()
        cursor = conn.cursor()

        # Check if store is empty first
        cursor.execute("SELECT COUNT(*) FROM c4_embedding_ids")
        if cursor.fetchone()[0] == 0:
            return []

        query_json = json.dumps(query_embedding)

        # KNN query using sqlite-vec syntax (k = ? instead of LIMIT)
        cursor.execute(
            f"""
            SELECT
                ids.external_id,
                vec.distance
            FROM {self.table_name} AS vec
            JOIN c4_embedding_ids AS ids ON vec.rowid = ids.rowid
            WHERE vec.embedding MATCH ? AND k = ?
            """,
            (query_json, limit)
        )

        results = []
        for external_id, distance in cursor.fetchall():
            # Apply threshold filter if specified
            if threshold is not None and distance > threshold:
                continue

            # Convert distance to similarity score (1 / (1 + distance))
            score = 1.0 / (1.0 + distance)

            results.append(SearchResult(
                id=external_id,
                distance=distance,
                score=score
            ))

        return results

    def delete(self, id: str) -> bool:
        """Delete an embedding by its ID.

        Args:
            id: The unique identifier of the embedding to delete.

        Returns:
            True if the embedding was deleted, False if not found.

        Example:
            >>> store = VectorStore(":memory:", dimension=3)
            >>> store.add("doc1", [1.0, 0.0, 0.0])
            >>> store.delete("doc1")
            True
            >>> store.delete("nonexistent")
            False
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        # Get rowid for external_id
        cursor.execute(
            "SELECT rowid FROM c4_embedding_ids WHERE external_id = ?",
            (id,)
        )
        row = cursor.fetchone()

        if row is None:
            return False

        rowid = row[0]

        # Delete from vector table
        cursor.execute(
            f"DELETE FROM {self.table_name} WHERE rowid = ?",
            (rowid,)
        )

        # Delete from ID mapping
        cursor.execute(
            "DELETE FROM c4_embedding_ids WHERE rowid = ?",
            (rowid,)
        )

        conn.commit()
        return True

    def count(self) -> int:
        """Return the number of embeddings in the store.

        Returns:
            Total count of stored embeddings.
        """
        conn = self._get_connection()
        cursor = conn.cursor()
        cursor.execute("SELECT COUNT(*) FROM c4_embedding_ids")
        return cursor.fetchone()[0]

    def exists(self, id: str) -> bool:
        """Check if an embedding with the given ID exists.

        Args:
            id: The unique identifier to check.

        Returns:
            True if the embedding exists, False otherwise.
        """
        conn = self._get_connection()
        cursor = conn.cursor()
        cursor.execute(
            "SELECT 1 FROM c4_embedding_ids WHERE external_id = ?",
            (id,)
        )
        return cursor.fetchone() is not None

    def close(self) -> None:
        """Close the database connection."""
        if self._conn is not None:
            self._conn.close()
            self._conn = None

    def __enter__(self):
        """Context manager entry."""
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit - close connection."""
        self.close()
        return False
