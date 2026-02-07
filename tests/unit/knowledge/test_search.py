"""Tests for KnowledgeSearcher - 2-way RRF hybrid search."""

import pytest

from c4.knowledge.documents import DocumentStore
from c4.knowledge.embeddings import KnowledgeEmbedder
from c4.knowledge.search import KnowledgeSearcher, rrf_merge


class TestRRFMerge:
    def test_single_list(self):
        results = rrf_merge(
            [{"id": "a", "score": 1.0}, {"id": "b", "score": 0.5}],
            k=60,
        )
        assert results[0]["id"] == "a"
        assert results[1]["id"] == "b"
        assert "rrf_score" in results[0]

    def test_two_lists_overlap(self):
        list1 = [{"id": "a"}, {"id": "b"}, {"id": "c"}]
        list2 = [{"id": "b"}, {"id": "a"}, {"id": "d"}]

        results = rrf_merge(list1, list2, k=60)

        # "a" and "b" appear in both lists, should score higher
        ids = [r["id"] for r in results]
        # Both "a" and "b" should be in top 2
        assert set(ids[:2]) == {"a", "b"}
        # "c" and "d" appear in only one list
        assert set(ids[2:]) == {"c", "d"}

    def test_no_overlap(self):
        list1 = [{"id": "a"}]
        list2 = [{"id": "b"}]
        results = rrf_merge(list1, list2, k=60)
        assert len(results) == 2

    def test_empty_lists(self):
        results = rrf_merge([], [], k=60)
        assert results == []

    def test_rrf_score_higher_for_overlapping(self):
        list1 = [{"id": "a"}, {"id": "b"}]
        list2 = [{"id": "a"}, {"id": "c"}]
        results = rrf_merge(list1, list2, k=60)

        # "a" appears in both, should have highest score
        assert results[0]["id"] == "a"
        assert results[0]["rrf_score"] > results[1]["rrf_score"]


@pytest.fixture
def searcher(tmp_path):
    """Create a KnowledgeSearcher with mock embeddings."""
    base = tmp_path / "knowledge"
    return KnowledgeSearcher(base_path=base, embedding_model="mock")


@pytest.fixture
def populated_searcher(searcher):
    """Create a searcher with pre-populated documents."""
    store = searcher._doc_store

    store.create("experiment", {
        "title": "RandomForest Baseline",
        "domain": "ml",
        "tags": ["sklearn", "classification"],
        "hypothesis": "RF achieves 85%+ accuracy",
        "hypothesis_status": "supported",
    }, body="# RandomForest Baseline\n\nTrained RF with 100 estimators.\nAccuracy: 0.87\nF1: 0.82")

    store.create("experiment", {
        "title": "XGBoost Comparison",
        "domain": "ml",
        "tags": ["xgboost", "classification"],
        "hypothesis": "XGBoost outperforms RF",
        "hypothesis_status": "supported",
    }, body="# XGBoost Comparison\n\nXGBoost with default params.\nAccuracy: 0.91")

    store.create("pattern", {
        "title": "High Learning Rate Pattern",
        "domain": "ml",
        "confidence": 0.85,
        "evidence_count": 3,
    }, body="Learning rate > 0.1 leads to unstable training")

    store.create("insight", {
        "title": "Data Augmentation Best Practice",
        "domain": "cv",
        "insight_type": "best-practice",
    }, body="Always apply augmentation for image classification tasks")

    return searcher


class TestKnowledgeSearcher:
    def test_search_returns_results(self, populated_searcher):
        results = populated_searcher.search("RandomForest Baseline")
        assert len(results) > 0

    def test_search_fts_only(self, populated_searcher):
        """FTS should find documents by keyword even without embeddings indexed."""
        results = populated_searcher.search("RandomForest")
        assert any("RandomForest" in r.get("title", "") for r in results)

    def test_search_empty_query(self, populated_searcher):
        results = populated_searcher.search("")
        # Empty query may return empty or all - just shouldn't crash
        assert isinstance(results, list)

    def test_search_no_results(self, populated_searcher):
        results = populated_searcher.search("quantum_computing_xyz_nonexistent")
        assert len(results) == 0

    def test_search_with_type_filter(self, populated_searcher):
        results = populated_searcher.search(
            "learning", filters={"type": "pattern"}
        )
        for r in results:
            assert r.get("type") == "pattern"

    def test_search_with_domain_filter(self, populated_searcher):
        results = populated_searcher.search(
            "classification", filters={"domain": "ml"}
        )
        for r in results:
            assert r.get("domain") == "ml"

    def test_search_by_type_convenience(self, populated_searcher):
        results = populated_searcher.search_by_type("accuracy", "experiment")
        for r in results:
            assert r.get("type") == "experiment"

    def test_top_k_limit(self, populated_searcher):
        results = populated_searcher.search("ml classification", top_k=2)
        assert len(results) <= 2


class TestKnowledgeSearcherWithEmbeddings:
    """Test hybrid search with actual (mock) embeddings indexed."""

    def test_hybrid_search_with_embeddings(self, tmp_path):
        base = tmp_path / "knowledge"
        store = DocumentStore(base_path=base)
        embedder = KnowledgeEmbedder(base_path=base, embedding_model="mock")

        # Create and index documents
        doc_id1 = store.create("experiment", {
            "title": "Neural Network Training",
            "domain": "dl",
        }, body="Deep learning with PyTorch on ImageNet")

        doc_id2 = store.create("experiment", {
            "title": "Linear Regression Analysis",
            "domain": "stats",
        }, body="Simple linear regression on housing prices")

        # Index embeddings
        doc1 = store.get(doc_id1)
        doc2 = store.get(doc_id2)
        embedder.index_document(doc_id1, doc1.model_dump())
        embedder.index_document(doc_id2, doc2.model_dump())

        # Search should combine FTS + vector
        searcher = KnowledgeSearcher(base_path=base, embedding_model="mock")
        results = searcher.search("neural network deep learning")
        assert len(results) > 0

        embedder.close()
        searcher.close()

    def test_close_is_safe(self, searcher):
        searcher.close()
        searcher.close()  # Double close should not error


class TestEnrichAndFilter:
    """Tests for A1-fix (O(m+n) caching) and A3-fix (metadata always enriched)."""

    def test_results_always_have_metadata_without_filter(self, populated_searcher):
        """A3-fix: results should have title/type/domain even without filters."""
        results = populated_searcher.search("RandomForest")
        for r in results:
            assert "title" in r
            assert "type" in r
            assert "domain" in r

    def test_results_have_metadata_with_filter(self, populated_searcher):
        results = populated_searcher.search("classification", filters={"domain": "ml"})
        for r in results:
            assert r["domain"] == "ml"
            assert "title" in r
            assert "type" in r

    def test_filter_performance_many_docs(self, tmp_path):
        """A1-fix: verify filter doesn't call list_documents per result."""
        import time

        base = tmp_path / "knowledge"
        searcher = KnowledgeSearcher(base_path=base, embedding_model="mock")
        store = searcher._doc_store

        # Create 100 documents
        for i in range(100):
            store.create("experiment", {
                "title": f"Exp {i}",
                "domain": "ml" if i % 2 == 0 else "web",
            }, body=f"Experiment number {i}")

        t0 = time.time()
        results = searcher.search("Exp", top_k=10, filters={"domain": "ml"})
        elapsed = time.time() - t0

        assert elapsed < 2.0  # Should be fast with O(m+n)
        assert all(r["domain"] == "ml" for r in results)
        searcher.close()
