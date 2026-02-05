"""Hybrid search module for memory retrieval.

This module provides semantic search capabilities combining vector similarity
search (using sqlite-vec) with keyword-based search and Reciprocal Rank Fusion
(RRF) for optimal results.

Usage:
    from c4.memory.search import MemorySearcher, get_memory_searcher

    searcher = get_memory_searcher(project_id, db_path)
    results = searcher.search("authentication flow", limit=10)

    for result in results:
        print(f"{result.id}: {result.preview} (score: {result.score})")
"""

import logging
import sqlite3
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


# RRF constant - typically 60 for good balance
RRF_K = 60


@dataclass
class MemorySearchResult:
    """A search result from memory search.

    Attributes:
        id: The observation ID
        title: Short title or source of the observation
        preview: Preview text (truncated content)
        tokens: Estimated token count of full content
        score: Relevance score (higher = more relevant)
        source: The source of the observation (tool name, etc.)
        importance: Importance level (1-10)
        created_at: When the observation was created
    """

    id: str
    title: str
    preview: str
    tokens: int
    score: float
    source: str = ""
    importance: int = 5
    created_at: datetime | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert result to dictionary."""
        return {
            "id": self.id,
            "title": self.title,
            "preview": self.preview,
            "tokens": self.tokens,
            "score": self.score,
            "source": self.source,
            "importance": self.importance,
            "created_at": self.created_at.isoformat() if self.created_at else None,
        }


@dataclass
class SearchFilters:
    """Filters for memory search.

    Attributes:
        memory_type: Filter by source/memory type (e.g., "read_file", "user_message")
        tags: Filter by tags (observation must have at least one matching tag)
        since: Only return observations created after this datetime
        min_importance: Minimum importance level (1-10)
    """

    memory_type: str | None = None
    tags: list[str] | None = None
    since: datetime | None = None
    min_importance: int | None = None


class MemorySearcher:
    """Hybrid search combining vector similarity and keyword matching.

    Uses sqlite-vec for semantic (vector) search and FTS5/LIKE for keyword
    search, with Reciprocal Rank Fusion (RRF) to combine results.

    Attributes:
        project_id: The project to search within
        db_path: Path to the SQLite database
        embedding_provider: Provider for generating query embeddings
        vector_store: Vector store for similarity search
    """

    def __init__(
        self,
        project_id: str,
        db_path: str | Path,
        embedding_provider: Any = None,
        vector_store: Any = None,
    ) -> None:
        """Initialize the memory searcher.

        Args:
            project_id: The project to search within.
            db_path: Path to the SQLite database.
            embedding_provider: Optional embedding provider for vector search.
            vector_store: Optional vector store for similarity search.
        """
        self.project_id = project_id
        self.db_path = Path(db_path)
        self._embedding_provider = embedding_provider
        self._vector_store = vector_store

    def _get_connection(self) -> sqlite3.Connection:
        """Get a database connection."""
        conn = sqlite3.connect(
            self.db_path,
            detect_types=sqlite3.PARSE_DECLTYPES | sqlite3.PARSE_COLNAMES,
        )
        conn.row_factory = sqlite3.Row
        return conn

    def search(
        self,
        query: str,
        limit: int = 10,
        filters: SearchFilters | None = None,
    ) -> list[MemorySearchResult]:
        """Search memories using hybrid search.

        Combines vector similarity search with keyword search using
        Reciprocal Rank Fusion (RRF) for optimal results.

        Args:
            query: The search query.
            limit: Maximum number of results to return.
            filters: Optional filters to apply.

        Returns:
            List of MemorySearchResult ordered by relevance score.

        Example:
            >>> results = searcher.search("user authentication", limit=5)
            >>> for r in results:
            ...     print(f"{r.title}: {r.score}")
        """
        if not query:
            return []

        # Perform both search types
        vector_results = self._vector_search(query, limit * 2)  # Get more for fusion
        keyword_results = self._keyword_search(query, limit * 2, filters)

        # Merge using RRF
        merged = self._rrf_merge(vector_results, keyword_results)

        # Apply filters to merged results
        if filters:
            merged = self._apply_filters(merged, filters)

        return merged[:limit]

    def _vector_search(
        self, query: str, limit: int
    ) -> list[tuple[str, float]]:
        """Perform vector similarity search using sqlite-vec.

        Args:
            query: The search query.
            limit: Maximum number of results.

        Returns:
            List of (observation_id, score) tuples.
        """
        if self._embedding_provider is None or self._vector_store is None:
            logger.debug("Vector search skipped: no embedding provider or vector store")
            return []

        try:
            # Generate query embedding
            query_embedding = self._embedding_provider.embed(query)

            # Search vector store
            search_results = self._vector_store.search(query_embedding, limit=limit)

            # Map embedding IDs back to observation IDs
            result_pairs: list[tuple[str, float]] = []
            for result in search_results:
                # Embedding ID format: "emb-{observation_id}"
                if result.id.startswith("emb-"):
                    obs_id = result.id[4:]  # Remove "emb-" prefix
                    result_pairs.append((obs_id, result.score))

            return result_pairs
        except Exception as e:
            logger.error(f"Vector search failed: {e}")
            return []

    def _keyword_search(
        self,
        query: str,
        limit: int,
        filters: SearchFilters | None = None,
    ) -> list[tuple[str, float]]:
        """Perform keyword search using LIKE matching.

        Args:
            query: The search query.
            limit: Maximum number of results.
            filters: Optional filters.

        Returns:
            List of (observation_id, score) tuples.
        """
        conn = self._get_connection()
        try:
            # Build query with filters
            sql = """
                SELECT id, content, importance
                FROM c4_observations
                WHERE project_id = ?
                AND content LIKE ?
            """
            params: list[Any] = [self.project_id, f"%{query}%"]

            if filters:
                if filters.memory_type:
                    sql += " AND source = ?"
                    params.append(filters.memory_type)

                if filters.since:
                    sql += " AND created_at >= ?"
                    params.append(filters.since.isoformat())

                if filters.min_importance:
                    sql += " AND importance >= ?"
                    params.append(filters.min_importance)

                if filters.tags:
                    # Match any tag
                    tag_conditions = " OR ".join(
                        "tags LIKE ?" for _ in filters.tags
                    )
                    sql += f" AND ({tag_conditions})"
                    for tag in filters.tags:
                        params.append(f"%{tag}%")

            sql += " ORDER BY importance DESC, created_at DESC LIMIT ?"
            params.append(limit)

            cursor = conn.execute(sql, params)
            rows = cursor.fetchall()

            # Calculate simple keyword relevance score
            result_pairs: list[tuple[str, float]] = []
            for row in rows:
                obs_id = row["id"]
                content = row["content"].lower()
                query_lower = query.lower()

                # Score based on:
                # 1. Number of query word matches
                # 2. Position of match (earlier = better)
                # 3. Importance weight
                words = query_lower.split()
                word_matches = sum(1 for word in words if word in content)
                match_position = content.find(query_lower)

                # Normalize scores
                word_score = word_matches / max(len(words), 1)
                position_score = 1.0 / (1 + match_position) if match_position >= 0 else 0.1
                importance_weight = row["importance"] / 10.0

                score = (word_score * 0.5 + position_score * 0.3 + importance_weight * 0.2)
                result_pairs.append((obs_id, score))

            return result_pairs
        finally:
            conn.close()

    def _rrf_merge(
        self,
        vector_results: list[tuple[str, float]],
        keyword_results: list[tuple[str, float]],
    ) -> list[MemorySearchResult]:
        """Merge results using Reciprocal Rank Fusion.

        RRF formula: score(d) = sum(1 / (k + rank_i(d)))
        where k is a constant (typically 60) and rank_i is the rank in list i.

        Args:
            vector_results: Results from vector search as (id, score) tuples.
            keyword_results: Results from keyword search as (id, score) tuples.

        Returns:
            Merged and sorted list of MemorySearchResult.
        """
        # Calculate RRF scores
        rrf_scores: dict[str, float] = {}

        # Vector results contribution
        for rank, (obs_id, _) in enumerate(vector_results, start=1):
            rrf_scores[obs_id] = rrf_scores.get(obs_id, 0) + 1.0 / (RRF_K + rank)

        # Keyword results contribution
        for rank, (obs_id, _) in enumerate(keyword_results, start=1):
            rrf_scores[obs_id] = rrf_scores.get(obs_id, 0) + 1.0 / (RRF_K + rank)

        # Sort by RRF score
        sorted_ids = sorted(rrf_scores.keys(), key=lambda x: rrf_scores[x], reverse=True)

        # Fetch observation details
        results: list[MemorySearchResult] = []
        conn = self._get_connection()
        try:
            for obs_id in sorted_ids:
                cursor = conn.execute(
                    """
                    SELECT id, source, content, importance, tags, created_at
                    FROM c4_observations
                    WHERE id = ? AND project_id = ?
                    """,
                    (obs_id, self.project_id),
                )
                row = cursor.fetchone()
                if row:
                    content = row["content"]
                    results.append(
                        MemorySearchResult(
                            id=row["id"],
                            title=row["source"],
                            preview=content[:200] + "..." if len(content) > 200 else content,
                            tokens=self._estimate_tokens(content),
                            score=rrf_scores[obs_id],
                            source=row["source"],
                            importance=row["importance"],
                            created_at=self._parse_datetime(row["created_at"]),
                        )
                    )
        finally:
            conn.close()

        return results

    def _apply_filters(
        self,
        results: list[MemorySearchResult],
        filters: SearchFilters,
    ) -> list[MemorySearchResult]:
        """Apply filters to search results.

        Args:
            results: List of search results.
            filters: Filters to apply.

        Returns:
            Filtered list of results.
        """
        filtered = results

        if filters.memory_type:
            filtered = [r for r in filtered if r.source == filters.memory_type]

        if filters.min_importance:
            filtered = [r for r in filtered if r.importance >= filters.min_importance]

        if filters.since:
            filtered = [
                r for r in filtered
                if r.created_at and r.created_at >= filters.since
            ]

        # Tags filter would require additional DB lookup, skipped for post-filter
        # (already handled in keyword search)

        return filtered

    def _estimate_tokens(self, text: str) -> int:
        """Estimate token count for text.

        Args:
            text: The text to estimate.

        Returns:
            Estimated token count.
        """
        # Simple estimation: ~4 chars per token
        if not text:
            return 0
        return max(1, len(text) // 4)

    def _parse_datetime(self, value: Any) -> datetime | None:
        """Parse datetime from various formats.

        Args:
            value: The value to parse.

        Returns:
            Parsed datetime or None.
        """
        if value is None:
            return None
        if isinstance(value, datetime):
            return value
        if isinstance(value, str):
            try:
                return datetime.fromisoformat(value)
            except ValueError:
                return None
        return None

    def search_by_tags(
        self, tags: list[str], limit: int = 10
    ) -> list[MemorySearchResult]:
        """Search memories by tags.

        Args:
            tags: List of tags to search for (OR logic).
            limit: Maximum number of results.

        Returns:
            List of matching MemorySearchResult.
        """
        if not tags:
            return []

        conn = self._get_connection()
        try:
            # Build OR conditions for tags
            tag_conditions = " OR ".join("tags LIKE ?" for _ in tags)
            sql = f"""
                SELECT id, source, content, importance, tags, created_at
                FROM c4_observations
                WHERE project_id = ?
                AND ({tag_conditions})
                ORDER BY importance DESC, created_at DESC
                LIMIT ?
            """
            params = [self.project_id] + [f'%"{tag}"%' for tag in tags] + [limit]

            cursor = conn.execute(sql, params)
            rows = cursor.fetchall()

            results: list[MemorySearchResult] = []
            for row in rows:
                content = row["content"]
                results.append(
                    MemorySearchResult(
                        id=row["id"],
                        title=row["source"],
                        preview=content[:200] + "..." if len(content) > 200 else content,
                        tokens=self._estimate_tokens(content),
                        score=row["importance"] / 10.0,  # Normalize importance as score
                        source=row["source"],
                        importance=row["importance"],
                        created_at=self._parse_datetime(row["created_at"]),
                    )
                )

            return results
        finally:
            conn.close()

    def get_recent(
        self, limit: int = 10, source: str | None = None
    ) -> list[MemorySearchResult]:
        """Get recent observations.

        Args:
            limit: Maximum number of results.
            source: Optional filter by source.

        Returns:
            List of recent MemorySearchResult.
        """
        conn = self._get_connection()
        try:
            sql = """
                SELECT id, source, content, importance, tags, created_at
                FROM c4_observations
                WHERE project_id = ?
            """
            params: list[Any] = [self.project_id]

            if source:
                sql += " AND source = ?"
                params.append(source)

            sql += " ORDER BY created_at DESC LIMIT ?"
            params.append(limit)

            cursor = conn.execute(sql, params)
            rows = cursor.fetchall()

            results: list[MemorySearchResult] = []
            for row in rows:
                content = row["content"]
                results.append(
                    MemorySearchResult(
                        id=row["id"],
                        title=row["source"],
                        preview=content[:200] + "..." if len(content) > 200 else content,
                        tokens=self._estimate_tokens(content),
                        score=1.0,  # No score for recency query
                        source=row["source"],
                        importance=row["importance"],
                        created_at=self._parse_datetime(row["created_at"]),
                    )
                )

            return results
        finally:
            conn.close()


def get_memory_searcher(
    project_id: str,
    db_path: str | Path,
    enable_vector_search: bool = False,
) -> MemorySearcher:
    """Factory function to create a MemorySearcher.

    Args:
        project_id: The project to search within.
        db_path: Path to the SQLite database.
        enable_vector_search: Whether to enable vector (semantic) search.

    Returns:
        Configured MemorySearcher instance.

    Example:
        >>> searcher = get_memory_searcher("my-project", ".c4/tasks.db")
        >>> results = searcher.search("user login")
    """
    embedding_provider = None
    vector_store = None

    if enable_vector_search:
        try:
            from .embeddings import get_embeddings_provider
            from .vector_store import VectorStore

            embedding_provider = get_embeddings_provider()
            vector_store = VectorStore(
                db_path=db_path,
                dimension=embedding_provider.dimension,
            )
        except Exception as e:
            logger.warning(f"Failed to initialize vector search: {e}")

    return MemorySearcher(
        project_id=project_id,
        db_path=db_path,
        embedding_provider=embedding_provider,
        vector_store=vector_store,
    )
