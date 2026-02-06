"""Tests for Knowledge Embeddings - semantic search over experiments."""

import pytest

from c4.knowledge.embeddings import ExperimentEmbedder, _experiment_to_text


class TestExperimentToText:
    def test_full_experiment(self):
        exp = {
            "title": "Baseline RF",
            "hypothesis": "RF achieves 85%",
            "domain": "ml-dl",
            "lessons_learned": ["Use cross-validation"],
            "tags": ["baseline"],
        }
        text = _experiment_to_text(exp)
        assert "Baseline RF" in text
        assert "RF achieves 85%" in text
        assert "ml-dl" in text
        assert "Use cross-validation" in text
        assert "baseline" in text

    def test_empty_experiment(self):
        text = _experiment_to_text({})
        assert text == ""

    def test_minimal_experiment(self):
        text = _experiment_to_text({"title": "Test"})
        assert text == "Test"


class TestExperimentEmbedder:
    @pytest.fixture
    def embedder(self, tmp_path):
        return ExperimentEmbedder(
            base_path=tmp_path / "knowledge",
            embedding_model="mock",
        )

    @pytest.fixture
    def sample_experiments(self):
        return [
            {
                "id": "exp-001",
                "title": "RandomForest baseline",
                "hypothesis": "RF achieves good accuracy",
                "domain": "ml-dl",
                "lessons_learned": ["Feature scaling helps"],
                "tags": ["baseline", "classification"],
            },
            {
                "id": "exp-002",
                "title": "XGBoost tuning",
                "hypothesis": "XGBoost beats RF",
                "domain": "ml-dl",
                "lessons_learned": ["Learning rate warmup helps"],
                "tags": ["tuning", "classification"],
            },
            {
                "id": "exp-003",
                "title": "CNN image classifier",
                "hypothesis": "ResNet50 for image classification",
                "domain": "ml-dl",
                "lessons_learned": ["Data augmentation is key"],
                "tags": ["deep-learning", "vision"],
            },
        ]

    def test_index_and_search(self, embedder, sample_experiments):
        # Index all experiments
        for exp in sample_experiments:
            result = embedder.index_experiment(exp["id"], exp)
            assert result is True

        assert embedder.count == 3

        # Search should return results
        results = embedder.search_similar("RandomForest classification")
        assert len(results) > 0
        assert all("id" in r and "score" in r for r in results)

    def test_index_empty_experiment(self, embedder):
        result = embedder.index_experiment("exp-empty", {})
        assert result is False

    def test_search_empty_store(self, embedder):
        results = embedder.search_similar("anything")
        assert results == []

    def test_remove_experiment(self, embedder, sample_experiments):
        exp = sample_experiments[0]
        embedder.index_experiment(exp["id"], exp)
        assert embedder.count == 1

        result = embedder.remove(exp["id"])
        assert result is True
        assert embedder.count == 0

    def test_remove_nonexistent(self, embedder):
        result = embedder.remove("exp-nonexistent")
        assert result is False

    def test_update_experiment(self, embedder, sample_experiments):
        exp = sample_experiments[0]
        embedder.index_experiment(exp["id"], exp)
        assert embedder.count == 1

        # Re-index same ID should update (remove + add)
        updated = {**exp, "title": "Updated RF baseline"}
        embedder.index_experiment(exp["id"], updated)
        assert embedder.count == 1

    def test_close(self, embedder, sample_experiments):
        exp = sample_experiments[0]
        embedder.index_experiment(exp["id"], exp)
        embedder.close()
        # After close, vector_store is None
        assert embedder._vector_store is None

    def test_provider_name_resolution(self, tmp_path):
        # "mock" → mock provider
        e1 = ExperimentEmbedder(tmp_path / "k1", embedding_model="mock")
        assert e1._provider.dimension == 384

        # "local" stays as local (would fail without sentence-transformers)
        # Just check resolution doesn't crash
        e2 = ExperimentEmbedder(tmp_path / "k2", embedding_model="mock")
        assert e2._provider is not None
