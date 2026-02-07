"""Knowledge Embeddings - Semantic search for experiment knowledge.

Bridges embedding infrastructure with the Knowledge Store,
enabling vector-based similarity search over experiment records.

Uses:
- c4.knowledge.embeddings_provider: EmbeddingProvider (OpenAI, local, mock)
- c4.knowledge.vector_store: VectorStore (sqlite-vec)
"""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any

from c4.knowledge.embeddings_provider import get_embeddings_provider

logger = logging.getLogger(__name__)


def _experiment_to_text(exp: dict[str, Any]) -> str:
    """Convert an experiment dict to searchable text for embedding.

    Combines title, hypothesis, lessons, tags, and domain into
    a single text representation for embedding generation.
    """
    parts = []
    if exp.get("title"):
        parts.append(exp["title"])
    if exp.get("hypothesis"):
        parts.append(exp["hypothesis"])
    if exp.get("domain"):
        parts.append(f"domain: {exp['domain']}")
    for lesson in exp.get("lessons_learned", []):
        parts.append(lesson)
    for tag in exp.get("tags", []):
        parts.append(tag)
    return " | ".join(parts) if parts else ""


class ExperimentEmbedder:
    """Manages embeddings for experiment knowledge records.

    Wraps the c4.memory embedding and vector store infrastructure
    to provide semantic search over experiments.

    Args:
        base_path: Directory for the vector store DB file.
        embedding_model: Provider name ('openai', 'local', 'mock').
            Defaults to config.experiments.embedding_model.
    """

    def __init__(
        self,
        base_path: str | Path = ".c4/knowledge",
        embedding_model: str = "mock",
    ) -> None:
        self.base_path = Path(base_path)
        self.base_path.mkdir(parents=True, exist_ok=True)

        # Resolve provider name
        provider_name = embedding_model
        if embedding_model.startswith("text-embedding"):
            provider_name = "openai"
        elif embedding_model == "local":
            provider_name = "local"

        self._provider = get_embeddings_provider(provider_name)
        self._vector_store = None
        self._db_path = self.base_path / "vectors.db"

    def _get_vector_store(self):
        """Lazy-init vector store (requires sqlite-vec)."""
        if self._vector_store is None:
            try:
                from c4.knowledge.vector_store import VectorStore

                self._vector_store = VectorStore(
                    db_path=str(self._db_path),
                    dimension=self._provider.dimension,
                    table_name="knowledge_embeddings",
                )
            except ImportError:
                logger.warning(
                    "sqlite-vec not available. Semantic search disabled. "
                    "Install with: uv add sqlite-vec"
                )
                return None
        return self._vector_store

    def index_experiment(self, exp_id: str, experiment: dict[str, Any]) -> bool:
        """Generate and store embedding for an experiment.

        Args:
            exp_id: Experiment ID.
            experiment: Experiment dict.

        Returns:
            True if indexed successfully, False if vector store unavailable.
        """
        store = self._get_vector_store()
        if store is None:
            return False

        text = _experiment_to_text(experiment)
        if not text:
            logger.debug("Skipping empty experiment text for %s", exp_id)
            return False

        try:
            # Remove existing if updating
            if store.exists(exp_id):
                store.delete(exp_id)

            embedding = self._provider.embed(text)
            store.add(exp_id, embedding)
            logger.debug("Indexed experiment %s (%d dims)", exp_id, len(embedding))
            return True
        except Exception:
            logger.exception("Failed to index experiment %s", exp_id)
            return False

    def search_similar(
        self,
        query: str,
        top_k: int = 5,
    ) -> list[dict[str, Any]]:
        """Search for experiments semantically similar to query.

        Args:
            query: Natural language search query.
            top_k: Maximum results to return.

        Returns:
            List of dicts with 'id', 'score', 'distance'.
        """
        store = self._get_vector_store()
        if store is None:
            return []

        try:
            query_embedding = self._provider.embed(query)
            results = store.search(query_embedding, limit=top_k)
            return [
                {"id": r.id, "score": r.score, "distance": r.distance}
                for r in results
            ]
        except Exception:
            logger.exception("Semantic search failed for query: %s", query)
            return []

    def remove(self, exp_id: str) -> bool:
        """Remove experiment embedding from the index.

        Args:
            exp_id: Experiment ID to remove.

        Returns:
            True if removed, False if not found or store unavailable.
        """
        store = self._get_vector_store()
        if store is None:
            return False
        return store.delete(exp_id)

    @property
    def count(self) -> int:
        """Number of indexed experiments."""
        store = self._get_vector_store()
        if store is None:
            return 0
        return store.count()

    def close(self) -> None:
        """Close the vector store connection."""
        if self._vector_store is not None:
            self._vector_store.close()
            self._vector_store = None
