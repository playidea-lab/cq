"""Auto capture module for automatic observation collection.

This module provides automatic capture and storage of tool outputs,
creating observations that can be searched semantically.

Usage:
    from c4.memory.auto_capture import AutoCaptureHandler, get_auto_capture_handler

    handler = get_auto_capture_handler(project_id, db_path)

    # Capture tool output
    observation = handler.capture_tool_output(
        tool_name="read_file",
        input_data={"path": "/src/main.py"},
        output="def main(): ..."
    )

    # Store observation (returns observation_id)
    obs_id = handler.store_observation(observation)

    # Create embedding for semantic search
    handler.create_embedding(observation)
"""

import json
import logging
import sqlite3
import uuid
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


@dataclass
class Observation:
    """A captured observation from tool output or user interaction.

    Observations are stored in the c4_observations table and can be
    indexed for semantic search.

    Attributes:
        id: Unique identifier for the observation
        project_id: The project this observation belongs to
        source: Where the observation came from (tool name, "user", "file", etc.)
        content: The actual text content of the observation
        importance: Importance score from 1-10 (higher = more important)
        tags: List of tags for categorization
        metadata: Additional metadata as a dictionary
        created_at: When the observation was created
    """

    id: str
    project_id: str
    source: str
    content: str
    importance: int = 5
    tags: list[str] = field(default_factory=list)
    metadata: dict[str, Any] = field(default_factory=dict)
    created_at: datetime = field(default_factory=datetime.now)

    def to_dict(self) -> dict[str, Any]:
        """Convert observation to dictionary for storage.

        Returns:
            Dictionary representation suitable for SQLite storage.
        """
        return {
            "id": self.id,
            "project_id": self.project_id,
            "source": self.source,
            "content": self.content,
            "importance": self.importance,
            "tags": json.dumps(self.tags),
            "metadata": json.dumps(self.metadata),
            "created_at": self.created_at.isoformat(),
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Observation":
        """Create an Observation from a dictionary.

        Args:
            data: Dictionary with observation data.

        Returns:
            Observation instance.
        """
        tags = data.get("tags", "[]")
        if isinstance(tags, str):
            tags = json.loads(tags)

        metadata = data.get("metadata", "{}")
        if isinstance(metadata, str):
            metadata = json.loads(metadata)

        created_at = data.get("created_at")
        if isinstance(created_at, str):
            created_at = datetime.fromisoformat(created_at)
        elif created_at is None:
            created_at = datetime.now()

        return cls(
            id=data["id"],
            project_id=data["project_id"],
            source=data["source"],
            content=data["content"],
            importance=data.get("importance", 5),
            tags=tags,
            metadata=metadata,
            created_at=created_at,
        )


# Importance levels for different tool outputs
TOOL_IMPORTANCE: dict[str, int] = {
    # High importance - user actions and key operations
    "user_message": 9,
    "file_write": 8,
    "git_commit": 8,
    "test_result": 8,
    # Medium-high importance - code analysis
    "find_symbol": 7,
    "get_symbols_overview": 7,
    "read_file": 6,
    "search_for_pattern": 6,
    # Medium importance - navigation
    "list_dir": 5,
    "find_file": 5,
    # Lower importance - metadata
    "get_current_config": 4,
    "status": 4,
    # Default
    "default": 5,
}


class AutoCaptureHandler:
    """Handler for automatic capture of tool outputs.

    Captures tool outputs, stores them as observations, and creates
    embeddings for semantic search.

    Attributes:
        project_id: The project ID for observations
        db_path: Path to the SQLite database
        embedding_provider: Provider for generating embeddings (optional)
        vector_store: Vector store for storing embeddings (optional)
    """

    def __init__(
        self,
        project_id: str,
        db_path: str | Path,
        embedding_provider: Any = None,
        vector_store: Any = None,
    ) -> None:
        """Initialize the auto capture handler.

        Args:
            project_id: The project ID for observations.
            db_path: Path to the SQLite database.
            embedding_provider: Optional embedding provider for vectors.
            vector_store: Optional vector store for semantic search.
        """
        self.project_id = project_id
        self.db_path = Path(db_path)
        self._embedding_provider = embedding_provider
        self._vector_store = vector_store
        self._ensure_tables()

    def _get_connection(self) -> sqlite3.Connection:
        """Get a database connection."""
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        conn = sqlite3.connect(
            self.db_path,
            detect_types=sqlite3.PARSE_DECLTYPES | sqlite3.PARSE_COLNAMES,
        )
        conn.row_factory = sqlite3.Row
        return conn

    def _ensure_tables(self) -> None:
        """Ensure required tables exist."""
        conn = self._get_connection()
        try:
            # c4_observations table
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
            # c4_memory_index table
            conn.execute("""
                CREATE TABLE IF NOT EXISTS c4_memory_index (
                    id TEXT PRIMARY KEY,
                    project_id TEXT NOT NULL,
                    observation_id TEXT NOT NULL REFERENCES c4_observations(id),
                    embedding_id TEXT NOT NULL,
                    chunk_index INTEGER DEFAULT 0,
                    chunk_text TEXT,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    UNIQUE (observation_id, chunk_index)
                )
            """)
            conn.commit()
        finally:
            conn.close()

    def capture_tool_output(
        self,
        tool_name: str,
        input_data: dict[str, Any] | str | None,
        output: str | dict[str, Any],
        importance: int | None = None,
        tags: list[str] | None = None,
    ) -> Observation:
        """Capture a tool output as an observation.

        Creates an Observation from tool execution results without
        storing it yet (call store_observation to persist).

        Args:
            tool_name: Name of the tool that was executed.
            input_data: Input parameters passed to the tool.
            output: Output from the tool execution.
            importance: Optional importance override (1-10).
            tags: Optional tags for categorization.

        Returns:
            Observation instance ready for storage.

        Example:
            >>> obs = handler.capture_tool_output(
            ...     "read_file",
            ...     {"path": "src/main.py"},
            ...     "def main(): pass"
            ... )
            >>> obs.source
            'read_file'
        """
        # Generate unique ID
        obs_id = f"obs-{uuid.uuid4().hex[:12]}"

        # Determine importance
        if importance is None:
            importance = TOOL_IMPORTANCE.get(tool_name, TOOL_IMPORTANCE["default"])

        # Prepare content
        if isinstance(output, dict):
            content = json.dumps(output, indent=2)
        else:
            content = str(output)

        # Prepare metadata
        metadata: dict[str, Any] = {
            "tool_name": tool_name,
        }
        if input_data is not None:
            if isinstance(input_data, str):
                metadata["input"] = input_data
            else:
                metadata["input"] = input_data

        # Default tags
        if tags is None:
            tags = [f"tool:{tool_name}"]
        else:
            tags = list(tags)  # Copy to avoid mutation
            if f"tool:{tool_name}" not in tags:
                tags.append(f"tool:{tool_name}")

        return Observation(
            id=obs_id,
            project_id=self.project_id,
            source=tool_name,
            content=content,
            importance=importance,
            tags=tags,
            metadata=metadata,
            created_at=datetime.now(),
        )

    def store_observation(self, observation: Observation) -> str:
        """Store an observation in the database.

        Args:
            observation: The observation to store.

        Returns:
            The observation ID.

        Raises:
            sqlite3.IntegrityError: If an observation with the same ID exists.
        """
        conn = self._get_connection()
        try:
            data = observation.to_dict()
            conn.execute(
                """
                INSERT INTO c4_observations
                (id, project_id, source, content, importance, tags, metadata, created_at)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    data["id"],
                    data["project_id"],
                    data["source"],
                    data["content"],
                    data["importance"],
                    data["tags"],
                    data["metadata"],
                    data["created_at"],
                ),
            )
            conn.commit()
            logger.debug(f"Stored observation {observation.id}")
            return observation.id
        finally:
            conn.close()

    def create_embedding(self, observation: Observation) -> None:
        """Create an embedding for an observation and store it.

        Generates an embedding vector from the observation content
        and stores it in the vector store for semantic search.

        Args:
            observation: The observation to create an embedding for.

        Note:
            Requires embedding_provider and vector_store to be configured.
            If not configured, this is a no-op with a warning.
        """
        if self._embedding_provider is None or self._vector_store is None:
            logger.warning(
                f"Cannot create embedding for {observation.id}: "
                "embedding_provider or vector_store not configured"
            )
            return

        try:
            # Generate embedding
            embedding = self._embedding_provider.embed(observation.content)

            # Store in vector store
            embedding_id = f"emb-{observation.id}"
            self._vector_store.add(embedding_id, embedding)

            # Create memory index entry
            self._store_memory_index(
                observation_id=observation.id,
                embedding_id=embedding_id,
                chunk_index=0,
                chunk_text=observation.content[:500],  # First 500 chars as preview
            )

            logger.debug(f"Created embedding {embedding_id} for observation {observation.id}")
        except Exception as e:
            logger.error(f"Failed to create embedding for {observation.id}: {e}")

    def _store_memory_index(
        self,
        observation_id: str,
        embedding_id: str,
        chunk_index: int,
        chunk_text: str | None,
    ) -> str:
        """Store a memory index entry linking observation to embedding.

        Args:
            observation_id: The observation ID.
            embedding_id: The embedding ID in the vector store.
            chunk_index: The chunk index (0 for single-chunk observations).
            chunk_text: Optional preview text of the chunk.

        Returns:
            The memory index entry ID.
        """
        index_id = f"idx-{uuid.uuid4().hex[:8]}"

        conn = self._get_connection()
        try:
            conn.execute(
                """
                INSERT INTO c4_memory_index
                (id, project_id, observation_id, embedding_id, chunk_index, chunk_text)
                VALUES (?, ?, ?, ?, ?, ?)
                """,
                (
                    index_id,
                    self.project_id,
                    observation_id,
                    embedding_id,
                    chunk_index,
                    chunk_text,
                ),
            )
            conn.commit()
            return index_id
        finally:
            conn.close()

    def get_observation(self, observation_id: str) -> Observation | None:
        """Retrieve an observation by ID.

        Args:
            observation_id: The observation ID to retrieve.

        Returns:
            The Observation if found, None otherwise.
        """
        conn = self._get_connection()
        try:
            cursor = conn.execute(
                """
                SELECT id, project_id, source, content, importance, tags, metadata, created_at
                FROM c4_observations
                WHERE id = ? AND project_id = ?
                """,
                (observation_id, self.project_id),
            )
            row = cursor.fetchone()
            if row is None:
                return None
            return Observation.from_dict(dict(row))
        finally:
            conn.close()

    def list_observations(
        self,
        source: str | None = None,
        min_importance: int | None = None,
        limit: int = 100,
    ) -> list[Observation]:
        """List observations with optional filters.

        Args:
            source: Filter by source (tool name).
            min_importance: Filter by minimum importance.
            limit: Maximum number of observations to return.

        Returns:
            List of matching observations.
        """
        conn = self._get_connection()
        try:
            query = "SELECT * FROM c4_observations WHERE project_id = ?"
            params: list[Any] = [self.project_id]

            if source is not None:
                query += " AND source = ?"
                params.append(source)

            if min_importance is not None:
                query += " AND importance >= ?"
                params.append(min_importance)

            query += " ORDER BY created_at DESC LIMIT ?"
            params.append(limit)

            cursor = conn.execute(query, params)
            return [Observation.from_dict(dict(row)) for row in cursor.fetchall()]
        finally:
            conn.close()


def get_auto_capture_handler(
    project_id: str,
    db_path: str | Path,
    enable_embeddings: bool = False,
    embedding_dimension: int = 384,
) -> AutoCaptureHandler:
    """Factory function to create an AutoCaptureHandler.

    Args:
        project_id: The project ID for observations.
        db_path: Path to the SQLite database.
        enable_embeddings: Whether to enable embedding generation.
        embedding_dimension: Dimension for embeddings (if enabled).

    Returns:
        Configured AutoCaptureHandler instance.

    Example:
        >>> handler = get_auto_capture_handler("my-project", ".c4/tasks.db")
        >>> obs = handler.capture_tool_output("read_file", {"path": "x"}, "content")
        >>> handler.store_observation(obs)
    """
    embedding_provider = None
    vector_store = None

    if enable_embeddings:
        try:
            from .embeddings import get_embeddings_provider
            from .vector_store import VectorStore

            embedding_provider = get_embeddings_provider()
            vector_store = VectorStore(
                db_path=db_path,
                dimension=embedding_provider.dimension,
            )
        except Exception as e:
            logger.warning(f"Failed to initialize embeddings: {e}")

    return AutoCaptureHandler(
        project_id=project_id,
        db_path=db_path,
        embedding_provider=embedding_provider,
        vector_store=vector_store,
    )
