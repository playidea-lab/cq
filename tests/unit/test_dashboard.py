"""Unit tests for Team Dashboard Service."""

from datetime import datetime
from unittest.mock import MagicMock

import pytest

from c4.models import (
    C4State,
    CheckpointQueueItem,
    CheckpointState,
    ProjectStatus,
    TaskQueue,
    WorkerInfo,
)
from c4.web.dashboard import (
    DashboardService,
    ProjectSummary,
    RealtimeStatus,
    ReviewStatus,
    WorkerStatus,
)


@pytest.fixture
def mock_store():
    """Create a mock state store."""
    return MagicMock()


@pytest.fixture
def sample_state():
    """Create a sample project state for testing."""
    now = datetime.now()
    return C4State(
        project_id="test-project",
        status=ProjectStatus.EXECUTE,
        queue=TaskQueue(
            pending=["T-003", "T-004"],
            in_progress={"T-002": "worker-1"},
            done=["T-001"],
        ),
        workers={
            "worker-1": WorkerInfo(
                worker_id="worker-1",
                state="busy",
                task_id="T-002",
                scope="src/",
                branch="c4/w-T-002",
                joined_at=now,
                last_seen=now,
            ),
            "worker-2": WorkerInfo(
                worker_id="worker-2",
                state="idle",
                task_id=None,
                scope=None,
                branch=None,
                joined_at=now,
                last_seen=now,
            ),
        },
        checkpoint=CheckpointState(current=None),  # state defaults to "pending"
        passed_checkpoints=["CP-001"],
        checkpoint_queue=[],
        repair_queue=[],
    )


@pytest.fixture
def dashboard_service(mock_store, tmp_path):
    """Create a dashboard service with mock store."""
    return DashboardService(mock_store, tmp_path)


class TestProjectSummary:
    """Tests for ProjectSummary data class."""

    def test_to_dict(self):
        """Test ProjectSummary serialization."""
        summary = ProjectSummary(
            project_id="test-proj",
            name="Test Project",
            status="EXECUTE",
            tasks_pending=3,
            tasks_in_progress=1,
            tasks_done=2,
            workers_active=1,
            workers_idle=1,
            last_activity="2025-01-01T12:00:00",
            checkpoint_state=None,
        )

        result = summary.to_dict()

        assert result["project_id"] == "test-proj"
        assert result["name"] == "Test Project"
        assert result["status"] == "EXECUTE"
        assert result["tasks"]["pending"] == 3
        assert result["tasks"]["in_progress"] == 1
        assert result["tasks"]["done"] == 2
        assert result["tasks"]["total"] == 6
        assert result["workers"]["active"] == 1
        assert result["workers"]["idle"] == 1
        assert result["workers"]["total"] == 2


class TestWorkerStatus:
    """Tests for WorkerStatus data class."""

    def test_to_dict(self):
        """Test WorkerStatus serialization."""
        status = WorkerStatus(
            worker_id="worker-1",
            state="busy",
            task_id="T-001",
            scope="src/",
            branch="c4/w-T-001",
            last_seen="2025-01-01T12:00:00",
        )

        result = status.to_dict()

        assert result["worker_id"] == "worker-1"
        assert result["state"] == "busy"
        assert result["task_id"] == "T-001"
        assert result["scope"] == "src/"
        assert result["branch"] == "c4/w-T-001"


class TestReviewStatus:
    """Tests for ReviewStatus data class."""

    def test_to_dict(self):
        """Test ReviewStatus serialization."""
        status = ReviewStatus(
            checkpoint_id="CP-001",
            state="in_review",
            tasks_completed=5,
            checkpoints_passed=2,
            checkpoints_total=3,
            repair_queue_size=1,
        )

        result = status.to_dict()

        assert result["checkpoint_id"] == "CP-001"
        assert result["state"] == "in_review"
        assert result["tasks_completed"] == 5
        assert result["checkpoints"]["passed"] == 2
        assert result["checkpoints"]["total"] == 3
        assert result["repair_queue_size"] == 1


class TestRealtimeStatus:
    """Tests for RealtimeStatus data class."""

    def test_to_dict(self):
        """Test RealtimeStatus serialization."""
        worker = WorkerStatus(
            worker_id="w-1",
            state="busy",
            task_id="T-001",
            scope=None,
            branch=None,
            last_seen=None,
        )
        review = ReviewStatus(
            checkpoint_id=None,
            state="idle",
            tasks_completed=1,
            checkpoints_passed=0,
            checkpoints_total=0,
            repair_queue_size=0,
        )
        status = RealtimeStatus(
            timestamp="2025-01-01T12:00:00",
            project_id="test",
            status="EXECUTE",
            progress=0.5,
            tasks={"pending": 1, "in_progress": 1, "done": 1},
            workers=[worker],
            review=review,
            events=[],
        )

        result = status.to_dict()

        assert result["timestamp"] == "2025-01-01T12:00:00"
        assert result["project_id"] == "test"
        assert result["progress"] == 0.5
        assert len(result["workers"]) == 1
        assert result["workers"][0]["worker_id"] == "w-1"


class TestDashboardService:
    """Tests for DashboardService."""

    def test_list_projects_empty(self, dashboard_service, tmp_path):
        """Test listing projects when none exist."""
        projects = dashboard_service.list_projects()
        assert projects == []

    def test_list_projects_with_projects(
        self, dashboard_service, mock_store, sample_state, tmp_path
    ):
        """Test listing projects with existing projects."""
        # Create projects directory structure
        projects_dir = tmp_path / "projects"
        projects_dir.mkdir()
        (projects_dir / "test-project").mkdir()

        mock_store.load.return_value = sample_state

        projects = dashboard_service.list_projects()

        assert len(projects) == 1
        assert projects[0].project_id == "test-project"
        assert projects[0].status == "EXECUTE"
        assert projects[0].tasks_pending == 2
        assert projects[0].tasks_in_progress == 1
        assert projects[0].tasks_done == 1
        assert projects[0].workers_active == 1
        assert projects[0].workers_idle == 1

    def test_get_realtime_status(self, dashboard_service, mock_store, sample_state):
        """Test getting realtime status for a project."""
        mock_store.load.return_value = sample_state

        status = dashboard_service.get_realtime_status("test-project")

        assert status is not None
        assert status.project_id == "test-project"
        assert status.status == "EXECUTE"
        assert status.progress == 0.25  # 1 done out of 4 total
        assert status.tasks["pending"] == 2
        assert status.tasks["in_progress"] == 1
        assert status.tasks["done"] == 1
        assert len(status.workers) == 2

    def test_get_realtime_status_not_found(self, dashboard_service, mock_store):
        """Test getting status for non-existent project."""
        mock_store.load.return_value = None

        status = dashboard_service.get_realtime_status("non-existent")

        assert status is None

    def test_get_worker_status(self, dashboard_service, mock_store, sample_state):
        """Test getting worker status for a project."""
        mock_store.load.return_value = sample_state

        workers = dashboard_service.get_worker_status("test-project")

        assert len(workers) == 2

        busy_worker = next(w for w in workers if w.state == "busy")
        assert busy_worker.worker_id == "worker-1"
        assert busy_worker.task_id == "T-002"
        assert busy_worker.scope == "src/"

        idle_worker = next(w for w in workers if w.state == "idle")
        assert idle_worker.worker_id == "worker-2"
        assert idle_worker.task_id is None

    def test_get_review_status(self, dashboard_service, mock_store, sample_state):
        """Test getting review status for a project."""
        mock_store.load.return_value = sample_state

        review = dashboard_service.get_review_status("test-project")

        assert review is not None
        assert review.state == "idle"
        assert review.tasks_completed == 1
        assert review.checkpoints_passed == 1

    def test_get_review_status_in_checkpoint(self, dashboard_service, mock_store):
        """Test review status when in checkpoint state."""
        state = C4State(
            project_id="test",
            status=ProjectStatus.CHECKPOINT,
            queue=TaskQueue(pending=[], in_progress={}, done=["T-001"]),
            workers={},
            checkpoint=CheckpointState(current="CP-002", state="pending"),
            passed_checkpoints=["CP-001"],
            checkpoint_queue=[],
            repair_queue=[],
        )
        mock_store.load.return_value = state

        review = dashboard_service.get_review_status("test")

        assert review is not None
        assert review.state == "in_review"
        assert review.checkpoint_id == "CP-002"

    def test_recent_events_from_checkpoint_queue(
        self, dashboard_service, mock_store
    ):
        """Test that checkpoint queue items become events."""
        state = C4State(
            project_id="test",
            status=ProjectStatus.CHECKPOINT,
            queue=TaskQueue(pending=[], in_progress={}, done=["T-001"]),
            workers={},
            checkpoint=CheckpointState(current="CP-001", state="pending"),
            passed_checkpoints=[],
            checkpoint_queue=[
                CheckpointQueueItem(
                    checkpoint_id="CP-001",
                    triggered_at="2025-01-01T10:00:00",
                    tasks_completed=["T-001"],
                    validation_results=[],
                )
            ],
            repair_queue=[],
        )
        mock_store.load.return_value = state

        status = dashboard_service.get_realtime_status("test")

        assert status is not None
        assert len(status.events) == 1
        assert status.events[0]["type"] == "checkpoint_triggered"
        assert status.events[0]["data"]["checkpoint_id"] == "CP-001"


class TestDataClassSerialization:
    """Tests for data class JSON serialization."""

    def test_project_summary_round_trip(self):
        """Test ProjectSummary can be serialized and used in JSON."""
        import json

        summary = ProjectSummary(
            project_id="p1",
            name="Project 1",
            status="EXECUTE",
            tasks_pending=1,
            tasks_in_progress=2,
            tasks_done=3,
            workers_active=1,
            workers_idle=0,
            last_activity=None,
            checkpoint_state=None,
        )

        # Should be JSON serializable
        json_str = json.dumps(summary.to_dict())
        data = json.loads(json_str)

        assert data["project_id"] == "p1"
        assert data["tasks"]["total"] == 6

    def test_realtime_status_nested_serialization(self):
        """Test RealtimeStatus with nested objects serializes correctly."""
        import json

        status = RealtimeStatus(
            timestamp="2025-01-01T00:00:00",
            project_id="test",
            status="EXECUTE",
            progress=0.75,
            tasks={"pending": 1, "in_progress": 0, "done": 3},
            workers=[
                WorkerStatus(
                    worker_id="w1",
                    state="idle",
                    task_id=None,
                    scope=None,
                    branch=None,
                    last_seen=None,
                )
            ],
            review=ReviewStatus(
                checkpoint_id=None,
                state="idle",
                tasks_completed=3,
                checkpoints_passed=1,
                checkpoints_total=1,
                repair_queue_size=0,
            ),
        )

        json_str = json.dumps(status.to_dict())
        data = json.loads(json_str)

        assert data["progress"] == 0.75
        assert data["workers"][0]["state"] == "idle"
        assert data["review"]["checkpoints"]["passed"] == 1
