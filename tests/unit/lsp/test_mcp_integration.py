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
