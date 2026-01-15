"""Tests for C4 Chat API module."""

import json

import pytest
from fastapi.testclient import TestClient

from c4.api import create_app
from c4.api.chat import ChatHandler, get_handler
from c4.api.models import (
    ChatMessage,
    ChatRequest,
    ChatResponse,
    HealthResponse,
    MessageRole,
    StreamChunk,
)


class TestModels:
    """Test API models."""

    def test_chat_message(self) -> None:
        """Test ChatMessage creation."""
        msg = ChatMessage(
            role=MessageRole.USER,
            content="Hello, world!",
        )
        assert msg.role == MessageRole.USER
        assert msg.content == "Hello, world!"
        assert msg.timestamp is not None

    def test_chat_request_minimal(self) -> None:
        """Test ChatRequest with minimal fields."""
        req = ChatRequest(message="Test message")
        assert req.message == "Test message"
        assert req.stream is True
        assert req.context == []

    def test_chat_request_full(self) -> None:
        """Test ChatRequest with all fields."""
        req = ChatRequest(
            message="Test",
            conversation_id="conv-123",
            stream=False,
            model="custom",
            options={"temperature": 0.7},
        )
        assert req.conversation_id == "conv-123"
        assert req.stream is False
        assert req.model == "custom"
        assert req.options["temperature"] == 0.7

    def test_chat_response(self) -> None:
        """Test ChatResponse creation."""
        msg = ChatMessage(role=MessageRole.ASSISTANT, content="Response")
        resp = ChatResponse(
            message=msg,
            conversation_id="conv-123",
            usage={"input_tokens": 10, "output_tokens": 20},
        )
        assert resp.message.content == "Response"
        assert resp.conversation_id == "conv-123"

    def test_stream_chunk(self) -> None:
        """Test StreamChunk creation."""
        chunk = StreamChunk(
            type="content",
            content="Hello",
            conversation_id="conv-123",
        )
        assert chunk.type == "content"
        assert chunk.content == "Hello"

    def test_health_response(self) -> None:
        """Test HealthResponse creation."""
        resp = HealthResponse()
        assert resp.status == "ok"
        assert resp.version == "0.1.0"


class TestChatHandler:
    """Test ChatHandler class."""

    @pytest.fixture
    def handler(self) -> ChatHandler:
        """Create fresh ChatHandler."""
        return ChatHandler()

    def test_add_message(self, handler: ChatHandler) -> None:
        """Test adding message to conversation."""
        msg = ChatMessage(role=MessageRole.USER, content="Test")
        handler.add_message("conv-1", msg)

        history = handler.get_conversation("conv-1")
        assert len(history) == 1
        assert history[0].content == "Test"

    def test_multiple_messages(self, handler: ChatHandler) -> None:
        """Test adding multiple messages."""
        handler.add_message(
            "conv-1",
            ChatMessage(role=MessageRole.USER, content="Hello"),
        )
        handler.add_message(
            "conv-1",
            ChatMessage(role=MessageRole.ASSISTANT, content="Hi there"),
        )

        history = handler.get_conversation("conv-1")
        assert len(history) == 2
        assert history[0].role == MessageRole.USER
        assert history[1].role == MessageRole.ASSISTANT

    def test_separate_conversations(self, handler: ChatHandler) -> None:
        """Test that conversations are separate."""
        handler.add_message(
            "conv-1",
            ChatMessage(role=MessageRole.USER, content="Conv 1"),
        )
        handler.add_message(
            "conv-2",
            ChatMessage(role=MessageRole.USER, content="Conv 2"),
        )

        assert len(handler.get_conversation("conv-1")) == 1
        assert len(handler.get_conversation("conv-2")) == 1

    def test_empty_conversation(self, handler: ChatHandler) -> None:
        """Test getting non-existent conversation."""
        history = handler.get_conversation("non-existent")
        assert history == []

    @pytest.mark.asyncio
    async def test_generate_response(self, handler: ChatHandler) -> None:
        """Test response generation."""
        chunks = []
        async for chunk in handler.generate_response(
            message="Hello",
            context=[],
            options={},
        ):
            chunks.append(chunk)

        assert len(chunks) > 0
        full_response = "".join(chunks)
        assert "Hello" in full_response

    @pytest.mark.asyncio
    async def test_process_message(self, handler: ChatHandler) -> None:
        """Test processing a message request."""
        request = ChatRequest(message="Test message")

        conversation_id, response_gen = await handler.process_message(request)

        assert conversation_id is not None
        # Collect response
        chunks = []
        async for chunk in response_gen:
            chunks.append(chunk)
        assert len(chunks) > 0


class TestChatAPI:
    """Test Chat API endpoints."""

    @pytest.fixture
    def client(self) -> TestClient:
        """Create test client."""
        app = create_app()
        return TestClient(app)

    def test_health_check(self, client: TestClient) -> None:
        """Test health check endpoint."""
        response = client.get("/health")
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "ok"
        assert "version" in data

    def test_root_endpoint(self, client: TestClient) -> None:
        """Test root endpoint."""
        response = client.get("/")
        assert response.status_code == 200

        data = response.json()
        assert "name" in data
        assert "docs" in data

    def test_send_message_non_streaming(self, client: TestClient) -> None:
        """Test sending message without streaming."""
        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": False},
        )
        assert response.status_code == 200

        data = response.json()
        assert "message" in data
        assert "conversation_id" in data
        assert data["message"]["role"] == "assistant"

    def test_send_message_streaming(self, client: TestClient) -> None:
        """Test sending message with streaming."""
        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": True},
        )
        assert response.status_code == 200
        assert response.headers["content-type"] == "text/event-stream; charset=utf-8"

        # Parse SSE events
        events = []
        for line in response.iter_lines():
            if line.startswith("data: "):
                event_data = json.loads(line[6:])
                events.append(event_data)

        assert len(events) > 0
        # Should have content events and done event
        types = [e["type"] for e in events]
        assert "content" in types
        assert "done" in types

    def test_send_message_with_conversation_id(self, client: TestClient) -> None:
        """Test sending message with conversation ID."""
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

    def test_get_history_not_found(self, client: TestClient) -> None:
        """Test getting history for non-existent conversation."""
        response = client.get("/api/chat/history/non-existent-conv")
        assert response.status_code == 404

    def test_delete_history_not_found(self, client: TestClient) -> None:
        """Test deleting non-existent conversation."""
        response = client.delete("/api/chat/history/non-existent-conv")
        assert response.status_code == 404

    def test_full_conversation_flow(self, client: TestClient) -> None:
        """Test complete conversation flow."""
        # Send first message
        response1 = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": False},
        )
        assert response1.status_code == 200
        conv_id = response1.json()["conversation_id"]

        # Send follow-up message
        response2 = client.post(
            "/api/chat/message",
            json={
                "message": "How are you?",
                "conversation_id": conv_id,
                "stream": False,
            },
        )
        assert response2.status_code == 200
        assert response2.json()["conversation_id"] == conv_id

    def test_invalid_request(self, client: TestClient) -> None:
        """Test invalid request handling."""
        # Empty message
        response = client.post(
            "/api/chat/message",
            json={"message": ""},
        )
        assert response.status_code == 422  # Validation error


class TestServerConfig:
    """Test server configuration."""

    def test_create_app_default(self) -> None:
        """Test creating app with defaults."""
        app = create_app()
        assert app.title == "C4 API"
        assert app.version == "0.1.0"

    def test_create_app_custom(self) -> None:
        """Test creating app with custom config."""
        app = create_app(
            title="Custom API",
            description="Custom description",
            version="1.0.0",
        )
        assert app.title == "Custom API"
        assert app.version == "1.0.0"

    def test_cors_middleware(self) -> None:
        """Test CORS middleware is added."""
        app = create_app(cors_origins=["http://example.com"])
        # Check middleware stack
        middleware_classes = [m.cls.__name__ for m in app.user_middleware]
        assert "CORSMiddleware" in middleware_classes
