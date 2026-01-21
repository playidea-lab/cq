"""Tests for C4 Chat API with AgenticWorker integration."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from c4.api.chat import (
    AgenticChatService,
    ChatMessage,
    ChatRequest,
    Conversation,
    ConversationStore,
    MessageRole,
    SSEEvent,
    ToolCallInfo,
)


# =============================================================================
# SSEEvent Tests
# =============================================================================


class TestSSEEvent:
    """Test SSE event encoding."""

    def test_encode_basic(self):
        """Test basic event encoding."""
        event = SSEEvent(event="message", data='{"content": "hello"}')
        encoded = event.encode()
        assert 'data: {"content": "hello"}' in encoded
        assert encoded.endswith("\n\n")

    def test_encode_with_event_type(self):
        """Test encoding with custom event type."""
        event = SSEEvent(event="chunk", data='{"content": "hello"}')
        encoded = event.encode()
        assert "event: chunk" in encoded
        assert "data:" in encoded

    def test_encode_with_id(self):
        """Test encoding with event ID."""
        event = SSEEvent(event="message", data='{"test": 1}', id="123")
        encoded = event.encode()
        assert "id: 123" in encoded


# =============================================================================
# ConversationStore Tests
# =============================================================================


class TestConversationStore:
    """Test conversation storage."""

    def test_create_conversation(self):
        """Test creating a new conversation."""
        store = ConversationStore()
        conv = store.create("conv-1", "workspace-1")

        assert conv.id == "conv-1"
        assert conv.workspace_id == "workspace-1"
        assert len(conv.messages) == 0

    def test_create_without_id(self):
        """Test creating conversation without ID generates one."""
        store = ConversationStore()
        conv = store.create()

        assert conv.id is not None
        assert len(conv.id) > 0

    def test_get_conversation(self):
        """Test getting a conversation."""
        store = ConversationStore()
        store.create("conv-1")

        conv = store.get("conv-1")
        assert conv is not None
        assert conv.id == "conv-1"

    def test_get_nonexistent(self):
        """Test getting nonexistent conversation."""
        store = ConversationStore()
        assert store.get("nonexistent") is None

    def test_get_or_create_existing(self):
        """Test get_or_create with existing conversation."""
        store = ConversationStore()
        original = store.create("conv-1", "workspace-1")

        retrieved = store.get_or_create("conv-1")
        assert retrieved is original

    def test_get_or_create_new(self):
        """Test get_or_create creates new conversation."""
        store = ConversationStore()

        conv = store.get_or_create("new-conv", "workspace-1")
        assert conv.id == "new-conv"
        assert conv.workspace_id == "workspace-1"

    def test_add_message(self):
        """Test adding message to conversation."""
        store = ConversationStore()
        store.create("conv-1")

        msg = ChatMessage(role=MessageRole.USER, content="Hello")
        store.add_message("conv-1", msg)

        conv = store.get("conv-1")
        assert len(conv.messages) == 1
        assert conv.messages[0].content == "Hello"

    def test_delete_conversation(self):
        """Test deleting conversation."""
        store = ConversationStore()
        store.create("conv-1")

        assert store.delete("conv-1") is True
        assert store.get("conv-1") is None

    def test_delete_nonexistent(self):
        """Test deleting nonexistent conversation."""
        store = ConversationStore()
        assert store.delete("nonexistent") is False

    def test_create_with_user_id(self):
        """Test creating conversation with user_id."""
        store = ConversationStore()
        conv = store.create("conv-1", "workspace-1", "user-1")

        assert conv.id == "conv-1"
        assert conv.workspace_id == "workspace-1"
        assert conv.user_id == "user-1"

    def test_get_for_user_success(self):
        """Test getting conversation for correct user."""
        store = ConversationStore()
        store.create("conv-1", "workspace-1", "user-1")

        conv = store.get_for_user("conv-1", "user-1")
        assert conv is not None
        assert conv.id == "conv-1"
        assert conv.user_id == "user-1"

    def test_get_for_user_wrong_user(self):
        """Test getting conversation for wrong user returns None."""
        store = ConversationStore()
        store.create("conv-1", "workspace-1", "user-1")

        conv = store.get_for_user("conv-1", "user-2")
        assert conv is None

    def test_get_for_user_nonexistent(self):
        """Test getting nonexistent conversation for user."""
        store = ConversationStore()
        assert store.get_for_user("nonexistent", "user-1") is None

    def test_get_or_create_user_isolation(self):
        """Test that get_or_create respects user isolation."""
        store = ConversationStore()
        # User 1 creates conversation
        conv1 = store.create("conv-1", "workspace-1", "user-1")

        # User 2 tries to access same conversation_id
        conv2 = store.get_or_create("conv-1", "workspace-2", "user-2")

        # Should create new conversation for user 2
        assert conv2.id != conv1.id
        assert conv2.user_id == "user-2"

    def test_get_or_create_sets_user_id(self):
        """Test that get_or_create sets user_id on existing conversation."""
        store = ConversationStore()
        # Create without user_id
        conv = store.create("conv-1", "workspace-1")
        assert conv.user_id is None

        # get_or_create with user_id
        conv2 = store.get_or_create("conv-1", None, "user-1")
        assert conv2 is conv
        assert conv.user_id == "user-1"


# =============================================================================
# ChatMessage Tests
# =============================================================================


class TestChatMessage:
    """Test chat message model."""

    def test_create_user_message(self):
        """Test creating user message."""
        msg = ChatMessage(role=MessageRole.USER, content="Hello")

        assert msg.role == MessageRole.USER
        assert msg.content == "Hello"
        assert msg.id is not None

    def test_create_with_tool_calls(self):
        """Test creating message with tool calls."""
        tool_call = ToolCallInfo(
            name="read_file",
            input={"path": "test.txt"},
            result="content",
            success=True,
            duration_ms=100,
        )
        msg = ChatMessage(
            role=MessageRole.ASSISTANT,
            content="I read the file",
            tool_calls=[tool_call],
        )

        assert len(msg.tool_calls) == 1
        assert msg.tool_calls[0].name == "read_file"


# =============================================================================
# AgenticChatService Tests
# =============================================================================


class TestAgenticChatService:
    """Test the agentic chat service."""

    def test_init(self):
        """Test service initialization."""
        service = AgenticChatService(api_base_url="http://test:8000")

        assert service._api_base_url == "http://test:8000"
        assert service._store is not None

    def test_get_conversation_empty(self):
        """Test getting empty conversation."""
        service = AgenticChatService()
        history = service.get_conversation("nonexistent")

        assert history == []

    def test_get_conversation_with_messages(self):
        """Test getting conversation with messages."""
        service = AgenticChatService()

        # Add messages through store
        conv = service._store.create("conv-1")
        msg = ChatMessage(role=MessageRole.USER, content="Test")
        service._store.add_message("conv-1", msg)

        history = service.get_conversation("conv-1")
        assert len(history) == 1
        assert history[0].content == "Test"

    def test_build_messages(self):
        """Test building message list for Claude API."""
        service = AgenticChatService()

        conv = Conversation(id="conv-1")
        conv.messages = [
            ChatMessage(role=MessageRole.USER, content="Hello"),
            ChatMessage(role=MessageRole.ASSISTANT, content="Hi"),
        ]

        messages = service._build_messages(conv, "New message")

        assert len(messages) == 3
        assert messages[0] == {"role": "user", "content": "Hello"}
        assert messages[1] == {"role": "assistant", "content": "Hi"}
        assert messages[2] == {"role": "user", "content": "New message"}

    def test_build_messages_limits_history(self):
        """Test that build_messages limits history to 20 messages."""
        service = AgenticChatService()

        conv = Conversation(id="conv-1")
        # Add 30 messages
        for i in range(30):
            conv.messages.append(
                ChatMessage(role=MessageRole.USER, content=f"Message {i}")
            )

        messages = service._build_messages(conv, "Current")

        # Should have 20 history + 1 current = 21
        assert len(messages) == 21

    def test_agentic_system_prompt(self):
        """Test agentic system prompt generation."""
        service = AgenticChatService()
        conv = Conversation(id="conv-1", workspace_id="ws-123")

        prompt = service._agentic_system_prompt(conv)

        assert "ws-123" in prompt
        assert "Read and write files" in prompt


# =============================================================================
# Async Service Tests
# =============================================================================


class TestAgenticChatServiceAsync:
    """Async tests for chat service."""

    @pytest.mark.asyncio
    async def test_simple_chat_without_workspace(self):
        """Test simple chat mode without workspace."""
        service = AgenticChatService()

        # Mock the LLM client
        mock_llm = MagicMock()
        mock_response = MagicMock()
        mock_block = MagicMock()
        mock_block.text = "Hello! I'm Claude."
        mock_response.content = [mock_block]
        mock_llm.messages.create.return_value = mock_response

        service._llm_client = mock_llm

        # Collect events
        events = []
        async for event in service.generate_response(
            "conv-1", "Hello", workspace_id=None
        ):
            events.append(event)

        # Check events
        assert any(e["event"] == "start" for e in events)
        assert any(e["event"] == "chunk" for e in events)
        assert any(e["event"] == "done" for e in events)

    @pytest.mark.asyncio
    async def test_agentic_chat_with_workspace(self):
        """Test agentic chat mode with workspace."""
        service = AgenticChatService()

        # Create mock LLM
        mock_llm = MagicMock()
        service._llm_client = mock_llm

        # Create mock agent result
        with patch("c4.web_worker.agent.AgenticWorker") as MockWorker:
            mock_worker = AsyncMock()
            mock_result = MagicMock()
            mock_result.success = True
            mock_result.final_message = "Task completed"
            mock_result.turns = 2
            mock_result.total_tool_calls = 3
            mock_result.stop_reason.value = "completed"
            mock_result.turn_history = []
            mock_worker.execute_task.return_value = mock_result
            MockWorker.return_value = mock_worker

            with patch("c4.web_worker.client.C4APIClient"):
                events = []
                async for event in service.generate_response(
                    "conv-1", "Build a calculator", workspace_id="ws-123"
                ):
                    events.append(event)

        # Check events
        assert any(e["event"] == "start" for e in events)
        assert any(e["event"] == "thinking" for e in events)
        assert any(e["event"] == "done" for e in events)

        # Check done event has agent info
        done_event = next(e for e in events if e["event"] == "done")
        assert done_event["data"]["success"] is True
        assert done_event["data"]["turns"] == 2

    @pytest.mark.asyncio
    async def test_error_handling(self):
        """Test error handling in chat service."""
        service = AgenticChatService()

        # Mock LLM to raise error
        mock_llm = MagicMock()
        mock_llm.messages.create.side_effect = Exception("API Error")
        service._llm_client = mock_llm

        events = []
        async for event in service.generate_response(
            "conv-1", "Hello", workspace_id=None
        ):
            events.append(event)

        # Should have error event
        assert any(e["event"] == "error" for e in events)


# =============================================================================
# ChatRequest Tests
# =============================================================================


class TestChatRequest:
    """Test chat request model."""

    def test_default_values(self):
        """Test default request values."""
        request = ChatRequest(message="Hello")

        assert request.message == "Hello"
        assert request.conversation_id is None
        assert request.workspace_id is None
        assert request.stream is True
        assert request.context == {}

    def test_with_workspace(self):
        """Test request with workspace."""
        request = ChatRequest(
            message="Build a feature",
            workspace_id="ws-123",
            stream=False,
        )

        assert request.workspace_id == "ws-123"
        assert request.stream is False
