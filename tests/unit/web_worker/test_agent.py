"""Tests for c4.web_worker.agent module."""

import json
from dataclasses import dataclass
from typing import Any
from unittest.mock import AsyncMock, MagicMock

import pytest

from c4.web_worker.agent import (
    AgenticWorker,
    AgentResult,
    AgentStopReason,
    AgentTurn,
    ToolCall,
)
from c4.web_worker.client import C4APIClient, C4APIError, FileEntry, SearchResult, ShellResult

# =============================================================================
# Test Fixtures and Mocks
# =============================================================================


@dataclass
class MockTextBlock:
    """Mock text content block."""

    type: str = "text"
    text: str = ""


@dataclass
class MockToolUseBlock:
    """Mock tool use content block."""

    type: str = "tool_use"
    id: str = ""
    name: str = ""
    input: dict = None

    def __post_init__(self):
        if self.input is None:
            self.input = {}


@dataclass
class MockResponse:
    """Mock LLM response."""

    content: list = None
    stop_reason: str = "end_turn"

    def __post_init__(self):
        if self.content is None:
            self.content = []


class MockLLMClient:
    """Mock LLM client for testing."""

    def __init__(self, responses: list[MockResponse] | None = None):
        self.responses = responses or []
        self.call_count = 0
        self.call_history: list[dict] = []

    def messages_create(
        self,
        model: str,
        max_tokens: int,
        system: str | None,
        tools: list[dict[str, Any]],
        messages: list[dict[str, Any]],
    ) -> MockResponse:
        """Record call and return next response."""
        self.call_history.append(
            {
                "model": model,
                "max_tokens": max_tokens,
                "system": system,
                "tools": tools,
                "messages": messages,
            }
        )
        if self.call_count >= len(self.responses):
            raise ValueError(f"No more mock responses (call {self.call_count})")
        response = self.responses[self.call_count]
        self.call_count += 1
        return response


@pytest.fixture
def mock_api_client():
    """Create a mock API client."""
    client = MagicMock(spec=C4APIClient)
    client.read_file = AsyncMock(return_value="file content")
    client.write_file = AsyncMock(return_value={"success": True})
    client.run_shell = AsyncMock(return_value=ShellResult(stdout="output", stderr="", exit_code=0))
    client.search_files = AsyncMock(return_value=[SearchResult(path="/test.py")])
    client.list_directory = AsyncMock(return_value=[FileEntry(name="test.py", path="/test.py", is_directory=False)])
    return client


# =============================================================================
# Test Dataclasses
# =============================================================================


class TestToolCall:
    """Test ToolCall dataclass."""

    def test_creation(self):
        """Should create ToolCall with all fields."""
        call = ToolCall(
            tool_name="read_file",
            tool_input={"path": "/test.txt"},
            tool_use_id="tool-123",
            result="content",
            success=True,
            duration_ms=50,
        )
        assert call.tool_name == "read_file"
        assert call.tool_input == {"path": "/test.txt"}
        assert call.tool_use_id == "tool-123"
        assert call.result == "content"
        assert call.success is True
        assert call.duration_ms == 50


class TestAgentTurn:
    """Test AgentTurn dataclass."""

    def test_creation(self):
        """Should create AgentTurn with defaults."""
        turn = AgentTurn(turn_number=1)
        assert turn.turn_number == 1
        assert turn.assistant_message is None
        assert turn.tool_calls == []
        assert turn.stop_reason is None

    def test_with_tool_calls(self):
        """Should store tool calls."""
        call = ToolCall(
            tool_name="read_file",
            tool_input={},
            tool_use_id="123",
            result="ok",
            success=True,
        )
        turn = AgentTurn(turn_number=1, tool_calls=[call])
        assert len(turn.tool_calls) == 1


class TestAgentResult:
    """Test AgentResult dataclass."""

    def test_success_result(self):
        """Should create successful result."""
        result = AgentResult(
            success=True,
            stop_reason=AgentStopReason.COMPLETED,
            turns=3,
            total_tool_calls=5,
            final_message="Task completed",
        )
        assert result.success is True
        assert result.stop_reason == AgentStopReason.COMPLETED
        assert result.turns == 3
        assert result.total_tool_calls == 5
        assert result.error is None

    def test_failure_result(self):
        """Should create failed result."""
        result = AgentResult(
            success=False,
            stop_reason=AgentStopReason.ERROR,
            turns=1,
            total_tool_calls=0,
            error="API error",
        )
        assert result.success is False
        assert result.error == "API error"

    def test_to_dict(self):
        """Should convert to dictionary."""
        result = AgentResult(
            success=True,
            stop_reason=AgentStopReason.COMPLETED,
            turns=2,
            total_tool_calls=3,
            final_message="done",
        )
        d = result.to_dict()
        assert d["success"] is True
        assert d["stop_reason"] == "completed"
        assert d["turns"] == 2
        assert d["total_tool_calls"] == 3


class TestAgentStopReason:
    """Test AgentStopReason enum."""

    def test_values(self):
        """Should have expected values."""
        assert AgentStopReason.COMPLETED.value == "completed"
        assert AgentStopReason.MAX_TURNS.value == "max_turns"
        assert AgentStopReason.ERROR.value == "error"
        assert AgentStopReason.TOOL_ERROR.value == "tool_error"
        assert AgentStopReason.NO_RESPONSE.value == "no_response"


# =============================================================================
# Test AgenticWorker Initialization
# =============================================================================


class TestAgenticWorkerInit:
    """Test AgenticWorker initialization."""

    def test_init_with_llm_client(self, mock_api_client):
        """Should initialize with provided LLM client."""
        mock_llm = MockLLMClient()
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )
        assert worker.workspace_id == "ws-123"
        assert worker.api_client == mock_api_client
        assert worker._llm == mock_llm
        assert worker.model == AgenticWorker.DEFAULT_MODEL

    def test_init_with_custom_model(self, mock_api_client):
        """Should accept custom model."""
        mock_llm = MockLLMClient()
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
            model="claude-opus-4-20250514",
        )
        assert worker.model == "claude-opus-4-20250514"

    def test_init_with_custom_limits(self, mock_api_client):
        """Should accept custom limits."""
        mock_llm = MockLLMClient()
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
            max_turns=10,
            max_tool_errors=5,
        )
        assert worker.max_turns == 10
        assert worker.max_tool_errors == 5

    def test_init_with_custom_system_prompt(self, mock_api_client):
        """Should accept custom system prompt."""
        mock_llm = MockLLMClient()
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
            system_prompt="Custom prompt",
        )
        assert worker.system_prompt == "Custom prompt"

    def test_default_system_prompt(self, mock_api_client):
        """Should have meaningful default system prompt."""
        mock_llm = MockLLMClient()
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )
        assert "software engineer" in worker.system_prompt.lower()
        assert "tools" in worker.system_prompt.lower()


# =============================================================================
# Test Task Execution - Success Cases
# =============================================================================


@pytest.mark.asyncio
class TestAgenticWorkerExecuteTask:
    """Test execute_task method."""

    async def test_simple_completion(self, mock_api_client):
        """Should complete task with end_turn."""
        responses = [
            MockResponse(
                content=[MockTextBlock(text="Task completed successfully")],
                stop_reason="end_turn",
            )
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("Complete this task")

        assert result.success is True
        assert result.stop_reason == AgentStopReason.COMPLETED
        assert result.turns == 1
        assert result.total_tool_calls == 0
        assert result.final_message == "Task completed successfully"

    async def test_single_tool_call(self, mock_api_client):
        """Should execute single tool call."""
        responses = [
            MockResponse(
                content=[
                    MockToolUseBlock(
                        id="tool-1",
                        name="read_file",
                        input={"path": "/test.txt"},
                    )
                ],
                stop_reason="tool_use",
            ),
            MockResponse(
                content=[MockTextBlock(text="File read successfully")],
                stop_reason="end_turn",
            ),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("Read the file")

        assert result.success is True
        assert result.turns == 2
        assert result.total_tool_calls == 1
        mock_api_client.read_file.assert_called_once_with("ws-123", "/test.txt")

    async def test_multiple_tool_calls_single_turn(self, mock_api_client):
        """Should execute multiple tools in one turn."""
        responses = [
            MockResponse(
                content=[
                    MockToolUseBlock(id="tool-1", name="read_file", input={"path": "/a.txt"}),
                    MockToolUseBlock(id="tool-2", name="read_file", input={"path": "/b.txt"}),
                ],
                stop_reason="tool_use",
            ),
            MockResponse(
                content=[MockTextBlock(text="Both files read")],
                stop_reason="end_turn",
            ),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("Read both files")

        assert result.success is True
        assert result.total_tool_calls == 2
        assert mock_api_client.read_file.call_count == 2

    async def test_multi_turn_conversation(self, mock_api_client):
        """Should handle multi-turn conversation."""
        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="list_directory", input={})],
                stop_reason="tool_use",
            ),
            MockResponse(
                content=[MockToolUseBlock(id="t2", name="read_file", input={"path": "/test.py"})],
                stop_reason="tool_use",
            ),
            MockResponse(
                content=[MockToolUseBlock(id="t3", name="write_file", input={"path": "/test.py", "content": "updated"})],
                stop_reason="tool_use",
            ),
            MockResponse(
                content=[MockTextBlock(text="File updated")],
                stop_reason="end_turn",
            ),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("Update the test file")

        assert result.success is True
        assert result.turns == 4
        assert result.total_tool_calls == 3


# =============================================================================
# Test Task Execution - Tool Calls
# =============================================================================


@pytest.mark.asyncio
class TestAgenticWorkerToolExecution:
    """Test tool execution."""

    async def test_read_file_tool(self, mock_api_client):
        """Should execute read_file tool."""
        mock_api_client.read_file = AsyncMock(return_value="file content here")

        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="read_file", input={"path": "/src/main.py"})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        result = await worker.execute_task("Read main.py")

        assert result.success is True
        mock_api_client.read_file.assert_called_with("ws-123", "/src/main.py")

        # Check tool result was passed to LLM in the second call
        # Messages: [user prompt, assistant tool_use, user tool_result]
        second_call = mock_llm.call_history[1]
        messages = second_call["messages"]
        # Find the tool_result message
        tool_result_msg = None
        for msg in messages:
            if msg["role"] == "user" and isinstance(msg.get("content"), list):
                for item in msg["content"]:
                    if isinstance(item, dict) and item.get("type") == "tool_result":
                        tool_result_msg = msg
                        break
        assert tool_result_msg is not None, "Tool result message not found"
        assert "file content here" in tool_result_msg["content"][0]["content"]

    async def test_write_file_tool(self, mock_api_client):
        """Should execute write_file tool."""
        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="write_file", input={"path": "/out.txt", "content": "hello"})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        await worker.execute_task("Write file")

        mock_api_client.write_file.assert_called_with("ws-123", "/out.txt", "hello")

    async def test_run_shell_tool(self, mock_api_client):
        """Should execute run_shell tool."""
        mock_api_client.run_shell = AsyncMock(return_value=ShellResult(stdout="file1.py\nfile2.py", stderr="", exit_code=0))

        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="run_shell", input={"command": "ls *.py"})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        await worker.execute_task("List python files")

        mock_api_client.run_shell.assert_called_with("ws-123", "ls *.py", timeout=60)

    async def test_run_shell_with_timeout(self, mock_api_client):
        """Should pass timeout to run_shell."""
        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="run_shell", input={"command": "sleep 10", "timeout": 120})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        await worker.execute_task("Run slow command")

        mock_api_client.run_shell.assert_called_with("ws-123", "sleep 10", timeout=120)

    async def test_search_files_tool(self, mock_api_client):
        """Should execute search_files tool."""
        mock_api_client.search_files = AsyncMock(
            return_value=[
                SearchResult(path="/a.py", line_number=10),
                SearchResult(path="/b.py", line_number=20),
            ]
        )

        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="search_files", input={"pattern": "def test", "search_type": "grep"})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        await worker.execute_task("Search for tests")

        mock_api_client.search_files.assert_called_with("ws-123", "def test", "grep", ".")

    async def test_list_directory_tool(self, mock_api_client):
        """Should execute list_directory tool."""
        mock_api_client.list_directory = AsyncMock(
            return_value=[
                FileEntry(name="src", path="/src", is_directory=True),
                FileEntry(name="main.py", path="/main.py", is_directory=False, size=100),
            ]
        )

        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="list_directory", input={"path": "/", "recursive": True})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        await worker.execute_task("List files")

        mock_api_client.list_directory.assert_called_with("ws-123", "/", True)


# =============================================================================
# Test Task Execution - Error Cases
# =============================================================================


@pytest.mark.asyncio
class TestAgenticWorkerErrors:
    """Test error handling."""

    async def test_max_turns_exceeded(self, mock_api_client):
        """Should stop after max turns."""
        # Create responses that never complete
        responses = [MockResponse(content=[MockToolUseBlock(id=f"t{i}", name="list_directory", input={})], stop_reason="tool_use") for i in range(10)]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
            max_turns=5,
        )

        result = await worker.execute_task("Infinite loop task")

        assert result.success is False
        assert result.stop_reason == AgentStopReason.MAX_TURNS
        assert result.turns == 5
        assert "Max turns exceeded" in result.error

    async def test_llm_api_error(self, mock_api_client):
        """Should handle LLM API errors."""
        mock_llm = MockLLMClient([])  # No responses will cause error
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("Test task")

        assert result.success is False
        assert result.stop_reason == AgentStopReason.ERROR
        assert "LLM API error" in result.error

    async def test_no_response_content(self, mock_api_client):
        """Should handle empty response content."""
        responses = [MockResponse(content=[], stop_reason="end_turn")]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("Test task")

        assert result.success is False
        assert result.stop_reason == AgentStopReason.NO_RESPONSE

    async def test_tool_api_error(self, mock_api_client):
        """Should handle tool API errors."""
        mock_api_client.read_file = AsyncMock(side_effect=C4APIError("File not found", 404))

        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="read_file", input={"path": "/missing.txt"})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("Read missing file")

        # Should still succeed but tool result contains error
        assert result.success is True
        assert result.turn_history[0].tool_calls[0].success is False
        assert "API error" in result.turn_history[0].tool_calls[0].result

    async def test_consecutive_tool_errors(self, mock_api_client):
        """Should stop after too many consecutive errors."""
        mock_api_client.read_file = AsyncMock(side_effect=C4APIError("Error", 500))

        responses = [MockResponse(content=[MockToolUseBlock(id=f"t{i}", name="read_file", input={"path": f"/file{i}.txt"})], stop_reason="tool_use") for i in range(5)]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
            max_tool_errors=3,
        )

        result = await worker.execute_task("Read files with errors")

        assert result.success is False
        assert result.stop_reason == AgentStopReason.TOOL_ERROR
        assert "consecutive tool errors" in result.error

    async def test_tool_input_validation_error(self, mock_api_client):
        """Should handle invalid tool input."""
        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="read_file", input={})],  # Missing path
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("Test")

        # Tool call should fail but agent continues
        assert result.turn_history[0].tool_calls[0].success is False
        assert "validation error" in result.turn_history[0].tool_calls[0].result.lower()

    async def test_unknown_tool(self, mock_api_client):
        """Should handle unknown tool name."""
        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="unknown_tool", input={"foo": "bar"})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("Test")

        assert result.turn_history[0].tool_calls[0].success is False
        assert "Unknown tool" in result.turn_history[0].tool_calls[0].result


# =============================================================================
# Test Turn History
# =============================================================================


@pytest.mark.asyncio
class TestAgenticWorkerTurnHistory:
    """Test turn history tracking."""

    async def test_turn_history_recorded(self, mock_api_client):
        """Should record turn history."""
        responses = [
            MockResponse(
                content=[
                    MockTextBlock(text="Let me read the file"),
                    MockToolUseBlock(id="t1", name="read_file", input={"path": "/a.txt"}),
                ],
                stop_reason="tool_use",
            ),
            MockResponse(
                content=[MockTextBlock(text="File contents look good")],
                stop_reason="end_turn",
            ),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("Check file")

        assert len(result.turn_history) == 2

        # First turn
        turn1 = result.turn_history[0]
        assert turn1.turn_number == 1
        assert turn1.assistant_message == "Let me read the file"
        assert len(turn1.tool_calls) == 1
        assert turn1.tool_calls[0].tool_name == "read_file"

        # Second turn
        turn2 = result.turn_history[1]
        assert turn2.turn_number == 2
        assert turn2.assistant_message == "File contents look good"
        assert len(turn2.tool_calls) == 0

    async def test_tool_call_duration_recorded(self, mock_api_client):
        """Should record tool call duration."""
        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="list_directory", input={})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
        )

        result = await worker.execute_task("List")

        tool_call = result.turn_history[0].tool_calls[0]
        assert tool_call.duration_ms is not None
        assert tool_call.duration_ms >= 0


# =============================================================================
# Test Shell Command Result Formatting
# =============================================================================


@pytest.mark.asyncio
class TestShellResultFormatting:
    """Test shell command result formatting."""

    async def test_shell_success_formatting(self, mock_api_client):
        """Should format successful shell output."""
        mock_api_client.run_shell = AsyncMock(return_value=ShellResult(stdout="success output", stderr="", exit_code=0))

        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="run_shell", input={"command": "echo test"})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        result = await worker.execute_task("Run command")

        tool_result = result.turn_history[0].tool_calls[0].result
        assert "stdout:" in tool_result
        assert "success output" in tool_result
        assert "exit_code: 0" in tool_result

    async def test_shell_error_formatting(self, mock_api_client):
        """Should format shell errors properly."""
        mock_api_client.run_shell = AsyncMock(return_value=ShellResult(stdout="", stderr="command not found", exit_code=127))

        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="run_shell", input={"command": "invalid_cmd"})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        result = await worker.execute_task("Run invalid")

        tool_call = result.turn_history[0].tool_calls[0]
        assert tool_call.success is False
        assert "stderr:" in tool_call.result
        assert "command not found" in tool_call.result
        assert "exit_code: 127" in tool_call.result


# =============================================================================
# Test Search Results Formatting
# =============================================================================


@pytest.mark.asyncio
class TestSearchResultsFormatting:
    """Test search results formatting."""

    async def test_empty_search_results(self, mock_api_client):
        """Should handle empty search results."""
        mock_api_client.search_files = AsyncMock(return_value=[])

        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="search_files", input={"pattern": "nonexistent", "search_type": "glob"})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        result = await worker.execute_task("Search")

        tool_result = result.turn_history[0].tool_calls[0].result
        assert "No matches found" in tool_result

    async def test_search_results_json_formatted(self, mock_api_client):
        """Should format search results as JSON."""
        mock_api_client.search_files = AsyncMock(
            return_value=[
                SearchResult(path="/test.py", line_number=5, line_content="def test():"),
            ]
        )

        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="search_files", input={"pattern": "def test", "search_type": "grep"})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        result = await worker.execute_task("Search")

        tool_result = result.turn_history[0].tool_calls[0].result
        parsed = json.loads(tool_result)
        assert len(parsed) == 1
        assert parsed[0]["path"] == "/test.py"


# =============================================================================
# Test Message History Building
# =============================================================================


@pytest.mark.asyncio
class TestMessageHistoryBuilding:
    """Test that message history is built correctly."""

    async def test_initial_message(self, mock_api_client):
        """Should start with user message."""
        responses = [
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        await worker.execute_task("Test prompt")

        first_call = mock_llm.call_history[0]
        assert first_call["messages"][0]["role"] == "user"
        assert first_call["messages"][0]["content"] == "Test prompt"

    async def test_system_prompt_passed(self, mock_api_client):
        """Should pass system prompt to LLM."""
        responses = [
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(
            workspace_id="ws-123",
            api_client=mock_api_client,
            llm_client=mock_llm,
            system_prompt="Custom system prompt",
        )

        await worker.execute_task("Test")

        assert mock_llm.call_history[0]["system"] == "Custom system prompt"

    async def test_tools_passed(self, mock_api_client):
        """Should pass tools to LLM."""
        responses = [
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        await worker.execute_task("Test")

        tools = mock_llm.call_history[0]["tools"]
        tool_names = {t["name"] for t in tools}
        assert "read_file" in tool_names
        assert "write_file" in tool_names
        assert "run_shell" in tool_names

    async def test_assistant_message_added_to_history(self, mock_api_client):
        """Should add assistant response to message history."""
        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="list_directory", input={})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        await worker.execute_task("List files")

        # Second call should have assistant message in history
        second_call = mock_llm.call_history[1]
        messages = second_call["messages"]
        assert len(messages) >= 3  # user, assistant, tool_result
        assert messages[1]["role"] == "assistant"

    async def test_tool_results_added_as_user_message(self, mock_api_client):
        """Should add tool results as user message."""
        responses = [
            MockResponse(
                content=[MockToolUseBlock(id="t1", name="list_directory", input={})],
                stop_reason="tool_use",
            ),
            MockResponse(content=[MockTextBlock(text="Done")], stop_reason="end_turn"),
        ]
        mock_llm = MockLLMClient(responses)
        worker = AgenticWorker(workspace_id="ws-123", api_client=mock_api_client, llm_client=mock_llm)

        await worker.execute_task("List files")

        second_call = mock_llm.call_history[1]
        messages = second_call["messages"]
        # Find the tool_result user message
        tool_result_msg = None
        for msg in messages:
            if msg["role"] == "user" and isinstance(msg.get("content"), list):
                for item in msg["content"]:
                    if isinstance(item, dict) and item.get("type") == "tool_result":
                        tool_result_msg = msg
                        break
        assert tool_result_msg is not None, "Tool result message not found"
        assert tool_result_msg["content"][0]["type"] == "tool_result"
        assert tool_result_msg["content"][0]["tool_use_id"] == "t1"
