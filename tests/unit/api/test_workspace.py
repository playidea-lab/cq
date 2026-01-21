"""Tests for Workspace API Routes.

TDD RED Phase: Define tests for workspace management API endpoints.

Endpoints:
- POST /api/workspace/create - Create workspace from git repo
- GET /api/workspace/list - List user's workspaces
- GET /api/workspace/{workspace_id} - Get workspace details
- DELETE /api/workspace/{workspace_id} - Delete workspace
- GET /api/workspace/{workspace_id}/status - Get workspace status/resources
- POST /api/workspace/{workspace_id}/exec - Execute command in workspace

Security:
- All endpoints require authentication
- Users can only access their own workspaces
"""

from datetime import datetime, timedelta, timezone
from unittest.mock import AsyncMock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from c4.workspace import (
    ExecResult,
    Workspace,
    WorkspaceManager,
    WorkspaceStats,
    WorkspaceStatus,
)

# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture
def mock_workspace_manager():
    """Create a mock workspace manager."""
    manager = AsyncMock(spec=WorkspaceManager)
    return manager


@pytest.fixture
def sample_workspace():
    """Create a sample workspace for testing."""
    return Workspace(
        id="ws-test123",
        user_id="user-123",
        git_url="https://github.com/user/repo",
        branch="main",
        status=WorkspaceStatus.READY,
        created_at=datetime.now(timezone.utc),
        container_id="container-abc",
    )


@pytest.fixture
def sample_workspace_other_user():
    """Create a workspace owned by another user."""
    return Workspace(
        id="ws-other456",
        user_id="user-other",
        git_url="https://github.com/other/repo",
        branch="main",
        status=WorkspaceStatus.READY,
        created_at=datetime.now(timezone.utc),
    )


@pytest.fixture
def auth_config():
    """Test auth configuration."""
    from c4.api.auth import AuthConfig

    config = AuthConfig()
    config.jwt_secret = "test-jwt-secret-key-for-workspace-tests"
    config.api_keys = ["test-api-key-workspace"]
    return config


@pytest.fixture
def valid_token(auth_config):
    """Create a valid JWT token for user-123."""
    import jwt

    now = datetime.now(timezone.utc)
    payload = {
        "sub": "user-123",
        "email": "test@example.com",
        "aud": "authenticated",
        "role": "authenticated",
        "iat": int(now.timestamp()),
        "exp": int((now + timedelta(hours=1)).timestamp()),
    }
    return jwt.encode(payload, auth_config.jwt_secret, algorithm="HS256")


@pytest.fixture
def other_user_token(auth_config):
    """Create a valid JWT token for a different user."""
    import jwt

    now = datetime.now(timezone.utc)
    payload = {
        "sub": "user-other",
        "email": "other@example.com",
        "aud": "authenticated",
        "role": "authenticated",
        "iat": int(now.timestamp()),
        "exp": int((now + timedelta(hours=1)).timestamp()),
    }
    return jwt.encode(payload, auth_config.jwt_secret, algorithm="HS256")


@pytest.fixture
def app_with_workspace(auth_config, mock_workspace_manager):
    """Create FastAPI app with workspace routes."""
    from c4.api.auth import get_auth_config
    from c4.api.routes.workspace import get_workspace_manager, router

    app = FastAPI()
    app.include_router(router, prefix="/api")

    # Override dependencies
    app.dependency_overrides[get_auth_config] = lambda: auth_config
    app.dependency_overrides[get_workspace_manager] = lambda: mock_workspace_manager

    yield app

    app.dependency_overrides.clear()


@pytest.fixture
def client(app_with_workspace):
    """Create test client."""
    return TestClient(app_with_workspace)


# ============================================================================
# Authentication Tests
# ============================================================================


class TestWorkspaceAuthentication:
    """Tests for workspace API authentication requirements."""

    def test_create_requires_auth(self, client):
        """Test that POST /create requires authentication."""
        response = client.post(
            "/api/workspace/create",
            json={"git_url": "https://github.com/user/repo"},
        )
        assert response.status_code == 401

    def test_list_requires_auth(self, client):
        """Test that GET /list requires authentication."""
        response = client.get("/api/workspace/list")
        assert response.status_code == 401

    def test_get_requires_auth(self, client):
        """Test that GET /{workspace_id} requires authentication."""
        response = client.get("/api/workspace/ws-123")
        assert response.status_code == 401

    def test_delete_requires_auth(self, client):
        """Test that DELETE /{workspace_id} requires authentication."""
        response = client.delete("/api/workspace/ws-123")
        assert response.status_code == 401

    def test_status_requires_auth(self, client):
        """Test that GET /{workspace_id}/status requires authentication."""
        response = client.get("/api/workspace/ws-123/status")
        assert response.status_code == 401

    def test_exec_requires_auth(self, client):
        """Test that POST /{workspace_id}/exec requires authentication."""
        response = client.post(
            "/api/workspace/ws-123/exec",
            json={"command": "ls"},
        )
        assert response.status_code == 401


# ============================================================================
# Create Workspace Tests
# ============================================================================


class TestCreateWorkspace:
    """Tests for POST /api/workspace/create endpoint."""

    def test_create_workspace_success(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test successful workspace creation."""
        mock_workspace_manager.create.return_value = sample_workspace

        response = client.post(
            "/api/workspace/create",
            json={
                "git_url": "https://github.com/user/repo",
                "branch": "main",
            },
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 201
        data = response.json()
        assert data["id"] == "ws-test123"
        assert data["user_id"] == "user-123"
        assert data["git_url"] == "https://github.com/user/repo"
        assert data["branch"] == "main"
        assert data["status"] == "ready"

        # Verify manager was called with correct args
        mock_workspace_manager.create.assert_called_once_with(
            user_id="user-123",
            git_url="https://github.com/user/repo",
            branch="main",
        )

    def test_create_workspace_default_branch(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test workspace creation with default branch."""
        mock_workspace_manager.create.return_value = sample_workspace

        response = client.post(
            "/api/workspace/create",
            json={"git_url": "https://github.com/user/repo"},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 201
        mock_workspace_manager.create.assert_called_once_with(
            user_id="user-123",
            git_url="https://github.com/user/repo",
            branch="main",
        )

    def test_create_workspace_missing_git_url(self, client, valid_token):
        """Test workspace creation fails without git_url."""
        response = client.post(
            "/api/workspace/create",
            json={},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 422

    def test_create_workspace_creation_error(
        self, client, valid_token, mock_workspace_manager
    ):
        """Test workspace creation handles errors."""
        from c4.workspace import WorkspaceCreationError

        mock_workspace_manager.create.side_effect = WorkspaceCreationError(
            "Git clone failed"
        )

        response = client.post(
            "/api/workspace/create",
            json={"git_url": "https://github.com/invalid/repo"},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 400
        data = response.json()
        assert "Git clone failed" in data["detail"]

    def test_create_workspace_with_api_key(
        self, client, mock_workspace_manager, sample_workspace
    ):
        """Test workspace creation with API key auth."""
        # API key user gets a fixed user_id
        ws = Workspace(
            id="ws-api123",
            user_id="api-key",
            git_url="https://github.com/user/repo",
            branch="main",
            status=WorkspaceStatus.READY,
            created_at=datetime.now(timezone.utc),
        )
        mock_workspace_manager.create.return_value = ws

        response = client.post(
            "/api/workspace/create",
            json={"git_url": "https://github.com/user/repo"},
            headers={"X-API-Key": "test-api-key-workspace"},
        )

        assert response.status_code == 201
        mock_workspace_manager.create.assert_called_once()


# ============================================================================
# List Workspaces Tests
# ============================================================================


class TestListWorkspaces:
    """Tests for GET /api/workspace/list endpoint."""

    def test_list_workspaces_success(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test listing user's workspaces."""
        mock_workspace_manager.list_by_user.return_value = [sample_workspace]

        response = client.get(
            "/api/workspace/list",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 1
        assert len(data["workspaces"]) == 1
        assert data["workspaces"][0]["id"] == "ws-test123"

        mock_workspace_manager.list_by_user.assert_called_once_with("user-123")

    def test_list_workspaces_empty(self, client, valid_token, mock_workspace_manager):
        """Test listing workspaces when user has none."""
        mock_workspace_manager.list_by_user.return_value = []

        response = client.get(
            "/api/workspace/list",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 0
        assert data["workspaces"] == []

    def test_list_workspaces_multiple(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test listing multiple workspaces."""
        ws2 = Workspace(
            id="ws-test456",
            user_id="user-123",
            git_url="https://github.com/user/repo2",
            branch="develop",
            status=WorkspaceStatus.CREATING,
            created_at=datetime.now(timezone.utc),
        )
        mock_workspace_manager.list_by_user.return_value = [sample_workspace, ws2]

        response = client.get(
            "/api/workspace/list",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 2
        assert len(data["workspaces"]) == 2


# ============================================================================
# Get Workspace Tests
# ============================================================================


class TestGetWorkspace:
    """Tests for GET /api/workspace/{workspace_id} endpoint."""

    def test_get_workspace_success(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test getting workspace details."""
        mock_workspace_manager.get.return_value = sample_workspace

        response = client.get(
            "/api/workspace/ws-test123",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["id"] == "ws-test123"
        assert data["user_id"] == "user-123"
        assert data["git_url"] == "https://github.com/user/repo"
        assert data["status"] == "ready"
        assert data["container_id"] == "container-abc"

    def test_get_workspace_not_found(
        self, client, valid_token, mock_workspace_manager
    ):
        """Test getting non-existent workspace returns 404."""
        mock_workspace_manager.get.return_value = None

        response = client.get(
            "/api/workspace/ws-nonexistent",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 404
        data = response.json()
        assert "not found" in data["detail"].lower()

    def test_get_workspace_unauthorized(
        self,
        client,
        valid_token,
        mock_workspace_manager,
        sample_workspace_other_user,
    ):
        """Test getting another user's workspace returns 403."""
        mock_workspace_manager.get.return_value = sample_workspace_other_user

        response = client.get(
            "/api/workspace/ws-other456",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 403
        data = response.json()
        assert "not authorized" in data["detail"].lower()


# ============================================================================
# Delete Workspace Tests
# ============================================================================


class TestDeleteWorkspace:
    """Tests for DELETE /api/workspace/{workspace_id} endpoint."""

    def test_delete_workspace_success(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test successful workspace deletion."""
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.destroy.return_value = True

        response = client.delete(
            "/api/workspace/ws-test123",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert "deleted" in data["message"].lower()

        mock_workspace_manager.destroy.assert_called_once_with("ws-test123")

    def test_delete_workspace_not_found(
        self, client, valid_token, mock_workspace_manager
    ):
        """Test deleting non-existent workspace returns 404."""
        mock_workspace_manager.get.return_value = None

        response = client.delete(
            "/api/workspace/ws-nonexistent",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 404

    def test_delete_workspace_unauthorized(
        self,
        client,
        valid_token,
        mock_workspace_manager,
        sample_workspace_other_user,
    ):
        """Test deleting another user's workspace returns 403."""
        mock_workspace_manager.get.return_value = sample_workspace_other_user

        response = client.delete(
            "/api/workspace/ws-other456",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 403


# ============================================================================
# Get Workspace Status Tests
# ============================================================================


class TestGetWorkspaceStatus:
    """Tests for GET /api/workspace/{workspace_id}/status endpoint."""

    def test_get_status_success(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test getting workspace status with resource usage."""
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.get_stats.return_value = WorkspaceStats(
            cpu_percent=25.5,
            memory_mb=512.0,
            disk_mb=1024.0,
        )
        mock_workspace_manager.health_check.return_value = True

        response = client.get(
            "/api/workspace/ws-test123/status",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["id"] == "ws-test123"
        assert data["status"] == "ready"
        assert data["cpu_percent"] == 25.5
        assert data["memory_mb"] == 512.0
        assert data["disk_mb"] == 1024.0
        assert data["is_healthy"] is True

    def test_get_status_no_stats(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test getting workspace status when stats unavailable."""
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.get_stats.return_value = None
        mock_workspace_manager.health_check.return_value = True

        response = client.get(
            "/api/workspace/ws-test123/status",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["cpu_percent"] is None
        assert data["memory_mb"] is None
        assert data["disk_mb"] is None

    def test_get_status_not_found(
        self, client, valid_token, mock_workspace_manager
    ):
        """Test getting status of non-existent workspace."""
        mock_workspace_manager.get.return_value = None

        response = client.get(
            "/api/workspace/ws-nonexistent/status",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 404

    def test_get_status_unauthorized(
        self,
        client,
        valid_token,
        mock_workspace_manager,
        sample_workspace_other_user,
    ):
        """Test getting status of another user's workspace."""
        mock_workspace_manager.get.return_value = sample_workspace_other_user

        response = client.get(
            "/api/workspace/ws-other456/status",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 403

    def test_get_status_unhealthy(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test getting status when workspace is unhealthy."""
        sample_workspace.status = WorkspaceStatus.ERROR
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.get_stats.return_value = None
        mock_workspace_manager.health_check.return_value = False

        response = client.get(
            "/api/workspace/ws-test123/status",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "error"
        assert data["is_healthy"] is False


# ============================================================================
# Execute Command Tests
# ============================================================================


class TestExecInWorkspace:
    """Tests for POST /api/workspace/{workspace_id}/exec endpoint."""

    def test_exec_success(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test successful command execution."""
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.exec.return_value = ExecResult(
            exit_code=0,
            stdout="hello world\n",
            stderr="",
            timed_out=False,
            duration_seconds=0.5,
        )

        response = client.post(
            "/api/workspace/ws-test123/exec",
            json={"command": "echo hello world"},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["exit_code"] == 0
        assert data["stdout"] == "hello world\n"
        assert data["stderr"] == ""
        assert data["timed_out"] is False
        assert data["duration_seconds"] == 0.5

        mock_workspace_manager.exec.assert_called_once_with(
            "ws-test123", "echo hello world", 60
        )

    def test_exec_with_timeout(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test command execution with custom timeout."""
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.exec.return_value = ExecResult(
            exit_code=0,
            stdout="done\n",
            stderr="",
            timed_out=False,
            duration_seconds=1.0,
        )

        response = client.post(
            "/api/workspace/ws-test123/exec",
            json={"command": "sleep 1 && echo done", "timeout": 120},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        mock_workspace_manager.exec.assert_called_once_with(
            "ws-test123", "sleep 1 && echo done", 120
        )

    def test_exec_timeout_exceeded(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test command execution that times out."""
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.exec.return_value = ExecResult(
            exit_code=-1,
            stdout="",
            stderr="Command timed out",
            timed_out=True,
            duration_seconds=60.0,
        )

        response = client.post(
            "/api/workspace/ws-test123/exec",
            json={"command": "sleep 100", "timeout": 60},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["timed_out"] is True
        assert data["exit_code"] == -1

    def test_exec_command_failure(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test command execution with non-zero exit code."""
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.exec.return_value = ExecResult(
            exit_code=1,
            stdout="",
            stderr="command not found\n",
            timed_out=False,
            duration_seconds=0.1,
        )

        response = client.post(
            "/api/workspace/ws-test123/exec",
            json={"command": "nonexistent_command"},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["exit_code"] == 1
        assert "not found" in data["stderr"].lower()

    def test_exec_not_found(
        self, client, valid_token, mock_workspace_manager
    ):
        """Test exec on non-existent workspace."""
        mock_workspace_manager.get.return_value = None

        response = client.post(
            "/api/workspace/ws-nonexistent/exec",
            json={"command": "ls"},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 404

    def test_exec_unauthorized(
        self,
        client,
        valid_token,
        mock_workspace_manager,
        sample_workspace_other_user,
    ):
        """Test exec on another user's workspace."""
        mock_workspace_manager.get.return_value = sample_workspace_other_user

        response = client.post(
            "/api/workspace/ws-other456/exec",
            json={"command": "ls"},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 403

    def test_exec_workspace_not_ready(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test exec on workspace that is not ready."""
        from c4.workspace import WorkspaceNotReadyError

        sample_workspace.status = WorkspaceStatus.CREATING
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.exec.side_effect = WorkspaceNotReadyError(
            "ws-test123", "creating"
        )

        response = client.post(
            "/api/workspace/ws-test123/exec",
            json={"command": "ls"},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 409
        data = response.json()
        assert "not ready" in data["detail"].lower()

    def test_exec_missing_command(self, client, valid_token):
        """Test exec with missing command field."""
        response = client.post(
            "/api/workspace/ws-test123/exec",
            json={},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 422

    def test_exec_timeout_validation(self, client, valid_token):
        """Test exec rejects invalid timeout values."""
        # Too low
        response = client.post(
            "/api/workspace/ws-test123/exec",
            json={"command": "ls", "timeout": 0},
            headers={"Authorization": f"Bearer {valid_token}"},
        )
        assert response.status_code == 422

        # Too high
        response = client.post(
            "/api/workspace/ws-test123/exec",
            json={"command": "ls", "timeout": 500},
            headers={"Authorization": f"Bearer {valid_token}"},
        )
        assert response.status_code == 422


# ============================================================================
# Response Format Tests
# ============================================================================


class TestResponseFormats:
    """Tests for response format compliance."""

    def test_workspace_response_format(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test WorkspaceResponse has all required fields."""
        mock_workspace_manager.get.return_value = sample_workspace

        response = client.get(
            "/api/workspace/ws-test123",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()

        # Required fields
        assert "id" in data
        assert "user_id" in data
        assert "git_url" in data
        assert "branch" in data
        assert "status" in data
        assert "created_at" in data
        # Optional fields may be None
        assert "container_id" in data
        assert "error_message" in data

    def test_workspace_list_response_format(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test WorkspaceListResponse has all required fields."""
        mock_workspace_manager.list_by_user.return_value = [sample_workspace]

        response = client.get(
            "/api/workspace/list",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()

        assert "workspaces" in data
        assert "total" in data
        assert isinstance(data["workspaces"], list)
        assert isinstance(data["total"], int)

    def test_workspace_status_response_format(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test WorkspaceStatusResponse has all required fields."""
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.get_stats.return_value = WorkspaceStats(
            cpu_percent=10.0, memory_mb=256.0, disk_mb=512.0
        )
        mock_workspace_manager.health_check.return_value = True

        response = client.get(
            "/api/workspace/ws-test123/status",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()

        assert "id" in data
        assert "status" in data
        assert "cpu_percent" in data
        assert "memory_mb" in data
        assert "disk_mb" in data
        assert "is_healthy" in data

    def test_exec_response_format(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test WorkspaceExecResponse has all required fields."""
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.exec.return_value = ExecResult(
            exit_code=0,
            stdout="output\n",
            stderr="",
            timed_out=False,
            duration_seconds=0.1,
        )

        response = client.post(
            "/api/workspace/ws-test123/exec",
            json={"command": "echo output"},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        assert response.status_code == 200
        data = response.json()

        assert "exit_code" in data
        assert "stdout" in data
        assert "stderr" in data
        assert "timed_out" in data
        assert "duration_seconds" in data


# ============================================================================
# User Isolation Tests
# ============================================================================


class TestUserIsolation:
    """Tests for user workspace isolation."""

    def test_user_can_only_list_own_workspaces(
        self, client, valid_token, other_user_token, mock_workspace_manager
    ):
        """Test users only see their own workspaces in list."""
        user1_ws = Workspace(
            id="ws-user1",
            user_id="user-123",
            git_url="https://github.com/user1/repo",
            branch="main",
            status=WorkspaceStatus.READY,
            created_at=datetime.now(timezone.utc),
        )
        user2_ws = Workspace(
            id="ws-user2",
            user_id="user-other",
            git_url="https://github.com/user2/repo",
            branch="main",
            status=WorkspaceStatus.READY,
            created_at=datetime.now(timezone.utc),
        )

        # When user-123 lists workspaces
        mock_workspace_manager.list_by_user.return_value = [user1_ws]
        response = client.get(
            "/api/workspace/list",
            headers={"Authorization": f"Bearer {valid_token}"},
        )
        assert response.status_code == 200
        data = response.json()
        assert all(ws["user_id"] == "user-123" for ws in data["workspaces"])

        # When user-other lists workspaces
        mock_workspace_manager.list_by_user.return_value = [user2_ws]
        response = client.get(
            "/api/workspace/list",
            headers={"Authorization": f"Bearer {other_user_token}"},
        )
        assert response.status_code == 200
        data = response.json()
        assert all(ws["user_id"] == "user-other" for ws in data["workspaces"])

    def test_cannot_access_other_users_workspace(
        self,
        client,
        valid_token,
        mock_workspace_manager,
        sample_workspace_other_user,
    ):
        """Test user cannot access another user's workspace."""
        mock_workspace_manager.get.return_value = sample_workspace_other_user

        # Try to get
        response = client.get(
            "/api/workspace/ws-other456",
            headers={"Authorization": f"Bearer {valid_token}"},
        )
        assert response.status_code == 403

        # Try to delete
        response = client.delete(
            "/api/workspace/ws-other456",
            headers={"Authorization": f"Bearer {valid_token}"},
        )
        assert response.status_code == 403

        # Try to get status
        response = client.get(
            "/api/workspace/ws-other456/status",
            headers={"Authorization": f"Bearer {valid_token}"},
        )
        assert response.status_code == 403

        # Try to exec
        response = client.post(
            "/api/workspace/ws-other456/exec",
            json={"command": "ls"},
            headers={"Authorization": f"Bearer {valid_token}"},
        )
        assert response.status_code == 403


# ============================================================================
# Edge Cases
# ============================================================================


class TestEdgeCases:
    """Tests for edge cases and error handling."""

    def test_create_workspace_with_error_status(
        self, client, valid_token, mock_workspace_manager
    ):
        """Test workspace creation that results in error status."""
        error_ws = Workspace(
            id="ws-error",
            user_id="user-123",
            git_url="https://github.com/user/repo",
            branch="main",
            status=WorkspaceStatus.ERROR,
            created_at=datetime.now(timezone.utc),
            error_message="Git clone failed: repository not found",
        )
        mock_workspace_manager.create.return_value = error_ws

        response = client.post(
            "/api/workspace/create",
            json={"git_url": "https://github.com/user/repo"},
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        # Should still return 201 - the workspace exists, just in error state
        assert response.status_code == 201
        data = response.json()
        assert data["status"] == "error"
        assert data["error_message"] == "Git clone failed: repository not found"

    def test_get_workspace_with_all_statuses(
        self, client, valid_token, mock_workspace_manager
    ):
        """Test getting workspaces with different statuses."""
        for status in WorkspaceStatus:
            ws = Workspace(
                id=f"ws-{status.value}",
                user_id="user-123",
                git_url="https://github.com/user/repo",
                branch="main",
                status=status,
                created_at=datetime.now(timezone.utc),
            )
            mock_workspace_manager.get.return_value = ws

            response = client.get(
                f"/api/workspace/ws-{status.value}",
                headers={"Authorization": f"Bearer {valid_token}"},
            )

            assert response.status_code == 200
            data = response.json()
            assert data["status"] == status.value

    def test_delete_workspace_destroy_fails(
        self, client, valid_token, mock_workspace_manager, sample_workspace
    ):
        """Test handling when destroy operation fails."""
        mock_workspace_manager.get.return_value = sample_workspace
        mock_workspace_manager.destroy.return_value = False

        response = client.delete(
            "/api/workspace/ws-test123",
            headers={"Authorization": f"Bearer {valid_token}"},
        )

        # Should return 500 if destroy fails
        assert response.status_code == 500
