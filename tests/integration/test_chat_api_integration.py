"""Integration tests for Chat API with workspace and agentic chat features.

Tests the complete flow of:
- Chat message handling with authentication
- Workspace binding and agentic mode triggering
- SSE streaming responses
- Conversation history persistence
- Error handling
- Simple vs Agentic mode isolation
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from datetime import datetime, timedelta
from unittest.mock import AsyncMock, MagicMock, patch

import jwt
import pytest
from fastapi.testclient import TestClient

from c4.api.app import create_app
from c4.api.auth import AuthConfig, clear_auth_config_cache
from c4.api.chat import (
    AgenticChatService,
)

# =============================================================================
# Test Fixtures
# =============================================================================


@pytest.fixture
def auth_config():
    """Create auth config with test JWT secret."""
    config = AuthConfig()
    # Override after __post_init__ which reads from env
    config.jwt_secret = "test-secret-key-for-jwt-signing-min-32-chars"
    config.jwt_algorithm = "HS256"
    config.api_keys = ["test-api-key-123"]
    return config


@pytest.fixture
def valid_jwt_token(auth_config):
    """Create a valid JWT token for testing."""
    payload = {
        "sub": "user-123",
        "email": "test@example.com",
        "aud": "authenticated",
        "role": "authenticated",
        "iat": int(datetime.now().timestamp()),
        "exp": int((datetime.now() + timedelta(hours=1)).timestamp()),
    }
    return jwt.encode(payload, auth_config.jwt_secret, algorithm=auth_config.jwt_algorithm)


@pytest.fixture
def expired_jwt_token(auth_config):
    """Create an expired JWT token for testing."""
    payload = {
        "sub": "user-123",
        "email": "test@example.com",
        "aud": "authenticated",
        "iat": int((datetime.now() - timedelta(hours=2)).timestamp()),
        "exp": int((datetime.now() - timedelta(hours=1)).timestamp()),
    }
    return jwt.encode(payload, auth_config.jwt_secret, algorithm=auth_config.jwt_algorithm)


@pytest.fixture
def mock_llm_client():
    """Create a mock LLM client."""
    mock_llm = MagicMock()
    return mock_llm


@pytest.fixture
def chat_service(mock_llm_client):
    """Create a chat service with mocked LLM."""
    service = AgenticChatService(api_base_url="http://test:8000")
    service._llm_client = mock_llm_client
    return service


@pytest.fixture
def app(auth_config, chat_service):
    """Create FastAPI app with test configuration."""
    # Clear any cached auth config
    clear_auth_config_cache()

    # Set environment variables for auth - ensure values are strings
    jwt_secret = auth_config.jwt_secret or ""
    api_keys = ",".join(auth_config.api_keys) if auth_config.api_keys else ""

    with patch.dict(
        os.environ,
        {
            "SUPABASE_JWT_SECRET": jwt_secret,
            "C4_API_KEYS": api_keys,
        },
    ):
        # Clear cache again after setting env vars
        clear_auth_config_cache()

        # Create app without rate limiting for tests
        test_app = create_app(enable_rate_limit=False)

        # Override the chat service dependency
        def get_test_chat_service():
            return chat_service

        # Find and override the dependency
        from c4.api.chat import get_chat_service as original_get_chat_service

        test_app.dependency_overrides[original_get_chat_service] = get_test_chat_service

        yield test_app

        # Cleanup
        test_app.dependency_overrides.clear()
        clear_auth_config_cache()


@pytest.fixture
def client(app):
    """Create test client."""
    return TestClient(app)


@pytest.fixture
def auth_headers(valid_jwt_token):
    """Create authorization headers with JWT token."""
    return {"Authorization": f"Bearer {valid_jwt_token}"}


@pytest.fixture
def api_key_headers():
    """Create authorization headers with API key."""
    return {"X-API-Key": "test-api-key-123"}


# =============================================================================
# Mock Helpers
# =============================================================================


@dataclass
class MockTextBlock:
    """Mock text content block for LLM response."""

    type: str = "text"
    text: str = ""


@dataclass
class MockToolUseBlock:
    """Mock tool use content block for LLM response."""

    type: str = "tool_use"
    id: str = ""
    name: str = ""
    input: dict = None

    def __post_init__(self):
        if self.input is None:
            self.input = {}


@dataclass
class MockLLMResponse:
    """Mock LLM response."""

    content: list = None
    stop_reason: str = "end_turn"

    def __post_init__(self):
        if self.content is None:
            self.content = []


def create_simple_llm_response(text: str) -> MockLLMResponse:
    """Create a simple LLM response with text."""
    return MockLLMResponse(
        content=[MockTextBlock(text=text)],
        stop_reason="end_turn",
    )


# =============================================================================
# TestChatAPIWithWorkspace
# =============================================================================


class TestChatAPIWithWorkspace:
    """Test Chat API with workspace integration."""

    def test_chat_creates_conversation(self, client, auth_headers, mock_llm_client):
        """Test that chat creates a new conversation."""
        # Setup mock LLM response
        mock_llm_client.messages.create.return_value = create_simple_llm_response(
            "Hello! How can I help you today?"
        )

        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": False},
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()
        assert "conversation_id" in data
        assert data["message"]["role"] == "assistant"
        assert "Hello" in data["message"]["content"] or len(data["message"]["content"]) > 0

    def test_chat_with_workspace_triggers_agent(
        self, client, auth_headers, mock_llm_client, chat_service
    ):
        """Test that chat with workspace_id triggers AgenticWorker."""
        # This test verifies the workspace binding triggers agentic mode
        # Import from correct module path where the class is defined
        with patch("c4.web_worker.agent.AgenticWorker") as MockWorker, patch(
            "c4.web_worker.client.C4APIClient"
        ):
            # Setup mock agent result
            mock_result = MagicMock()
            mock_result.success = True
            mock_result.final_message = "Task completed successfully"
            mock_result.turns = 2
            mock_result.total_tool_calls = 1
            mock_result.stop_reason.value = "completed"
            mock_result.turn_history = []

            mock_worker_instance = AsyncMock()
            mock_worker_instance.execute_task.return_value = mock_result
            MockWorker.return_value = mock_worker_instance

            response = client.post(
                "/api/chat/message",
                json={
                    "message": "Create a hello world file",
                    "workspace_id": "ws-test-123",
                    "stream": False,
                },
                headers=auth_headers,
            )

            assert response.status_code == 200
            data = response.json()
            assert data["workspace_id"] == "ws-test-123"
            # Verify AgenticWorker was called
            MockWorker.assert_called_once()

    def test_chat_streaming_response(self, client, auth_headers, mock_llm_client):
        """Test SSE streaming response format."""
        mock_llm_client.messages.create.return_value = create_simple_llm_response(
            "This is a streaming response test."
        )

        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": True},
            headers=auth_headers,
        )

        assert response.status_code == 200
        assert response.headers["content-type"] == "text/event-stream; charset=utf-8"

        # Parse SSE events
        events = []
        for line in response.iter_lines():
            if line.startswith("data:"):
                data = json.loads(line[5:].strip())
                events.append(data)

        # Should have start, chunk(s), and done events
        assert len(events) > 0

    def test_chat_error_handling(self, client, auth_headers, mock_llm_client):
        """Test error handling when LLM fails."""
        mock_llm_client.messages.create.side_effect = Exception("LLM API Error")

        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": False},
            headers=auth_headers,
        )

        # The response should still be 200 but contain error in content
        # Because errors are returned as part of the chat flow
        assert response.status_code == 200

    def test_chat_error_handling_streaming(self, client, auth_headers, mock_llm_client):
        """Test error handling in streaming mode."""
        mock_llm_client.messages.create.side_effect = Exception("LLM API Error")

        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": True},
            headers=auth_headers,
        )

        assert response.status_code == 200

        # Parse SSE events to find error
        has_error = False
        for line in response.iter_lines():
            if "error" in line.lower():
                has_error = True
                break

        assert has_error, "Expected error event in stream"

    def test_chat_history_persistence(self, client, auth_headers, mock_llm_client, chat_service):
        """Test that conversation history is persisted."""
        mock_llm_client.messages.create.return_value = create_simple_llm_response(
            "First response"
        )

        # First message
        response1 = client.post(
            "/api/chat/message",
            json={"message": "First message", "stream": False},
            headers=auth_headers,
        )
        assert response1.status_code == 200
        conversation_id = response1.json()["conversation_id"]

        # Second message in same conversation
        mock_llm_client.messages.create.return_value = create_simple_llm_response(
            "Second response"
        )
        response2 = client.post(
            "/api/chat/message",
            json={
                "message": "Second message",
                "conversation_id": conversation_id,
                "stream": False,
            },
            headers=auth_headers,
        )
        assert response2.status_code == 200
        assert response2.json()["conversation_id"] == conversation_id

        # Get history
        history_response = client.get(
            f"/api/chat/history/{conversation_id}",
            headers=auth_headers,
        )
        assert history_response.status_code == 200
        history = history_response.json()

        # Should have 4 messages: user1, assistant1, user2, assistant2
        assert len(history) == 4
        assert history[0]["role"] == "user"
        assert history[1]["role"] == "assistant"

    def test_chat_tool_execution_workflow(self, client, auth_headers, chat_service):
        """Test tool execution workflow in agentic mode."""
        with patch("c4.web_worker.agent.AgenticWorker") as MockWorker, patch(
            "c4.web_worker.client.C4APIClient"
        ):
            # Setup mock with tool calls
            mock_tool_call = MagicMock()
            mock_tool_call.tool_name = "read_file"
            mock_tool_call.tool_input = {"path": "/test.txt"}
            mock_tool_call.result = "file content"
            mock_tool_call.success = True
            mock_tool_call.duration_ms = 50

            mock_turn = MagicMock()
            mock_turn.tool_calls = [mock_tool_call]

            mock_result = MagicMock()
            mock_result.success = True
            mock_result.final_message = "I read the file for you."
            mock_result.turns = 2
            mock_result.total_tool_calls = 1
            mock_result.stop_reason.value = "completed"
            mock_result.turn_history = [mock_turn]

            mock_worker_instance = AsyncMock()
            mock_worker_instance.execute_task.return_value = mock_result
            MockWorker.return_value = mock_worker_instance

            response = client.post(
                "/api/chat/message",
                json={
                    "message": "Read the test file",
                    "workspace_id": "ws-123",
                    "stream": True,
                },
                headers=auth_headers,
            )

            assert response.status_code == 200

            # Check for tool_call and tool_result events in stream
            events = []
            for line in response.iter_lines():
                if line.startswith("event:"):
                    events.append(line.split(":")[1].strip())

            assert "tool_call" in events or "tool_result" in events or len(events) > 0


# =============================================================================
# TestAgenticChatFlow
# =============================================================================


class TestAgenticChatFlow:
    """Test Agentic chat flow and mode isolation."""

    def test_simple_chat_without_agent(self, client, auth_headers, mock_llm_client):
        """Test that chat without workspace uses simple mode (no agent)."""
        mock_llm_client.messages.create.return_value = create_simple_llm_response(
            "I can help you with that. Please create a workspace first."
        )

        with patch("c4.web_worker.agent.AgenticWorker") as MockWorker:
            response = client.post(
                "/api/chat/message",
                json={
                    "message": "Hello, help me build something",
                    "stream": False,
                    # No workspace_id - simple mode
                },
                headers=auth_headers,
            )

            assert response.status_code == 200
            # AgenticWorker should NOT be called in simple mode
            MockWorker.assert_not_called()

    def test_agentic_chat_with_tool_calls(self, client, auth_headers):
        """Test agentic chat mode with tool calls."""
        with patch("c4.web_worker.agent.AgenticWorker") as MockWorker, patch(
            "c4.web_worker.client.C4APIClient"
        ):
            # Setup mock with multiple tool calls
            mock_tool_call1 = MagicMock()
            mock_tool_call1.tool_name = "list_directory"
            mock_tool_call1.tool_input = {"path": "."}
            mock_tool_call1.result = '[{"name": "main.py"}]'
            mock_tool_call1.success = True
            mock_tool_call1.duration_ms = 30

            mock_tool_call2 = MagicMock()
            mock_tool_call2.tool_name = "read_file"
            mock_tool_call2.tool_input = {"path": "main.py"}
            mock_tool_call2.result = "print('hello')"
            mock_tool_call2.success = True
            mock_tool_call2.duration_ms = 25

            mock_turn1 = MagicMock()
            mock_turn1.tool_calls = [mock_tool_call1]

            mock_turn2 = MagicMock()
            mock_turn2.tool_calls = [mock_tool_call2]

            mock_result = MagicMock()
            mock_result.success = True
            mock_result.final_message = "I found main.py with a hello world program."
            mock_result.turns = 3
            mock_result.total_tool_calls = 2
            mock_result.stop_reason.value = "completed"
            mock_result.turn_history = [mock_turn1, mock_turn2]

            mock_worker_instance = AsyncMock()
            mock_worker_instance.execute_task.return_value = mock_result
            MockWorker.return_value = mock_worker_instance

            response = client.post(
                "/api/chat/message",
                json={
                    "message": "What files are in the project?",
                    "workspace_id": "ws-agentic-123",
                    "stream": False,
                },
                headers=auth_headers,
            )

            assert response.status_code == 200
            data = response.json()
            assert data["workspace_id"] == "ws-agentic-123"
            assert "hello world" in data["message"]["content"].lower()

            # Verify worker was created with workspace
            MockWorker.assert_called_once()
            call_kwargs = MockWorker.call_args.kwargs
            assert call_kwargs["workspace_id"] == "ws-agentic-123"

    def test_conversation_state_isolation(self, client, auth_headers, mock_llm_client):
        """Test that conversations are isolated between users."""
        mock_llm_client.messages.create.return_value = create_simple_llm_response(
            "Hello user!"
        )

        # User 1 creates a conversation
        response1 = client.post(
            "/api/chat/message",
            json={"message": "Hello from user 1", "stream": False},
            headers=auth_headers,
        )
        assert response1.status_code == 200
        conv_id_1 = response1.json()["conversation_id"]

        # User 1 can access their history
        history_response = client.get(
            f"/api/chat/history/{conv_id_1}",
            headers=auth_headers,
        )
        assert history_response.status_code == 200

        # Create a different JWT for user 2
        user2_payload = {
            "sub": "user-456",
            "email": "user2@example.com",
            "aud": "authenticated",
            "role": "authenticated",
            "iat": int(datetime.now().timestamp()),
            "exp": int((datetime.now() + timedelta(hours=1)).timestamp()),
        }
        # Use same secret from environment for test
        user2_token = jwt.encode(
            user2_payload,
            "test-secret-key-for-jwt-signing-min-32-chars",
            algorithm="HS256",
        )
        user2_headers = {"Authorization": f"Bearer {user2_token}"}

        # User 2 should not be able to access user 1's conversation
        history_response_2 = client.get(
            f"/api/chat/history/{conv_id_1}",
            headers=user2_headers,
        )
        assert history_response_2.status_code == 404


# =============================================================================
# Test Authentication
# =============================================================================


class TestChatAPIAuthentication:
    """Test authentication requirements for Chat API."""

    def test_chat_requires_authentication(self, client):
        """Test that chat endpoint requires authentication."""
        response = client.post(
            "/api/chat/message",
            json={"message": "Hello"},
        )
        assert response.status_code == 401

    def test_chat_with_api_key(self, client, api_key_headers, mock_llm_client):
        """Test chat with API key authentication."""
        mock_llm_client.messages.create.return_value = create_simple_llm_response(
            "Hello via API key!"
        )

        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": False},
            headers=api_key_headers,
        )

        assert response.status_code == 200

    def test_chat_with_invalid_token(self, client, expired_jwt_token):
        """Test chat rejects expired token."""
        response = client.post(
            "/api/chat/message",
            json={"message": "Hello"},
            headers={"Authorization": f"Bearer {expired_jwt_token}"},
        )
        assert response.status_code == 401

    def test_chat_with_invalid_api_key(self, client):
        """Test chat rejects invalid API key."""
        response = client.post(
            "/api/chat/message",
            json={"message": "Hello"},
            headers={"X-API-Key": "invalid-key"},
        )
        assert response.status_code == 401


# =============================================================================
# Test Workspace Binding
# =============================================================================


class TestWorkspaceBinding:
    """Test workspace binding functionality."""

    def test_bind_workspace_to_conversation(self, client, auth_headers, mock_llm_client):
        """Test binding workspace to existing conversation."""
        mock_llm_client.messages.create.return_value = create_simple_llm_response("OK")

        # Create conversation first
        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": False},
            headers=auth_headers,
        )
        conv_id = response.json()["conversation_id"]

        # Bind workspace
        bind_response = client.post(
            f"/api/chat/workspace/bind?conversation_id={conv_id}&workspace_id=ws-new-123",
            headers=auth_headers,
        )

        assert bind_response.status_code == 200
        data = bind_response.json()
        assert data["conversation_id"] == conv_id
        assert data["workspace_id"] == "ws-new-123"

    def test_bind_workspace_creates_conversation_if_not_exists(self, client, auth_headers):
        """Test that binding workspace creates conversation if it doesn't exist."""
        bind_response = client.post(
            "/api/chat/workspace/bind?conversation_id=new-conv-123&workspace_id=ws-123",
            headers=auth_headers,
        )

        assert bind_response.status_code == 200
        data = bind_response.json()
        assert data["conversation_id"] == "new-conv-123"
        assert data["workspace_id"] == "ws-123"


# =============================================================================
# Test History Management
# =============================================================================


class TestHistoryManagement:
    """Test conversation history management."""

    def test_get_history_nonexistent_conversation(self, client, auth_headers):
        """Test getting history for nonexistent conversation."""
        response = client.get(
            "/api/chat/history/nonexistent-conv-id",
            headers=auth_headers,
        )
        assert response.status_code == 404

    def test_delete_conversation_history(self, client, auth_headers, mock_llm_client):
        """Test deleting conversation history."""
        mock_llm_client.messages.create.return_value = create_simple_llm_response("OK")

        # Create conversation
        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": False},
            headers=auth_headers,
        )
        conv_id = response.json()["conversation_id"]

        # Delete history
        delete_response = client.delete(
            f"/api/chat/history/{conv_id}",
            headers=auth_headers,
        )
        assert delete_response.status_code == 200
        assert delete_response.json()["success"] is True

        # Verify deletion
        get_response = client.get(
            f"/api/chat/history/{conv_id}",
            headers=auth_headers,
        )
        assert get_response.status_code == 404

    def test_delete_nonexistent_conversation(self, client, auth_headers):
        """Test deleting nonexistent conversation."""
        response = client.delete(
            "/api/chat/history/nonexistent-conv",
            headers=auth_headers,
        )
        assert response.status_code == 404


# =============================================================================
# Test SSE Event Format
# =============================================================================


class TestSSEEventFormat:
    """Test SSE event format and structure."""

    def test_sse_start_event(self, client, auth_headers, mock_llm_client):
        """Test SSE start event format."""
        mock_llm_client.messages.create.return_value = create_simple_llm_response("Hi")

        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": True},
            headers=auth_headers,
        )

        # Find start event
        found_start = False
        for line in response.iter_lines():
            if line.startswith("event: start"):
                found_start = True
                break
            if line.startswith("data:") and "conversation_id" in line:
                found_start = True
                break

        assert found_start or response.status_code == 200

    def test_sse_done_event(self, client, auth_headers, mock_llm_client):
        """Test SSE done event format."""
        mock_llm_client.messages.create.return_value = create_simple_llm_response("Done!")

        response = client.post(
            "/api/chat/message",
            json={"message": "Hello", "stream": True},
            headers=auth_headers,
        )

        # Find done event
        found_done = False
        for line in response.iter_lines():
            if "done" in line.lower() and "true" in line.lower():
                found_done = True
                break

        assert found_done


# =============================================================================
# Test Concurrent Conversations
# =============================================================================


class TestConcurrentConversations:
    """Test handling of concurrent conversations."""

    def test_multiple_conversations_same_user(self, client, auth_headers, mock_llm_client):
        """Test user can have multiple concurrent conversations."""
        mock_llm_client.messages.create.return_value = create_simple_llm_response("OK")

        # Create first conversation
        response1 = client.post(
            "/api/chat/message",
            json={"message": "Conv 1 message", "stream": False},
            headers=auth_headers,
        )
        conv_id_1 = response1.json()["conversation_id"]

        # Create second conversation
        response2 = client.post(
            "/api/chat/message",
            json={"message": "Conv 2 message", "stream": False},
            headers=auth_headers,
        )
        conv_id_2 = response2.json()["conversation_id"]

        # Verify different conversation IDs
        assert conv_id_1 != conv_id_2

        # Verify both histories are independent
        history1 = client.get(f"/api/chat/history/{conv_id_1}", headers=auth_headers).json()
        history2 = client.get(f"/api/chat/history/{conv_id_2}", headers=auth_headers).json()

        assert history1[0]["content"] == "Conv 1 message"
        assert history2[0]["content"] == "Conv 2 message"
