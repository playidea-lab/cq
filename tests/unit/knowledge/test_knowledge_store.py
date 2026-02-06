"""Tests for Knowledge Store, Aggregator, and Miner."""

import asyncio

import pytest

from c4.knowledge.aggregator import KnowledgeAggregator
from c4.knowledge.miner import PatternMiner
from c4.knowledge.models import (
    Pattern,
)
from c4.knowledge.store import LocalKnowledgeStore


@pytest.fixture
def store(tmp_path):
    return LocalKnowledgeStore(base_path=tmp_path / "knowledge")


@pytest.fixture
def sample_experiment():
    return {
        "task_id": "T-001-0",
        "title": "Baseline RandomForest",
        "hypothesis": "RF baseline achieves 85%+ accuracy",
        "hypothesis_status": "supported",
        "config": {"algorithm": "RandomForest", "n_estimators": 100, "seed": 42},
        "result": {"metrics": {"accuracy": 0.87}, "success": True},
        "observations": [],
        "lessons_learned": ["Use stratified split for imbalanced data"],
        "tags": ["baseline", "classification"],
        "domain": "ml-dl",
    }


def run(coro):
    """Helper to run async functions in tests."""
    return asyncio.get_event_loop().run_until_complete(coro)


class TestLocalKnowledgeStore:
    def test_save_and_get_experiment(self, store, sample_experiment):
        exp_id = run(store.save_experiment(sample_experiment))
        assert exp_id.startswith("exp-")

        result = run(store.get_experiment(exp_id))
        assert result is not None
        assert result["title"] == "Baseline RandomForest"
        assert result["task_id"] == "T-001-0"
        assert result["domain"] == "ml-dl"

    def test_get_nonexistent_experiment(self, store):
        result = run(store.get_experiment("exp-nonexistent"))
        assert result is None

    def test_search_by_keyword(self, store, sample_experiment):
        run(store.save_experiment(sample_experiment))

        # Search by title keyword
        results = run(store.search("RandomForest"))
        assert len(results) >= 1
        assert results[0]["title"] == "Baseline RandomForest"

    def test_search_no_match(self, store, sample_experiment):
        run(store.save_experiment(sample_experiment))
        results = run(store.search("nonexistent_keyword_xyz"))
        assert len(results) == 0

    def test_search_by_tag(self, store, sample_experiment):
        run(store.save_experiment(sample_experiment))
        results = run(store.search("baseline"))
        assert len(results) >= 1

    def test_list_experiments(self, store, sample_experiment):
        run(store.save_experiment(sample_experiment))

        results = run(store.list_experiments())
        assert len(results) == 1

    def test_list_experiments_by_task(self, store, sample_experiment):
        run(store.save_experiment(sample_experiment))

        results = run(store.list_experiments(task_id="T-001-0"))
        assert len(results) == 1

        results = run(store.list_experiments(task_id="T-999-0"))
        assert len(results) == 0

    def test_list_experiments_by_domain(self, store, sample_experiment):
        run(store.save_experiment(sample_experiment))

        results = run(store.list_experiments(domain="ml-dl"))
        assert len(results) == 1

        results = run(store.list_experiments(domain="web-frontend"))
        assert len(results) == 0

    def test_save_and_get_pattern(self, store):
        pattern = Pattern(
            name="high_lr_warmup",
            description="Learning rate warmup improves convergence",
            domain="ml-dl",
            confidence=0.85,
            evidence_count=5,
            evidence_ids=["exp-1", "exp-2", "exp-3"],
            config_pattern={"lr_warmup": True},
        )

        pat_id = run(store.save_pattern(pattern))
        assert pat_id.startswith("pat-")

        patterns = run(store.get_patterns())
        assert len(patterns) == 1
        assert patterns[0]["name"] == "high_lr_warmup"
        assert patterns[0]["confidence"] == 0.85

    def test_get_patterns_by_domain(self, store):
        p1 = Pattern(name="p1", domain="ml-dl", confidence=0.9)
        p2 = Pattern(name="p2", domain="web-frontend", confidence=0.8)
        run(store.save_pattern(p1))
        run(store.save_pattern(p2))

        ml_patterns = run(store.get_patterns(domain="ml-dl"))
        assert len(ml_patterns) == 1
        assert ml_patterns[0]["name"] == "p1"

    def test_update_experiment(self, store, sample_experiment):
        exp_id = run(store.save_experiment(sample_experiment))

        # Update by saving with same id
        updated = sample_experiment.copy()
        updated["id"] = exp_id
        updated["hypothesis_status"] = "refuted"
        run(store.save_experiment(updated))

        result = run(store.get_experiment(exp_id))
        assert result["hypothesis_status"] == "refuted"

    def test_multiple_experiments(self, store):
        for i in range(5):
            exp = {
                "task_id": f"T-{i:03d}-0",
                "title": f"Experiment {i}",
                "domain": "ml-dl",
            }
            run(store.save_experiment(exp))

        results = run(store.list_experiments(limit=3))
        assert len(results) == 3


class TestKnowledgeAggregator:
    @pytest.fixture
    def aggregator(self):
        return KnowledgeAggregator()

    @pytest.fixture
    def mixed_experiments(self):
        return [
            {
                "id": "exp-1",
                "domain": "ml-dl",
                "config": {"algorithm": "RF", "n_estimators": 100},
                "result": {"success": True},
                "lessons_learned": ["Use cross-validation"],
            },
            {
                "id": "exp-2",
                "domain": "ml-dl",
                "config": {"algorithm": "RF", "n_estimators": 200},
                "result": {"success": True},
                "lessons_learned": ["Use cross-validation", "Feature scaling helps"],
            },
            {
                "id": "exp-3",
                "domain": "ml-dl",
                "config": {"algorithm": "XGBoost"},
                "result": {"success": False, "error_message": "OOM"},
                "lessons_learned": ["Reduce batch size for large models"],
            },
            {
                "id": "exp-4",
                "domain": "web-frontend",
                "config": {"framework": "React"},
                "result": {"success": True},
                "lessons_learned": [],
            },
        ]

    def test_success_rate(self, aggregator, mixed_experiments):
        stats = aggregator.compute_success_rate(mixed_experiments)
        assert stats["total"] == 4
        assert stats["success_count"] == 3
        assert stats["failure_count"] == 1
        assert stats["success_rate"] == 0.75

    def test_success_rate_by_domain(self, aggregator, mixed_experiments):
        stats = aggregator.compute_success_rate(mixed_experiments, domain="ml-dl")
        assert stats["total"] == 3
        assert stats["success_count"] == 2

    def test_success_rate_empty(self, aggregator):
        stats = aggregator.compute_success_rate([])
        assert stats["total"] == 0
        assert stats["success_rate"] == 0.0

    def test_extract_common_configs(self, aggregator, mixed_experiments):
        configs = aggregator.extract_common_configs(
            mixed_experiments, success_only=True
        )
        assert "algorithm" in configs
        # RF appears twice in successful experiments
        assert configs["algorithm"][0] == ("RF", 2)

    def test_best_practices(self, aggregator, mixed_experiments):
        recs = aggregator.get_best_practices(mixed_experiments)
        # "Use cross-validation" appears in 2 successful experiments
        best_practices = [r for r in recs if r["type"] == "best_practice"]
        assert len(best_practices) > 0
        assert best_practices[0]["content"] == "Use cross-validation"
        assert best_practices[0]["source_count"] == 2

    def test_failure_report(self, aggregator, mixed_experiments):
        report = aggregator.generate_failure_report(mixed_experiments)
        assert report["failure_count"] == 1
        assert report["common_errors"][0] == ("OOM", 1)
        assert ("ml-dl", 1) in report["affected_domains"]


class TestPatternMiner:
    @pytest.fixture
    def miner(self):
        return PatternMiner()

    @pytest.fixture
    def experiments(self):
        return [
            {
                "id": "exp-1",
                "hypothesis": "LR warmup helps",
                "hypothesis_status": "supported",
                "config": {"lr_warmup": "true", "batch_size": "32"},
                "result": {"success": True},
                "lessons_learned": [],
                "domain": "ml-dl",
            },
            {
                "id": "exp-2",
                "hypothesis": "LR warmup helps",
                "hypothesis_status": "supported",
                "config": {"lr_warmup": "true", "batch_size": "64"},
                "result": {"success": True},
                "lessons_learned": [],
                "domain": "ml-dl",
            },
            {
                "id": "exp-3",
                "hypothesis": "LR warmup helps",
                "hypothesis_status": "refuted",
                "config": {"lr_warmup": "false", "batch_size": "32"},
                "result": {"success": False},
                "lessons_learned": ["LR warmup needed", "LR warmup needed"],
                "domain": "ml-dl",
            },
            {
                "id": "exp-4",
                "hypothesis": "LR warmup helps",
                "hypothesis_status": "refuted",
                "config": {"lr_warmup": "false"},
                "result": {"success": False},
                "lessons_learned": ["LR warmup needed"],
                "domain": "ml-dl",
            },
        ]

    def test_mine_success_patterns(self, miner, experiments):
        patterns = miner.mine_success_patterns(experiments, min_support=2)
        assert len(patterns) >= 1
        # lr_warmup=true appears in both successful experiments
        lr_pattern = next(
            (p for p in patterns if "lr_warmup" in p.config_pattern), None
        )
        assert lr_pattern is not None
        assert lr_pattern.evidence_count == 2
        assert lr_pattern.confidence == 1.0  # 2/2 successful

    def test_mine_success_patterns_insufficient_data(self, miner):
        patterns = miner.mine_success_patterns([], min_support=2)
        assert patterns == []

    def test_extract_failure_lessons(self, miner, experiments):
        lessons = miner.extract_failure_lessons(experiments, min_occurrences=2)
        assert len(lessons) >= 1
        assert lessons[0]["content"] == "LR warmup needed"
        assert lessons[0]["count"] >= 2

    def test_extract_failure_lessons_no_recurring(self, miner):
        exps = [
            {
                "result": {"success": False},
                "lessons_learned": ["unique lesson"],
            }
        ]
        lessons = miner.extract_failure_lessons(exps, min_occurrences=2)
        assert lessons == []

    def test_update_hypothesis_confidence_supported(self, miner, experiments):
        result = miner.update_hypothesis_confidence(
            experiments, "LR warmup helps"
        )
        assert result["total"] == 4
        assert result["supported"] == 2
        assert result["refuted"] == 2
        # 50/50 split → testing status
        assert result["suggested_status"] == "testing"

    def test_update_hypothesis_confidence_no_data(self, miner):
        result = miner.update_hypothesis_confidence([], "Unknown hypothesis")
        assert result["total"] == 0
        assert result["confidence"] == 0.0
        assert result["suggested_status"] == "proposed"

    def test_update_hypothesis_strong_support(self, miner):
        exps = [
            {"hypothesis": "H1", "hypothesis_status": "supported"},
            {"hypothesis": "H1", "hypothesis_status": "supported"},
            {"hypothesis": "H1", "hypothesis_status": "supported"},
            {"hypothesis": "H1", "hypothesis_status": "refuted"},
        ]
        result = miner.update_hypothesis_confidence(exps, "H1")
        assert result["supported"] == 3
        assert result["refuted"] == 1
        assert result["confidence"] == 0.75
        assert result["suggested_status"] == "supported"
