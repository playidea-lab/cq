"""Unit tests for C4 LSP Server

Tests cover:
- Server initialization
- LSP lifecycle (initialize, shutdown, exit)
- Document synchronization (didOpen, didClose)
- C4 state and task integration
- Message serialization
"""

from datetime import datetime
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from c4.lsp.server import (
    ClientInfo,
    LSPServer,
    OpenDocument,
    ServerCapabilities,
)
from c4.models import C4State, ProjectStatus, Task, TaskStatus
from c4.store import StateNotFoundError


class TestServerCapabilities:
    """Test ServerCapabilities dataclass."""

    def test_default_values(self):
        """Default capabilities are set correctly."""
        caps = ServerCapabilities()

        assert caps.text_document_sync == 1  # Full sync
        assert caps.workspace_folders is True

    def test_custom_values(self):
        """Custom capabilities can be set."""
        caps = ServerCapabilities(text_document_sync=2, workspace_folders=False)

        assert caps.text_document_sync == 2
        assert caps.workspace_folders is False


class TestClientInfo:
    """Test ClientInfo dataclass."""

    def test_default_values(self):
        """Default client info is unknown."""
        info = ClientInfo()

        assert info.name == "unknown"
        assert info.version == "unknown"
        assert info.capabilities == {}

    def test_custom_values(self):
        """Client info from IDE connection."""
        info = ClientInfo(
            name="vscode",
            version="1.80.0",
            capabilities={"textDocument": {"hover": True}},
        )

        assert info.name == "vscode"
        assert info.version == "1.80.0"
        assert info.capabilities["textDocument"]["hover"] is True


class TestOpenDocument:
    """Test OpenDocument dataclass."""

    def test_document_tracking(self):
        """Document is tracked with all properties."""
        doc = OpenDocument(
            uri="file:///test/file.py",
            language_id="python",
            version=1,
            text="print('hello')",
        )

        assert doc.uri == "file:///test/file.py"
        assert doc.language_id == "python"
        assert doc.version == 1
        assert doc.text == "print('hello')"


class TestLSPServerInit:
    """Test LSPServer initialization."""

    def test_init_with_defaults(self, tmp_path: Path):
        """Server initializes with default stores."""
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()

        server = LSPServer(c4_dir)

        assert server.c4_dir == c4_dir
        assert server.initialized is False
        assert server.client_info.name == "unknown"
        assert server.open_documents == {}

    def test_init_with_custom_stores(self, tmp_path: Path):
        """Server initializes with custom stores."""
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()

        mock_state_store = MagicMock()
        mock_task_store = MagicMock()

        server = LSPServer(
            c4_dir,
            state_store=mock_state_store,
            task_store=mock_task_store,
        )

        assert server._state_store == mock_state_store
        assert server._task_store == mock_task_store


class TestLSPLifecycle:
    """Test LSP lifecycle handlers."""

    @pytest.fixture
    def server(self, tmp_path: Path) -> LSPServer:
        """Create a server instance for testing."""
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()
        return LSPServer(c4_dir)

    def test_initialize_returns_capabilities(self, server: LSPServer):
        """Initialize returns server capabilities."""
        params = {
            "clientInfo": {
                "name": "test-client",
                "version": "1.0.0",
            },
            "capabilities": {},
        }

        result = server._handle_initialize(params)

        assert "capabilities" in result
        assert "textDocumentSync" in result["capabilities"]
        assert "serverInfo" in result
        assert result["serverInfo"]["name"] == "c4-lsp"

    def test_initialize_extracts_client_info(self, server: LSPServer):
        """Initialize extracts client information."""
        params = {
            "clientInfo": {
                "name": "vscode",
                "version": "1.80.0",
            },
            "capabilities": {"textDocument": {}},
        }

        server._handle_initialize(params)

        assert server.client_info.name == "vscode"
        assert server.client_info.version == "1.80.0"
        assert "textDocument" in server.client_info.capabilities

    def test_initialized_sets_flag(self, server: LSPServer):
        """Initialized notification sets ready flag."""
        assert server.initialized is False

        server._handle_initialized({})

        assert server.initialized is True

    def test_shutdown_sets_flag(self, server: LSPServer):
        """Shutdown request sets shutdown flag."""
        assert server._shutdown_requested is False

        result = server._handle_shutdown({})

        assert server._shutdown_requested is True
        assert result is None  # LSP spec: return null

    def test_exit_with_shutdown(self, server: LSPServer):
        """Exit after shutdown returns code 0."""
        server._shutdown_requested = True

        with pytest.raises(SystemExit) as exc_info:
            server._handle_exit({})

        assert exc_info.value.code == 0

    def test_exit_without_shutdown(self, server: LSPServer):
        """Exit without shutdown returns code 1."""
        assert server._shutdown_requested is False

        with pytest.raises(SystemExit) as exc_info:
            server._handle_exit({})

        assert exc_info.value.code == 1


class TestDocumentSynchronization:
    """Test document synchronization handlers."""

    @pytest.fixture
    def server(self, tmp_path: Path) -> LSPServer:
        """Create a server instance for testing."""
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()
        return LSPServer(c4_dir)

    def test_did_open_tracks_document(self, server: LSPServer):
        """didOpen adds document to tracking."""
        params = {
            "textDocument": {
                "uri": "file:///test/file.py",
                "languageId": "python",
                "version": 1,
                "text": "print('hello')",
            }
        }

        server._handle_did_open(params)

        assert "file:///test/file.py" in server.open_documents
        doc = server.open_documents["file:///test/file.py"]
        assert doc.language_id == "python"
        assert doc.version == 1
        assert doc.text == "print('hello')"

    def test_did_close_removes_document(self, server: LSPServer):
        """didClose removes document from tracking."""
        # First open
        server._open_documents["file:///test/file.py"] = OpenDocument(
            uri="file:///test/file.py",
            language_id="python",
            version=1,
            text="",
        )

        params = {
            "textDocument": {
                "uri": "file:///test/file.py",
            }
        }

        server._handle_did_close(params)

        assert "file:///test/file.py" not in server.open_documents

    def test_did_close_nonexistent_document(self, server: LSPServer):
        """didClose handles non-tracked document gracefully."""
        params = {
            "textDocument": {
                "uri": "file:///nonexistent.py",
            }
        }

        # Should not raise
        server._handle_did_close(params)

    def test_c4_relevant_file_detection(self, server: LSPServer):
        """C4-relevant files are detected correctly."""
        # .c4 directory files
        assert server._is_c4_relevant("file:///project/.c4/state.json") is True
        assert server._is_c4_relevant("file:///project/.c4/tasks.db") is True

        # Non-C4 files
        assert server._is_c4_relevant("file:///project/src/main.py") is False
        assert server._is_c4_relevant("file:///project/README.md") is False


class TestC4Integration:
    """Test C4 state and task integration."""

    @pytest.fixture
    def server(self, tmp_path: Path) -> LSPServer:
        """Create a server instance with mocked stores."""
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()

        mock_state_store = MagicMock()
        mock_task_store = MagicMock()

        server = LSPServer(
            c4_dir,
            state_store=mock_state_store,
            task_store=mock_task_store,
        )
        return server

    def test_get_status_returns_state(self, server: LSPServer):
        """c4/getStatus returns serialized state."""
        mock_state = C4State(
            project_id="test-project",
            status=ProjectStatus.EXECUTE,
        )
        mock_state.queue.pending.append("T-001-0")
        mock_state.queue.in_progress["T-002-0"] = "worker-1"
        mock_state.queue.done.append("T-000-0")
        mock_state.updated_at = datetime(2025, 1, 1, 12, 0, 0)

        server._state_store.load.return_value = mock_state

        result = server._handle_c4_get_status({"projectId": "test-project"})

        assert result["projectId"] == "test-project"
        assert result["status"] == "EXECUTE"
        assert "T-001-0" in result["queue"]["pending"]
        assert "T-002-0" in result["queue"]["inProgress"]
        assert "T-000-0" in result["queue"]["done"]

    def test_get_status_not_found(self, server: LSPServer):
        """c4/getStatus handles missing state."""
        server._state_store.load.side_effect = StateNotFoundError("Not found")

        result = server._handle_c4_get_status({"projectId": "missing"})

        assert result["status"] == "not_initialized"
        assert "message" in result

    def test_get_tasks_returns_all(self, server: LSPServer):
        """c4/getTasks returns all tasks."""
        mock_tasks = {
            "T-001-0": Task(
                id="T-001-0",
                title="Task 1",
                dod="DoD 1",
                status=TaskStatus.PENDING,
            ),
            "T-002-0": Task(
                id="T-002-0",
                title="Task 2",
                dod="DoD 2",
                status=TaskStatus.IN_PROGRESS,
            ),
        }
        server._task_store.load_all.return_value = mock_tasks

        result = server._handle_c4_get_tasks({"projectId": "test"})

        assert result["count"] == 2
        assert len(result["tasks"]) == 2

    def test_get_tasks_with_filter(self, server: LSPServer):
        """c4/getTasks applies status filter."""
        mock_tasks = {
            "T-001-0": Task(
                id="T-001-0",
                title="Task 1",
                dod="DoD 1",
                status=TaskStatus.PENDING,
            ),
            "T-002-0": Task(
                id="T-002-0",
                title="Task 2",
                dod="DoD 2",
                status=TaskStatus.IN_PROGRESS,
            ),
        }
        server._task_store.load_all.return_value = mock_tasks

        result = server._handle_c4_get_tasks({
            "projectId": "test",
            "status": "pending",
        })

        assert result["count"] == 1
        assert result["tasks"][0]["id"] == "T-001-0"


class TestMessageProcessing:
    """Test JSON-RPC message processing."""

    @pytest.fixture
    def server(self, tmp_path: Path) -> LSPServer:
        """Create a server instance for testing."""
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()
        return LSPServer(c4_dir)

    def test_process_request_returns_response(self, server: LSPServer):
        """Request message gets a response."""
        message = {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": {},
        }

        response = server._process_message(message)

        assert response is not None
        assert response["jsonrpc"] == "2.0"
        assert response["id"] == 1
        assert "result" in response

    def test_process_notification_no_response(self, server: LSPServer):
        """Notification message gets no response."""
        # First initialize
        server._handle_initialize({})

        message = {
            "jsonrpc": "2.0",
            "method": "initialized",
            "params": {},
        }

        response = server._process_message(message)

        assert response is None

    def test_process_unknown_method(self, server: LSPServer):
        """Unknown method returns error for requests."""
        message = {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "unknownMethod",
            "params": {},
        }

        response = server._process_message(message)

        assert response is not None
        assert "error" in response
        assert response["error"]["code"] == -32601  # Method not found

    def test_make_response(self, server: LSPServer):
        """Response is correctly formatted."""
        response = server._make_response(42, {"key": "value"})

        assert response["jsonrpc"] == "2.0"
        assert response["id"] == 42
        assert response["result"] == {"key": "value"}

    def test_make_error_response(self, server: LSPServer):
        """Error response is correctly formatted."""
        response = server._make_error_response(
            42, -32600, "Invalid Request", {"detail": "test"}
        )

        assert response["jsonrpc"] == "2.0"
        assert response["id"] == 42
        assert response["error"]["code"] == -32600
        assert response["error"]["message"] == "Invalid Request"
        assert response["error"]["data"]["detail"] == "test"

    def test_make_notification(self, server: LSPServer):
        """Notification is correctly formatted."""
        notification = server._make_notification("test/event", {"data": 123})

        assert notification["jsonrpc"] == "2.0"
        assert notification["method"] == "test/event"
        assert notification["params"] == {"data": 123}
        assert "id" not in notification


class TestSerialization:
    """Test state and task serialization."""

    @pytest.fixture
    def server(self, tmp_path: Path) -> LSPServer:
        """Create a server instance for testing."""
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()
        return LSPServer(c4_dir)

    def test_serialize_state(self, server: LSPServer):
        """C4State is serialized correctly."""
        state = C4State(
            project_id="test",
            status=ProjectStatus.PLAN,
        )
        state.queue.pending.extend(["T-001", "T-002"])
        state.metrics.events_emitted = 10

        result = server._serialize_state(state)

        assert result["projectId"] == "test"
        assert result["status"] == "PLAN"
        assert result["queue"]["pending"] == ["T-001", "T-002"]
        assert result["metrics"]["eventsEmitted"] == 10

    def test_serialize_task(self, server: LSPServer):
        """Task is serialized correctly."""
        task = Task(
            id="T-001-0",
            title="Test Task",
            dod="Do something",
            status=TaskStatus.IN_PROGRESS,
            scope="src/",
            priority=5,
            dependencies=["T-000-0"],
            assigned_to="worker-1",
            branch="c4/w-T-001-0",
            domain="web-backend",
        )

        result = server._serialize_task(task)

        assert result["id"] == "T-001-0"
        assert result["title"] == "Test Task"
        assert result["status"] == "in_progress"
        assert result["scope"] == "src/"
        assert result["priority"] == 5
        assert result["dependencies"] == ["T-000-0"]
        assert result["assignedTo"] == "worker-1"
        assert result["branch"] == "c4/w-T-001-0"
        assert result["domain"] == "web-backend"
