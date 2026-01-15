"""C4 Chat API - Chat endpoint with SSE streaming support."""

import asyncio
import uuid
from collections.abc import AsyncGenerator
from typing import Any

from fastapi import APIRouter, HTTPException
from fastapi.responses import StreamingResponse

from .models import (
    ChatMessage,
    ChatRequest,
    ChatResponse,
    MessageRole,
    StreamChunk,
)

router = APIRouter(prefix="/api/chat", tags=["chat"])


class ChatHandler:
    """Handles chat message processing."""

    def __init__(self) -> None:
        """Initialize chat handler."""
        self._conversations: dict[str, list[ChatMessage]] = {}

    def get_conversation(self, conversation_id: str) -> list[ChatMessage]:
        """Get conversation history."""
        return self._conversations.get(conversation_id, [])

    def add_message(self, conversation_id: str, message: ChatMessage) -> None:
        """Add message to conversation history."""
        if conversation_id not in self._conversations:
            self._conversations[conversation_id] = []
        self._conversations[conversation_id].append(message)

    async def generate_response(
        self,
        message: str,
        context: list[ChatMessage],
        options: dict[str, Any],
    ) -> AsyncGenerator[str, None]:
        """Generate streaming response chunks.

        This is a placeholder implementation that echoes the message.
        In production, this would call an LLM API.
        """
        response_parts = [
            f"Received your message: ",
            f'"{message[:100]}',
            '..." ' if len(message) > 100 else '" ',
            "Processing with C4 Chat API. ",
            "This is a placeholder response. ",
            "Connect to an LLM provider for real responses.",
        ]

        for part in response_parts:
            yield part
            await asyncio.sleep(0.05)

    async def process_message(
        self,
        request: ChatRequest,
    ) -> tuple[str, AsyncGenerator[str, None]]:
        """Process a chat message request."""
        conversation_id = request.conversation_id or str(uuid.uuid4())

        user_message = ChatMessage(
            role=MessageRole.USER,
            content=request.message,
        )
        self.add_message(conversation_id, user_message)

        context = request.context + self.get_conversation(conversation_id)

        response_gen = self.generate_response(
            message=request.message,
            context=context,
            options=request.options,
        )

        return conversation_id, response_gen


_handler = ChatHandler()


def get_handler() -> ChatHandler:
    """Get the chat handler instance."""
    return _handler


async def stream_response(
    conversation_id: str,
    response_gen: AsyncGenerator[str, None],
) -> AsyncGenerator[str, None]:
    """Convert response generator to SSE format."""
    full_response = ""

    try:
        async for chunk in response_gen:
            full_response += chunk
            event = StreamChunk(
                type="content",
                content=chunk,
                conversation_id=conversation_id,
            )
            yield f"data: {event.model_dump_json()}\n\n"

        done_event = StreamChunk(
            type="done",
            conversation_id=conversation_id,
            usage={"input_tokens": 0, "output_tokens": len(full_response)},
        )
        yield f"data: {done_event.model_dump_json()}\n\n"

    except Exception as e:
        error_event = StreamChunk(
            type="error",
            error=str(e),
            conversation_id=conversation_id,
        )
        yield f"data: {error_event.model_dump_json()}\n\n"


@router.post("/message", response_model=None)
async def send_message(request: ChatRequest) -> StreamingResponse | ChatResponse:
    """Send a chat message and receive response.

    For streaming (default), returns SSE stream.
    For non-streaming, returns complete response.
    """
    handler = get_handler()

    try:
        conversation_id, response_gen = await handler.process_message(request)
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

    if request.stream:
        return StreamingResponse(
            stream_response(conversation_id, response_gen),
            media_type="text/event-stream",
            headers={
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
                "X-Conversation-ID": conversation_id,
            },
        )
    else:
        full_response = ""
        async for chunk in response_gen:
            full_response += chunk

        assistant_message = ChatMessage(
            role=MessageRole.ASSISTANT,
            content=full_response,
        )
        handler.add_message(conversation_id, assistant_message)

        return ChatResponse(
            message=assistant_message,
            conversation_id=conversation_id,
            usage={"input_tokens": 0, "output_tokens": len(full_response)},
        )


@router.get("/history/{conversation_id}")
async def get_history(conversation_id: str) -> list[ChatMessage]:
    """Get conversation history."""
    handler = get_handler()
    history = handler.get_conversation(conversation_id)

    if not history:
        raise HTTPException(
            status_code=404,
            detail=f"Conversation {conversation_id} not found",
        )

    return history


@router.delete("/history/{conversation_id}")
async def clear_history(conversation_id: str) -> dict[str, str]:
    """Clear conversation history."""
    handler = get_handler()

    if conversation_id in handler._conversations:
        del handler._conversations[conversation_id]
        return {"status": "cleared", "conversation_id": conversation_id}

    raise HTTPException(
        status_code=404,
        detail=f"Conversation {conversation_id} not found",
    )
