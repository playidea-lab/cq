"""Tests for C4 Chat API module."""

import json

import pytest
from fastapi.testclient import TestClient

from c4.api import create_app
from c4.api.chat import ChatHandler
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
        msg = ChatMessage(role=MessageRole.USER, content="Hello, world!")
        assert msg.role == MessageRole.USER
        assert msg.content == "Hello, world!"

    def test_chat_request_minimal(self) -> None:
        """Test ChatRequest with minimal fields."""
        req = ChatRequest(message="Test message")
        assert req.message == "Test message"
        assert req.stream is True

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

    def test_chat_response(self) -> None:
        """Test ChatResponse creation."""
        msg = ChatMessage(role=MessageRole.ASSISTANT, content="Response")
        resp = ChatResponse(message=msg, conversation_id="conv-123")
        assert resp.message.content == "Response"

    def test_stream_chunk(self) -> None:
        """Test StreamChunk creation."""
        chunk = StreamChunk(type="content", content="Hello")
        assert chunk.type == "content"

    def test_health_response(self) -> None:
        """Test HealthResponse creation."""
        resp = HealthResponse()
        assert resp.status == "ok"


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

    def test_multiple_messages(self, handler: ChatHandler) -> None:
        """Test adding multiple messages."""
        handler.add_message(
            "conv-1", ChatMessage(role=MessageRole.USER, content="Hello")
        )
        handler.add_message(
            "conv-1", ChatMessage(role=MessageRole.ASSISTANT, content="Hi")
        )
        assert len(handler.get_conversation("conv-1")) == 2

    def test_separate_conversations(self, handler: ChatHandler) -> None:
        """Test that conversations are separate."""
        handler.add_message(
            "conv-1", ChatMessage(role=MessageRole.USER, content="Conv 1")
        )
        handler.add_message(
            "conv-2", ChatMessage(role=MessageRole.USER, content="Conv 2")
        )
        assert len(handler.get_conversation("conv-1")) == 1
        assert len(handler.get_conversation("conv-2")) == 1

    def test_empty_conversation(self, handler: ChatHandler) -> None:
        """Test getting non-existent conversation."""
        assert handler.get_conversation("non-existent") == []

    @pytest.mark.asyncio
    async def test_generate_response(self, handler: ChatHandler) -> None:
        """Test response generation."""
        chunks = []
        async for chunk in handler.generate_response("Hello", [], {}):
            chunks.append(chunk)
        assert len(chunks) > 0
        assert "Hello" in "".join(chunks)

    @pytest.mark.asyncio
    async def test_process_message(self, handler: ChatHandler) -> None:
        """Test processing a message request."""
        request = ChatRequest(message="Test message")
        conversation_id, response_gen = await handler.process_message(request)
        assert conversation_id is not None
        chunks = []
        async for chunk in response_gen:
            chunks.append(chunk)
        assert len(chunks) > 0


class TestChatAPI:
    """Test Chat API endpoints."""

    @pytest.fixture
    def client(self) -> TestClient:
        """Create test client."""
        return TestClient(create_app())

    def test_health_check(self, client: TestClient) -> None:
        """Test health check endpoint."""
        response = client.get("/health")
        assert response.status_code == 200
        assert response.json()["status"] == "ok"

    def test_root_endpoint(self, client: TestClient) -> None:
        """Test root endpoint."""
        response = client.get("/")
        assert response.status_code == 200
        assert "name" in response.json()

    def test_send_message_non_streaming(self, client: TestClient) -> None:
        """Test sending message without streaming."""
        response = client.post(
            "/api/chat/message", json={"message": "Hello", "stream": False}
        )
        assert response.status_code == 200
        data = response.json()
        assert "message" in data
        assert "conversation_id" in data

    def test_send_message_streaming(self, client: TestClient) -> None:
        """Test sending message with streaming."""
        response = client.post(
            "/api/chat/message", json={"message": "Hello", "stream": True}
        )
        assert response.status_code == 200
        assert "text/event-stream" in response.headers["content-type"]

        events = []
        for line in response.iter_lines():
            if line.startswith("data: "):
                events.append(json.loads(line[6:]))
        assert len(events) > 0
        assert "done" in [e["type"] for e in events]

    def test_send_message_with_conversation_id(self, client: TestClient) -> None:
        """Test sending message with conversation ID."""
        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "conversation_id": "test-123", "stream": False},
        )
        assert response.status_code == 200
        assert response.json()["conversation_id"] == "test-123"

    def test_get_history_not_found(self, client: TestClient) -> None:
        """Test getting history for non-existent conversation."""
        response = client.get("/api/chat/history/non-existent")
        assert response.status_code == 404

    def test_delete_history_not_found(self, client: TestClient) -> None:
        """Test deleting non-existent conversation."""
        response = client.delete("/api/chat/history/non-existent")
        assert response.status_code == 404

    def test_full_conversation_flow(self, client: TestClient) -> None:
        """Test complete conversation flow."""
        r1 = client.post(
            "/api/chat/message", json={"message": "Hello", "stream": False}
        )
        conv_id = r1.json()["conversation_id"]

        r2 = client.post(
            "/api/chat/message",
            json={"message": "Hi again", "conversation_id": conv_id, "stream": False},
        )
        assert r2.json()["conversation_id"] == conv_id

    def test_invalid_request(self, client: TestClient) -> None:
        """Test invalid request handling."""
        response = client.post("/api/chat/message", json={"message": ""})
        assert response.status_code == 422


class TestServerConfig:
    """Test server configuration."""

    def test_create_app_default(self) -> None:
        """Test creating app with defaults."""
        app = create_app()
        assert app.title == "C4 API"

    def test_create_app_custom(self) -> None:
        """Test creating app with custom config."""
        app = create_app(title="Custom API", version="1.0.0")
        assert app.title == "Custom API"
        assert app.version == "1.0.0"

    def test_cors_middleware(self) -> None:
        """Test CORS middleware is added."""
        app = create_app(cors_origins=["http://example.com"])
        middleware_classes = [m.cls.__name__ for m in app.user_middleware]
        assert "CORSMiddleware" in middleware_classes
