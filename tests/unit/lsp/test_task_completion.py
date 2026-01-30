"""Tests for C4 Task ID completion functionality."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

# Check if pygls is available
try:
    from lsprotocol import types as lsp

    from c4.lsp.server import C4LSPServer

    PYGLS_AVAILABLE = True
except ImportError:
    PYGLS_AVAILABLE = False


pytestmark = pytest.mark.skipif(
    not PYGLS_AVAILABLE,
    reason="pygls not installed",
)


class TestTaskCompletionPrefix:
    """Tests for task completion prefix detection."""

    def test_detects_t_prefix(self):
        """Should detect T- prefix for task completion."""
        server = C4LSPServer()
        test_content = "See T-\n"
        server.analyzer.add_file("/test/file.py", test_content)

        position = lsp.Position(line=0, character=6)
        prefix = server._get_task_completion_prefix("file:///test/file.py", position)

        assert prefix == "T-"

    def test_detects_t_with_numbers(self):
        """Should detect T-00 prefix."""
        server = C4LSPServer()
        test_content = "See T-00\n"
        server.analyzer.add_file("/test/file.py", test_content)

        position = lsp.Position(line=0, character=8)
        prefix = server._get_task_completion_prefix("file:///test/file.py", position)

        assert prefix == "T-00"

    def test_detects_r_prefix(self):
        """Should detect R- prefix for review task completion."""
        server = C4LSPServer()
        test_content = "Review R-\n"
        server.analyzer.add_file("/test/file.py", test_content)

        position = lsp.Position(line=0, character=9)
        prefix = server._get_task_completion_prefix("file:///test/file.py", position)

        assert prefix == "R-"

    def test_detects_cp_prefix(self):
        """Should detect CP- prefix for checkpoint completion."""
        server = C4LSPServer()
        test_content = "Checkpoint CP-\n"
        server.analyzer.add_file("/test/file.py", test_content)

        position = lsp.Position(line=0, character=14)
        prefix = server._get_task_completion_prefix("file:///test/file.py", position)

        assert prefix == "CP-"

    def test_does_not_detect_single_t(self):
        """Should not detect single T without dash as task prefix."""
        server = C4LSPServer()
        test_content = "See T\n"
        server.analyzer.add_file("/test/file.py", test_content)

        position = lsp.Position(line=0, character=5)
        prefix = server._get_task_completion_prefix("file:///test/file.py", position)

        # Single "T" is too generic, only "T-" should trigger task completion
        assert prefix is None

    def test_returns_none_for_regular_text(self):
        """Should return None for non-task prefix."""
        server = C4LSPServer()
        test_content = "def hello():\n"
        server.analyzer.add_file("/test/file.py", test_content)

        position = lsp.Position(line=0, character=6)
        prefix = server._get_task_completion_prefix("file:///test/file.py", position)

        assert prefix is None


class TestGetTaskCompletions:
    """Tests for getting task completion items."""

    def test_returns_empty_without_store(self):
        """Should return empty list when store is not available."""
        server = C4LSPServer()

        items = server._get_task_completions("T-")

        assert items == []

    def test_returns_matching_tasks(self):
        """Should return tasks matching the prefix."""
        server = C4LSPServer()

        # Mock task store
        mock_task1 = MagicMock()
        mock_task1.id = "T-001"
        mock_task1.title = "First task"
        mock_task1.status.value = "pending"
        mock_task1.assigned_to = None
        mock_task1.dod = "Do something"

        mock_task2 = MagicMock()
        mock_task2.id = "T-002"
        mock_task2.title = "Second task"
        mock_task2.status.value = "in_progress"
        mock_task2.assigned_to = "worker-1"
        mock_task2.dod = "Do more"

        mock_task3 = MagicMock()
        mock_task3.id = "T-003"
        mock_task3.title = "Done task"
        mock_task3.status.value = "done"
        mock_task3.assigned_to = None
        mock_task3.dod = "Already done"

        mock_store = MagicMock()
        mock_store.load_all.return_value = [mock_task1, mock_task2, mock_task3]

        server._task_store = mock_store
        server._c4_project_id = "test"

        items = server._get_task_completions("T-")

        # Should only return pending and in_progress tasks
        assert len(items) == 2
        labels = [item.label for item in items]
        assert "T-001" in labels
        assert "T-002" in labels
        assert "T-003" not in labels  # done task excluded

    def test_filters_by_prefix(self):
        """Should filter tasks by prefix."""
        server = C4LSPServer()

        mock_task1 = MagicMock()
        mock_task1.id = "T-001"
        mock_task1.title = "Task one"
        mock_task1.status.value = "pending"
        mock_task1.assigned_to = None
        mock_task1.dod = "Do it"

        mock_task2 = MagicMock()
        mock_task2.id = "R-001"
        mock_task2.title = "Review one"
        mock_task2.status.value = "pending"
        mock_task2.assigned_to = None
        mock_task2.dod = "Review it"

        mock_store = MagicMock()
        mock_store.load_all.return_value = [mock_task1, mock_task2]

        server._task_store = mock_store
        server._c4_project_id = "test"

        # Filter by T-
        t_items = server._get_task_completions("T-")
        assert len(t_items) == 1
        assert t_items[0].label == "T-001"

        # Filter by R-
        r_items = server._get_task_completions("R-")
        assert len(r_items) == 1
        assert r_items[0].label == "R-001"

    def test_completion_item_has_correct_fields(self):
        """Should create completion items with correct fields."""
        server = C4LSPServer()

        mock_task = MagicMock()
        mock_task.id = "T-001"
        mock_task.title = "Test task"
        mock_task.status.value = "pending"
        mock_task.assigned_to = "worker-1"
        mock_task.dod = "Definition of done"

        mock_store = MagicMock()
        mock_store.load_all.return_value = [mock_task]

        server._task_store = mock_store
        server._c4_project_id = "test"

        items = server._get_task_completions("T-")

        assert len(items) == 1
        item = items[0]

        assert item.label == "T-001"
        assert item.kind == lsp.CompletionItemKind.Reference
        assert "Test task" in item.detail
        assert item.insert_text == "T-001"
        assert item.data["type"] == "c4_task"
        assert item.data["task_id"] == "T-001"


class TestHandleCompletionWithTasks:
    """Integration tests for completion with task support."""

    def test_completion_returns_tasks_for_t_prefix(self):
        """Completion should return tasks when T- is typed."""
        server = C4LSPServer()
        test_content = "# See T-\n"
        server.analyzer.add_file("/test/file.py", test_content)

        mock_task = MagicMock()
        mock_task.id = "T-001"
        mock_task.title = "Test task"
        mock_task.status.value = "pending"
        mock_task.assigned_to = None
        mock_task.dod = "Do it"

        mock_store = MagicMock()
        mock_store.load_all.return_value = [mock_task]

        server._task_store = mock_store
        server._c4_project_id = "test"

        params = lsp.CompletionParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=0, character=8),
        )

        result = server._handle_completion(params)

        assert result is not None
        assert len(result.items) >= 1

        # Find the task completion
        task_items = [i for i in result.items if i.label == "T-001"]
        assert len(task_items) == 1

    def test_completion_includes_symbols_without_task_prefix(self):
        """Completion without task prefix should only include symbols."""
        server = C4LSPServer()
        test_content = "def Task_function():\n    pass\n"
        server.analyzer.add_file("/test/file.py", test_content)

        mock_task = MagicMock()
        mock_task.id = "T-001"
        mock_task.title = "Test task"
        mock_task.status.value = "pending"
        mock_task.assigned_to = None
        mock_task.dod = "Do it"

        mock_store = MagicMock()
        mock_store.load_all.return_value = [mock_task]

        server._task_store = mock_store
        server._c4_project_id = "test"

        params = lsp.CompletionParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=0, character=4),  # At "Task" - not a task prefix
        )

        result = server._handle_completion(params)

        assert result is not None
        labels = [item.label for item in result.items]

        # "Task" is not a task prefix (no dash), so only symbols should be returned
        assert "Task_function" in labels
        assert "T-001" not in labels  # Task should not be included without T- prefix

    def test_tasks_sorted_before_symbols(self):
        """Tasks should be sorted before symbols in completion."""
        server = C4LSPServer()
        test_content = "def test():\n    T\n"
        server.analyzer.add_file("/test/file.py", test_content)

        mock_task = MagicMock()
        mock_task.id = "T-001"
        mock_task.title = "Test task"
        mock_task.status.value = "pending"
        mock_task.assigned_to = None
        mock_task.dod = "Do it"

        mock_store = MagicMock()
        mock_store.load_all.return_value = [mock_task]

        server._task_store = mock_store
        server._c4_project_id = "test"

        params = lsp.CompletionParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=1, character=5),
        )

        result = server._handle_completion(params)

        if result and result.items:
            # Check that task items have lower sort_text
            for item in result.items:
                if item.data and item.data.get("type") == "c4_task":
                    assert item.sort_text.startswith("0_")
                elif item.data and item.data.get("type") == "symbol":
                    assert item.sort_text.startswith("1_")


class TestCompletionResolveWithTasks:
    """Tests for completion item resolve with task support."""

    def test_resolve_task_completion(self):
        """Should resolve task completion with full details."""
        server = C4LSPServer()

        mock_task = MagicMock()
        mock_task.id = "T-001"
        mock_task.title = "Test task"
        mock_task.status.value = "pending"
        mock_task.assigned_to = "worker-1"
        mock_task.domain = "web-backend"
        mock_task.task_type = "feature"
        mock_task.dod = "Complete this task"
        mock_task.dependencies = []

        with patch.object(server, "_get_task_info", return_value=mock_task):
            item = lsp.CompletionItem(
                label="T-001",
                data={"type": "c4_task", "task_id": "T-001"},
            )

            resolved = server._handle_completion_resolve(item)

        assert resolved.documentation is not None
        doc_value = resolved.documentation.value
        assert "T-001" in doc_value
        assert "Test task" in doc_value
        assert "pending" in doc_value

    def test_resolve_symbol_completion(self):
        """Should resolve symbol completion as before."""
        server = C4LSPServer()

        test_content = '''def greet(name):
    """Say hello."""
    return f"Hello, {name}"
'''
        server.analyzer.add_file("/test/file.py", test_content)

        item = lsp.CompletionItem(
            label="greet",
            data={
                "type": "symbol",
                "name": "greet",
                "file_path": "/test/file.py",
                "line": 1,
            },
        )

        resolved = server._handle_completion_resolve(item)

        assert resolved.documentation is not None
        doc_value = resolved.documentation.value
        assert "greet" in doc_value or "Say hello" in doc_value

    def test_resolve_without_data(self):
        """Should return item unchanged without data."""
        server = C4LSPServer()

        item = lsp.CompletionItem(label="test")

        resolved = server._handle_completion_resolve(item)

        assert resolved is item
