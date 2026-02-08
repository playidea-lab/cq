"""Tests for the C4 Bridge Server (JSON-RPC over TCP).

Tests the bridge layer that allows Go MCP server to call Python
implementations for LSP, Knowledge, and GPU operations.

Follows TDD: RED phase defines expectations, GREEN makes them pass.
"""

import asyncio
import json
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

async def send_rpc(reader: asyncio.StreamReader, writer: asyncio.StreamWriter, method: str, params: dict) -> dict:
    """Send a JSON-RPC request and read the response."""
    request = json.dumps({"method": method, "params": params}) + "\n"
    writer.write(request.encode())
    await writer.drain()
    line = await asyncio.wait_for(reader.readline(), timeout=5.0)
    return json.loads(line.decode())


# ---------------------------------------------------------------------------
# Import Tests
# ---------------------------------------------------------------------------

class TestImports:
    """Verify the bridge module is importable."""

    def test_import_bridge_package(self):
        import c4.bridge
        assert hasattr(c4.bridge, "__version__")

    def test_import_bridge_server(self):
        from c4.bridge.rpc_server import BridgeServer
        assert BridgeServer is not None

    def test_import_sidecar(self):
        from c4.bridge.sidecar import main
        assert callable(main)


# ---------------------------------------------------------------------------
# BridgeServer Construction
# ---------------------------------------------------------------------------

class TestBridgeServerConstruction:
    """Test server instantiation and configuration."""

    def test_default_port(self):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer()
        assert server.port == 50051

    def test_custom_port(self):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer(port=9999)
        assert server.port == 9999

    def test_env_port(self, monkeypatch):
        monkeypatch.setenv("C4_BRIDGE_PORT", "12345")
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer()
        assert server.port == 12345

    def test_explicit_port_overrides_env(self, monkeypatch):
        monkeypatch.setenv("C4_BRIDGE_PORT", "12345")
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer(port=9999)
        assert server.port == 9999

    def test_custom_project_root(self, tmp_path):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer(project_root=tmp_path)
        assert server.project_root == tmp_path

    def test_default_project_root_is_cwd(self):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer()
        assert server.project_root == Path.cwd()


# ---------------------------------------------------------------------------
# Method Registry
# ---------------------------------------------------------------------------

class TestMethodRegistry:
    """Test that all expected RPC methods are registered."""

    def test_has_lsp_methods(self):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer()
        expected = [
            "FindSymbol",
            "GetSymbolsOverview",
            "ReplaceSymbolBody",
            "InsertBeforeSymbol",
            "InsertAfterSymbol",
            "RenameSymbol",
        ]
        for method in expected:
            assert method in server.methods, f"Missing method: {method}"

    def test_has_knowledge_methods(self):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer()
        expected = [
            "KnowledgeSearch",
            "KnowledgeRecord",
            "KnowledgeGet",
        ]
        for method in expected:
            assert method in server.methods, f"Missing method: {method}"

    def test_has_gpu_methods(self):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer()
        expected = [
            "GPUStatus",
            "JobSubmit",
        ]
        for method in expected:
            assert method in server.methods, f"Missing method: {method}"

    def test_unknown_method_returns_error(self):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer()
        assert "NonExistentMethod" not in server.methods


# ---------------------------------------------------------------------------
# Protocol Tests (JSON-RPC over TCP)
# ---------------------------------------------------------------------------

class TestProtocol:
    """Test the JSON-RPC wire protocol."""

    @pytest.fixture
    async def server_and_port(self, tmp_path):
        """Start a bridge server on a random port for testing."""
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer(port=0, project_root=tmp_path)  # port=0 -> OS-assigned
        actual_port = await server.start()
        yield server, actual_port
        await server.stop()

    @pytest.mark.asyncio
    async def test_valid_request_gets_response(self, server_and_port):
        server, port = server_and_port
        reader, writer = await asyncio.open_connection("127.0.0.1", port)
        try:
            # GPUStatus has minimal deps, good for protocol testing
            resp = await send_rpc(reader, writer, "GPUStatus", {})
            assert "result" in resp or "error" in resp
        finally:
            writer.close()
            await writer.wait_closed()

    @pytest.mark.asyncio
    async def test_unknown_method_returns_error(self, server_and_port):
        server, port = server_and_port
        reader, writer = await asyncio.open_connection("127.0.0.1", port)
        try:
            resp = await send_rpc(reader, writer, "DoesNotExist", {})
            assert resp.get("error") is not None
            assert "unknown method" in resp["error"].lower()
        finally:
            writer.close()
            await writer.wait_closed()

    @pytest.mark.asyncio
    async def test_malformed_json_returns_error(self, server_and_port):
        server, port = server_and_port
        reader, writer = await asyncio.open_connection("127.0.0.1", port)
        try:
            writer.write(b"this is not json\n")
            await writer.drain()
            line = await asyncio.wait_for(reader.readline(), timeout=5.0)
            resp = json.loads(line.decode())
            assert resp.get("error") is not None
        finally:
            writer.close()
            await writer.wait_closed()

    @pytest.mark.asyncio
    async def test_missing_method_field_returns_error(self, server_and_port):
        server, port = server_and_port
        reader, writer = await asyncio.open_connection("127.0.0.1", port)
        try:
            writer.write(json.dumps({"params": {}}).encode() + b"\n")
            await writer.drain()
            line = await asyncio.wait_for(reader.readline(), timeout=5.0)
            resp = json.loads(line.decode())
            assert resp.get("error") is not None
        finally:
            writer.close()
            await writer.wait_closed()

    @pytest.mark.asyncio
    async def test_concurrent_requests(self, server_and_port):
        """Multiple clients can connect simultaneously."""
        server, port = server_and_port

        async def single_request():
            reader, writer = await asyncio.open_connection("127.0.0.1", port)
            try:
                resp = await send_rpc(reader, writer, "GPUStatus", {})
                return resp
            finally:
                writer.close()
                await writer.wait_closed()

        results = await asyncio.gather(*[single_request() for _ in range(5)])
        assert len(results) == 5
        for resp in results:
            assert "result" in resp or "error" in resp

    @pytest.mark.asyncio
    async def test_multiple_requests_same_connection(self, server_and_port):
        """Multiple sequential requests on one connection."""
        server, port = server_and_port
        reader, writer = await asyncio.open_connection("127.0.0.1", port)
        try:
            for _ in range(3):
                resp = await send_rpc(reader, writer, "GPUStatus", {})
                assert "result" in resp or "error" in resp
        finally:
            writer.close()
            await writer.wait_closed()


# ---------------------------------------------------------------------------
# LSP Method Delegation Tests
# ---------------------------------------------------------------------------

class TestLSPDelegation:
    """Test that LSP methods correctly delegate to CodeOps."""

    @pytest.fixture
    def server(self, tmp_path):
        from c4.bridge.rpc_server import BridgeServer
        return BridgeServer(project_root=tmp_path)

    @pytest.mark.asyncio
    async def test_find_symbol_delegates(self, server):
        mock_result = {
            "success": True,
            "symbols": [{"name": "MyClass", "kind": "class"}],
            "count": 1,
        }
        with patch.object(server._code_ops, "find_symbol", return_value=mock_result) as mock:
            result = await server.dispatch("FindSymbol", {
                "name": "MyClass",
                "file_path": "test.py",
            })
            mock.assert_called_once()
            assert result["success"] is True

    @pytest.mark.asyncio
    async def test_get_symbols_overview_delegates(self, server):
        mock_result = {
            "success": True,
            "file": "test.py",
            "symbols": [],
        }
        with patch.object(server._code_ops, "get_symbols_overview", return_value=mock_result) as mock:
            result = await server.dispatch("GetSymbolsOverview", {
                "file_path": "test.py",
            })
            mock.assert_called_once()
            assert result["success"] is True

    @pytest.mark.asyncio
    async def test_replace_symbol_body_delegates(self, server):
        mock_result = {"success": True, "lines_replaced": 5}
        with patch.object(server._code_ops, "replace_symbol_body", return_value=mock_result) as mock:
            result = await server.dispatch("ReplaceSymbolBody", {
                "file_path": "test.py",
                "symbol_name": "my_func",
                "new_body": "def my_func():\n    pass\n",
            })
            mock.assert_called_once()
            assert result["success"] is True

    @pytest.mark.asyncio
    async def test_insert_before_symbol_delegates(self, server):
        mock_result = {"success": True, "lines_inserted": 2}
        with patch.object(server._code_ops, "insert_before_symbol", return_value=mock_result) as mock:
            result = await server.dispatch("InsertBeforeSymbol", {
                "file_path": "test.py",
                "symbol_name": "my_func",
                "content": "# comment\n",
            })
            mock.assert_called_once()
            assert result["success"] is True

    @pytest.mark.asyncio
    async def test_insert_after_symbol_delegates(self, server):
        mock_result = {"success": True, "lines_inserted": 2}
        with patch.object(server._code_ops, "insert_after_symbol", return_value=mock_result) as mock:
            result = await server.dispatch("InsertAfterSymbol", {
                "file_path": "test.py",
                "symbol_name": "my_func",
                "content": "# comment\n",
            })
            mock.assert_called_once()
            assert result["success"] is True

    @pytest.mark.asyncio
    async def test_rename_symbol_delegates(self, server):
        mock_result = {"success": True, "total_replacements": 3}
        with patch.object(server._code_ops, "rename_symbol", return_value=mock_result) as mock:
            result = await server.dispatch("RenameSymbol", {
                "file_path": "test.py",
                "old_name": "old_func",
                "new_name": "new_func",
            })
            mock.assert_called_once()
            assert result["success"] is True


# ---------------------------------------------------------------------------
# Knowledge Method Delegation Tests
# ---------------------------------------------------------------------------

class TestKnowledgeDelegation:
    """Test that Knowledge methods correctly delegate to document store."""

    @pytest.fixture
    def server(self, tmp_path):
        from c4.bridge.rpc_server import BridgeServer
        return BridgeServer(project_root=tmp_path)

    @pytest.mark.asyncio
    async def test_knowledge_search_delegates(self, server):
        mock_results = [{"slug": "exp-001", "title": "Test", "score": 0.9}]
        with patch("c4.bridge.rpc_server.KnowledgeSearcher") as MockSearcher:
            MockSearcher.return_value.search.return_value = mock_results
            result = await server.dispatch("KnowledgeSearch", {
                "query": "test query",
                "top_k": 5,
            })
            assert result["count"] == 1

    @pytest.mark.asyncio
    async def test_knowledge_search_requires_query(self, server):
        result = await server.dispatch("KnowledgeSearch", {})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_knowledge_record_delegates(self, server):
        with patch("c4.bridge.rpc_server.DocumentStore") as MockStore:
            MockStore.return_value.create.return_value = "exp-001"
            MockStore.return_value.get.return_value = None  # skip embedding
            result = await server.dispatch("KnowledgeRecord", {
                "doc_type": "experiment",
                "title": "Test Experiment",
                "body": "Some content",
            })
            assert result["success"] is True
            assert result["doc_id"] == "exp-001"

    @pytest.mark.asyncio
    async def test_knowledge_record_requires_doc_type(self, server):
        result = await server.dispatch("KnowledgeRecord", {"title": "Test"})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_knowledge_record_requires_title(self, server):
        result = await server.dispatch("KnowledgeRecord", {"doc_type": "pattern"})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_knowledge_get_delegates(self, server):
        mock_doc = MagicMock()
        mock_doc.model_dump.return_value = {
            "slug": "exp-001",
            "title": "Test",
            "doc_type": "experiment",
        }
        with patch("c4.bridge.rpc_server.DocumentStore") as MockStore:
            MockStore.return_value.get.return_value = mock_doc
            MockStore.return_value.get_backlinks.return_value = []
            result = await server.dispatch("KnowledgeGet", {"doc_id": "exp-001"})
            assert result["slug"] == "exp-001"

    @pytest.mark.asyncio
    async def test_knowledge_get_not_found(self, server):
        with patch("c4.bridge.rpc_server.DocumentStore") as MockStore:
            MockStore.return_value.get.return_value = None
            result = await server.dispatch("KnowledgeGet", {"doc_id": "nope"})
            assert "error" in result

    @pytest.mark.asyncio
    async def test_knowledge_get_requires_doc_id(self, server):
        result = await server.dispatch("KnowledgeGet", {})
        assert "error" in result


# ---------------------------------------------------------------------------
# GPU Method Delegation Tests
# ---------------------------------------------------------------------------

class TestGPUDelegation:
    """Test that GPU methods correctly delegate to monitor/scheduler."""

    @pytest.fixture
    def server(self, tmp_path):
        from c4.bridge.rpc_server import BridgeServer
        return BridgeServer(project_root=tmp_path)

    @pytest.mark.asyncio
    async def test_gpu_status_delegates(self, server):
        mock_gpu = MagicMock()
        mock_gpu.index = 0
        mock_gpu.name = "Test GPU"
        mock_gpu.backend = "mps"
        mock_gpu.vram_total_gb = 16.0
        mock_gpu.vram_free_gb = 12.0
        mock_gpu.gpu_utilization = 25.0

        with patch("c4.bridge.rpc_server.GpuMonitor") as MockMonitor:
            # GpuMonitor is get_gpu_monitor factory; calling it returns a monitor
            MockMonitor.return_value.get_all_gpus.return_value = [mock_gpu]
            result = await server.dispatch("GPUStatus", {})
            assert result["gpu_count"] == 1
            assert result["gpus"][0]["name"] == "Test GPU"

    @pytest.mark.asyncio
    async def test_gpu_status_no_gpus(self, server):
        with patch("c4.bridge.rpc_server.GpuMonitor") as MockMonitor:
            MockMonitor.return_value.get_all_gpus.return_value = []
            result = await server.dispatch("GPUStatus", {})
            assert result["gpu_count"] == 0
            assert result["backend"] == "cpu"

    @pytest.mark.asyncio
    async def test_job_submit_delegates(self, server):
        mock_job = MagicMock()
        mock_job.job_id = "job-123"
        with patch("c4.bridge.rpc_server.GpuJobScheduler") as MockScheduler:
            MockScheduler.return_value.submit.return_value = mock_job
            result = await server.dispatch("JobSubmit", {
                "command": "python train.py",
                "task_id": "T-001-0",
            })
            assert result["success"] is True
            assert result["job_id"] == "job-123"

    @pytest.mark.asyncio
    async def test_job_submit_requires_command(self, server):
        result = await server.dispatch("JobSubmit", {})
        assert "error" in result


# ---------------------------------------------------------------------------
# Error Handling Tests
# ---------------------------------------------------------------------------

class TestErrorHandling:
    """Test graceful error handling for all method categories."""

    @pytest.fixture
    def server(self, tmp_path):
        from c4.bridge.rpc_server import BridgeServer
        return BridgeServer(project_root=tmp_path)

    @pytest.mark.asyncio
    async def test_lsp_exception_returns_error(self, server):
        with patch.object(
            server._code_ops, "find_symbol",
            side_effect=RuntimeError("LSP crashed"),
        ):
            result = await server.dispatch("FindSymbol", {"name": "x", "file_path": "a.py"})
            assert "error" in result
            assert "LSP crashed" in result["error"]

    @pytest.mark.asyncio
    async def test_knowledge_exception_returns_error(self, server):
        with patch("c4.bridge.rpc_server.KnowledgeSearcher") as MockSearcher:
            MockSearcher.return_value.search.side_effect = RuntimeError("DB locked")
            result = await server.dispatch("KnowledgeSearch", {"query": "test"})
            assert "error" in result

    @pytest.mark.asyncio
    async def test_gpu_exception_returns_error(self, server):
        with patch("c4.bridge.rpc_server.GpuMonitor") as MockMonitor:
            MockMonitor.return_value.get_all_gpus.side_effect = RuntimeError("No CUDA")
            result = await server.dispatch("GPUStatus", {})
            assert "error" in result


# ---------------------------------------------------------------------------
# Server Lifecycle Tests
# ---------------------------------------------------------------------------

class TestPing:
    """Test the Ping health check method."""

    @pytest.fixture
    def server(self, tmp_path):
        from c4.bridge.rpc_server import BridgeServer
        return BridgeServer(project_root=tmp_path)

    def test_ping_is_registered(self, server):
        assert "Ping" in server.methods

    @pytest.mark.asyncio
    async def test_ping_returns_ok(self, server):
        result = await server.dispatch("Ping", {})
        assert result == {"status": "ok"}

    @pytest.mark.asyncio
    async def test_ping_over_tcp(self, tmp_path):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer(port=0, project_root=tmp_path)
        port = await server.start()
        try:
            reader, writer = await asyncio.open_connection("127.0.0.1", port)
            resp = await send_rpc(reader, writer, "Ping", {})
            assert resp["error"] is None
            assert resp["result"]["status"] == "ok"
            writer.close()
            await writer.wait_closed()
        finally:
            await server.stop()


# ---------------------------------------------------------------------------
# Server Lifecycle Tests
# ---------------------------------------------------------------------------

class TestServerLifecycle:
    """Test start/stop behavior."""

    @pytest.mark.asyncio
    async def test_start_returns_port(self, tmp_path):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer(port=0, project_root=tmp_path)
        port = await server.start()
        assert isinstance(port, int)
        assert port > 0
        await server.stop()

    @pytest.mark.asyncio
    async def test_stop_is_idempotent(self, tmp_path):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer(port=0, project_root=tmp_path)
        await server.start()
        await server.stop()
        await server.stop()  # Should not raise

    @pytest.mark.asyncio
    async def test_server_refuses_connections_after_stop(self, tmp_path):
        from c4.bridge.rpc_server import BridgeServer
        server = BridgeServer(port=0, project_root=tmp_path)
        port = await server.start()
        await server.stop()
        with pytest.raises((ConnectionRefusedError, OSError)):
            await asyncio.open_connection("127.0.0.1", port)
