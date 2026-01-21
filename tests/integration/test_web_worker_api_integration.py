"""Integration tests for Web Worker API components.

Tests the complete flow of:
- C4APIClient file operations (read, write, list, search)
- C4APIClient shell execution
- AgenticWorker tool execution
- Error handling and recovery
- Authentication
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from c4.web_worker.agent import (
    AgenticWorker,
    AgentResult,
    AgentStopReason,
)
from c4.web_worker.client import (
    C4APIClient,
    C4APIError,
    FileEntry,
    SearchResult,
    ShellResult,
)
from c4.web_worker.tools import TOOLS, ToolName, validate_tool_input

# =============================================================================
# Test Fixtures
# =============================================================================


@pytest.fixture
def mock_httpx_client():
    """Create a mock httpx.AsyncClient."""
    return AsyncMock(spec=httpx.AsyncClient)


@pytest.fixture
def api_client():
    """Create a C4APIClient instance for testing."""
    return C4APIClient(
        base_url="http://test-api:8000",
        auth_token="test-auth-token",
        timeout=30.0,
    )


@pytest.fixture
def workspace_id():
    """Test workspace ID."""
    return "test-workspace-123"


@pytest.fixture
def mock_llm_client():
    """Create a mock LLM client for AgenticWorker."""
    return MagicMock()


@pytest.fixture
def mock_api_client():
    """Create a mock C4APIClient for AgenticWorker."""
    return AsyncMock(spec=C4APIClient)


# =============================================================================
# Mock Helpers for LLM Responses
# =============================================================================


@dataclass
class MockTextBlock:
    """Mock text content block for LLM response."""

    type: str = "text"
    text: str = ""


@dataclass
class MockToolUseBlock:
    """Mock tool use content block for LLM response."""

    type: str = "tool_use"
    id: str = ""
    name: str = ""
    input: dict = field(default_factory=dict)


@dataclass
class MockLLMResponse:
    """Mock LLM response."""

    content: list = field(default_factory=list)
    stop_reason: str = "end_turn"


def create_text_response(text: str) -> MockLLMResponse:
    """Create a simple text response."""
    return MockLLMResponse(
        content=[MockTextBlock(text=text)],
        stop_reason="end_turn",
    )


def create_tool_use_response(
    tool_name: str,
    tool_input: dict,
    tool_id: str = "tool-1",
) -> MockLLMResponse:
    """Create a response with tool use."""
    return MockLLMResponse(
        content=[MockToolUseBlock(id=tool_id, name=tool_name, input=tool_input)],
        stop_reason="tool_use",
    )


def create_multi_tool_response(
    tools: list[tuple[str, dict, str]],
) -> MockLLMResponse:
    """Create a response with multiple tool uses.

    Args:
        tools: List of (tool_name, tool_input, tool_id) tuples
    """
    content = [
        MockToolUseBlock(id=tool_id, name=name, input=inp)
        for name, inp, tool_id in tools
    ]
    return MockLLMResponse(content=content, stop_reason="tool_use")


# =============================================================================
# TestWebWorkerAPIClient
# =============================================================================


class TestWebWorkerAPIClient:
    """Test C4APIClient HTTP operations."""

    @pytest.mark.asyncio
    async def test_client_file_read(self, api_client, workspace_id):
        """Test reading a file through the API client."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "success": True,
            "content": "Hello, World!",
            "path": "test.txt",
        }

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.post.return_value = mock_response

            content = await api_client.read_file(workspace_id, "test.txt")

            assert content == "Hello, World!"
            mock_client.post.assert_called_once()
            call_args = mock_client.post.call_args
            assert f"/api/workspaces/{workspace_id}/files/read" in call_args[0][0]

    @pytest.mark.asyncio
    async def test_client_file_write(self, api_client, workspace_id):
        """Test writing a file through the API client."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "success": True,
            "path": "output.txt",
            "size": 13,
        }

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.post.return_value = mock_response

            result = await api_client.write_file(
                workspace_id, "output.txt", "Hello, World!"
            )

            assert result["success"] is True
            mock_client.post.assert_called_once()
            call_args = mock_client.post.call_args
            assert f"/api/workspaces/{workspace_id}/files/write" in call_args[0][0]
            assert call_args[1]["json"]["content"] == "Hello, World!"

    @pytest.mark.asyncio
    async def test_client_file_list(self, api_client, workspace_id):
        """Test listing files through the API client."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "success": True,
            "entries": [
                {"name": "file1.py", "path": "file1.py", "is_directory": False, "size": 100},
                {"name": "src", "path": "src", "is_directory": True, "size": None},
            ],
        }

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.post.return_value = mock_response

            entries = await api_client.list_directory(workspace_id, ".", recursive=False)

            assert len(entries) == 2
            assert entries[0].name == "file1.py"
            assert entries[0].is_directory is False
            assert entries[1].name == "src"
            assert entries[1].is_directory is True

    @pytest.mark.asyncio
    async def test_client_file_search(self, api_client, workspace_id):
        """Test file search through the API client."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "success": True,
            "results": [
                {"path": "main.py", "line_number": 10, "line_content": "def main():"},
                {"path": "test.py", "line_number": 5, "line_content": "def test_main():"},
            ],
        }

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.post.return_value = mock_response

            results = await api_client.search_files(
                workspace_id, pattern="def main", search_type="grep", path="."
            )

            assert len(results) == 2
            assert results[0].path == "main.py"
            assert results[0].line_number == 10
            assert results[1].path == "test.py"

    @pytest.mark.asyncio
    async def test_client_shell_execution(self, api_client, workspace_id):
        """Test shell command execution through the API client."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "success": True,
            "stdout": "Hello from shell\n",
            "stderr": "",
            "exit_code": 0,
        }

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.post.return_value = mock_response

            result = await api_client.run_shell(workspace_id, "echo 'Hello from shell'")

            assert isinstance(result, ShellResult)
            assert result.stdout == "Hello from shell\n"
            assert result.exit_code == 0
            mock_client.post.assert_called_once()

    @pytest.mark.asyncio
    async def test_client_shell_execution_with_timeout(self, api_client, workspace_id):
        """Test shell command with custom timeout."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "success": True,
            "stdout": "",
            "stderr": "",
            "exit_code": 0,
        }

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.post.return_value = mock_response

            result = await api_client.run_shell(
                workspace_id, "sleep 1", timeout=120
            )

            assert result.exit_code == 0
            call_args = mock_client.post.call_args
            assert call_args[1]["json"]["timeout"] == 120

    @pytest.mark.asyncio
    async def test_client_shell_timeout_clamping(self, api_client, workspace_id):
        """Test that shell timeout is clamped to MAX_SHELL_TIMEOUT."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "success": True,
            "stdout": "",
            "stderr": "",
            "exit_code": 0,
        }

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.post.return_value = mock_response

            # Request timeout > MAX_SHELL_TIMEOUT (300)
            await api_client.run_shell(workspace_id, "echo test", timeout=600)

            call_args = mock_client.post.call_args
            # Should be clamped to 300
            assert call_args[1]["json"]["timeout"] == 300

    @pytest.mark.asyncio
    async def test_client_error_handling(self, api_client, workspace_id):
        """Test API error handling."""
        mock_response = MagicMock()
        mock_response.status_code = 404
        mock_response.json.return_value = {"detail": "File not found: missing.txt"}
        mock_response.text = "File not found"

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.post.return_value = mock_response

            with pytest.raises(C4APIError) as exc_info:
                await api_client.read_file(workspace_id, "missing.txt")

            assert exc_info.value.status_code == 404
            assert "not found" in str(exc_info.value).lower()

    @pytest.mark.asyncio
    async def test_client_error_handling_non_json(self, api_client, workspace_id):
        """Test error handling when response is not JSON."""
        mock_response = MagicMock()
        mock_response.status_code = 500
        mock_response.json.side_effect = json.JSONDecodeError("", "", 0)
        mock_response.text = "Internal Server Error"

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.post.return_value = mock_response

            with pytest.raises(C4APIError) as exc_info:
                await api_client.read_file(workspace_id, "test.txt")

            assert exc_info.value.status_code == 500
            assert "Internal Server Error" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_client_authentication(self, workspace_id):
        """Test that authentication token is included in requests."""
        client = C4APIClient(
            base_url="http://test:8000",
            auth_token="my-secret-token",
        )

        # Check that the client would be created with auth headers
        await client._ensure_client()

        assert client._client is not None
        assert "Authorization" in client._client.headers
        assert client._client.headers["Authorization"] == "Bearer my-secret-token"

        await client.close()

    @pytest.mark.asyncio
    async def test_client_no_authentication(self, workspace_id):
        """Test client works without authentication token."""
        client = C4APIClient(base_url="http://test:8000")

        await client._ensure_client()

        assert client._client is not None
        assert "Authorization" not in client._client.headers

        await client.close()

    @pytest.mark.asyncio
    async def test_client_health_check_success(self, api_client):
        """Test successful health check."""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.get.return_value = mock_response

            is_healthy = await api_client.health_check()

            assert is_healthy is True
            mock_client.get.assert_called_once_with("/health")

    @pytest.mark.asyncio
    async def test_client_health_check_failure(self, api_client):
        """Test health check failure."""
        mock_response = MagicMock()
        mock_response.status_code = 503

        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.get.return_value = mock_response

            is_healthy = await api_client.health_check()

            assert is_healthy is False

    @pytest.mark.asyncio
    async def test_client_health_check_exception(self, api_client):
        """Test health check on connection error."""
        with patch.object(api_client, "_client", new_callable=AsyncMock) as mock_client:
            api_client._client = mock_client
            mock_client.get.side_effect = httpx.ConnectError("Connection refused")

            is_healthy = await api_client.health_check()

            assert is_healthy is False

    @pytest.mark.asyncio
    async def test_client_search_files_invalid_type(self, api_client, workspace_id):
        """Test that invalid search type raises error."""
        with pytest.raises(C4APIError) as exc_info:
            await api_client.search_files(
                workspace_id, pattern="*.py", search_type="invalid", path="."
            )

        assert "Invalid search_type" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_client_context_manager(self, workspace_id):
        """Test client works as async context manager."""
        async with C4APIClient(base_url="http://test:8000") as client:
            assert client._client is not None

        # Client should be closed after context exit
        assert client._client is None


# =============================================================================
# TestWebWorkerAgentExecution
# =============================================================================


class TestWebWorkerAgentExecution:
    """Test AgenticWorker tool execution."""

    @pytest.mark.asyncio
    async def test_agent_tool_execution_read_file(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent executing read_file tool."""
        # Setup mock LLM to request read_file then complete
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(
                ToolName.READ_FILE, {"path": "main.py"}, "tool-read-1"
            ),
            create_text_response("I read the file contents."),
        ]

        # Setup mock API client
        mock_api_client.read_file.return_value = "print('hello world')"

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("Read main.py")

        assert result.success is True
        assert result.stop_reason == AgentStopReason.COMPLETED
        assert result.total_tool_calls == 1
        mock_api_client.read_file.assert_called_once_with(workspace_id, "main.py")

    @pytest.mark.asyncio
    async def test_agent_tool_execution_write_file(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent executing write_file tool."""
        # Setup mock LLM to request write_file then complete
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(
                ToolName.WRITE_FILE,
                {"path": "output.txt", "content": "Hello, World!"},
                "tool-write-1",
            ),
            create_text_response("I wrote the file successfully."),
        ]

        # Setup mock API client
        mock_api_client.write_file.return_value = {"success": True}

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("Create output.txt")

        assert result.success is True
        assert result.total_tool_calls == 1
        mock_api_client.write_file.assert_called_once_with(
            workspace_id, "output.txt", "Hello, World!"
        )

    @pytest.mark.asyncio
    async def test_agent_tool_execution_run_shell(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent executing run_shell tool."""
        # Setup mock LLM
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(
                ToolName.RUN_SHELL, {"command": "python --version"}, "tool-shell-1"
            ),
            create_text_response("Python 3.12 is installed."),
        ]

        # Setup mock API client
        mock_api_client.run_shell.return_value = ShellResult(
            stdout="Python 3.12.0\n", stderr="", exit_code=0
        )

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("Check Python version")

        assert result.success is True
        assert result.total_tool_calls == 1
        mock_api_client.run_shell.assert_called_once()

    @pytest.mark.asyncio
    async def test_agent_multiple_tool_calls(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent executing multiple tool calls in sequence."""
        # Setup mock LLM to use multiple tools
        mock_llm_client.messages_create.side_effect = [
            # First: list directory
            create_tool_use_response(
                ToolName.LIST_DIRECTORY, {"path": "."}, "tool-list-1"
            ),
            # Second: read a file
            create_tool_use_response(
                ToolName.READ_FILE, {"path": "main.py"}, "tool-read-1"
            ),
            # Third: complete
            create_text_response("Found main.py and read its contents."),
        ]

        # Setup mock API client
        mock_api_client.list_directory.return_value = [
            FileEntry(name="main.py", path="main.py", is_directory=False, size=100)
        ]
        mock_api_client.read_file.return_value = "print('hello')"

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("List files and read main.py")

        assert result.success is True
        assert result.total_tool_calls == 2
        assert result.turns == 3  # list, read, final response

    @pytest.mark.asyncio
    async def test_agent_parallel_tool_calls(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent handling multiple tool calls in a single turn."""
        # Setup mock LLM to use multiple tools at once
        mock_llm_client.messages_create.side_effect = [
            create_multi_tool_response([
                (ToolName.READ_FILE, {"path": "a.py"}, "tool-1"),
                (ToolName.READ_FILE, {"path": "b.py"}, "tool-2"),
            ]),
            create_text_response("Read both files."),
        ]

        # Setup mock API client
        mock_api_client.read_file.side_effect = [
            "content of a.py",
            "content of b.py",
        ]

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("Read a.py and b.py")

        assert result.success is True
        assert result.total_tool_calls == 2
        assert result.turns == 2  # one turn with 2 tools, one final

    @pytest.mark.asyncio
    async def test_agent_tool_error_recovery(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent recovers from tool errors."""
        # Setup mock LLM to handle error and retry
        mock_llm_client.messages_create.side_effect = [
            # First: try to read missing file
            create_tool_use_response(
                ToolName.READ_FILE, {"path": "missing.py"}, "tool-read-1"
            ),
            # Second: try different file after error
            create_tool_use_response(
                ToolName.READ_FILE, {"path": "exists.py"}, "tool-read-2"
            ),
            # Third: complete
            create_text_response("Successfully read exists.py"),
        ]

        # Setup mock API client - first call fails, second succeeds
        mock_api_client.read_file.side_effect = [
            C4APIError("File not found", 404),
            "content of exists.py",
        ]

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
            max_tool_errors=3,  # Allow some errors
        )

        result = await agent.execute_task("Read a file")

        assert result.success is True
        assert result.total_tool_calls == 2

    @pytest.mark.asyncio
    async def test_agent_tool_error_max_consecutive(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent stops after max consecutive tool errors."""
        # Setup mock LLM to keep trying
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(ToolName.READ_FILE, {"path": "bad1.py"}, "t-1"),
            create_tool_use_response(ToolName.READ_FILE, {"path": "bad2.py"}, "t-2"),
            create_tool_use_response(ToolName.READ_FILE, {"path": "bad3.py"}, "t-3"),
        ]

        # Setup mock API client - all calls fail
        mock_api_client.read_file.side_effect = C4APIError("File not found", 404)

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
            max_tool_errors=3,
        )

        result = await agent.execute_task("Read files")

        assert result.success is False
        assert result.stop_reason == AgentStopReason.TOOL_ERROR
        assert "consecutive tool errors" in result.error.lower()

    @pytest.mark.asyncio
    async def test_agent_max_turns_limit(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent stops after max turns."""
        # Setup mock LLM to keep using tools (never complete)
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(ToolName.LIST_DIRECTORY, {"path": "."}, f"t-{i}")
            for i in range(10)
        ]

        # Setup mock API client
        mock_api_client.list_directory.return_value = []

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=5,
        )

        result = await agent.execute_task("Keep listing forever")

        assert result.success is False
        assert result.stop_reason == AgentStopReason.MAX_TURNS
        assert result.turns == 5

    @pytest.mark.asyncio
    async def test_agent_llm_api_error(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent handles LLM API errors."""
        mock_llm_client.messages_create.side_effect = Exception("LLM API is down")

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
        )

        result = await agent.execute_task("Do something")

        assert result.success is False
        assert result.stop_reason == AgentStopReason.ERROR
        assert "LLM API" in result.error

    @pytest.mark.asyncio
    async def test_agent_no_response(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent handles empty LLM response."""
        mock_response = MockLLMResponse(content=[], stop_reason="end_turn")
        mock_llm_client.messages_create.return_value = mock_response

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
        )

        result = await agent.execute_task("Do something")

        assert result.success is False
        assert result.stop_reason == AgentStopReason.NO_RESPONSE

    @pytest.mark.asyncio
    async def test_agent_invalid_tool_input(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent handles invalid tool input validation."""
        # Setup mock LLM to request tool with missing required field
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(
                ToolName.WRITE_FILE,
                {"path": "test.txt"},  # Missing 'content' field
                "tool-write-1",
            ),
            # After error, complete
            create_text_response("Failed to write file due to missing content."),
        ]

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("Write a file")

        # Should succeed overall but with validation error in tool call
        assert result.success is True
        assert result.total_tool_calls == 1
        # Check that the tool call recorded the validation error
        assert len(result.turn_history) >= 1
        first_turn = result.turn_history[0]
        assert len(first_turn.tool_calls) == 1
        assert first_turn.tool_calls[0].success is False
        assert "validation error" in first_turn.tool_calls[0].result.lower()

    @pytest.mark.asyncio
    async def test_agent_unknown_tool(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent handles unknown tool name."""
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(
                "unknown_tool", {"param": "value"}, "tool-unknown-1"
            ),
            create_text_response("Unknown tool failed."),
        ]

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("Use unknown tool")

        assert result.success is True
        assert result.total_tool_calls == 1
        # Check tool call failed
        first_turn = result.turn_history[0]
        assert first_turn.tool_calls[0].success is False
        assert "unknown" in first_turn.tool_calls[0].result.lower()

    @pytest.mark.asyncio
    async def test_agent_search_files_tool(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent executing search_files tool."""
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(
                ToolName.SEARCH_FILES,
                {"pattern": "def test", "search_type": "grep", "path": "."},
                "tool-search-1",
            ),
            create_text_response("Found test functions."),
        ]

        mock_api_client.search_files.return_value = [
            SearchResult(path="test_main.py", line_number=5, line_content="def test_main():"),
        ]

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("Search for test functions")

        assert result.success is True
        mock_api_client.search_files.assert_called_once()

    @pytest.mark.asyncio
    async def test_agent_list_directory_tool(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent executing list_directory tool."""
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(
                ToolName.LIST_DIRECTORY,
                {"path": "src", "recursive": True},
                "tool-list-1",
            ),
            create_text_response("Listed the src directory."),
        ]

        mock_api_client.list_directory.return_value = [
            FileEntry(name="main.py", path="src/main.py", is_directory=False, size=500),
            FileEntry(name="utils", path="src/utils", is_directory=True),
        ]

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("List src directory")

        assert result.success is True
        mock_api_client.list_directory.assert_called_once_with(
            workspace_id, "src", True
        )

    @pytest.mark.asyncio
    async def test_agent_shell_with_stderr(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent handles shell command with stderr output."""
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(
                ToolName.RUN_SHELL,
                {"command": "npm install"},
                "tool-shell-1",
            ),
            create_text_response("Installed dependencies with warnings."),
        ]

        mock_api_client.run_shell.return_value = ShellResult(
            stdout="added 100 packages\n",
            stderr="npm warn deprecated lodash@1.0.0\n",
            exit_code=0,
        )

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("Install npm packages")

        assert result.success is True
        # Verify the tool call result includes both stdout and stderr
        first_turn = result.turn_history[0]
        tool_result = first_turn.tool_calls[0].result
        assert "added 100 packages" in tool_result
        assert "npm warn" in tool_result

    @pytest.mark.asyncio
    async def test_agent_shell_nonzero_exit(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent handles shell command with non-zero exit code."""
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(
                ToolName.RUN_SHELL,
                {"command": "exit 1"},
                "tool-shell-1",
            ),
            create_text_response("Command failed."),
        ]

        mock_api_client.run_shell.return_value = ShellResult(
            stdout="",
            stderr="Error occurred",
            exit_code=1,
        )

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("Run failing command")

        assert result.success is True  # Agent completed, tool failed
        first_turn = result.turn_history[0]
        assert first_turn.tool_calls[0].success is False

    @pytest.mark.asyncio
    async def test_agent_turn_history_tracking(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test that agent correctly tracks turn history."""
        mock_llm_client.messages_create.side_effect = [
            create_tool_use_response(
                ToolName.READ_FILE, {"path": "file.txt"}, "t-1"
            ),
            create_tool_use_response(
                ToolName.WRITE_FILE, {"path": "out.txt", "content": "data"}, "t-2"
            ),
            create_text_response("All done!"),
        ]

        mock_api_client.read_file.return_value = "file content"
        mock_api_client.write_file.return_value = {"success": True}

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            max_turns=10,
        )

        result = await agent.execute_task("Read and write files")

        assert result.success is True
        assert result.turns == 3
        assert len(result.turn_history) == 3

        # Check turn 1
        assert result.turn_history[0].turn_number == 1
        assert len(result.turn_history[0].tool_calls) == 1
        assert result.turn_history[0].tool_calls[0].tool_name == ToolName.READ_FILE

        # Check turn 2
        assert result.turn_history[1].turn_number == 2
        assert len(result.turn_history[1].tool_calls) == 1
        assert result.turn_history[1].tool_calls[0].tool_name == ToolName.WRITE_FILE

        # Check turn 3 (final)
        assert result.turn_history[2].turn_number == 3
        assert result.turn_history[2].assistant_message == "All done!"

    @pytest.mark.asyncio
    async def test_agent_custom_system_prompt(
        self, mock_llm_client, mock_api_client, workspace_id
    ):
        """Test agent uses custom system prompt."""
        custom_prompt = "You are a helpful assistant for testing."

        mock_llm_client.messages_create.return_value = create_text_response("OK")

        agent = AgenticWorker(
            workspace_id=workspace_id,
            api_client=mock_api_client,
            llm_client=mock_llm_client,
            system_prompt=custom_prompt,
        )

        await agent.execute_task("Test prompt")

        # Verify the system prompt was passed to LLM
        call_kwargs = mock_llm_client.messages_create.call_args.kwargs
        assert call_kwargs["system"] == custom_prompt

    def test_agent_result_to_dict(self):
        """Test AgentResult.to_dict() serialization."""
        result = AgentResult(
            success=True,
            stop_reason=AgentStopReason.COMPLETED,
            turns=3,
            total_tool_calls=5,
            final_message="Done!",
            error=None,
            turn_history=[],
        )

        result_dict = result.to_dict()

        assert result_dict["success"] is True
        assert result_dict["stop_reason"] == "completed"
        assert result_dict["turns"] == 3
        assert result_dict["total_tool_calls"] == 5
        assert result_dict["final_message"] == "Done!"
        assert result_dict["error"] is None


# =============================================================================
# TestToolValidation
# =============================================================================


class TestToolValidation:
    """Test tool input validation."""

    def test_validate_read_file_valid(self):
        """Test valid read_file input."""
        is_valid, error = validate_tool_input(
            ToolName.READ_FILE, {"path": "test.txt"}
        )
        assert is_valid is True
        assert error == ""

    def test_validate_read_file_missing_path(self):
        """Test read_file with missing path."""
        is_valid, error = validate_tool_input(ToolName.READ_FILE, {})
        assert is_valid is False
        assert "path" in error.lower()

    def test_validate_write_file_valid(self):
        """Test valid write_file input."""
        is_valid, error = validate_tool_input(
            ToolName.WRITE_FILE, {"path": "test.txt", "content": "hello"}
        )
        assert is_valid is True

    def test_validate_write_file_missing_content(self):
        """Test write_file with missing content."""
        is_valid, error = validate_tool_input(
            ToolName.WRITE_FILE, {"path": "test.txt"}
        )
        assert is_valid is False
        assert "content" in error.lower()

    def test_validate_run_shell_valid(self):
        """Test valid run_shell input."""
        is_valid, error = validate_tool_input(
            ToolName.RUN_SHELL, {"command": "echo hello"}
        )
        assert is_valid is True

    def test_validate_run_shell_with_timeout(self):
        """Test run_shell with optional timeout."""
        is_valid, error = validate_tool_input(
            ToolName.RUN_SHELL, {"command": "sleep 5", "timeout": 120}
        )
        assert is_valid is True

    def test_validate_search_files_valid(self):
        """Test valid search_files input."""
        is_valid, error = validate_tool_input(
            ToolName.SEARCH_FILES,
            {"pattern": "*.py", "search_type": "glob"},
        )
        assert is_valid is True

    def test_validate_search_files_missing_type(self):
        """Test search_files with missing search_type."""
        is_valid, error = validate_tool_input(
            ToolName.SEARCH_FILES, {"pattern": "*.py"}
        )
        assert is_valid is False
        assert "search_type" in error.lower()

    def test_validate_list_directory_valid(self):
        """Test valid list_directory input."""
        is_valid, error = validate_tool_input(ToolName.LIST_DIRECTORY, {})
        assert is_valid is True  # No required fields

    def test_validate_list_directory_with_options(self):
        """Test list_directory with optional fields."""
        is_valid, error = validate_tool_input(
            ToolName.LIST_DIRECTORY,
            {"path": "src", "recursive": True},
        )
        assert is_valid is True

    def test_validate_unknown_tool(self):
        """Test validation of unknown tool."""
        is_valid, error = validate_tool_input("nonexistent_tool", {})
        assert is_valid is False
        assert "unknown" in error.lower()

    def test_tools_schema_structure(self):
        """Test that TOOLS have correct schema structure."""
        for tool in TOOLS:
            assert "name" in tool
            assert "description" in tool
            assert "input_schema" in tool
            assert "type" in tool["input_schema"]
            assert tool["input_schema"]["type"] == "object"
