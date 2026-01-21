"""AgenticWorker - Claude Tool Use multi-turn agent for task execution.

Implements a Claude Code-style agent that uses Tool Use to automatically
execute file operations and shell commands to complete tasks.
"""

import json
import time
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Protocol, runtime_checkable

from .client import C4APIClient, C4APIError
from .tools import TOOLS, ToolName, validate_tool_input


class AgentStopReason(str, Enum):
    """Reason why agent stopped execution."""

    COMPLETED = "completed"  # Task finished successfully
    MAX_TURNS = "max_turns"  # Hit turn limit
    ERROR = "error"  # Unrecoverable error
    TOOL_ERROR = "tool_error"  # Tool execution failed
    NO_RESPONSE = "no_response"  # LLM returned no content


@dataclass
class ToolCall:
    """Record of a tool call."""

    tool_name: str
    tool_input: dict[str, Any]
    tool_use_id: str
    result: str
    success: bool
    duration_ms: int | None = None


@dataclass
class AgentTurn:
    """Record of a single agent turn."""

    turn_number: int
    assistant_message: str | None = None
    tool_calls: list[ToolCall] = field(default_factory=list)
    stop_reason: str | None = None


@dataclass
class AgentResult:
    """Result of agent task execution."""

    success: bool
    stop_reason: AgentStopReason
    turns: int
    total_tool_calls: int
    final_message: str | None = None
    error: str | None = None
    turn_history: list[AgentTurn] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "success": self.success,
            "stop_reason": self.stop_reason.value,
            "turns": self.turns,
            "total_tool_calls": self.total_tool_calls,
            "final_message": self.final_message,
            "error": self.error,
        }


@runtime_checkable
class LLMClient(Protocol):
    """Protocol for LLM client (for dependency injection in tests)."""

    def messages_create(
        self,
        model: str,
        max_tokens: int,
        system: str | None,
        tools: list[dict[str, Any]],
        messages: list[dict[str, Any]],
    ) -> Any:
        """Create a message with the LLM."""
        ...


class AgenticWorker:
    """Claude Code-style multi-turn agent with Tool Use.

    Executes tasks by having Claude use tools to read/write files,
    run shell commands, and search the workspace. The agent continues
    in a loop until it signals completion or hits limits.
    """

    DEFAULT_MODEL = "claude-sonnet-4-20250514"
    DEFAULT_MAX_TOKENS = 8192
    DEFAULT_MAX_TURNS = 50
    DEFAULT_MAX_TOOL_ERRORS = 3

    def __init__(
        self,
        workspace_id: str,
        api_client: C4APIClient,
        llm_client: LLMClient | None = None,
        model: str = DEFAULT_MODEL,
        max_turns: int = DEFAULT_MAX_TURNS,
        max_tool_errors: int = DEFAULT_MAX_TOOL_ERRORS,
        system_prompt: str | None = None,
    ):
        """Initialize agentic worker.

        Args:
            workspace_id: ID of the workspace to operate in
            api_client: HTTP client for C4 API
            llm_client: Optional LLM client (defaults to anthropic.Anthropic)
            model: Claude model to use
            max_turns: Maximum turns before stopping
            max_tool_errors: Max consecutive tool errors before stopping
            system_prompt: Optional custom system prompt
        """
        self.workspace_id = workspace_id
        self.api_client = api_client
        self.model = model
        self.max_turns = max_turns
        self.max_tool_errors = max_tool_errors
        self.system_prompt = system_prompt or self._default_system_prompt()

        # Initialize LLM client (lazy import to avoid hard dependency)
        if llm_client is not None:
            self._llm = llm_client
        else:
            try:
                import anthropic

                self._llm = anthropic.Anthropic()
            except ImportError:
                raise ImportError("anthropic package required. Install with: uv add anthropic")

    def _default_system_prompt(self) -> str:
        """Get default system prompt for the agent."""
        return """You are a skilled software engineer working on a task. You have access to tools to:
- Read and write files in the workspace
- Run shell commands
- Search for files and content
- List directory contents

Work step by step to complete the task. When you're done, provide a summary of what you accomplished.

Important guidelines:
- Always check if files exist before modifying them
- Write clean, well-documented code
- Run tests if available to verify your changes
- If you encounter errors, try to fix them
- Ask clarifying questions if the task is unclear"""

    async def execute_task(self, task_prompt: str) -> AgentResult:
        """Execute a task using multi-turn Tool Use.

        The agent will continue executing tool calls until it signals
        completion (end_turn) or hits the turn limit.

        Args:
            task_prompt: Description of the task to complete

        Returns:
            AgentResult with success status and execution details
        """
        messages: list[dict[str, Any]] = [{"role": "user", "content": task_prompt}]
        turn_history: list[AgentTurn] = []
        turn_count = 0
        total_tool_calls = 0
        consecutive_tool_errors = 0
        final_message: str | None = None

        while turn_count < self.max_turns:
            turn_count += 1
            current_turn = AgentTurn(turn_number=turn_count)

            try:
                # Call Claude with tools
                response = self._llm.messages_create(
                    model=self.model,
                    max_tokens=self.DEFAULT_MAX_TOKENS,
                    system=self.system_prompt,
                    tools=TOOLS,
                    messages=messages,
                )
            except Exception as e:
                current_turn.stop_reason = "api_error"
                turn_history.append(current_turn)
                return AgentResult(
                    success=False,
                    stop_reason=AgentStopReason.ERROR,
                    turns=turn_count,
                    total_tool_calls=total_tool_calls,
                    error=f"LLM API error: {str(e)}",
                    turn_history=turn_history,
                )

            # Extract response content
            if not response.content:
                current_turn.stop_reason = "no_content"
                turn_history.append(current_turn)
                return AgentResult(
                    success=False,
                    stop_reason=AgentStopReason.NO_RESPONSE,
                    turns=turn_count,
                    total_tool_calls=total_tool_calls,
                    error="LLM returned no content",
                    turn_history=turn_history,
                )

            # Add assistant response to message history
            messages.append({"role": "assistant", "content": response.content})
            current_turn.stop_reason = response.stop_reason

            # Extract text content for final message
            for block in response.content:
                if hasattr(block, "text"):
                    current_turn.assistant_message = block.text
                    final_message = block.text

            # Check for completion
            if response.stop_reason == "end_turn":
                turn_history.append(current_turn)
                return AgentResult(
                    success=True,
                    stop_reason=AgentStopReason.COMPLETED,
                    turns=turn_count,
                    total_tool_calls=total_tool_calls,
                    final_message=final_message,
                    turn_history=turn_history,
                )

            # Process tool calls
            if response.stop_reason == "tool_use":
                tool_results = []
                turn_had_error = False

                for block in response.content:
                    if hasattr(block, "type") and block.type == "tool_use":
                        start_time = time.monotonic()

                        # Execute tool
                        result, success = await self._execute_tool(block.name, block.input)

                        duration_ms = int((time.monotonic() - start_time) * 1000)
                        total_tool_calls += 1

                        if not success:
                            turn_had_error = True

                        # Record tool call
                        tool_call = ToolCall(
                            tool_name=block.name,
                            tool_input=block.input,
                            tool_use_id=block.id,
                            result=result,
                            success=success,
                            duration_ms=duration_ms,
                        )
                        current_turn.tool_calls.append(tool_call)

                        # Add tool result for next turn
                        tool_results.append(
                            {
                                "type": "tool_result",
                                "tool_use_id": block.id,
                                "content": result,
                                "is_error": not success,
                            }
                        )

                # Track consecutive errors
                if turn_had_error:
                    consecutive_tool_errors += 1
                else:
                    consecutive_tool_errors = 0

                # Check if too many errors
                if consecutive_tool_errors >= self.max_tool_errors:
                    turn_history.append(current_turn)
                    return AgentResult(
                        success=False,
                        stop_reason=AgentStopReason.TOOL_ERROR,
                        turns=turn_count,
                        total_tool_calls=total_tool_calls,
                        error=f"Too many consecutive tool errors ({consecutive_tool_errors})",
                        final_message=final_message,
                        turn_history=turn_history,
                    )

                # Add tool results as user message
                messages.append({"role": "user", "content": tool_results})

            turn_history.append(current_turn)

        # Hit max turns
        return AgentResult(
            success=False,
            stop_reason=AgentStopReason.MAX_TURNS,
            turns=turn_count,
            total_tool_calls=total_tool_calls,
            error=f"Max turns exceeded ({self.max_turns})",
            final_message=final_message,
            turn_history=turn_history,
        )

    async def _execute_tool(self, name: str, input_data: dict[str, Any]) -> tuple[str, bool]:
        """Execute a single tool call via API.

        Args:
            name: Tool name
            input_data: Tool input parameters

        Returns:
            Tuple of (result_string, success_bool)
        """
        # Validate input
        is_valid, error_msg = validate_tool_input(name, input_data)
        if not is_valid:
            return f"Input validation error: {error_msg}", False

        try:
            if name == ToolName.READ_FILE:
                content = await self.api_client.read_file(
                    self.workspace_id,
                    input_data["path"],
                )
                return content, True

            elif name == ToolName.WRITE_FILE:
                await self.api_client.write_file(
                    self.workspace_id,
                    input_data["path"],
                    input_data["content"],
                )
                return f"Successfully wrote to {input_data['path']}", True

            elif name == ToolName.RUN_SHELL:
                timeout = input_data.get("timeout", 60)
                result = await self.api_client.run_shell(
                    self.workspace_id,
                    input_data["command"],
                    timeout=timeout,
                )
                # Format result
                output_parts = []
                if result.stdout:
                    output_parts.append(f"stdout:\n{result.stdout}")
                if result.stderr:
                    output_parts.append(f"stderr:\n{result.stderr}")
                output_parts.append(f"exit_code: {result.exit_code}")
                return "\n".join(output_parts), result.exit_code == 0

            elif name == ToolName.SEARCH_FILES:
                results = await self.api_client.search_files(
                    self.workspace_id,
                    input_data["pattern"],
                    input_data["search_type"],
                    input_data.get("path", "."),
                )
                # Format results
                if not results:
                    return "No matches found", True
                result_dicts = [r.to_dict() for r in results]
                return json.dumps(result_dicts, indent=2), True

            elif name == ToolName.LIST_DIRECTORY:
                entries = await self.api_client.list_directory(
                    self.workspace_id,
                    input_data.get("path", "."),
                    input_data.get("recursive", False),
                )
                # Format entries
                if not entries:
                    return "Directory is empty", True
                entry_dicts = [e.to_dict() for e in entries]
                return json.dumps(entry_dicts, indent=2), True

            else:
                return f"Unknown tool: {name}", False

        except C4APIError as e:
            return f"API error: {str(e)}", False
        except Exception as e:
            return f"Tool execution error: {str(e)}", False
