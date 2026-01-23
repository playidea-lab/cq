"""C4 Chat API - Message handling with AgenticWorker integration.

Provides a chat interface that connects user messages to the AgenticWorker
for autonomous task execution using Claude's Tool Use capability.
"""

import asyncio
import json
import logging
import os
import uuid
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any, AsyncGenerator

from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field

from .auth import CurrentUser

router = APIRouter()
logger = logging.getLogger(__name__)


# =============================================================================
# Models
# =============================================================================


class MessageRole(str, Enum):
    """Message role in conversation."""

    USER = "user"
    ASSISTANT = "assistant"
    SYSTEM = "system"
    TOOL = "tool"


class ToolCallInfo(BaseModel):
    """Information about a tool call."""

    name: str
    input: dict[str, Any]
    result: str | None = None
    success: bool | None = None
    duration_ms: int | None = None


class ChatMessage(BaseModel):
    """Chat message model."""

    id: str = Field(default_factory=lambda: str(uuid.uuid4()))
    role: MessageRole
    content: str
    timestamp: datetime = Field(default_factory=datetime.now)
    metadata: dict[str, Any] = Field(default_factory=dict)
    tool_calls: list[ToolCallInfo] = Field(default_factory=list)


class ChatRequest(BaseModel):
    """Chat request model."""

    message: str
    conversation_id: str | None = None
    workspace_id: str | None = None
    stream: bool = True
    context: dict[str, Any] = Field(default_factory=dict)


class ChatResponse(BaseModel):
    """Chat response model."""

    id: str = Field(default_factory=lambda: str(uuid.uuid4()))
    conversation_id: str
    workspace_id: str | None = None
    message: ChatMessage
    done: bool = False
    usage: dict[str, int] | None = None


class SSEEvent(BaseModel):
    """Server-Sent Event model."""

    event: str = "message"
    data: str
    id: str | None = None
    retry: int | None = None

    def encode(self) -> str:
        """Encode as SSE format."""
        lines = []
        if self.event != "message":
            lines.append(f"event: {self.event}")
        if self.id:
            lines.append(f"id: {self.id}")
        if self.retry:
            lines.append(f"retry: {self.retry}")
        lines.append(f"data: {self.data}")
        lines.append("")
        return "\n".join(lines) + "\n"


# =============================================================================
# Conversation Store
# =============================================================================


@dataclass
class Conversation:
    """Represents a conversation with its history and workspace."""

    id: str
    user_id: str | None = None
    workspace_id: str | None = None
    messages: list[ChatMessage] = field(default_factory=list)
    created_at: datetime = field(default_factory=datetime.now)
    updated_at: datetime = field(default_factory=datetime.now)


class ConversationStore:
    """In-memory conversation storage."""

    def __init__(self) -> None:
        self._conversations: dict[str, Conversation] = {}

    def get(self, conversation_id: str) -> Conversation | None:
        """Get conversation by ID."""
        return self._conversations.get(conversation_id)

    def get_for_user(self, conversation_id: str, user_id: str) -> Conversation | None:
        """Get conversation by ID, verifying user ownership.

        Args:
            conversation_id: Conversation ID
            user_id: User ID to verify ownership

        Returns:
            Conversation if found and owned by user, None otherwise
        """
        conv = self._conversations.get(conversation_id)
        if conv and conv.user_id == user_id:
            return conv
        return None

    def create(
        self,
        conversation_id: str | None = None,
        workspace_id: str | None = None,
        user_id: str | None = None,
    ) -> Conversation:
        """Create a new conversation."""
        conv_id = conversation_id or str(uuid.uuid4())
        conv = Conversation(id=conv_id, user_id=user_id, workspace_id=workspace_id)
        self._conversations[conv_id] = conv
        return conv

    def get_or_create(
        self,
        conversation_id: str | None,
        workspace_id: str | None = None,
        user_id: str | None = None,
    ) -> Conversation:
        """Get existing conversation or create new one."""
        if conversation_id and conversation_id in self._conversations:
            conv = self._conversations[conversation_id]
            # Verify ownership
            if user_id and conv.user_id and conv.user_id != user_id:
                # Different user, create new conversation
                return self.create(None, workspace_id, user_id)
            # Update user_id if not set
            if user_id and not conv.user_id:
                conv.user_id = user_id
            # Update workspace if provided
            if workspace_id and not conv.workspace_id:
                conv.workspace_id = workspace_id
            return conv
        return self.create(conversation_id, workspace_id, user_id)

    def add_message(self, conversation_id: str, message: ChatMessage) -> None:
        """Add message to conversation."""
        if conversation_id in self._conversations:
            conv = self._conversations[conversation_id]
            conv.messages.append(message)
            conv.updated_at = datetime.now()

    def delete(self, conversation_id: str) -> bool:
        """Delete conversation."""
        if conversation_id in self._conversations:
            del self._conversations[conversation_id]
            return True
        return False


# =============================================================================
# Chat Service with AgenticWorker
# =============================================================================


class AgenticChatService:
    """Chat service that integrates with AgenticWorker for task execution."""

    def __init__(self, api_base_url: str | None = None) -> None:
        """Initialize chat service.

        Args:
            api_base_url: Base URL for C4 API (for file/shell operations)
        """
        self._store = ConversationStore()
        self._api_base_url = api_base_url or os.getenv("C4_API_URL", "http://localhost:8000")
        self._llm_client: Any = None

    def _get_llm_client(self) -> Any:
        """Get or create LLM client (lazy initialization)."""
        if self._llm_client is None:
            try:
                import anthropic

                self._llm_client = anthropic.Anthropic()
            except ImportError:
                raise HTTPException(
                    status_code=500,
                    detail="anthropic package not installed. Run: uv add anthropic",
                )
        return self._llm_client

    def get_conversation(self, conversation_id: str) -> list[ChatMessage]:
        """Get conversation history."""
        conv = self._store.get(conversation_id)
        return conv.messages if conv else []

    async def generate_response(
        self,
        conversation_id: str,
        user_message: str,
        workspace_id: str | None = None,
        context: dict[str, Any] | None = None,
        user_id: str | None = None,
    ) -> AsyncGenerator[dict[str, Any], None]:
        """Generate streaming response with tool use.

        Yields SSE-compatible events:
        - start: Response started
        - thinking: Agent is thinking
        - tool_call: Tool being executed
        - tool_result: Tool execution result
        - chunk: Text chunk from assistant
        - done: Response complete

        Args:
            conversation_id: Conversation ID
            user_message: User's message
            workspace_id: Workspace for tool execution
            context: Additional context
            user_id: Authenticated user ID for ownership

        Yields:
            Event dictionaries for SSE streaming
        """
        # Get or create conversation
        conv = self._store.get_or_create(conversation_id, workspace_id, user_id)

        # Add user message
        user_msg = ChatMessage(role=MessageRole.USER, content=user_message)
        self._store.add_message(conversation_id, user_msg)

        # Check if we have workspace for tool use
        has_workspace = conv.workspace_id is not None

        # If no workspace, use simple chat mode (no tools)
        if not has_workspace:
            async for event in self._simple_chat(conv, user_message, context):
                yield event
            return

        # With workspace: use agentic mode with tools
        async for event in self._agentic_chat(conv, user_message, context):
            yield event

    async def _simple_chat(
        self,
        conv: Conversation,
        user_message: str,
        context: dict[str, Any] | None,
    ) -> AsyncGenerator[dict[str, Any], None]:
        """Simple chat without tools (for conversations without workspace)."""
        yield {"event": "start", "data": {"conversation_id": conv.id}}

        try:
            llm = self._get_llm_client()

            # Build messages from history
            messages = self._build_messages(conv, user_message)

            # System prompt for simple chat
            system = """You are a helpful AI assistant for C4, an AI-powered development system.

When users want to build something, guide them to:
1. First create a workspace with a git repository
2. Then describe what they want to build

For general questions, provide helpful answers.

Keep responses concise and actionable."""

            # Call Claude (non-streaming for simplicity)
            response = llm.messages.create(
                model="claude-sonnet-4-20250514",
                max_tokens=4096,
                system=system,
                messages=messages,
            )

            # Extract response text
            response_text = ""
            for block in response.content:
                if hasattr(block, "text"):
                    response_text = block.text
                    break

            # Yield response in chunks for SSE
            chunk_size = 50
            for i in range(0, len(response_text), chunk_size):
                chunk = response_text[i : i + chunk_size]
                yield {"event": "chunk", "data": {"content": chunk}}
                await asyncio.sleep(0.01)

            # Add assistant message to history
            assistant_msg = ChatMessage(role=MessageRole.ASSISTANT, content=response_text)
            self._store.add_message(conv.id, assistant_msg)

            yield {
                "event": "done",
                "data": {
                    "conversation_id": conv.id,
                    "content": response_text,
                    "done": True,
                },
            }

        except Exception as e:
            logger.exception("Error in simple chat")
            yield {"event": "error", "data": {"error": str(e)}}

    async def _agentic_chat(
        self,
        conv: Conversation,
        user_message: str,
        context: dict[str, Any] | None,
    ) -> AsyncGenerator[dict[str, Any], None]:
        """Agentic chat with tool use for workspace operations."""
        yield {
            "event": "start",
            "data": {"conversation_id": conv.id, "workspace_id": conv.workspace_id},
        }

        try:
            # Import AgenticWorker components
            from c4.web_worker.agent import AgenticWorker
            from c4.web_worker.client import C4APIClient

            # Create API client
            api_client = C4APIClient(base_url=self._api_base_url)

            # Create worker
            worker = AgenticWorker(
                workspace_id=conv.workspace_id,  # type: ignore
                api_client=api_client,
                llm_client=self._get_llm_client(),
                system_prompt=self._agentic_system_prompt(conv),
            )

            yield {"event": "thinking", "data": {"status": "Agent is analyzing the request..."}}

            # Execute task
            result = await worker.execute_task(user_message)

            # Stream tool calls from history
            for turn in result.turn_history:
                for tool_call in turn.tool_calls:
                    yield {
                        "event": "tool_call",
                        "data": {
                            "name": tool_call.tool_name,
                            "input": tool_call.tool_input,
                        },
                    }
                    yield {
                        "event": "tool_result",
                        "data": {
                            "name": tool_call.tool_name,
                            "result": (
                                tool_call.result[:500] + "..."
                                if len(tool_call.result) > 500
                                else tool_call.result
                            ),
                            "success": tool_call.success,
                            "duration_ms": tool_call.duration_ms,
                        },
                    }

            # Stream final message
            if result.final_message:
                chunk_size = 50
                for i in range(0, len(result.final_message), chunk_size):
                    chunk = result.final_message[i : i + chunk_size]
                    yield {"event": "chunk", "data": {"content": chunk}}
                    await asyncio.sleep(0.01)

            # Build tool call info for message
            all_tool_calls = []
            for turn in result.turn_history:
                for tc in turn.tool_calls:
                    all_tool_calls.append(
                        ToolCallInfo(
                            name=tc.tool_name,
                            input=tc.tool_input,
                            result=tc.result[:200] if len(tc.result) > 200 else tc.result,
                            success=tc.success,
                            duration_ms=tc.duration_ms,
                        )
                    )

            # Add assistant message
            assistant_msg = ChatMessage(
                role=MessageRole.ASSISTANT,
                content=result.final_message or "",
                metadata={
                    "success": result.success,
                    "turns": result.turns,
                    "stop_reason": result.stop_reason.value,
                },
                tool_calls=all_tool_calls,
            )
            self._store.add_message(conv.id, assistant_msg)

            yield {
                "event": "done",
                "data": {
                    "conversation_id": conv.id,
                    "workspace_id": conv.workspace_id,
                    "content": result.final_message or "",
                    "success": result.success,
                    "turns": result.turns,
                    "total_tool_calls": result.total_tool_calls,
                    "done": True,
                },
            }

        except ImportError as e:
            logger.exception("Missing dependency for agentic chat")
            yield {"event": "error", "data": {"error": f"Missing dependency: {str(e)}"}}
        except Exception as e:
            logger.exception("Error in agentic chat")
            yield {"event": "error", "data": {"error": str(e)}}

    def _build_messages(
        self, conv: Conversation, current_message: str
    ) -> list[dict[str, Any]]:
        """Build message list for Claude API from conversation history."""
        messages = []

        # Add history (limit to last 20 messages)
        for msg in conv.messages[-20:]:
            if msg.role in (MessageRole.USER, MessageRole.ASSISTANT):
                messages.append({"role": msg.role.value, "content": msg.content})

        # Add current message
        messages.append({"role": "user", "content": current_message})

        return messages

    def _agentic_system_prompt(self, conv: Conversation) -> str:
        """Build system prompt for agentic mode."""
        return f"""You are a skilled software engineer working in workspace '{conv.workspace_id}'.

You have tools to:
- Read and write files
- Run shell commands
- Search for files and content
- List directories

Work step by step to complete the user's request. Be thorough and verify your work.

Guidelines:
- Check file existence before modifying
- Write clean, documented code
- Run tests if available
- Fix errors if encountered
- Summarize what you accomplished when done"""


# =============================================================================
# Service Instance
# =============================================================================

_chat_service: AgenticChatService | None = None


def get_chat_service() -> AgenticChatService:
    """Get or create chat service instance."""
    global _chat_service
    if _chat_service is None:
        _chat_service = AgenticChatService()
    return _chat_service


# =============================================================================
# Routes
# =============================================================================


@router.post("/message", response_model=None)
async def send_message(
    request: ChatRequest,
    user: CurrentUser,
    chat_service: AgenticChatService = Depends(get_chat_service),
) -> StreamingResponse | ChatResponse:
    """Send a chat message and receive response.

    Requires authentication via JWT Bearer token or API key.

    When workspace_id is provided, the agent will use tools to execute
    file operations and shell commands in the workspace.

    Without workspace_id, simple conversational responses are returned.

    Streaming mode (default):
        Returns SSE stream with events:
        - start: Response started
        - thinking: Agent is processing
        - tool_call: Tool being executed
        - tool_result: Tool execution result
        - chunk: Text content chunk
        - done: Response complete
        - error: Error occurred

    Non-streaming mode:
        Returns complete ChatResponse.

    Args:
        request: Chat request with message and options
        user: Authenticated user

    Returns:
        Streaming SSE response or complete ChatResponse
    """
    conversation_id = request.conversation_id or str(uuid.uuid4())

    if request.stream:
        return StreamingResponse(
            _stream_response(
                chat_service,
                conversation_id,
                request.message,
                request.workspace_id,
                request.context,
                user.user_id,
            ),
            media_type="text/event-stream",
            headers={
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
                "X-Accel-Buffering": "no",
            },
        )
    else:
        # Non-streaming: collect full response
        response_content = ""
        workspace_id = request.workspace_id

        async for event in chat_service.generate_response(
            conversation_id,
            request.message,
            request.workspace_id,
            request.context,
            user.user_id,
        ):
            if event["event"] == "chunk":
                response_content += event["data"]["content"]
            elif event["event"] == "done":
                workspace_id = event["data"].get("workspace_id")

        return ChatResponse(
            conversation_id=conversation_id,
            workspace_id=workspace_id,
            message=ChatMessage(
                role=MessageRole.ASSISTANT,
                content=response_content,
            ),
            done=True,
        )


async def _stream_response(
    chat_service: AgenticChatService,
    conversation_id: str,
    message: str,
    workspace_id: str | None,
    context: dict[str, Any] | None,
    user_id: str | None = None,
) -> AsyncGenerator[str, None]:
    """Generate SSE stream for chat response."""
    async for event in chat_service.generate_response(
        conversation_id,
        message,
        workspace_id,
        context,
        user_id,
    ):
        sse = SSEEvent(
            event=event["event"],
            data=json.dumps(event["data"]),
        )
        yield sse.encode()


@router.get("/history/{conversation_id}")
async def get_history(
    conversation_id: str,
    user: CurrentUser,
    chat_service: AgenticChatService = Depends(get_chat_service),
) -> list[ChatMessage]:
    """Get conversation history.

    Requires authentication. Only returns history for conversations
    owned by the authenticated user.
    """
    store = chat_service._store
    conv = store.get_for_user(conversation_id, user.user_id)
    if not conv:
        raise HTTPException(status_code=404, detail="Conversation not found")
    return conv.messages


@router.delete("/history/{conversation_id}")
async def clear_history(
    conversation_id: str,
    user: CurrentUser,
    chat_service: AgenticChatService = Depends(get_chat_service),
) -> dict[str, bool]:
    """Clear conversation history.

    Requires authentication. Only allows deletion of conversations
    owned by the authenticated user.
    """
    store = chat_service._store
    conv = store.get_for_user(conversation_id, user.user_id)
    if not conv:
        raise HTTPException(status_code=404, detail="Conversation not found")
    success = store.delete(conversation_id)
    return {"success": success}


@router.post("/workspace/bind")
async def bind_workspace(
    conversation_id: str,
    workspace_id: str,
    user: CurrentUser,
    chat_service: AgenticChatService = Depends(get_chat_service),
) -> dict[str, Any]:
    """Bind a workspace to a conversation.

    Requires authentication. Creates conversation if not exists
    or verifies ownership if exists.

    Once bound, the chat will use agentic mode with tools to
    execute file and shell operations in the workspace.

    Args:
        conversation_id: Conversation to bind
        workspace_id: Workspace to bind
        user: Authenticated user

    Returns:
        Updated conversation info
    """
    conv = chat_service._store.get_or_create(conversation_id, workspace_id, user.user_id)
    conv.workspace_id = workspace_id
    return {
        "conversation_id": conv.id,
        "workspace_id": conv.workspace_id,
        "user_id": conv.user_id,
        "message_count": len(conv.messages),
    }
