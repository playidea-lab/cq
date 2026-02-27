"""Knowledge distillation - cluster similar documents by vector similarity.

Provides clustering of knowledge documents using cosine similarity
and connected components (Union-Find), matching the Go implementation
in c4-core/internal/knowledge/search.go.

Gracefully handles empty or small vector stores by returning a
skip result instead of crashing.
"""

from __future__ import annotations

import logging
import math
from typing import Any

from .vector_store import VectorStore

logger = logging.getLogger(__name__)

# Minimum vectors required for meaningful clustering
MIN_VECTORS_FOR_CLUSTERING = 5


def cosine_similarity(a: list[float], b: list[float]) -> float:
    """Compute cosine similarity between two vectors.

    Returns:
        Similarity in [-1, 1]. Higher means more similar.
    """
    dot = sum(x * y for x, y in zip(a, b))
    norm_a = math.sqrt(sum(x * x for x in a))
    norm_b = math.sqrt(sum(x * x for x in b))
    if norm_a == 0 or norm_b == 0:
        return 0.0
    return dot / (norm_a * norm_b)


def find_clusters(
    embeddings: dict[str, list[float]],
    threshold: float = 0.7,
    min_cluster_size: int = 3,
) -> list[list[str]]:
    """Group documents by vector similarity using connected components.

    Uses Union-Find to cluster documents whose pairwise cosine
    similarity >= threshold. Mirrors Go FindClusters logic.

    Args:
        embeddings: {doc_id: vector} dict.
        threshold: Minimum cosine similarity to link two docs.
        min_cluster_size: Minimum cluster size to return.

    Returns:
        List of clusters, each a list of doc IDs.
    """
    ids = sorted(embeddings.keys())
    n = len(ids)
    if n < min_cluster_size:
        return []

    vecs = [embeddings[doc_id] for doc_id in ids]

    # Union-Find
    parent = list(range(n))

    def find(x: int) -> int:
        while parent[x] != x:
            parent[x] = parent[parent[x]]  # path compression
            x = parent[x]
        return x

    def union(a: int, b: int) -> None:
        ra, rb = find(a), find(b)
        if ra != rb:
            parent[ra] = rb

    # Pairwise similarity check
    for i in range(n):
        for j in range(i + 1, n):
            if cosine_similarity(vecs[i], vecs[j]) >= threshold:
                union(i, j)

    # Collect clusters
    clusters_map: dict[int, list[str]] = {}
    for i in range(n):
        root = find(i)
        clusters_map.setdefault(root, []).append(ids[i])

    # Filter by min_cluster_size
    return [c for c in clusters_map.values() if len(c) >= min_cluster_size]


def distill(
    vector_store: VectorStore | None,
    threshold: float = 0.7,
    min_cluster: int = 3,
) -> dict[str, Any]:
    """Run knowledge distillation clustering with graceful guards.

    Returns a skip result instead of crashing when vectors are empty
    or the corpus is too small for meaningful clustering.

    Args:
        vector_store: VectorStore instance (may be None).
        threshold: Cosine similarity threshold for clustering.
        min_cluster: Minimum cluster size.

    Returns:
        Dict with clustering results or skip status.
    """
    # Guard: no vector store
    if vector_store is None:
        return {
            "status": "skipped",
            "reason": "no vector store available",
        }

    # Guard: check count before loading all embeddings
    count = vector_store.count()
    if count < MIN_VECTORS_FOR_CLUSTERING:
        return {
            "status": "skipped",
            "reason": "insufficient vectors",
            "vector_count": count,
            "min_required": MIN_VECTORS_FOR_CLUSTERING,
        }

    # Load all embeddings
    embeddings = vector_store.all_embeddings()
    if len(embeddings) < MIN_VECTORS_FOR_CLUSTERING:
        return {
            "status": "skipped",
            "reason": "insufficient vectors",
            "vector_count": len(embeddings),
            "min_required": MIN_VECTORS_FOR_CLUSTERING,
        }

    # Auto-adjust min_cluster_size
    auto_min = max(2, len(embeddings) // 10)
    effective_min = max(min_cluster, auto_min) if min_cluster > 0 else auto_min

    clusters = find_clusters(embeddings, threshold, effective_min)

    if not clusters:
        return {
            "status": "ok",
            "clusters": [],
            "total_clusters": 0,
            "message": "no clusters found at this threshold",
        }

    largest = max(len(c) for c in clusters)
    return {
        "status": "ok",
        "clusters": clusters,
        "total_clusters": len(clusters),
        "largest_cluster": largest,
        "total_docs_covered": sum(len(c) for c in clusters),
    }
