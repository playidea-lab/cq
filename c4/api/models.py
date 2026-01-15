"""C4 API Models - Request and response models for Chat API."""

from datetime import datetime
from enum import Enum
from typing import Any

from pydantic import BaseModel, Field


class MessageRole(str, Enum):
    """Message role types."""

    USER = "user"
    ASSISTANT = "assistant"
    SYSTEM = "system"


class ChatMessage(BaseModel):
    """A single chat message."""

    role: MessageRole
    content: str
    timestamp: datetime = Field(default_factory=datetime.now)
    metadata: dict[str, Any] = Field(default_factory=dict)


class ChatRequest(BaseModel):
    """Request body for chat message endpoint."""

    message: str = Field(..., min_length=1, max_length=32000)
    conversation_id: str | None = Field(
        None,
        description="Conversation ID for context continuity",
    )
    context: list[ChatMessage] = Field(
        default_factory=list,
        description="Previous messages for context",
    )
    stream: bool = Field(
        True,
        description="Whether to stream response via SSE",
    )
    model: str = Field(
        "default",
        description="Model to use for response",
    )
    options: dict[str, Any] = Field(
        default_factory=dict,
        description="Additional options for the chat",
    )


class ChatResponse(BaseModel):
    """Response body for chat message (non-streaming)."""

    message: ChatMessage
    conversation_id: str
    usage: dict[str, int] = Field(default_factory=dict)


class StreamChunk(BaseModel):
    """A chunk in SSE stream response."""

    type: str = Field(
        ...,
        description="Chunk type: content, done, error",
    )
    content: str = Field(
        "",
        description="Content text for this chunk",
    )
    conversation_id: str | None = None
    usage: dict[str, int] | None = None
    error: str | None = None


class HealthResponse(BaseModel):
    """Health check response."""

    status: str = "ok"
    version: str = "0.1.0"
    timestamp: datetime = Field(default_factory=datetime.now)
