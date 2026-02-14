"""Knowledge Embeddings - Semantic search for knowledge documents.

Bridges embedding infrastructure with the Knowledge Store,
enabling vector-based similarity search over all document types.

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


def _document_to_text(doc: dict[str, Any]) -> str:
    """Convert a knowledge document dict to searchable text for embedding.

    Handles all 4 document types: experiment, pattern, insight, hypothesis.
    """
    parts = []
    if doc.get("title"):
        parts.append(doc["title"])
    if doc.get("hypothesis"):
        parts.append(doc["hypothesis"])
    if doc.get("description"):
        parts.append(doc["description"])
    if doc.get("domain"):
        parts.append(f"domain: {doc['domain']}")
    for lesson in doc.get("lessons_learned", []):
        parts.append(lesson)
    for tag in doc.get("tags", []):
        parts.append(tag)
    if doc.get("body"):
        parts.append(doc["body"][:500])  # Limit body to avoid huge embeddings
    if doc.get("insight_type"):
        parts.append(f"type: {doc['insight_type']}")
    if doc.get("status"):
        parts.append(f"status: {doc['status']}")
    return " | ".join(parts) if parts else ""


# Backward compat alias
_experiment_to_text = _document_to_text


class KnowledgeEmbedder:
    """Manages embeddings for all knowledge document types.

    Wraps embedding and vector store infrastructure to provide
    semantic search over experiments, patterns, insights, and hypotheses.

    Args:
        base_path: Directory for the vector store DB file.
        embedding_model: Provider name ('openai', 'local', 'mock').
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

    def index_document(self, doc_id: str, document: dict[str, Any]) -> bool:
        """Generate and store embedding for a knowledge document.

        Args:
            doc_id: Document ID (exp-xxx, pat-xxx, ins-xxx, hyp-xxx).
            document: Document dict with title, body, tags, etc.

        Returns:
            True if indexed successfully, False if vector store unavailable.
        """
        store = self._get_vector_store()
        if store is None:
            return False

        text = _document_to_text(document)
        if not text:
            logger.debug("Skipping empty document text for %s", doc_id)
            return False

        try:
            if store.exists(doc_id):
                store.delete(doc_id)

            embedding = self._provider.embed(text)
            store.add(doc_id, embedding)
            logger.debug("Indexed document %s (%d dims)", doc_id, len(embedding))
            return True
        except Exception:
            logger.exception("Failed to index document %s", doc_id)
            return False

    # Backward compat alias
    def index_experiment(self, exp_id: str, experiment: dict[str, Any]) -> bool:
        """Backward-compatible alias for index_document."""
        return self.index_document(exp_id, experiment)

    def search_similar(
        self,
        query: str,
        top_k: int = 5,
    ) -> list[dict[str, Any]]:
        """Search for documents semantically similar to query.

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

    def remove(self, doc_id: str) -> bool:
        """Remove document embedding from the index."""
        store = self._get_vector_store()
        if store is None:
            return False
        return store.delete(doc_id)

    @property
    def count(self) -> int:
        """Number of indexed documents."""
        store = self._get_vector_store()
        if store is None:
            return 0
        return store.count()

    def close(self) -> None:
        """Close the vector store connection."""
        if self._vector_store is not None:
            if hasattr(self._vector_store, 'close'):
                self._vector_store.close()
            self._vector_store = None

    def __del__(self) -> None:
        """Cleanup on garbage collection (fallback)."""
        try:
            self.close()
        except Exception:
            pass  # Suppress exceptions during cleanup

    def __enter__(self):
        """Context manager entry."""
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit."""
        self.close()
        return False


# Backward compat: ExperimentEmbedder = KnowledgeEmbedder
ExperimentEmbedder = KnowledgeEmbedder
