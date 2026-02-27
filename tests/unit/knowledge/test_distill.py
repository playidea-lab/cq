"""Tests for knowledge distillation clustering with graceful guards."""

import math
from unittest.mock import MagicMock

import pytest

from c4.knowledge.distill import (
    MIN_VECTORS_FOR_CLUSTERING,
    cosine_similarity,
    distill,
    find_clusters,
)


# --- Unit tests for cosine_similarity ---


class TestCosineSimilarity:
    def test_identical_vectors(self):
        assert cosine_similarity([1.0, 0.0], [1.0, 0.0]) == pytest.approx(1.0)

    def test_orthogonal_vectors(self):
        assert cosine_similarity([1.0, 0.0], [0.0, 1.0]) == pytest.approx(0.0)

    def test_opposite_vectors(self):
        assert cosine_similarity([1.0, 0.0], [-1.0, 0.0]) == pytest.approx(-1.0)

    def test_zero_vector(self):
        assert cosine_similarity([0.0, 0.0], [1.0, 0.0]) == 0.0


# --- Unit tests for find_clusters ---


class TestFindClusters:
    def test_empty_embeddings(self):
        assert find_clusters({}, threshold=0.7, min_cluster_size=2) == []

    def test_below_min_cluster_size(self):
        embs = {"a": [1.0, 0.0], "b": [0.9, 0.1]}
        # 2 docs, min_cluster_size=3 → empty
        assert find_clusters(embs, threshold=0.5, min_cluster_size=3) == []

    def test_two_clusters(self):
        # Group 1: nearly identical
        # Group 2: nearly identical but orthogonal to group 1
        embs = {
            "a1": [1.0, 0.0, 0.0],
            "a2": [0.99, 0.01, 0.0],
            "b1": [0.0, 1.0, 0.0],
            "b2": [0.0, 0.99, 0.01],
        }
        clusters = find_clusters(embs, threshold=0.9, min_cluster_size=2)
        assert len(clusters) == 2
        cluster_sets = [set(c) for c in clusters]
        assert {"a1", "a2"} in cluster_sets
        assert {"b1", "b2"} in cluster_sets


# --- Integration tests for distill() ---


class TestDistillEmptyVectors:
    """0 vectors -> {status: skipped} (not an error)."""

    def test_none_vector_store(self):
        result = distill(None)
        assert result["status"] == "skipped"
        assert "no vector store" in result["reason"]

    def test_zero_count(self):
        store = MagicMock()
        store.count.return_value = 0
        result = distill(store)
        assert result["status"] == "skipped"
        assert result["reason"] == "insufficient vectors"
        assert result["vector_count"] == 0


class TestDistillSmallCorpus:
    """4 vectors (< 5) -> {status: skipped}."""

    def test_below_threshold(self):
        store = MagicMock()
        store.count.return_value = 4
        result = distill(store, threshold=0.7)
        assert result["status"] == "skipped"
        assert result["reason"] == "insufficient vectors"
        assert result["vector_count"] == 4
        assert result["min_required"] == MIN_VECTORS_FOR_CLUSTERING

    def test_count_ok_but_embeddings_filtered(self):
        """count() returns 6 but all_embeddings() returns 4 (chunks filtered)."""
        store = MagicMock()
        store.count.return_value = 6
        store.all_embeddings.return_value = {
            "a": [1.0, 0.0],
            "b": [0.9, 0.1],
            "c": [0.8, 0.2],
            "d": [0.0, 1.0],
        }
        result = distill(store, threshold=0.7)
        assert result["status"] == "skipped"
        assert result["reason"] == "insufficient vectors"


class TestDistillNormal:
    """50 vectors -> normal clustering (no regression)."""

    def _make_embeddings(self, n: int, dim: int = 4) -> dict[str, list[float]]:
        """Generate n embeddings in 2 distinct clusters."""
        embs = {}
        for i in range(n):
            if i < n // 2:
                # Cluster A: vectors near [1, 0, 0, 0]
                vec = [1.0 - i * 0.001, i * 0.001, 0.0, 0.0]
            else:
                # Cluster B: vectors near [0, 0, 1, 0]
                j = i - n // 2
                vec = [0.0, 0.0, 1.0 - j * 0.001, j * 0.001]
            # Normalize
            norm = math.sqrt(sum(x * x for x in vec))
            embs[f"doc-{i:03d}"] = [x / norm for x in vec]
        return embs

    def test_normal_clustering(self):
        embs = self._make_embeddings(50)
        store = MagicMock()
        store.count.return_value = 50
        store.all_embeddings.return_value = embs

        result = distill(store, threshold=0.9, min_cluster=2)
        assert result["status"] == "ok"
        assert result["total_clusters"] >= 1
        assert result["total_docs_covered"] > 0

    def test_no_clusters_at_high_threshold(self):
        """All vectors unique enough that nothing clusters at threshold=0.9999."""
        embs = {}
        for i in range(10):
            vec = [0.0] * 10
            vec[i] = 1.0
            embs[f"doc-{i}"] = vec

        store = MagicMock()
        store.count.return_value = 10
        store.all_embeddings.return_value = embs

        result = distill(store, threshold=0.9999, min_cluster=2)
        assert result["status"] == "ok"
        assert result["total_clusters"] == 0
        assert result["clusters"] == []
