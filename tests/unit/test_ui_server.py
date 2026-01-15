"""Tests for C4 UI Server."""

import os
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from fastapi.testclient import TestClient

from c4.ui.server import (
    DEFAULT_HOST,
    DEFAULT_PORT,
    _get_default_html,
    create_ui_app,
)


class TestCreateUIApp:
    """Test create_ui_app function."""

    def test_create_app_default(self) -> None:
        """Test creating app with default settings."""
        app = create_ui_app()

        assert app.title == "C4 Dashboard"
        assert app.version == "0.1.0"
        assert app.state.project_root == Path.cwd()

    def test_create_app_with_project_root(self, tmp_path: Path) -> None:
        """Test creating app with custom project root."""
        app = create_ui_app(project_root=tmp_path)

        assert app.state.project_root == tmp_path

    def test_create_app_with_static_dir(self, tmp_path: Path) -> None:
        """Test creating app with static directory."""
        static_dir = tmp_path / "static"
        static_dir.mkdir()

        app = create_ui_app(static_dir=static_dir)

        # Check that static route is mounted
        routes = [route.path for route in app.routes]
        assert "/static" in routes or any("/static" in str(r) for r in app.routes)

    def test_create_app_with_custom_title(self) -> None:
        """Test creating app with custom title."""
        app = create_ui_app(title="My Custom Dashboard")

        assert app.title == "My Custom Dashboard"


class TestUIRoutes:
    """Test UI server routes."""

    @pytest.fixture
    def client(self) -> TestClient:
        """Create test client."""
        app = create_ui_app()
        return TestClient(app)

    def test_root_returns_html(self, client: TestClient) -> None:
        """Test root endpoint returns HTML."""
        response = client.get("/")

        assert response.status_code == 200
        assert "text/html" in response.headers["content-type"]
        assert "C4 Dashboard" in response.text

    def test_health_endpoint(self, client: TestClient) -> None:
        """Test health check endpoint."""
        response = client.get("/health")

        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "healthy"
        assert data["service"] == "c4-ui"


class TestAPIEndpoints:
    """Test API endpoints."""

    @pytest.fixture
    def mock_daemon(self):
        """Create mock daemon."""
        daemon = MagicMock()
        daemon.is_initialized.return_value = True
        daemon.c4_status.return_value = {
            "project_id": "test-project",
            "status": "EXECUTE",
            "queue": {
                "pending": 5,
                "in_progress": 1,
                "done": 10,
            },
        }

        # Mock task_queue
        daemon.task_queue.pending = []
        daemon.task_queue.in_progress = {}
        daemon.task_queue.done = []

        # Mock worker_manager
        daemon.worker_manager.get_all_workers.return_value = {}

        return daemon

    def test_api_status_not_initialized(self, tmp_path: Path) -> None:
        """Test status API when not initialized."""
        app = create_ui_app(project_root=tmp_path)
        client = TestClient(app)

        with patch("c4.mcp_server.C4Daemon") as mock_cls:
            mock_daemon = MagicMock()
            mock_daemon.is_initialized.return_value = False
            mock_cls.return_value = mock_daemon

            response = client.get("/api/status")

            assert response.status_code == 404
            assert "not initialized" in response.json()["error"]

    def test_api_status_success(
        self, tmp_path: Path, mock_daemon: MagicMock
    ) -> None:
        """Test status API success."""
        app = create_ui_app(project_root=tmp_path)
        client = TestClient(app)

        with patch("c4.mcp_server.C4Daemon") as mock_cls:
            mock_cls.return_value = mock_daemon

            response = client.get("/api/status")

            assert response.status_code == 200
            data = response.json()
            assert data["project_id"] == "test-project"
            assert data["status"] == "EXECUTE"

    def test_api_tasks_not_initialized(self, tmp_path: Path) -> None:
        """Test tasks API when not initialized."""
        app = create_ui_app(project_root=tmp_path)
        client = TestClient(app)

        with patch("c4.mcp_server.C4Daemon") as mock_cls:
            mock_daemon = MagicMock()
            mock_daemon.is_initialized.return_value = False
            mock_cls.return_value = mock_daemon

            response = client.get("/api/tasks")

            assert response.status_code == 404

    def test_api_tasks_success(
        self, tmp_path: Path, mock_daemon: MagicMock
    ) -> None:
        """Test tasks API success."""
        app = create_ui_app(project_root=tmp_path)
        client = TestClient(app)

        with patch("c4.mcp_server.C4Daemon") as mock_cls:
            mock_cls.return_value = mock_daemon

            response = client.get("/api/tasks")

            assert response.status_code == 200
            data = response.json()
            assert "pending" in data
            assert "in_progress" in data
            assert "done" in data

    def test_api_workers_not_initialized(self, tmp_path: Path) -> None:
        """Test workers API when not initialized."""
        app = create_ui_app(project_root=tmp_path)
        client = TestClient(app)

        with patch("c4.mcp_server.C4Daemon") as mock_cls:
            mock_daemon = MagicMock()
            mock_daemon.is_initialized.return_value = False
            mock_cls.return_value = mock_daemon

            response = client.get("/api/workers")

            assert response.status_code == 404

    def test_api_workers_success(
        self, tmp_path: Path, mock_daemon: MagicMock
    ) -> None:
        """Test workers API success."""
        app = create_ui_app(project_root=tmp_path)
        client = TestClient(app)

        with patch("c4.mcp_server.C4Daemon") as mock_cls:
            mock_cls.return_value = mock_daemon

            response = client.get("/api/workers")

            assert response.status_code == 200
            data = response.json()
            assert "workers" in data

    def test_api_error_handling(self, tmp_path: Path) -> None:
        """Test API error handling."""
        app = create_ui_app(project_root=tmp_path)
        client = TestClient(app)

        with patch("c4.mcp_server.C4Daemon") as mock_cls:
            mock_cls.side_effect = Exception("Test error")

            response = client.get("/api/status")

            assert response.status_code == 500
            assert "Test error" in response.json()["error"]


class TestDefaultHTML:
    """Test default HTML generation."""

    def test_html_contains_title(self) -> None:
        """Test HTML contains title."""
        html = _get_default_html()

        assert "C4 Dashboard" in html
        assert "<title>" in html

    def test_html_contains_status_section(self) -> None:
        """Test HTML contains status section."""
        html = _get_default_html()

        assert "Project Status" in html
        assert "Task Queue" in html

    def test_html_contains_task_sections(self) -> None:
        """Test HTML contains task sections."""
        html = _get_default_html()

        assert "Pending Tasks" in html
        assert "In Progress" in html
        assert "Completed Tasks" in html

    def test_html_contains_javascript(self) -> None:
        """Test HTML contains JavaScript."""
        html = _get_default_html()

        assert "<script>" in html
        assert "loadData" in html
        assert "/api/status" in html
        assert "/api/tasks" in html

    def test_html_contains_auto_refresh(self) -> None:
        """Test HTML contains auto-refresh."""
        html = _get_default_html()

        assert "setInterval" in html

    def test_html_is_valid_structure(self) -> None:
        """Test HTML has valid structure."""
        html = _get_default_html()

        assert html.startswith("<!DOCTYPE html>")
        assert "<html" in html
        assert "</html>" in html
        assert "<head>" in html
        assert "</head>" in html
        assert "<body>" in html
        assert "</body>" in html


class TestConstants:
    """Test server constants."""

    def test_default_port(self) -> None:
        """Test default port value."""
        assert DEFAULT_PORT == 4000

    def test_default_host(self) -> None:
        """Test default host value."""
        assert DEFAULT_HOST == "127.0.0.1"


class TestStaticFiles:
    """Test static file serving."""

    def test_static_dir_mounted(self, tmp_path: Path) -> None:
        """Test static directory is mounted when provided."""
        static_dir = tmp_path / "static"
        static_dir.mkdir()

        # Create a test file
        test_file = static_dir / "test.txt"
        test_file.write_text("Hello, World!")

        app = create_ui_app(static_dir=static_dir)
        client = TestClient(app)

        response = client.get("/static/test.txt")

        assert response.status_code == 200
        assert response.text == "Hello, World!"

    def test_static_dir_not_mounted_when_missing(self, tmp_path: Path) -> None:
        """Test static directory is not mounted when it doesn't exist."""
        static_dir = tmp_path / "nonexistent"

        app = create_ui_app(static_dir=static_dir)
        client = TestClient(app)

        response = client.get("/static/test.txt")

        # Should return 404 since static route is not mounted
        assert response.status_code == 404

    def test_static_serves_html_files(self, tmp_path: Path) -> None:
        """Test static directory serves HTML files."""
        static_dir = tmp_path / "static"
        static_dir.mkdir()

        # Create an HTML file
        html_file = static_dir / "index.html"
        html_file.write_text("<html><body>Custom UI</body></html>")

        app = create_ui_app(static_dir=static_dir)
        client = TestClient(app)

        response = client.get("/static/index.html")

        assert response.status_code == 200
        assert "Custom UI" in response.text

    def test_static_serves_css_files(self, tmp_path: Path) -> None:
        """Test static directory serves CSS files."""
        static_dir = tmp_path / "static"
        static_dir.mkdir()

        # Create a CSS file
        css_file = static_dir / "style.css"
        css_file.write_text("body { color: red; }")

        app = create_ui_app(static_dir=static_dir)
        client = TestClient(app)

        response = client.get("/static/style.css")

        assert response.status_code == 200
        assert "color: red" in response.text

    def test_static_serves_js_files(self, tmp_path: Path) -> None:
        """Test static directory serves JavaScript files."""
        static_dir = tmp_path / "static"
        static_dir.mkdir()

        # Create a JS file
        js_file = static_dir / "app.js"
        js_file.write_text("console.log('Hello');")

        app = create_ui_app(static_dir=static_dir)
        client = TestClient(app)

        response = client.get("/static/app.js")

        assert response.status_code == 200
        assert "console.log" in response.text


class TestEnvironmentHandling:
    """Test environment variable handling."""

    def test_env_preserved_after_status_call(
        self, tmp_path: Path
    ) -> None:
        """Test C4_PROJECT_ROOT is preserved after API call."""
        original_value = "original_path"
        os.environ["C4_PROJECT_ROOT"] = original_value

        app = create_ui_app(project_root=tmp_path)
        client = TestClient(app)

        with patch("c4.mcp_server.C4Daemon") as mock_cls:
            mock_daemon = MagicMock()
            mock_daemon.is_initialized.return_value = False
            mock_cls.return_value = mock_daemon

            client.get("/api/status")

        assert os.environ.get("C4_PROJECT_ROOT") == original_value

        # Cleanup
        del os.environ["C4_PROJECT_ROOT"]

    def test_env_cleared_after_status_call_if_not_set(
        self, tmp_path: Path
    ) -> None:
        """Test C4_PROJECT_ROOT is cleared after API call if it wasn't set."""
        # Ensure env var is not set
        if "C4_PROJECT_ROOT" in os.environ:
            del os.environ["C4_PROJECT_ROOT"]

        app = create_ui_app(project_root=tmp_path)
        client = TestClient(app)

        with patch("c4.mcp_server.C4Daemon") as mock_cls:
            mock_daemon = MagicMock()
            mock_daemon.is_initialized.return_value = False
            mock_cls.return_value = mock_daemon

            client.get("/api/status")

        assert "C4_PROJECT_ROOT" not in os.environ
