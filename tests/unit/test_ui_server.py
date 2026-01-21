"""Tests for C4 UI Server."""

from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from c4.ui.server import UIServer


@pytest.fixture
def ui_server() -> UIServer:
    """Create UI server instance."""
    return UIServer()


@pytest.fixture
def client(ui_server: UIServer) -> TestClient:
    """Create test client."""
    return TestClient(ui_server.app)


class TestUIServer:
    """Test UIServer class."""

    def test_default_config(self) -> None:
        """Test default configuration."""
        server = UIServer()

        assert server.port == 4000
        assert server.host == "localhost"

    def test_custom_config(self) -> None:
        """Test custom configuration."""
        server = UIServer(port=8080, host="0.0.0.0")

        assert server.port == 8080
        assert server.host == "0.0.0.0"

    def test_app_created(self, ui_server: UIServer) -> None:
        """Test FastAPI app is created."""
        assert ui_server.app is not None
        assert ui_server.app.title == "C4 UI"


class TestUIEndpoints:
    """Test UI server endpoints."""

    def test_index_page(self, client: TestClient) -> None:
        """Test index page returns HTML."""
        response = client.get("/")

        assert response.status_code == 200
        assert "text/html" in response.headers["content-type"]
        assert "C4 Dashboard" in response.text

    def test_embedded_ui_content(self, client: TestClient) -> None:
        """Test embedded UI has required elements."""
        response = client.get("/")
        html = response.text

        # Check for essential UI elements
        assert "<title>C4 Dashboard</title>" in html
        assert "messages" in html  # Chat messages container
        assert "/api/chat/message" in html  # API endpoint
        assert "/api/status" in html  # Status endpoint

    def test_api_status_endpoint(self, client: TestClient) -> None:
        """Test API status endpoint."""
        response = client.get("/api/status")

        assert response.status_code == 200
        data = response.json()
        # Should return initialized status or error
        assert "initialized" in data or "error" in data

    def test_chat_api_available(self, client: TestClient, monkeypatch) -> None:
        """Test chat API is mounted and requires auth."""
        # Test without auth - should return 401
        response = client.post(
            "/api/chat/message",
            json={"message": "test", "stream": False},
        )
        assert response.status_code == 401

        # Test with API key auth
        monkeypatch.setenv("C4_API_KEYS", "test-api-key")
        # Clear the auth config cache to pick up new env
        from c4.api.auth import clear_auth_config_cache

        clear_auth_config_cache()

        response = client.post(
            "/api/chat/message",
            json={"message": "test", "stream": False},
            headers={"X-API-Key": "test-api-key"},
        )
        # Should pass auth (may fail with LLM error but that's OK)
        assert response.status_code in (200, 500)  # 500 if anthropic not configured

    def test_spa_routing(self, client: TestClient) -> None:
        """Test SPA catch-all routing."""
        # Random paths should return index
        response = client.get("/some/random/path")

        assert response.status_code == 200
        assert "C4 Dashboard" in response.text


class TestEmbeddedUI:
    """Test embedded UI functionality."""

    def test_ui_has_chat_input(self, client: TestClient) -> None:
        """Test UI has chat input field."""
        response = client.get("/")

        assert 'id="input"' in response.text
        assert 'type="text"' in response.text

    def test_ui_has_send_button(self, client: TestClient) -> None:
        """Test UI has send button."""
        response = client.get("/")

        assert 'id="send"' in response.text
        assert "sendMessage()" in response.text

    def test_ui_has_status_indicator(self, client: TestClient) -> None:
        """Test UI has status indicator."""
        response = client.get("/")

        assert "status-dot" in response.text
        assert "status-text" in response.text

    def test_ui_has_styles(self, client: TestClient) -> None:
        """Test UI has CSS styles."""
        response = client.get("/")

        assert "<style>" in response.text
        assert "font-family" in response.text
        assert "background" in response.text


class TestStaticFileServing:
    """Test static file serving."""

    def test_static_directory_config(self, tmp_path: Path) -> None:
        """Test custom static directory."""
        static_dir = tmp_path / "static"
        static_dir.mkdir()

        server = UIServer(static_dir=static_dir)
        assert server.static_dir == static_dir

    def test_serve_custom_index(self, tmp_path: Path) -> None:
        """Test serving custom index.html."""
        static_dir = tmp_path / "static"
        static_dir.mkdir()
        (static_dir / "index.html").write_text("<h1>Custom UI</h1>")

        server = UIServer(static_dir=static_dir)
        client = TestClient(server.app)

        response = client.get("/")
        assert "<h1>Custom UI</h1>" in response.text
