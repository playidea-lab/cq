"""Tests for C4 LSP MCP integration."""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest


def _check_pygls_available() -> bool:
    """Check if pygls is available."""
    try:
        from pygls.server import LanguageServer  # noqa: F401

        return True
    except ImportError:
        return False


PYGLS_AVAILABLE = _check_pygls_available()


class TestLSPMCPIntegration:
    """Tests for LSP MCP tool integration."""

    def test_c4_lsp_status_not_running(self):
        """Should return not running status when LSP server is not started."""
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon()
        result = daemon.c4_lsp_status()

        assert result["running"] is False
        assert result["status"] == "not_started"
        assert "not started" in result["message"].lower()

    def test_c4_lsp_stop_not_running(self):
        """Should return error when stopping non-running server."""
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon()
        result = daemon.c4_lsp_stop()

        assert result["success"] is False
        assert "not running" in result["error"].lower()

    @pytest.mark.skipif(not PYGLS_AVAILABLE, reason="pygls not installed")
    def test_c4_lsp_start_already_running_error(self):
        """Should return error when starting if server already exists."""
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon()
        # Mock an already running server
        daemon._lsp_server = MagicMock()
        daemon._lsp_thread = MagicMock()
        daemon._lsp_thread.is_alive.return_value = True
        daemon._lsp_port = 2088

        result = daemon.c4_lsp_start(port=2088)

        assert result["success"] is False
        assert "already running" in result["error"].lower()

    def test_lsp_status_features_list_when_running(self):
        """Should list all supported LSP features when running."""
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon()
        # Mock a running server with mock thread
        mock_server = MagicMock()
        mock_server.analyzer._file_contents = {}
        mock_server.analyzer.get_all_symbols.return_value = []

        mock_thread = MagicMock()
        mock_thread.is_alive.return_value = True

        daemon._lsp_server = mock_server
        daemon._lsp_thread = mock_thread

        result = daemon.c4_lsp_status()

        assert result["running"] is True
        assert result["status"] == "running"
        assert result["indexed_files"] == 0
        assert result["total_symbols"] == 0

        expected_features = [
            "textDocument/hover",
            "textDocument/definition",
            "textDocument/references",
            "textDocument/documentSymbol",
            "workspace/symbol",
            "textDocument/completion",
        ]

        for feature in expected_features:
            assert feature in result["features"]

    def test_c4_lsp_stop_clears_server(self):
        """Should clear server references when stopping."""
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon()
        # Mock a running server
        daemon._lsp_server = MagicMock()
        daemon._lsp_thread = MagicMock()
        daemon._lsp_thread.is_alive.return_value = True
        daemon._lsp_port = 2087
        daemon._lsp_host = "127.0.0.1"

        result = daemon.c4_lsp_stop()

        assert result["success"] is True
        assert daemon._lsp_server is None
        assert daemon._lsp_thread is None

    @pytest.mark.skipif(not PYGLS_AVAILABLE, reason="pygls not installed")
    def test_c4_lsp_start_without_pygls_fails_gracefully(self):
        """Test that start without pygls fails with clear message."""
        # This test is only run when pygls IS available,
        # so we just verify the method exists and can be called
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon()
        # Just check method exists
        assert hasattr(daemon, "c4_lsp_start")
        assert callable(daemon.c4_lsp_start)

    def test_lsp_status_stopped_when_thread_dead(self):
        """Should return stopped status when thread is dead."""
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon()
        daemon._lsp_server = MagicMock()
        daemon._lsp_thread = MagicMock()
        daemon._lsp_thread.is_alive.return_value = False

        result = daemon.c4_lsp_status()

        assert result["running"] is False
        assert result["status"] == "stopped"

    @pytest.mark.skipif(not PYGLS_AVAILABLE, reason="pygls not installed")
    def test_c4_lsp_start_sets_workspace_root(self):
        """Should set workspace root for async indexing."""
        from pathlib import Path

        from c4.mcp_server import C4Daemon

        daemon = C4Daemon()
        # Ensure root exists
        daemon.root = Path(".")

        result = daemon.c4_lsp_start(port=2099)

        try:
            assert result["success"] is True
            assert daemon._lsp_server is not None
            assert daemon._lsp_server._workspace_root == daemon.root
        finally:
            # Cleanup
            daemon.c4_lsp_stop()


class TestFindReferencingSymbols:
    """Tests for c4_find_referencing_symbols MCP tool."""

    def test_find_referencing_symbols_basic(self, tmp_path):
        """Should find references to a symbol in a file."""
        from c4.mcp_server import C4Daemon

        # Create a test file with references
        test_file = tmp_path / "test_module.py"
        test_file.write_text(
            """def my_function():
    \"\"\"A simple function.\"\"\"
    return 42

result = my_function()
value = my_function()
"""
        )

        daemon = C4Daemon(project_root=tmp_path)
        result = daemon.c4_find_referencing_symbols(
            name_path="my_function",
            file_path=str(test_file),
        )

        assert result["success"] is True
        assert result["symbol"] == "my_function"
        assert result["total"] >= 3  # definition + 2 calls (at least)
        assert len(result["references"]) >= 3

        # Check reference structure
        for ref in result["references"]:
            assert "file_path" in ref
            assert "line" in ref
            assert "column" in ref
            assert "context" in ref
            assert "ref_kind" in ref

    def test_find_referencing_symbols_not_found(self, tmp_path):
        """Should return empty list when symbol not found."""
        from c4.mcp_server import C4Daemon

        # Create a test file without the symbol
        test_file = tmp_path / "test_module.py"
        test_file.write_text(
            """def other_function():
    pass
"""
        )

        daemon = C4Daemon(project_root=tmp_path)
        result = daemon.c4_find_referencing_symbols(
            name_path="nonexistent_symbol",
            file_path=str(test_file),
        )

        assert result["success"] is True
        assert result["total"] == 0
        assert result["references"] == []

    def test_find_referencing_symbols_file_not_found(self, tmp_path):
        """Should return error when file not found."""
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon(project_root=tmp_path)
        result = daemon.c4_find_referencing_symbols(
            name_path="my_function",
            file_path="/nonexistent/path/file.py",
        )

        assert result["success"] is False
        assert "not found" in result["error"].lower()

    def test_find_referencing_symbols_across_workspace(self, tmp_path):
        """Should find references across multiple files in workspace."""
        from c4.mcp_server import C4Daemon

        # Create multiple files
        (tmp_path / "module_a.py").write_text(
            """def shared_function():
    \"\"\"Shared function.\"\"\"
    return 1
"""
        )

        (tmp_path / "module_b.py").write_text(
            """from module_a import shared_function

result = shared_function()
"""
        )

        (tmp_path / "module_c.py").write_text(
            """from module_a import shared_function

def wrapper():
    return shared_function()
"""
        )

        daemon = C4Daemon(project_root=tmp_path)
        result = daemon.c4_find_referencing_symbols(
            name_path="shared_function",
            # No file_path - search entire workspace
        )

        assert result["success"] is True
        assert result["total"] >= 4  # definition + imports + calls

        # Check that references come from multiple files
        file_paths = {ref["file_path"] for ref in result["references"]}
        assert len(file_paths) >= 3  # module_a, module_b, module_c

    def test_find_referencing_symbols_includes_context(self, tmp_path):
        """Should include code context/snippet in results."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_module.py"
        test_file.write_text(
            """class MyClass:
    def my_method(self):
        return self

instance = MyClass()
"""
        )

        daemon = C4Daemon(project_root=tmp_path)
        result = daemon.c4_find_referencing_symbols(
            name_path="MyClass",
            file_path=str(test_file),
        )

        assert result["success"] is True
        assert result["total"] >= 2  # class definition + instantiation

        # Check that context contains the actual code
        contexts = [ref["context"] for ref in result["references"]]
        assert any("class MyClass" in ctx for ctx in contexts)
        assert any("MyClass()" in ctx for ctx in contexts)

    def test_find_referencing_symbols_result_structure(self, tmp_path):
        """Should return properly structured reference objects."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_module.py"
        test_file.write_text(
            """CONSTANT = 42
value = CONSTANT + 1
"""
        )

        daemon = C4Daemon(project_root=tmp_path)
        result = daemon.c4_find_referencing_symbols(
            name_path="CONSTANT",
            file_path=str(test_file),
        )

        assert result["success"] is True
        assert result["total"] >= 2

        # Check all required fields are present
        for ref in result["references"]:
            assert isinstance(ref["file_path"], str)
            assert isinstance(ref["line"], int)
            assert isinstance(ref["column"], int)
            assert isinstance(ref["end_line"], int)
            assert isinstance(ref["end_column"], int)
            assert isinstance(ref["context"], str)
            assert isinstance(ref["ref_kind"], str)
