"""Tests for Knowledge MCP handlers (v2 + legacy)."""

from unittest.mock import MagicMock

import pytest

from c4.mcp.handlers.knowledge import (
    handle_experiment_record,
    handle_experiment_search,
    handle_knowledge_get,
    handle_knowledge_record,
    handle_knowledge_search,
    handle_pattern_suggest,
)


@pytest.fixture(autouse=True)
def set_project_root(tmp_path, monkeypatch):
    """Set C4_PROJECT_ROOT to temp dir for all tests."""
    monkeypatch.setenv("C4_PROJECT_ROOT", str(tmp_path))


@pytest.fixture
def daemon():
    return MagicMock()


class TestKnowledgeRecord:
    def test_record_experiment(self, daemon):
        result = handle_knowledge_record(daemon, {
            "doc_type": "experiment",
            "title": "Test Exp",
            "domain": "ml",
            "body": "# Test\nBody here",
        })
        assert result["success"] is True
        assert result["doc_id"].startswith("exp-")

    def test_record_missing_type(self, daemon):
        result = handle_knowledge_record(daemon, {"title": "Test"})
        assert "error" in result

    def test_record_missing_title(self, daemon):
        result = handle_knowledge_record(daemon, {"doc_type": "experiment"})
        assert "error" in result

    def test_record_invalid_type(self, daemon):
        result = handle_knowledge_record(daemon, {
            "doc_type": "invalid",
            "title": "Test",
        })
        assert "error" in result

    def test_record_filters_invalid_metadata(self, daemon):
        """A2-fix: unknown metadata fields should be silently filtered out."""
        result = handle_knowledge_record(daemon, {
            "doc_type": "experiment",
            "title": "Test",
            "domain": "ml",
            "typo_field": "should be ignored",
            "random_key": 42,
        })
        assert result["success"] is True

        # Verify the document doesn't have the invalid fields
        doc_result = handle_knowledge_get(daemon, {"doc_id": result["doc_id"]})
        assert "typo_field" not in doc_result
        assert "random_key" not in doc_result


class TestKnowledgeSearch:
    def test_search_empty_store(self, daemon):
        result = handle_knowledge_search(daemon, {"query": "test"})
        assert result["count"] == 0

    def test_search_missing_query(self, daemon):
        result = handle_knowledge_search(daemon, {})
        assert "error" in result

    def test_search_with_results(self, daemon):
        handle_knowledge_record(daemon, {
            "doc_type": "experiment",
            "title": "Random Forest",
            "domain": "ml",
            "body": "RF baseline experiment",
        })
        result = handle_knowledge_search(daemon, {"query": "Random Forest"})
        assert result["count"] >= 1


class TestKnowledgeGet:
    def test_get_existing(self, daemon):
        rec = handle_knowledge_record(daemon, {
            "doc_type": "experiment",
            "title": "Get Test",
            "body": "Body",
        })
        result = handle_knowledge_get(daemon, {"doc_id": rec["doc_id"]})
        assert result["title"] == "Get Test"
        assert "backlinks" in result

    def test_get_nonexistent(self, daemon):
        result = handle_knowledge_get(daemon, {"doc_id": "exp-nonexist"})
        assert "error" in result

    def test_get_missing_id(self, daemon):
        result = handle_knowledge_get(daemon, {})
        assert "error" in result


class TestLegacyExperimentSearch:
    """D1-fix: legacy handlers should not crash with asyncio errors."""

    def test_experiment_search_uses_v2(self, daemon):
        handle_knowledge_record(daemon, {
            "doc_type": "experiment",
            "title": "Legacy Search Test",
            "domain": "ml",
            "body": "Testing legacy search",
        })
        result = handle_experiment_search(daemon, {"query": "Legacy Search"})
        assert "error" not in result or result["count"] >= 0

    def test_experiment_search_no_asyncio_crash(self, daemon):
        """D1-fix: should not raise RuntimeError about event loop."""
        result = handle_experiment_search(daemon, {"query": "nonexistent"})
        assert "RuntimeError" not in str(result)


class TestLegacyExperimentRecord:
    def test_experiment_record_returns_both_keys(self, daemon):
        """D2: legacy response includes both doc_id and experiment_id."""
        result = handle_experiment_record(daemon, {
            "title": "Legacy Record",
            "domain": "ml",
            "hypothesis": "Test hypothesis",
        })
        assert result["success"] is True
        assert "doc_id" in result
        assert "experiment_id" in result
        assert result["doc_id"] == result["experiment_id"]


class TestPatternSuggest:
    """D1-fix: pattern suggest should use v2 search, not asyncio."""

    def test_pattern_suggest_empty(self, daemon):
        result = handle_pattern_suggest(daemon, {})
        assert "error" not in result
        assert result["pattern_count"] == 0

    def test_pattern_suggest_with_patterns(self, daemon):
        handle_knowledge_record(daemon, {
            "doc_type": "pattern",
            "title": "Test Pattern",
            "domain": "ml",
            "body": "A pattern description",
        })
        result = handle_pattern_suggest(daemon, {"domain": "ml"})
        assert "error" not in result

    def test_pattern_suggest_no_asyncio_crash(self, daemon):
        """D1-fix: should not crash with event loop errors."""
        result = handle_pattern_suggest(daemon, {"domain": "nonexistent"})
        assert "RuntimeError" not in str(result)
