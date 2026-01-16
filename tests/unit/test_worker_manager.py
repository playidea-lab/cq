"""Tests for Cloud Worker Manager."""

from datetime import datetime
from unittest.mock import MagicMock

import pytest

from c4.cloud.worker_manager import (
    MachineState,
    WorkerInstance,
    WorkerScaler,
)


class TestMachineState:
    """Test MachineState enum."""

    def test_state_values(self) -> None:
        """Test state enum values."""
        assert MachineState.CREATED.value == "created"
        assert MachineState.STARTED.value == "started"
        assert MachineState.STOPPED.value == "stopped"
        assert MachineState.DESTROYED.value == "destroyed"


class TestWorkerInstance:
    """Test WorkerInstance dataclass."""

    def test_basic_instance(self) -> None:
        """Test creating a basic instance."""
        instance = WorkerInstance(
            id="m-123",
            name="c4-worker-t001",
            state=MachineState.STARTED,
            region="nrt",
            created_at=datetime.now(),
        )

        assert instance.id == "m-123"
        assert instance.state == MachineState.STARTED
        assert instance.task_id is None

    def test_instance_with_task(self) -> None:
        """Test instance with assigned task."""
        instance = WorkerInstance(
            id="m-456",
            name="c4-worker-t002",
            state=MachineState.STARTED,
            region="nrt",
            created_at=datetime.now(),
            task_id="T-002",
            project_id="my-project",
        )

        assert instance.task_id == "T-002"
        assert instance.project_id == "my-project"


class TestWorkerScalerInit:
    """Test WorkerScaler initialization."""

    def test_init_with_params(self) -> None:
        """Test initialization with explicit params."""
        scaler = WorkerScaler(
            api_token="fly_test_token",
            app_name="my-app",
        )

        assert scaler._token == "fly_test_token"
        assert scaler._app == "my-app"

    def test_init_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test initialization from environment."""
        monkeypatch.setenv("FLY_API_TOKEN", "fly_env_token")
        monkeypatch.setenv("FLY_APP_NAME", "env-app")

        scaler = WorkerScaler()

        assert scaler._token == "fly_env_token"
        assert scaler._app == "env-app"

    def test_default_app_name(self) -> None:
        """Test default app name."""
        scaler = WorkerScaler(api_token="test")
        assert scaler._app == "c4-worker"

    def test_context_manager(self) -> None:
        """Test context manager protocol."""
        with WorkerScaler(api_token="test") as scaler:
            assert scaler is not None


class TestWorkerScalerOperations:
    """Test WorkerScaler machine operations."""

    @pytest.fixture
    def scaler(self) -> WorkerScaler:
        """Create scaler with mock client."""
        scaler = WorkerScaler(api_token="test")
        scaler._client = MagicMock()
        return scaler

    def test_create_worker(self, scaler: WorkerScaler) -> None:
        """Test creating a worker."""
        mock_response = MagicMock()
        mock_response.status_code = 201
        mock_response.json.return_value = {
            "id": "m-new",
            "name": "c4-worker-t001",
            "state": "started",
            "region": "nrt",
            "created_at": "2024-01-01T00:00:00Z",
        }
        scaler._client.post.return_value = mock_response

        worker = scaler.create_worker("project", "T-001")

        assert worker is not None
        assert worker.id == "m-new"
        assert worker.task_id == "T-001"

    def test_create_worker_failure(self, scaler: WorkerScaler) -> None:
        """Test create worker failure."""
        mock_response = MagicMock()
        mock_response.status_code = 500
        scaler._client.post.return_value = mock_response

        worker = scaler.create_worker("project", "T-001")

        assert worker is None

    def test_destroy_worker(self, scaler: WorkerScaler) -> None:
        """Test destroying a worker."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        scaler._client.delete.return_value = mock_response

        result = scaler.destroy_worker("m-123")

        assert result is True

    def test_stop_worker(self, scaler: WorkerScaler) -> None:
        """Test stopping a worker."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        scaler._client.post.return_value = mock_response

        result = scaler.stop_worker("m-123")

        assert result is True

    def test_start_worker(self, scaler: WorkerScaler) -> None:
        """Test starting a worker."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        scaler._client.post.return_value = mock_response

        result = scaler.start_worker("m-123")

        assert result is True


class TestWorkerScalerListing:
    """Test WorkerScaler listing operations."""

    @pytest.fixture
    def scaler(self) -> WorkerScaler:
        """Create scaler with mock client."""
        scaler = WorkerScaler(api_token="test")
        scaler._client = MagicMock()
        return scaler

    def test_list_workers(self, scaler: WorkerScaler) -> None:
        """Test listing workers."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = [
            {
                "id": "m-1",
                "name": "worker-1",
                "state": "started",
                "region": "nrt",
                "created_at": "2024-01-01T00:00:00Z",
                "config": {"env": {"C4_TASK_ID": "T-001"}},
            },
            {
                "id": "m-2",
                "name": "worker-2",
                "state": "stopped",
                "region": "lax",
                "created_at": "2024-01-02T00:00:00Z",
                "config": {"env": {}},
            },
        ]
        scaler._client.get.return_value = mock_response

        workers = scaler.list_workers()

        assert len(workers) == 2
        assert workers[0].id == "m-1"
        assert workers[0].task_id == "T-001"
        assert workers[1].task_id is None

    def test_list_workers_empty(self, scaler: WorkerScaler) -> None:
        """Test listing when no workers."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = []
        scaler._client.get.return_value = mock_response

        workers = scaler.list_workers()

        assert workers == []

    def test_get_worker(self, scaler: WorkerScaler) -> None:
        """Test getting a specific worker."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "id": "m-123",
            "name": "worker-1",
            "state": "started",
            "region": "nrt",
            "created_at": "2024-01-01T00:00:00Z",
            "config": {"env": {"C4_PROJECT_ID": "proj"}},
        }
        scaler._client.get.return_value = mock_response

        worker = scaler.get_worker("m-123")

        assert worker is not None
        assert worker.id == "m-123"
        assert worker.project_id == "proj"

    def test_get_worker_not_found(self, scaler: WorkerScaler) -> None:
        """Test getting non-existent worker."""
        mock_response = MagicMock()
        mock_response.status_code = 404
        scaler._client.get.return_value = mock_response

        worker = scaler.get_worker("m-unknown")

        assert worker is None


class TestAutoScaling:
    """Test auto-scaling functionality."""

    @pytest.fixture
    def scaler(self) -> WorkerScaler:
        """Create scaler with mock client."""
        scaler = WorkerScaler(api_token="test")
        scaler._client = MagicMock()
        return scaler

    def test_get_active_count(self, scaler: WorkerScaler) -> None:
        """Test getting active worker count."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = [
            {"id": "1", "name": "w1", "state": "started", "region": "nrt",
             "created_at": "2024-01-01T00:00:00Z", "config": {"env": {}}},
            {"id": "2", "name": "w2", "state": "started", "region": "nrt",
             "created_at": "2024-01-01T00:00:00Z", "config": {"env": {}}},
            {"id": "3", "name": "w3", "state": "stopped", "region": "nrt",
             "created_at": "2024-01-01T00:00:00Z", "config": {"env": {}}},
        ]
        scaler._client.get.return_value = mock_response

        count = scaler.get_active_count()

        assert count == 2

    def test_calculate_desired_workers_empty(self, scaler: WorkerScaler) -> None:
        """Test desired count with no tasks."""
        count = scaler.calculate_desired_workers(pending_tasks=0)
        assert count == 0

    def test_calculate_desired_workers_basic(self, scaler: WorkerScaler) -> None:
        """Test desired count calculation."""
        count = scaler.calculate_desired_workers(pending_tasks=5)
        assert count == 5

    def test_calculate_desired_workers_max(self, scaler: WorkerScaler) -> None:
        """Test desired count respects max."""
        count = scaler.calculate_desired_workers(
            pending_tasks=100,
            max_workers=10,
        )
        assert count == 10

    def test_calculate_desired_workers_batch(self, scaler: WorkerScaler) -> None:
        """Test desired count with batching."""
        count = scaler.calculate_desired_workers(
            pending_tasks=10,
            tasks_per_worker=3,
        )
        assert count == 4  # ceil(10/3) = 4
