"""C4 LSP Server - Language Server Protocol implementation

Implements LSP specification for IDE integration with C4 orchestration system.

Supported modes:
- stdio: Direct IDE connection via stdin/stdout
- TCP: Daemon-embedded server for persistent connections

Supported LSP methods:
- initialize / shutdown / exit (lifecycle)
- textDocument/didOpen / didClose (document sync)
- c4/* (custom methods for C4-specific features)
"""

from __future__ import annotations

import asyncio
import json
import logging
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from c4.models import C4State, Task
from c4.store import (
    SQLiteStateStore,
    SQLiteTaskStore,
    StateNotFoundError,
)

logger = logging.getLogger(__name__)


# LSP Constants
CONTENT_LENGTH_HEADER = "Content-Length: "
CONTENT_TYPE_HEADER = "Content-Type: "
HEADER_SEPARATOR = "\r\n\r\n"


@dataclass
class ServerCapabilities:
    """LSP Server capabilities"""

    text_document_sync: int = 1  # TextDocumentSyncKind.Full
    workspace_folders: bool = True


@dataclass
class ClientInfo:
    """Connected client information"""

    name: str = "unknown"
    version: str = "unknown"
    capabilities: dict[str, Any] = field(default_factory=dict)


@dataclass
class OpenDocument:
    """Tracked open document"""

    uri: str
    language_id: str
    version: int
    text: str


class LSPServer:
    """
    C4 LSP Server implementation.

    Provides Language Server Protocol interface for IDE integration,
    enabling real-time synchronization of project state and task queue.

    Attributes:
        c4_dir: Path to .c4 directory
        state_store: State storage backend
        task_store: Task storage backend
        initialized: Whether server has completed initialization
        client_info: Connected client information
        open_documents: Currently open documents tracked by the server

    Example:
        # Create server
        server = LSPServer(Path(".c4"))

        # Run in stdio mode
        server.run_stdio()

        # Or run in TCP mode
        await server.run_tcp("127.0.0.1", 8765)
    """

    def __init__(
        self,
        c4_dir: Path,
        state_store: SQLiteStateStore | None = None,
        task_store: SQLiteTaskStore | None = None,
    ) -> None:
        """
        Initialize LSP server.

        Args:
            c4_dir: Path to .c4 directory containing state and tasks
            state_store: Optional custom state store (defaults to SQLite)
            task_store: Optional custom task store (defaults to SQLite)
        """
        self.c4_dir = c4_dir
        self._initialized = False
        self._shutdown_requested = False
        self._client_info = ClientInfo()
        self._open_documents: dict[str, OpenDocument] = {}

        # Request ID tracking
        self._next_request_id = 1

        # Initialize stores
        if state_store is not None:
            self._state_store = state_store
        else:
            db_path = c4_dir / "state.db"
            self._state_store = SQLiteStateStore(db_path)

        if task_store is not None:
            self._task_store = task_store
        else:
            db_path = c4_dir / "tasks.db"
            self._task_store = SQLiteTaskStore(db_path)

        # Async transport references
        self._reader: asyncio.StreamReader | None = None
        self._writer: asyncio.StreamWriter | None = None

        # Method handlers registry
        self._handlers: dict[str, Any] = {
            # Lifecycle
            "initialize": self._handle_initialize,
            "initialized": self._handle_initialized,
            "shutdown": self._handle_shutdown,
            "exit": self._handle_exit,
            # Document synchronization
            "textDocument/didOpen": self._handle_did_open,
            "textDocument/didClose": self._handle_did_close,
            # C4 custom methods
            "c4/getStatus": self._handle_c4_get_status,
            "c4/getTasks": self._handle_c4_get_tasks,
        }

    @property
    def initialized(self) -> bool:
        """Whether the server has been initialized."""
        return self._initialized

    @property
    def client_info(self) -> ClientInfo:
        """Information about the connected client."""
        return self._client_info

    @property
    def open_documents(self) -> dict[str, OpenDocument]:
        """Currently tracked open documents."""
        return self._open_documents

    # =========================================================================
    # Transport - stdio mode
    # =========================================================================

    def run_stdio(self) -> None:
        """
        Run server in stdio mode (blocking).

        Reads JSON-RPC messages from stdin and writes responses to stdout.
        This mode is suitable for direct IDE connections.
        """
        logger.info("Starting LSP server in stdio mode")

        try:
            while not self._shutdown_requested:
                # Read message
                message = self._read_stdio_message()
                if message is None:
                    break

                # Process and respond
                response = self._process_message(message)
                if response is not None:
                    self._write_stdio_message(response)

        except Exception as e:
            logger.exception(f"Error in stdio loop: {e}")
            raise

        logger.info("LSP server stdio loop ended")

    def _read_stdio_message(self) -> dict[str, Any] | None:
        """Read a single LSP message from stdin."""
        # Read headers
        headers: dict[str, str] = {}
        while True:
            line = sys.stdin.readline()
            if not line:
                return None  # EOF
            line = line.strip()
            if not line:
                break  # End of headers

            if line.startswith(CONTENT_LENGTH_HEADER.strip()):
                headers["Content-Length"] = line[len(CONTENT_LENGTH_HEADER.strip()) + 1 :].strip()
            elif line.startswith(CONTENT_TYPE_HEADER.strip()):
                headers["Content-Type"] = line[len(CONTENT_TYPE_HEADER.strip()) + 1 :].strip()

        # Read content
        content_length = int(headers.get("Content-Length", "0"))
        if content_length == 0:
            return None

        content = sys.stdin.read(content_length)
        return json.loads(content)

    def _write_stdio_message(self, message: dict[str, Any]) -> None:
        """Write a single LSP message to stdout."""
        content = json.dumps(message)
        content_bytes = content.encode("utf-8")

        sys.stdout.write(f"Content-Length: {len(content_bytes)}\r\n")
        sys.stdout.write("\r\n")
        sys.stdout.write(content)
        sys.stdout.flush()

    # =========================================================================
    # Transport - TCP mode
    # =========================================================================

    async def run_tcp(self, host: str = "127.0.0.1", port: int = 8765) -> None:
        """
        Run server in TCP mode (async).

        Starts a TCP server that accepts LSP connections.
        This mode is suitable for daemon embedding.

        Args:
            host: Host address to bind to
            port: Port number to listen on
        """
        logger.info(f"Starting LSP server in TCP mode on {host}:{port}")

        server = await asyncio.start_server(
            self._handle_tcp_connection,
            host,
            port,
        )

        async with server:
            await server.serve_forever()

    async def _handle_tcp_connection(
        self,
        reader: asyncio.StreamReader,
        writer: asyncio.StreamWriter,
    ) -> None:
        """Handle a single TCP connection."""
        self._reader = reader
        self._writer = writer
        peer = writer.get_extra_info("peername")
        logger.info(f"New TCP connection from {peer}")

        try:
            while not self._shutdown_requested:
                message = await self._read_tcp_message()
                if message is None:
                    break

                response = self._process_message(message)
                if response is not None:
                    await self._write_tcp_message(response)

        except asyncio.CancelledError:
            logger.info(f"Connection cancelled: {peer}")
        except Exception as e:
            logger.exception(f"Error handling TCP connection: {e}")
        finally:
            writer.close()
            await writer.wait_closed()
            self._reader = None
            self._writer = None
            logger.info(f"TCP connection closed: {peer}")

    async def _read_tcp_message(self) -> dict[str, Any] | None:
        """Read a single LSP message from TCP stream."""
        if self._reader is None:
            return None

        # Read headers
        headers: dict[str, str] = {}
        while True:
            line = await self._reader.readline()
            if not line:
                return None  # Connection closed
            line_str = line.decode("utf-8").strip()
            if not line_str:
                break  # End of headers

            if ":" in line_str:
                key, value = line_str.split(":", 1)
                headers[key.strip()] = value.strip()

        # Read content
        content_length = int(headers.get("Content-Length", "0"))
        if content_length == 0:
            return None

        content = await self._reader.read(content_length)
        return json.loads(content.decode("utf-8"))

    async def _write_tcp_message(self, message: dict[str, Any]) -> None:
        """Write a single LSP message to TCP stream."""
        if self._writer is None:
            return

        content = json.dumps(message)
        content_bytes = content.encode("utf-8")

        header = f"Content-Length: {len(content_bytes)}\r\n\r\n"
        self._writer.write(header.encode("utf-8"))
        self._writer.write(content_bytes)
        await self._writer.drain()

    # =========================================================================
    # Message Processing
    # =========================================================================

    def _process_message(self, message: dict[str, Any]) -> dict[str, Any] | None:
        """
        Process an incoming JSON-RPC message.

        Args:
            message: The parsed JSON-RPC message

        Returns:
            Response message if request, None if notification
        """
        method = message.get("method")
        params = message.get("params", {})
        msg_id = message.get("id")

        logger.debug(f"Processing message: method={method}, id={msg_id}")

        # Find handler
        handler = self._handlers.get(method)
        if handler is None:
            logger.warning(f"Unknown method: {method}")
            if msg_id is not None:
                return self._make_error_response(
                    msg_id,
                    -32601,  # Method not found
                    f"Method not found: {method}",
                )
            return None

        try:
            result = handler(params)

            # Notifications (no id) don't get responses
            if msg_id is None:
                return None

            return self._make_response(msg_id, result)

        except Exception as e:
            logger.exception(f"Error handling {method}: {e}")
            if msg_id is not None:
                return self._make_error_response(
                    msg_id,
                    -32603,  # Internal error
                    str(e),
                )
            return None

    def _make_response(self, msg_id: int | str, result: Any) -> dict[str, Any]:
        """Create a success response."""
        return {
            "jsonrpc": "2.0",
            "id": msg_id,
            "result": result,
        }

    def _make_error_response(
        self,
        msg_id: int | str,
        code: int,
        message: str,
        data: Any = None,
    ) -> dict[str, Any]:
        """Create an error response."""
        error: dict[str, Any] = {
            "code": code,
            "message": message,
        }
        if data is not None:
            error["data"] = data

        return {
            "jsonrpc": "2.0",
            "id": msg_id,
            "error": error,
        }

    def _make_notification(self, method: str, params: Any) -> dict[str, Any]:
        """Create a notification (no id, no response expected)."""
        return {
            "jsonrpc": "2.0",
            "method": method,
            "params": params,
        }

    # =========================================================================
    # LSP Lifecycle Handlers
    # =========================================================================

    def _handle_initialize(self, params: dict[str, Any]) -> dict[str, Any]:
        """
        Handle 'initialize' request.

        This is the first message sent by the client.
        Server responds with its capabilities.
        """
        # Extract client info
        client_info = params.get("clientInfo", {})
        self._client_info = ClientInfo(
            name=client_info.get("name", "unknown"),
            version=client_info.get("version", "unknown"),
            capabilities=params.get("capabilities", {}),
        )

        logger.info(
            f"Client connected: {self._client_info.name} v{self._client_info.version}"
        )

        # Return server capabilities
        capabilities = ServerCapabilities()

        return {
            "capabilities": {
                "textDocumentSync": capabilities.text_document_sync,
                "workspace": {
                    "workspaceFolders": {
                        "supported": capabilities.workspace_folders,
                        "changeNotifications": True,
                    },
                },
            },
            "serverInfo": {
                "name": "c4-lsp",
                "version": "0.1.0",
            },
        }

    def _handle_initialized(self, params: dict[str, Any]) -> None:
        """
        Handle 'initialized' notification.

        Sent by the client after it received the initialize response.
        The server is now ready to process normal requests.
        """
        self._initialized = True
        logger.info("Server initialized, ready to process requests")

    def _handle_shutdown(self, params: dict[str, Any]) -> None:
        """
        Handle 'shutdown' request.

        Server should prepare to exit but not exit yet.
        Client will send 'exit' notification after this.
        """
        logger.info("Shutdown requested")
        self._shutdown_requested = True
        return None  # Return null per LSP spec

    def _handle_exit(self, params: dict[str, Any]) -> None:
        """
        Handle 'exit' notification.

        Server should exit immediately.
        Exit code is 0 if shutdown was requested, 1 otherwise.
        """
        exit_code = 0 if self._shutdown_requested else 1
        logger.info(f"Exiting with code {exit_code}")
        sys.exit(exit_code)

    # =========================================================================
    # Document Synchronization Handlers
    # =========================================================================

    def _handle_did_open(self, params: dict[str, Any]) -> None:
        """
        Handle 'textDocument/didOpen' notification.

        Track opened documents for state synchronization.
        """
        text_document = params.get("textDocument", {})
        uri = text_document.get("uri", "")
        language_id = text_document.get("languageId", "")
        version = text_document.get("version", 0)
        text = text_document.get("text", "")

        doc = OpenDocument(
            uri=uri,
            language_id=language_id,
            version=version,
            text=text,
        )
        self._open_documents[uri] = doc

        logger.debug(f"Document opened: {uri}")

        # Check if this is a C4-relevant file
        if self._is_c4_relevant(uri):
            self._on_c4_file_opened(uri, doc)

    def _handle_did_close(self, params: dict[str, Any]) -> None:
        """
        Handle 'textDocument/didClose' notification.

        Stop tracking closed documents.
        """
        text_document = params.get("textDocument", {})
        uri = text_document.get("uri", "")

        if uri in self._open_documents:
            del self._open_documents[uri]
            logger.debug(f"Document closed: {uri}")

    def _is_c4_relevant(self, uri: str) -> bool:
        """Check if a URI is relevant to C4 state tracking."""
        # Track files in .c4 directory
        if "/.c4/" in uri or "\\.c4\\" in uri:
            return True
        # Track state.json and tasks.db
        if uri.endswith("state.json") or uri.endswith("tasks.db"):
            return True
        return False

    def _on_c4_file_opened(self, uri: str, doc: OpenDocument) -> None:
        """Handle opening of a C4-relevant file."""
        logger.info(f"C4 file opened: {uri}")
        # Could trigger state refresh or notifications here

    # =========================================================================
    # C4 Custom Methods
    # =========================================================================

    def _handle_c4_get_status(self, params: dict[str, Any]) -> dict[str, Any]:
        """
        Handle 'c4/getStatus' request.

        Returns current C4 project status.
        """
        project_id = params.get("projectId", "")

        try:
            state = self._state_store.load(project_id)
            return self._serialize_state(state)
        except StateNotFoundError:
            return {
                "status": "not_initialized",
                "projectId": project_id,
                "message": "C4 project not found",
            }

    def _handle_c4_get_tasks(self, params: dict[str, Any]) -> dict[str, Any]:
        """
        Handle 'c4/getTasks' request.

        Returns task list with optional filtering.
        """
        project_id = params.get("projectId", "")
        status_filter = params.get("status")  # Optional: pending, in_progress, done

        tasks = self._task_store.load_all(project_id)

        # Apply filter
        if status_filter:
            tasks = {
                tid: task
                for tid, task in tasks.items()
                if task.status.value == status_filter
            }

        return {
            "projectId": project_id,
            "tasks": [self._serialize_task(t) for t in tasks.values()],
            "count": len(tasks),
        }

    def _serialize_state(self, state: C4State) -> dict[str, Any]:
        """Serialize C4State for JSON-RPC response."""
        return {
            "projectId": state.project_id,
            "status": state.status.value,
            "executionMode": state.execution_mode.value if state.execution_mode else None,
            "queue": {
                "pending": list(state.queue.pending),
                "inProgress": list(state.queue.in_progress.keys()),
                "done": list(state.queue.done),
            },
            "metrics": {
                "eventsEmitted": state.metrics.events_emitted,
            },
            "updatedAt": state.updated_at.isoformat() if state.updated_at else None,
        }

    def _serialize_task(self, task: Task) -> dict[str, Any]:
        """Serialize Task for JSON-RPC response."""
        return {
            "id": task.id,
            "title": task.title,
            "status": task.status.value,
            "scope": task.scope,
            "priority": task.priority,
            "dod": task.dod,
            "dependencies": task.dependencies,
            "assignedTo": task.assigned_to,
            "branch": task.branch,
            "commitSha": task.commit_sha,
            "domain": task.domain,
            "taskType": task.task_type,
        }

    # =========================================================================
    # Notification Sending
    # =========================================================================

    def send_notification(self, method: str, params: Any) -> None:
        """
        Send a notification to the client (stdio mode).

        Args:
            method: The notification method name
            params: The notification parameters
        """
        notification = self._make_notification(method, params)
        self._write_stdio_message(notification)

    async def send_notification_async(self, method: str, params: Any) -> None:
        """
        Send a notification to the client (TCP mode).

        Args:
            method: The notification method name
            params: The notification parameters
        """
        notification = self._make_notification(method, params)
        await self._write_tcp_message(notification)

    def notify_state_changed(self, state: C4State) -> None:
        """Send c4/stateChanged notification (stdio mode)."""
        self.send_notification("c4/stateChanged", self._serialize_state(state))

    async def notify_state_changed_async(self, state: C4State) -> None:
        """Send c4/stateChanged notification (TCP mode)."""
        await self.send_notification_async("c4/stateChanged", self._serialize_state(state))

    def notify_task_updated(self, task: Task) -> None:
        """Send c4/taskUpdated notification (stdio mode)."""
        self.send_notification("c4/taskUpdated", self._serialize_task(task))

    async def notify_task_updated_async(self, task: Task) -> None:
        """Send c4/taskUpdated notification (TCP mode)."""
        await self.send_notification_async("c4/taskUpdated", self._serialize_task(task))
