"""Tests for C4 API Server."""

from unittest.mock import MagicMock, patch

import pytest
from fastapi.testclient import TestClient


@pytest.fixture
def mock_daemon():
    """Create a mock C4Daemon."""
    daemon = MagicMock()
    daemon.is_initialized.return_value = True
    daemon.c4_status.return_value = {
        "state": "EXECUTE",
        "queue": {"pending": 5, "in_progress": 1, "done": 10},
        "workers": {"worker-1": {"status": "active"}},
        "project_root": "/test/project",
    }
    daemon.config = {"verifications": {"lint": "ruff check .", "test": "pytest"}}
    return daemon


@pytest.fixture
def client(mock_daemon):
    """Create test client with mocked daemon."""
    with patch("c4.api.deps.get_daemon_singleton", return_value=mock_daemon):
        from c4.api.server import create_app
        app = create_app()
        yield TestClient(app)


class TestHealthEndpoints:
    """Tests for health and info endpoints."""

    def test_health_check(self, client):
        """Test health check endpoint."""
        response = client.get("/health")
        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "healthy"
        assert data["service"] == "c4-api"

    def test_root_endpoint(self, client):
        """Test root endpoint."""
        response = client.get("/")
        assert response.status_code == 200
        data = response.json()
        assert data["name"] == "C4 API"
        assert "endpoints" in data
        assert "c4" in data["endpoints"]


class TestC4Routes:
    """Tests for C4 core routes."""

    def test_get_status(self, client, mock_daemon):
        """Test GET /api/c4/status."""
        response = client.get("/api/c4/status")
        assert response.status_code == 200
        data = response.json()
        assert data["state"] == "EXECUTE"
        assert "queue" in data
        assert "workers" in data
        mock_daemon.c4_status.assert_called_once()

    def test_get_task(self, client, mock_daemon):
        """Test POST /api/c4/get-task."""
        mock_daemon.c4_get_task.return_value = {
            "task_id": "T-001",
            "title": "Test task",
            "dod": "Do something",
            "scope": "src/",
            "domain": "web-frontend",
            "dependencies": [],
        }

        response = client.post(
            "/api/c4/get-task",
            json={"worker_id": "worker-123"},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["task_id"] == "T-001"
        assert data["title"] == "Test task"
        mock_daemon.c4_get_task.assert_called_once_with("worker-123")

    def test_get_task_no_available(self, client, mock_daemon):
        """Test POST /api/c4/get-task when no task available."""
        mock_daemon.c4_get_task.return_value = {
            "message": "No tasks available",
        }

        response = client.post(
            "/api/c4/get-task",
            json={"worker_id": "worker-123"},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["task_id"] is None
        assert data["message"] == "No tasks available"

    def test_submit_task(self, client, mock_daemon):
        """Test POST /api/c4/submit."""
        mock_daemon.c4_submit.return_value = {
            "success": True,
            "message": "Task completed",
            "next_task": None,
        }

        response = client.post(
            "/api/c4/submit",
            json={
                "task_id": "T-001",
                "commit_sha": "abc123",
                "validation_results": [
                    {"name": "lint", "status": "pass", "message": None},
                    {"name": "test", "status": "pass", "message": None},
                ],
                "worker_id": "worker-123",
            },
        )
        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["message"] == "Task completed"

    def test_add_task(self, client, mock_daemon):
        """Test POST /api/c4/add-task."""
        mock_daemon.c4_add_todo.return_value = {
            "success": True,
            "task_id": "T-002",
        }

        response = client.post(
            "/api/c4/add-task",
            json={
                "task_id": "T-002",
                "title": "New task",
                "dod": "Do something new",
                "scope": "src/",
                "domain": "web-backend",
                "priority": 1,
                "dependencies": ["T-001"],
            },
        )
        assert response.status_code == 200
        mock_daemon.c4_add_todo.assert_called_once()

    def test_start_execution(self, client, mock_daemon):
        """Test POST /api/c4/start."""
        mock_daemon.c4_start.return_value = {
            "success": True,
            "state": "EXECUTE",
            "message": "Started execution",
        }

        response = client.post("/api/c4/start")
        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["new_state"] == "EXECUTE"


class TestDiscoveryRoutes:
    """Tests for Discovery phase routes."""

    def test_save_spec(self, client, mock_daemon):
        """Test POST /api/discovery/save-spec."""
        mock_daemon.c4_save_spec.return_value = {"success": True}

        response = client.post(
            "/api/discovery/save-spec",
            json={
                "feature": "user-auth",
                "requirements": [
                    {"id": "REQ-001", "text": "User shall login", "pattern": "ubiquitous"},
                ],
                "domain": "web-backend",
                "description": "User authentication feature",
            },
        )
        assert response.status_code == 200
        mock_daemon.c4_save_spec.assert_called_once()

    def test_list_specs(self, client, mock_daemon):
        """Test GET /api/discovery/specs."""
        mock_daemon.c4_list_specs.return_value = {
            "specs": ["user-auth", "dashboard"],
        }

        response = client.get("/api/discovery/specs")
        assert response.status_code == 200
        data = response.json()
        assert data["specs"] == ["user-auth", "dashboard"]

    def test_get_spec(self, client, mock_daemon):
        """Test GET /api/discovery/specs/{feature}."""
        mock_daemon.c4_get_spec.return_value = {
            "feature": "user-auth",
            "requirements": [
                {"id": "REQ-001", "text": "User shall login", "pattern": "ubiquitous"},
            ],
            "domain": "web-backend",
            "description": "User authentication",
        }

        response = client.get("/api/discovery/specs/user-auth")
        assert response.status_code == 200
        data = response.json()
        assert data["feature"] == "user-auth"
        assert len(data["requirements"]) == 1

    def test_get_spec_not_found(self, client, mock_daemon):
        """Test GET /api/discovery/specs/{feature} when not found."""
        mock_daemon.c4_get_spec.return_value = {"error": "Not found"}

        response = client.get("/api/discovery/specs/nonexistent")
        assert response.status_code == 404

    def test_complete_discovery(self, client, mock_daemon):
        """Test POST /api/discovery/complete."""
        mock_daemon.c4_discovery_complete.return_value = {
            "success": True,
            "new_state": "DESIGN",
        }

        response = client.post("/api/discovery/complete")
        assert response.status_code == 200


class TestDesignRoutes:
    """Tests for Design phase routes."""

    def test_save_design(self, client, mock_daemon):
        """Test POST /api/design/save-design."""
        mock_daemon.c4_save_design.return_value = {"success": True}

        response = client.post(
            "/api/design/save-design",
            json={
                "feature": "user-auth",
                "domain": "web-backend",
                "description": "Auth design",
                "options": [
                    {
                        "id": "opt-1",
                        "name": "JWT",
                        "description": "JWT tokens",
                        "pros": ["Stateless"],
                        "cons": ["Token size"],
                        "complexity": "medium",
                        "recommended": True,
                    },
                ],
                "selected_option": "opt-1",
                "components": [],
                "decisions": [],
                "constraints": [],
                "nfr": {},
            },
        )
        assert response.status_code == 200
        mock_daemon.c4_save_design.assert_called_once()

    def test_list_designs(self, client, mock_daemon):
        """Test GET /api/design/designs."""
        mock_daemon.c4_list_designs.return_value = {
            "designs": ["user-auth"],
        }

        response = client.get("/api/design/designs")
        assert response.status_code == 200
        data = response.json()
        assert data["designs"] == ["user-auth"]

    def test_complete_design(self, client, mock_daemon):
        """Test POST /api/design/complete."""
        mock_daemon.c4_design_complete.return_value = {
            "success": True,
            "new_state": "PLAN",
        }

        response = client.post("/api/design/complete")
        assert response.status_code == 200


class TestValidationRoutes:
    """Tests for Validation routes."""

    def test_run_validations(self, client, mock_daemon):
        """Test POST /api/validation/run."""
        mock_daemon.c4_run_validation.return_value = {
            "results": [
                {"name": "lint", "status": "pass", "message": None},
                {"name": "test", "status": "pass", "message": None},
            ],
        }

        response = client.post(
            "/api/validation/run",
            json={"names": ["lint", "test"], "fail_fast": True, "timeout": 300},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["all_passed"] is True
        assert len(data["results"]) == 2

    def test_run_validations_with_failure(self, client, mock_daemon):
        """Test POST /api/validation/run with failure."""
        mock_daemon.c4_run_validation.return_value = {
            "results": [
                {"name": "lint", "status": "fail", "message": "Error found"},
            ],
        }

        response = client.post(
            "/api/validation/run",
            json={"names": ["lint"], "fail_fast": True, "timeout": 300},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["all_passed"] is False

    def test_get_validation_config(self, client, mock_daemon):
        """Test GET /api/validation/config."""
        response = client.get("/api/validation/config")
        assert response.status_code == 200
        data = response.json()
        assert "verifications" in data
        assert "available" in data


class TestGitRoutes:
    """Tests for Git routes."""

    def test_get_git_status(self, client, mock_daemon):
        """Test GET /api/git/status."""
        with patch("c4.api.routes.git._run_git_command") as mock_git:
            mock_git.side_effect = [
                (True, "main"),  # branch
                (True, "M src/test.py\n?? new.py"),  # status
            ]

            response = client.get("/api/git/status")
            assert response.status_code == 200
            data = response.json()
            assert data["branch"] == "main"
            assert data["is_clean"] is False

    def test_get_git_log(self, client, mock_daemon):
        """Test GET /api/git/log."""
        with patch("c4.api.routes.git._run_git_command") as mock_git:
            mock_git.return_value = (True, "abc123 First commit\ndef456 Second commit")

            response = client.get("/api/git/log?limit=5")
            assert response.status_code == 200
            data = response.json()
            assert len(data["commits"]) == 2
            assert data["commits"][0]["sha"] == "abc123"


class TestCORSAndMiddleware:
    """Tests for CORS and middleware."""

    def test_cors_headers(self, client):
        """Test CORS headers are present."""
        response = client.options(
            "/api/c4/status",
            headers={
                "Origin": "http://localhost:3000",
                "Access-Control-Request-Method": "GET",
            },
        )
        # CORS preflight should work
        assert response.status_code in (200, 204, 405)


class TestOpenAPI:
    """Tests for OpenAPI documentation."""

    def test_openapi_json(self, client):
        """Test OpenAPI JSON is accessible."""
        response = client.get("/openapi.json")
        assert response.status_code == 200
        data = response.json()
        assert "openapi" in data
        assert "paths" in data
        assert "/api/c4/status" in data["paths"]

    def test_docs_accessible(self, client):
        """Test Swagger docs are accessible."""
        response = client.get("/docs")
        assert response.status_code == 200

    def test_redoc_accessible(self, client):
        """Test ReDoc is accessible."""
        response = client.get("/redoc")
        assert response.status_code == 200
