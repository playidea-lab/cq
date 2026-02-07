"""Tests for KnowledgeHook - auto-save experiment results."""

from unittest.mock import MagicMock, patch

from c4.hooks.builtin.knowledge_hook import (
    KnowledgeHook,
    _build_experiment_record,
    _save_to_knowledge_store,
)


class TestBuildExperimentRecord:
    def test_basic_record(self):
        record = _build_experiment_record(
            "T-001-0",
            {"title": "Test Task", "domain": "ml"},
            {"metrics": {"accuracy": 0.95}},
        )
        assert record["task_id"] == "T-001-0"
        assert record["title"] == "Test Task"
        assert record["domain"] == "ml"
        assert record["metrics"]["accuracy"] == 0.95

    def test_domain_from_task_data(self):
        """C1-fix: domain should come from task_data, not hardcoded 'ml'."""
        record = _build_experiment_record(
            "T-001-0",
            {"title": "Web Task", "domain": "web-backend"},
            {"metrics": {}},
        )
        assert record["domain"] == "web-backend"

    def test_domain_default_unknown(self):
        """C1-fix: missing domain should default to 'unknown'."""
        record = _build_experiment_record(
            "T-001-0",
            {"title": "No Domain"},
            {"metrics": {}},
        )
        assert record["domain"] == "unknown"


class TestSaveToKnowledgeStore:
    def test_save_creates_document(self, tmp_path):
        """Basic save should create a Markdown document."""
        with patch("c4.knowledge.documents.DocumentStore") as MockStore:
            mock_store = MagicMock()
            mock_store.list_documents.return_value = []
            MockStore.return_value = mock_store

            _save_to_knowledge_store({
                "title": "Test",
                "task_id": "T-001-0",
                "domain": "ml",
                "metrics": {"accuracy": 0.95},
                "code_features": {},
                "data_profile": {},
            })

            mock_store.create.assert_called_once()
            call_args = mock_store.create.call_args
            assert call_args[0][0] == "experiment"
            assert call_args[0][1]["domain"] == "ml"

    def test_save_includes_code_features_in_body(self, tmp_path):
        """C3-fix: code_features should be included in body."""
        with patch("c4.knowledge.documents.DocumentStore") as MockStore:
            mock_store = MagicMock()
            mock_store.list_documents.return_value = []
            MockStore.return_value = mock_store

            _save_to_knowledge_store({
                "title": "Test",
                "task_id": "T-001-0",
                "domain": "ml",
                "metrics": {},
                "code_features": {"language": "python", "imports": ["torch"]},
                "data_profile": {"rows": 1000},
            })

            call_args = mock_store.create.call_args
            body = call_args.kwargs.get("body") or call_args[1].get("body", "")
            assert "Code Features" in body
            assert "Data Profile" in body

    def test_save_updates_existing_document(self, tmp_path):
        """C2-fix: duplicate task_id should update instead of creating new."""
        with patch("c4.knowledge.documents.DocumentStore") as MockStore:
            mock_store = MagicMock()
            mock_store.list_documents.return_value = [
                {"id": "exp-existing1", "task_id": "T-001-0", "type": "experiment"}
            ]
            MockStore.return_value = mock_store

            _save_to_knowledge_store({
                "title": "Updated",
                "task_id": "T-001-0",
                "domain": "ml",
                "metrics": {},
                "code_features": {},
                "data_profile": {},
            })

            # Should update, not create
            mock_store.update.assert_called_once()
            mock_store.create.assert_not_called()
            update_call = mock_store.update.call_args
            assert update_call[0][0] == "exp-existing1"


class TestKnowledgeHookIntegration:
    def test_hook_name(self):
        hook = KnowledgeHook()
        assert hook.name == "knowledge_auto_save"

    def test_hook_skips_without_execution_stats(self):
        hook = KnowledgeHook()
        context = MagicMock()
        context.get.return_value = None
        assert hook.execute(context) is True
