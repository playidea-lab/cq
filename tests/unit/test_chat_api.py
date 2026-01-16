"""Tests for C4 Chat API."""

import json
from datetime import datetime

import pytest
from fastapi.testclient import TestClient

from c4.api import create_app
from c4.api.chat import (
    ChatMessage,
    ChatRequest,
    ChatResponse,
    ChatService,
    MessageRole,
    SSEEvent,
)


@pytest.fixture
def client() -> TestClient:
    """Create test client."""
    app = create_app()
    return TestClient(app)


@pytest.fixture
def chat_service() -> ChatService:
    """Create chat service."""
    return ChatService()


class TestChatModels:
    """Test chat data models."""

    def test_chat_message_defaults(self) -> None:
        """Test ChatMessage default values."""
        msg = ChatMessage(role=MessageRole.USER, content="Hello")

        assert msg.role == MessageRole.USER
        assert msg.content == "Hello"
        assert msg.id is not None
        assert msg.timestamp is not None
        assert msg.metadata == {}

    def test_chat_message_serialization(self) -> None:
        """Test ChatMessage JSON serialization."""
        msg = ChatMessage(
            role=MessageRole.ASSISTANT,
            content="Hi there",
            metadata={"tokens": 10},
        )
        data = msg.model_dump()

        assert data["role"] == "assistant"
        assert data["content"] == "Hi there"
        assert data["metadata"] == {"tokens": 10}

    def test_chat_request(self) -> None:
        """Test ChatRequest model."""
        req = ChatRequest(message="Test message")

        assert req.message == "Test message"
        assert req.conversation_id is None
        assert req.stream is True

    def test_chat_response(self) -> None:
        """Test ChatResponse model."""
        msg = ChatMessage(role=MessageRole.ASSISTANT, content="Response")
        resp = ChatResponse(
            conversation_id="conv-123",
            message=msg,
            done=True,
        )

        assert resp.conversation_id == "conv-123"
        assert resp.message.content == "Response"
        assert resp.done is True

    def test_sse_event_encode(self) -> None:
        """Test SSEEvent encoding."""
        event = SSEEvent(
            event="chunk",
            data='{"content": "hello"}',
            id="123",
        )
        encoded = event.encode()

        assert "event: chunk" in encoded
        assert "id: 123" in encoded
        assert 'data: {"content": "hello"}' in encoded

    def test_sse_event_default_message(self) -> None:
        """Test SSEEvent default message event."""
        event = SSEEvent(data='{"test": true}')
        encoded = event.encode()

        # Default "message" event should not include event line
        assert "event:" not in encoded
        assert 'data: {"test": true}' in encoded


class TestChatService:
    """Test ChatService class."""

    def test_get_empty_conversation(self, chat_service: ChatService) -> None:
        """Test getting non-existent conversation."""
        history = chat_service.get_conversation("non-existent")
        assert history == []

    def test_add_message(self, chat_service: ChatService) -> None:
        """Test adding message to conversation."""
        msg = ChatMessage(role=MessageRole.USER, content="Hello")
        chat_service.add_message("conv-1", msg)

        history = chat_service.get_conversation("conv-1")
        assert len(history) == 1
        assert history[0].content == "Hello"

    def test_add_multiple_messages(self, chat_service: ChatService) -> None:
        """Test adding multiple messages."""
        msg1 = ChatMessage(role=MessageRole.USER, content="Hi")
        msg2 = ChatMessage(role=MessageRole.ASSISTANT, content="Hello!")

        chat_service.add_message("conv-1", msg1)
        chat_service.add_message("conv-1", msg2)

        history = chat_service.get_conversation("conv-1")
        assert len(history) == 2
        assert history[0].role == MessageRole.USER
        assert history[1].role == MessageRole.ASSISTANT

    @pytest.mark.asyncio
    async def test_generate_response(self, chat_service: ChatService) -> None:
        """Test generating response."""
        chunks = []
        async for chunk in chat_service.generate_response(
            "conv-1",
            "Test message",
        ):
            chunks.append(chunk)

        assert len(chunks) > 0
        full_response = "".join(chunks)
        assert "Test message" in full_response

    @pytest.mark.asyncio
    async def test_generate_response_with_project(
        self, chat_service: ChatService
    ) -> None:
        """Test generating response with project context."""
        chunks = []
        async for chunk in chat_service.generate_response(
            "conv-1",
            "Hello",
            project_id="test-project",
        ):
            chunks.append(chunk)

        full_response = "".join(chunks)
        assert "test-project" in full_response


class TestChatAPI:
    """Test Chat API endpoints."""

    def test_health_check(self, client: TestClient) -> None:
        """Test health check endpoint."""
        response = client.get("/api/health")

        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "healthy"

    def test_send_message_non_streaming(self, client: TestClient) -> None:
        """Test sending message without streaming."""
        response = client.post(
            "/api/chat/message",
            json={
                "message": "Hello, C4!",
                "stream": False,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert "conversation_id" in data
        assert "message" in data
        assert data["done"] is True
        assert "Hello, C4!" in data["message"]["content"]

    def test_send_message_streaming(self, client: TestClient) -> None:
        """Test sending message with SSE streaming."""
        response = client.post(
            "/api/chat/message",
            json={
                "message": "Test streaming",
                "stream": True,
            },
        )

        assert response.status_code == 200
        assert response.headers["content-type"] == "text/event-stream; charset=utf-8"

        # Parse SSE events
        events = []
        for line in response.text.split("\n"):
            if line.startswith("data:"):
                data = line[5:].strip()
                events.append(json.loads(data))

        # Should have start, chunks, and done events
        assert len(events) >= 2  # At least start and done

    def test_send_message_with_conversation_id(self, client: TestClient) -> None:
        """Test sending message with specific conversation ID."""
        conv_id = "test-conv-123"
        response = client.post(
            "/api/chat/message",
            json={
                "message": "Hello",
                "conversation_id": conv_id,
                "stream": False,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["conversation_id"] == conv_id

    def test_get_history(self, client: TestClient) -> None:
        """Test getting conversation history."""
        # Send a message first
        conv_id = "history-test"
        client.post(
            "/api/chat/message",
            json={
                "message": "Test history",
                "conversation_id": conv_id,
                "stream": False,
            },
        )

        # Get history
        response = client.get(f"/api/chat/history/{conv_id}")

        assert response.status_code == 200
        history = response.json()
        assert len(history) >= 2  # User + assistant messages

    def test_clear_history(self, client: TestClient) -> None:
        """Test clearing conversation history."""
        conv_id = "clear-test"

        # Send a message
        client.post(
            "/api/chat/message",
            json={
                "message": "To be cleared",
                "conversation_id": conv_id,
                "stream": False,
            },
        )

        # Clear history
        response = client.delete(f"/api/chat/history/{conv_id}")
        assert response.status_code == 200
        assert response.json()["success"] is True

        # Verify cleared
        history_response = client.get(f"/api/chat/history/{conv_id}")
        assert history_response.json() == []
