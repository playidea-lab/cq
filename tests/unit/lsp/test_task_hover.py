"""Tests for C4 Task ID hover functionality."""

from __future__ import annotations

from pathlib import Path
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


class TestTaskIdPattern:
    """Tests for task ID pattern matching."""

    def test_pattern_matches_simple_task_id(self):
        """Should match simple T-XXX pattern."""
        server = C4LSPServer()
        pattern = server._task_id_pattern

        assert pattern.search("T-001") is not None
        assert pattern.search("T-123") is not None
        assert pattern.search("T-999") is not None

    def test_pattern_matches_versioned_task_id(self):
        """Should match versioned T-XXX-N pattern."""
        server = C4LSPServer()
        pattern = server._task_id_pattern

        assert pattern.search("T-001-0") is not None
        assert pattern.search("T-123-5") is not None
        assert pattern.search("T-999-99") is not None

    def test_pattern_matches_review_task_id(self):
        """Should match review R-XXX pattern."""
        server = C4LSPServer()
        pattern = server._task_id_pattern

        assert pattern.search("R-001") is not None
        assert pattern.search("R-001-0") is not None
        assert pattern.search("R-123-2") is not None

    def test_pattern_matches_checkpoint_id(self):
        """Should match checkpoint CP-XXX pattern."""
        server = C4LSPServer()
        pattern = server._task_id_pattern

        assert pattern.search("CP-001") is not None
        assert pattern.search("CP-123") is not None

    def test_pattern_extracts_task_id(self):
        """Should extract the correct task ID."""
        server = C4LSPServer()
        pattern = server._task_id_pattern

        match = pattern.search("See task T-001-0 for details")
        assert match is not None
        assert match.group(1) == "T-001-0"

        match = pattern.search("Review R-042 completed")
        assert match is not None
        assert match.group(1) == "R-042"

    def test_pattern_rejects_invalid_ids(self):
        """Should not match invalid patterns."""
        server = C4LSPServer()
        pattern = server._task_id_pattern

        # Too few digits
        assert pattern.search("T-01") is None
        assert pattern.search("T-1") is None

        # Invalid prefix
        assert pattern.search("X-001") is None
        assert pattern.search("TASK-001") is None

        # Missing hyphen
        assert pattern.search("T001") is None


class TestGetTaskIdAtPosition:
    """Tests for extracting task ID at cursor position."""

    def test_extract_task_id_at_start(self):
        """Should extract task ID when cursor is at start."""
        server = C4LSPServer()
        test_content = "T-001-0 is the first task\n"
        server.analyzer.add_file("/test/file.py", test_content)

        position = lsp.Position(line=0, character=0)
        task_id = server._get_task_id_at_position("file:///test/file.py", position)

        assert task_id == "T-001-0"

    def test_extract_task_id_at_middle(self):
        """Should extract task ID when cursor is in middle."""
        server = C4LSPServer()
        test_content = "See T-123 for details\n"
        server.analyzer.add_file("/test/file.py", test_content)

        # Cursor at 'T' in T-123
        position = lsp.Position(line=0, character=4)
        task_id = server._get_task_id_at_position("file:///test/file.py", position)
        assert task_id == "T-123"

        # Cursor at '2' in T-123
        position = lsp.Position(line=0, character=7)
        task_id = server._get_task_id_at_position("file:///test/file.py", position)
        assert task_id == "T-123"

    def test_extract_task_id_at_end(self):
        """Should extract task ID when cursor is at end."""
        server = C4LSPServer()
        test_content = "Complete T-001-5\n"
        server.analyzer.add_file("/test/file.py", test_content)

        # Cursor at '5' at end
        position = lsp.Position(line=0, character=15)
        task_id = server._get_task_id_at_position("file:///test/file.py", position)

        assert task_id == "T-001-5"

    def test_no_task_id_at_position(self):
        """Should return None when no task ID at position."""
        server = C4LSPServer()
        test_content = "This is regular text\n"
        server.analyzer.add_file("/test/file.py", test_content)

        position = lsp.Position(line=0, character=5)
        task_id = server._get_task_id_at_position("file:///test/file.py", position)

        assert task_id is None

    def test_cursor_between_task_ids(self):
        """Should not match when cursor is between task IDs."""
        server = C4LSPServer()
        test_content = "T-001 and T-002\n"
        server.analyzer.add_file("/test/file.py", test_content)

        # Cursor at 'a' in 'and'
        position = lsp.Position(line=0, character=6)
        task_id = server._get_task_id_at_position("file:///test/file.py", position)

        assert task_id is None


class TestFormatTaskHover:
    """Tests for formatting task hover content."""

    def test_format_task_not_found(self):
        """Should show 'Task not found' when task doesn't exist."""
        server = C4LSPServer()
        # Don't set up store - will return None

        content = server._format_task_hover("T-999")

        assert "T-999" in content
        assert "Task not found" in content

    def test_format_task_with_full_info(self):
        """Should format all task fields correctly."""
        server = C4LSPServer()

        # Mock the task store
        mock_task = MagicMock()
        mock_task.id = "T-001-0"
        mock_task.title = "Implement hover feature"
        mock_task.status.value = "in_progress"
        mock_task.assigned_to = "worker-1"
        mock_task.domain = "web-backend"
        mock_task.task_type = "feature"
        mock_task.dod = "1. Implement feature\n2. Write tests\n3. Pass lint"
        mock_task.dependencies = ["T-000"]

        with patch.object(server, "_get_task_info", return_value=mock_task):
            content = server._format_task_hover("T-001-0")

        assert "T-001-0" in content
        assert "Implement hover feature" in content
        assert "in_progress" in content
        assert "worker-1" in content
        assert "web-backend" in content
        assert "Definition of Done" in content
        assert "Dependencies" in content
        assert "T-000" in content

    def test_format_task_minimal_info(self):
        """Should handle task with minimal info."""
        server = C4LSPServer()

        mock_task = MagicMock()
        mock_task.id = "T-002"
        mock_task.title = "Simple task"
        mock_task.status.value = "pending"
        mock_task.assigned_to = None
        mock_task.domain = None
        mock_task.task_type = None
        mock_task.dod = "Do something"
        mock_task.dependencies = []

        with patch.object(server, "_get_task_info", return_value=mock_task):
            content = server._format_task_hover("T-002")

        assert "T-002" in content
        assert "Simple task" in content
        assert "pending" in content
        assert "Assigned to" not in content  # Not shown when None
        assert "Domain" not in content  # Not shown when None

    def test_format_task_truncates_long_dod(self):
        """Should truncate very long DoD."""
        server = C4LSPServer()

        long_dod = "A" * 600  # Over 500 chars

        mock_task = MagicMock()
        mock_task.id = "T-003"
        mock_task.title = "Task with long DoD"
        mock_task.status.value = "pending"
        mock_task.assigned_to = None
        mock_task.domain = None
        mock_task.task_type = None
        mock_task.dod = long_dod
        mock_task.dependencies = []

        with patch.object(server, "_get_task_info", return_value=mock_task):
            content = server._format_task_hover("T-003")

        # Should be truncated with ellipsis
        assert "..." in content
        assert len(content) < len(long_dod) + 200  # Some overhead for formatting


class TestHoverOnTaskId:
    """Integration tests for hover on task IDs."""

    def test_hover_returns_task_info(self):
        """Hover on task ID should return task information."""
        server = C4LSPServer()
        test_content = "# See T-001-0 for implementation details\n"
        server.analyzer.add_file("/test/file.py", test_content)

        mock_task = MagicMock()
        mock_task.id = "T-001-0"
        mock_task.title = "Test hover feature"
        mock_task.status.value = "done"
        mock_task.assigned_to = "worker-1"
        mock_task.domain = None
        mock_task.task_type = None
        mock_task.dod = "Test DoD"
        mock_task.dependencies = []

        with patch.object(server, "_get_task_info", return_value=mock_task):
            params = lsp.HoverParams(
                text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
                position=lsp.Position(line=0, character=6),  # At 'T' of T-001-0
            )

            result = server._handle_hover(params)

        assert result is not None
        assert "T-001-0" in result.contents.value
        assert "Test hover feature" in result.contents.value
        assert "done" in result.contents.value

    def test_hover_on_task_id_not_found(self):
        """Hover on task ID should show 'not found' when task doesn't exist."""
        server = C4LSPServer()
        test_content = "# See T-999 for implementation details\n"
        server.analyzer.add_file("/test/file.py", test_content)

        # No mock - will return None from _get_task_info
        params = lsp.HoverParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=0, character=6),
        )

        result = server._handle_hover(params)

        assert result is not None
        assert "T-999" in result.contents.value
        assert "Task not found" in result.contents.value

    def test_hover_falls_back_to_symbol_hover(self):
        """Hover should fall back to symbol hover when not on task ID."""
        server = C4LSPServer()
        test_content = '''def greet(name):
    """Say hello."""
    return f"Hello, {name}"
'''
        server.analyzer.add_file("/test/file.py", test_content)

        params = lsp.HoverParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=0, character=5),  # At 'greet'
        )

        result = server._handle_hover(params)

        assert result is not None
        assert "greet" in result.contents.value
        assert "Function" in result.contents.value
        # Should not have task-specific content
        assert "Task" not in result.contents.value or "Task not found" not in result.contents.value


class TestTaskStoreInitialization:
    """Tests for task store lazy initialization."""

    def test_get_task_store_returns_none_without_c4_dir(self):
        """Should return None when .c4 directory doesn't exist."""
        server = C4LSPServer()
        server._workspace_root = Path("/nonexistent/path")

        store = server._get_task_store()

        assert store is None

    def test_get_task_store_caches_result(self):
        """Should cache the task store instance."""
        server = C4LSPServer()

        # Set a mock store
        mock_store = MagicMock()
        server._task_store = mock_store

        # Should return cached store
        assert server._get_task_store() is mock_store

    def test_get_task_info_returns_none_without_store(self):
        """Should return None when store is not available."""
        server = C4LSPServer()

        result = server._get_task_info("T-001")

        assert result is None
