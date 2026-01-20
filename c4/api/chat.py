"""C4 Chat API - Message handling and SSE streaming."""

import asyncio
import json
import uuid
from datetime import datetime
from enum import Enum
from typing import Any, AsyncGenerator

from fastapi import APIRouter, Depends
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field

router = APIRouter()


# =============================================================================
# Models
# =============================================================================


class MessageRole(str, Enum):
    """Message role in conversation."""

    USER = "user"
    ASSISTANT = "assistant"
    SYSTEM = "system"


class ChatMessage(BaseModel):
    """Chat message model."""

    id: str = Field(default_factory=lambda: str(uuid.uuid4()))
    role: MessageRole
    content: str
    timestamp: datetime = Field(default_factory=datetime.now)
    metadata: dict[str, Any] = Field(default_factory=dict)


class ChatRequest(BaseModel):
    """Chat request model."""

    message: str
    conversation_id: str | None = None
    project_id: str | None = None
    stream: bool = True
    context: dict[str, Any] = Field(default_factory=dict)


class ChatResponse(BaseModel):
    """Chat response model."""

    id: str = Field(default_factory=lambda: str(uuid.uuid4()))
    conversation_id: str
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
        lines.append("")  # Empty line to end event
        return "\n".join(lines) + "\n"


# =============================================================================
# Chat Service
# =============================================================================


class ChatService:
    """Service for handling chat interactions."""

    def __init__(self) -> None:
        """Initialize chat service."""
        self._conversations: dict[str, list[ChatMessage]] = {}

    def get_conversation(self, conversation_id: str) -> list[ChatMessage]:
        """Get conversation history.

        Args:
            conversation_id: Conversation ID

        Returns:
            List of messages
        """
        return self._conversations.get(conversation_id, [])

    def add_message(self, conversation_id: str, message: ChatMessage) -> None:
        """Add message to conversation.

        Args:
            conversation_id: Conversation ID
            message: Message to add
        """
        if conversation_id not in self._conversations:
            self._conversations[conversation_id] = []
        self._conversations[conversation_id].append(message)

    async def generate_response(
        self,
        conversation_id: str,
        user_message: str,
        project_id: str | None = None,
        context: dict[str, Any] | None = None,
    ) -> AsyncGenerator[str, None]:
        """Generate streaming response.

        Args:
            conversation_id: Conversation ID
            user_message: User's message
            project_id: Optional project context
            context: Additional context

        Yields:
            Response chunks
        """
        # Add user message to history
        user_msg = ChatMessage(
            role=MessageRole.USER,
            content=user_message,
        )
        self.add_message(conversation_id, user_msg)

        # Simulate streaming response
        # In production, this would call the LLM backend
        # Build response message
        msg_preview = user_message[:50] + "..." if len(user_message) > 50 else user_message
        response_parts = [
            "I received your message",
            f": '{msg_preview}'.",
            "\n\nThis is a placeholder response from the C4 Chat API.",
            " In production, this would be connected to an LLM backend",
            " for intelligent responses.",
        ]

        if project_id:
            response_parts.append(f"\n\nProject context: {project_id}")

        for part in response_parts:
            yield part
            await asyncio.sleep(0.05)  # Simulate streaming delay

        # Add assistant message to history
        full_response = "".join(response_parts)
        assistant_msg = ChatMessage(
            role=MessageRole.ASSISTANT,
            content=full_response,
        )
        self.add_message(conversation_id, assistant_msg)


# Global service instance
_chat_service: ChatService | None = None


def get_chat_service() -> ChatService:
    """Get or create chat service instance."""
    global _chat_service
    if _chat_service is None:
        _chat_service = ChatService()
    return _chat_service


# =============================================================================
# Routes
# =============================================================================


@router.post("/message", response_model=None)
async def send_message(
    request: ChatRequest,
    chat_service: ChatService = Depends(get_chat_service),
) -> StreamingResponse | ChatResponse:
    """Send a chat message and receive response.

    Supports both streaming (SSE) and non-streaming responses.

    Args:
        request: Chat request with message and options
        chat_service: Injected chat service

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
                request.project_id,
                request.context,
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
        response_parts = []
        async for chunk in chat_service.generate_response(
            conversation_id,
            request.message,
            request.project_id,
            request.context,
        ):
            response_parts.append(chunk)

        full_response = "".join(response_parts)
        return ChatResponse(
            conversation_id=conversation_id,
            message=ChatMessage(
                role=MessageRole.ASSISTANT,
                content=full_response,
            ),
            done=True,
        )


async def _stream_response(
    chat_service: ChatService,
    conversation_id: str,
    message: str,
    project_id: str | None,
    context: dict[str, Any] | None,
) -> AsyncGenerator[str, None]:
    """Generate SSE stream for chat response.

    Args:
        chat_service: Chat service instance
        conversation_id: Conversation ID
        message: User message
        project_id: Project context
        context: Additional context

    Yields:
        SSE formatted events
    """
    response_id = str(uuid.uuid4())

    # Send start event
    start_event = SSEEvent(
        event="start",
        data=json.dumps(
            {
                "id": response_id,
                "conversation_id": conversation_id,
            }
        ),
    )
    yield start_event.encode()

    # Stream content chunks
    full_content = ""
    async for chunk in chat_service.generate_response(
        conversation_id,
        message,
        project_id,
        context,
    ):
        full_content += chunk
        chunk_event = SSEEvent(
            event="chunk",
            data=json.dumps({"content": chunk}),
            id=response_id,
        )
        yield chunk_event.encode()

    # Send done event
    done_event = SSEEvent(
        event="done",
        data=json.dumps(
            {
                "id": response_id,
                "conversation_id": conversation_id,
                "content": full_content,
                "done": True,
            }
        ),
    )
    yield done_event.encode()


@router.get("/history/{conversation_id}")
async def get_history(
    conversation_id: str,
    chat_service: ChatService = Depends(get_chat_service),
) -> list[ChatMessage]:
    """Get conversation history.

    Args:
        conversation_id: Conversation ID

    Returns:
        List of messages in conversation
    """
    return chat_service.get_conversation(conversation_id)


@router.delete("/history/{conversation_id}")
async def clear_history(
    conversation_id: str,
    chat_service: ChatService = Depends(get_chat_service),
) -> dict[str, bool]:
    """Clear conversation history.

    Args:
        conversation_id: Conversation ID

    Returns:
        Success status
    """
    if conversation_id in chat_service._conversations:
        del chat_service._conversations[conversation_id]
    return {"success": True}
