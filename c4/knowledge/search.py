"""KnowledgeSearcher - 2-way RRF hybrid search (Vector + FTS5).

Combines semantic vector search with keyword FTS5 search using
Reciprocal Rank Fusion (RRF) for optimal retrieval quality.

Usage:
    from c4.knowledge.search import KnowledgeSearcher

    searcher = KnowledgeSearcher(base_path=".c4/knowledge")
    results = searcher.search("random forest accuracy", top_k=10)
"""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any

from .documents import DocumentStore
from .embeddings import KnowledgeEmbedder

logger = logging.getLogger(__name__)


def rrf_merge(
    *result_lists: list[dict[str, Any]],
    k: int = 60,
) -> list[dict[str, Any]]:
    """Reciprocal Rank Fusion merge of multiple ranked result lists.

    RRF score = sum(1 / (k + rank_i)) for each list where doc appears.

    Args:
        *result_lists: Each list contains dicts with at least 'id' key.
        k: RRF constant (default 60, standard value from literature).

    Returns:
        Merged list sorted by RRF score descending.
    """
    scores: dict[str, float] = {}
    docs: dict[str, dict[str, Any]] = {}

    for results in result_lists:
        for rank, doc in enumerate(results):
            doc_id = doc["id"]
            scores[doc_id] = scores.get(doc_id, 0.0) + 1.0 / (k + rank + 1)
            if doc_id not in docs:
                docs[doc_id] = doc

    # Sort by RRF score descending
    sorted_ids = sorted(scores, key=lambda x: scores[x], reverse=True)

    merged = []
    for doc_id in sorted_ids:
        entry = dict(docs[doc_id])
        entry["rrf_score"] = scores[doc_id]
        merged.append(entry)

    return merged


class KnowledgeSearcher:
    """Hybrid search combining vector similarity and FTS5 keyword search.

    Uses 2-way RRF (Reciprocal Rank Fusion) to merge results from:
    1. Vector search (semantic similarity via embeddings)
    2. FTS5 keyword search (BM25-based)

    Args:
        base_path: Path to knowledge store directory.
        embedding_model: Embedding provider ('openai', 'local', 'mock').
    """

    def __init__(
        self,
        base_path: str | Path = ".c4/knowledge",
        embedding_model: str = "mock",
    ) -> None:
        self.base_path = Path(base_path)
        self._doc_store = DocumentStore(base_path=self.base_path)
        self._embedder = KnowledgeEmbedder(
            base_path=self.base_path,
            embedding_model=embedding_model,
        )

    def search(
        self,
        query: str,
        top_k: int = 10,
        filters: dict[str, str] | None = None,
    ) -> list[dict[str, Any]]:
        """Hybrid search with optional metadata filters.

        Args:
            query: Natural language search query.
            top_k: Maximum results to return.
            filters: Optional filters (type, domain, hypothesis_status).

        Returns:
            List of results with id, title, type, rrf_score, etc.
        """
        fetch_k = top_k * 2  # Over-fetch for better RRF merge

        # 1. Vector search (semantic)
        vector_results = self._embedder.search_similar(query, top_k=fetch_k)

        # 2. FTS5 keyword search
        fts_results = self._doc_store.search_fts(query, top_k=fetch_k)

        # 3. RRF merge
        merged = rrf_merge(vector_results, fts_results, k=60)

        # 4. Enrich results with metadata and apply filters
        merged = self._enrich_and_filter(merged, filters)

        return merged[:top_k]

    def search_by_type(
        self,
        query: str,
        doc_type: str,
        top_k: int = 10,
    ) -> list[dict[str, Any]]:
        """Search filtered by document type.

        Convenience method for type-filtered search.
        """
        return self.search(query, top_k=top_k, filters={"type": doc_type})

    def _enrich_and_filter(
        self,
        results: list[dict[str, Any]],
        filters: dict[str, str] | None,
    ) -> list[dict[str, Any]]:
        """Enrich results with metadata and optionally filter.

        Always adds title/type/domain to results.
        Loads document metadata once via dict lookup (O(m+n)).
        """
        all_docs = {d["id"]: d for d in self._doc_store.list_documents(limit=10000)}

        enriched = []
        for result in results:
            doc_meta = all_docs.get(result["id"])
            if doc_meta is None:
                continue

            # Always enrich with metadata
            result["title"] = doc_meta.get("title", result.get("title", ""))
            result["type"] = doc_meta.get("type", result.get("type", ""))
            result["domain"] = doc_meta.get("domain", "")

            # Apply filters if provided
            if filters:
                match = all(
                    doc_meta.get(key, "") == value
                    for key, value in filters.items()
                )
                if not match:
                    continue

            enriched.append(result)

        return enriched

    def close(self) -> None:
        """Close resources."""
        self._doc_store.close()
        self._embedder.close()
