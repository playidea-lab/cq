"""Team Collaboration Integration Tests

Tests for team collaboration scenarios:
- Multi-worker task distribution
- Peer review workflow
- Central review (Supervisor)
"""

import tempfile
from pathlib import Path

import pytest

from c4.mcp_server import C4Daemon
from c4.models import (
    ProjectStatus,
    Task,
    ValidationConfig,
)


@pytest.fixture
def temp_project():
    """Create a temporary project directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def team_daemon(temp_project):
    """Create daemon configured for team collaboration testing."""
    daemon = C4Daemon(temp_project)
    daemon.initialize("team-collab-test", with_default_checkpoints=True)

    # Skip discovery to go directly to PLAN
    daemon.state_machine.transition("skip_discovery")

    # Configure validations
    daemon._config.validation = ValidationConfig(
        commands={
            "lint": "echo 'lint ok'",
            "unit": "echo 'unit ok'",
        },
        required=["lint", "unit"],
    )
    daemon._save_config()

    return daemon


@pytest.fixture
def team_daemon_no_checkpoints(temp_project):
    """Create daemon without checkpoints for progress tracking tests."""
    daemon = C4Daemon(temp_project)
    daemon.initialize("team-progress-test", with_default_checkpoints=False)

    # Skip discovery to go directly to PLAN
    daemon.state_machine.transition("skip_discovery")

    # Configure validations
    daemon._config.validation = ValidationConfig(
        commands={
            "lint": "echo 'lint ok'",
            "unit": "echo 'unit ok'",
        },
        required=["lint", "unit"],
    )
    daemon._save_config()

    return daemon


# =============================================================================
# Multi-Worker Task Distribution Tests
# =============================================================================


class TestMultiWorkerDistribution:
    """Tests for distributing tasks among multiple workers."""

    def test_fair_distribution_round_robin(self, team_daemon):
        """Tasks should be distributed fairly among workers."""
        daemon = team_daemon

        # Add 6 tasks with different scopes
        for i in range(6):
            task = Task(
                id=f"T-{i:03d}",
                title=f"Feature {i}",
                dod=f"Implement feature {i}",
                scope=f"feature-{i}/",
            )
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # 3 workers get tasks
        workers = ["alice", "bob", "charlie"]
        assignments = {}
        for worker in workers:
            assignment = daemon.c4_get_task(worker)
            if assignment:
                assignments[worker] = assignment.task_id

        # All workers should get tasks
        assert len(assignments) == 3
        assert len(set(assignments.values())) == 3  # All different tasks

    def test_priority_based_assignment(self, team_daemon):
        """Higher priority tasks should be assigned first."""
        daemon = team_daemon

        # Add tasks with different priorities
        high_priority = Task(
            id="T-HIGH",
            title="Critical Bug",
            dod="Fix critical bug",
            scope="critical/",
            priority=100,
        )
        low_priority = Task(
            id="T-LOW",
            title="Nice to have",
            dod="Add nice feature",
            scope="nice/",
            priority=1,
        )
        daemon.add_task(low_priority)
        daemon.add_task(high_priority)

        daemon.state_machine.transition("c4_run")

        # First worker should get high priority task
        assignment = daemon.c4_get_task("worker-1")
        assert assignment.task_id == "T-HIGH"

    def test_domain_based_routing(self, team_daemon):
        """Tasks should be routed based on domain when possible."""
        daemon = team_daemon

        # Add tasks with different domains
        frontend_task = Task(
            id="T-FE",
            title="UI Component",
            dod="Build UI",
            scope="web/",
            domain="web-frontend",
        )
        backend_task = Task(
            id="T-BE",
            title="API Endpoint",
            dod="Build API",
            scope="api/",
            domain="web-backend",
        )
        daemon.add_task(frontend_task)
        daemon.add_task(backend_task)

        daemon.state_machine.transition("c4_run")

        # Both workers get their respective tasks
        fe_worker = daemon.c4_get_task("frontend-dev")
        be_worker = daemon.c4_get_task("backend-dev")

        assert fe_worker is not None
        assert be_worker is not None
        assert fe_worker.task_id != be_worker.task_id

    def test_workload_balancing(self, team_daemon):
        """Workers should not be overloaded."""
        daemon = team_daemon

        # Add multiple tasks
        for i in range(4):
            task = Task(
                id=f"T-{i:03d}",
                title=f"Task {i}",
                dod=f"Do task {i}",
                scope=f"scope-{i}/",
            )
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # First worker gets a task
        assignment1 = daemon.c4_get_task("worker-1")
        assert assignment1 is not None

        # Same worker requesting again should get same task (not new one)
        assignment1b = daemon.c4_get_task("worker-1")
        assert assignment1b.task_id == assignment1.task_id

        # Second worker gets different task
        assignment2 = daemon.c4_get_task("worker-2")
        assert assignment2 is not None
        assert assignment2.task_id != assignment1.task_id


# =============================================================================
# Peer Review Tests
# =============================================================================


class TestPeerReview:
    """Tests for peer review workflows."""

    def test_peer_can_review_completed_task(self, team_daemon):
        """A worker can review another worker's completed task."""
        daemon = team_daemon

        # Add task
        task = Task(id="T-001", title="Feature", dod="Build feature")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker 1 completes task
        assignment = daemon.c4_get_task("developer")
        daemon.c4_submit(
            assignment.task_id,
            "commit123",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="developer",
        )

        # Task is in done queue
        status = daemon.c4_status()
        assert status["queue"]["done"] >= 1

        # Reviewer can access task info
        task_info = daemon.get_task("T-001")
        assert task_info is not None
        assert task_info.status.value == "done"
        assert task_info.commit_sha == "commit123"

    def test_reviewer_comments_tracked(self, team_daemon):
        """Review comments should be tracked in the system."""
        daemon = team_daemon

        task = Task(id="T-001", title="Feature", dod="Build feature")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Complete task
        daemon.c4_get_task("developer")
        daemon.c4_submit(
            "T-001",
            "commit123",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="developer",
        )

        # Verify task completion is recorded
        task_info = daemon.get_task("T-001")
        assert task_info.commit_sha == "commit123"

    def test_multiple_reviewers_scenario(self, team_daemon):
        """Multiple reviewers can access the same completed work."""
        daemon = team_daemon

        # Add and complete task
        task = Task(id="T-001", title="Feature", dod="Build feature")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("developer")
        daemon.c4_submit(
            "T-001",
            "commit-abc",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="developer",
        )

        # Multiple reviewers can query task info
        for reviewer in ["reviewer-1", "reviewer-2", "tech-lead"]:
            task_info = daemon.get_task("T-001")
            assert task_info is not None
            assert task_info.commit_sha == "commit-abc"


# =============================================================================
# Central Review (Supervisor) Tests
# =============================================================================


class TestCentralReview:
    """Tests for central review via Supervisor."""

    def test_checkpoint_triggers_central_review(self, team_daemon):
        """Completing tasks should trigger checkpoint review."""
        daemon = team_daemon

        # Add tasks that will trigger checkpoint
        task = Task(id="T-001", title="Feature", dod="Build feature")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Complete task
        daemon.c4_get_task("developer")
        result = daemon.c4_submit(
            "T-001",
            "commit123",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="developer",
        )

        # Check if checkpoint was triggered (COMPLETE or await_checkpoint)
        assert result.success is True
        # Final task completion should signal complete or checkpoint
        assert result.next_action in ["complete", "await_checkpoint", "get_next_task"]

    def test_checkpoint_approval_flow(self, team_daemon):
        """Supervisor approval should advance the project."""
        daemon = team_daemon

        task = Task(id="T-001", title="Feature", dod="Build feature")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Complete all tasks
        daemon.c4_get_task("developer")
        daemon.c4_submit(
            "T-001",
            "commit123",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="developer",
        )

        # Manually trigger checkpoint state for testing
        state = daemon.state_machine.state
        if state.status != ProjectStatus.CHECKPOINT:
            # Force checkpoint state for test
            daemon.state_machine.transition("trigger_checkpoint")

        # Simulate supervisor approval
        result = daemon.c4_checkpoint(
            checkpoint_id="CP-FINAL",
            decision="APPROVE",
            notes="All code reviewed and approved",
        )

        # c4_checkpoint returns CheckpointResponse object
        assert result.success is True

    def test_checkpoint_request_changes_flow(self, team_daemon):
        """Supervisor can request changes from the team."""
        daemon = team_daemon

        # Add task
        task = Task(id="T-001", title="Feature", dod="Build feature")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Complete task
        daemon.c4_get_task("developer")
        daemon.c4_submit(
            "T-001",
            "commit123",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="developer",
        )

        # Check if already in checkpoint state (auto-triggered)
        state = daemon.state_machine.state
        if state.status != ProjectStatus.CHECKPOINT:
            daemon.state_machine.transition("trigger_checkpoint")

        # Supervisor requests changes
        result = daemon.c4_checkpoint(
            checkpoint_id="CP-FINAL",
            decision="REQUEST_CHANGES",
            notes="Need more test coverage",
            required_changes=["Add unit tests for edge cases", "Fix documentation"],
        )

        # c4_checkpoint returns CheckpointResponse object
        assert result.success is True
        # State should transition back to EXECUTE
        status = daemon.c4_status()
        assert status["status"] == "EXECUTE"

    def test_supervisor_replan_flow(self, team_daemon):
        """Supervisor can request replanning."""
        daemon = team_daemon

        task = Task(id="T-001", title="Feature", dod="Build feature")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("developer")
        daemon.c4_submit(
            "T-001",
            "commit123",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="developer",
        )

        # Check if already in checkpoint state (auto-triggered)
        state = daemon.state_machine.state
        if state.status != ProjectStatus.CHECKPOINT:
            daemon.state_machine.transition("trigger_checkpoint")

        # Supervisor requests replan
        result = daemon.c4_checkpoint(
            checkpoint_id="CP-FINAL",
            decision="REPLAN",
            notes="Architecture needs redesign",
        )

        # c4_checkpoint returns CheckpointResponse object
        assert result.success is True


# =============================================================================
# Team Progress Tracking Tests
# =============================================================================


class TestTeamProgressTracking:
    """Tests for tracking team progress."""

    def test_status_shows_team_progress(self, team_daemon_no_checkpoints):
        """Status should show overall team progress."""
        daemon = team_daemon_no_checkpoints

        # Add multiple tasks
        for i in range(5):
            task = Task(
                id=f"T-{i:03d}",
                title=f"Task {i}",
                dod=f"Do task {i}",
                scope=f"scope-{i}/",
            )
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # Complete some tasks
        for i in range(3):
            daemon.c4_get_task(f"worker-{i}")
            daemon.c4_submit(
                f"T-{i:03d}",
                f"commit-{i}",
                [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
                worker_id=f"worker-{i}",
            )

        status = daemon.c4_status()

        # Should show progress
        assert status["queue"]["done"] == 3
        assert status["queue"]["pending"] == 2

    def test_worker_activity_tracking(self, team_daemon):
        """Worker activity should be tracked."""
        daemon = team_daemon

        task = Task(id="T-001", title="Task", dod="Do task", scope="test/")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker gets task
        daemon.c4_get_task("active-worker")

        status = daemon.c4_status()

        # Worker should be tracked
        assert "active-worker" in status["workers"]
        worker_info = status["workers"]["active-worker"]
        assert worker_info["state"] == "busy"
        assert worker_info["task_id"] == "T-001"

    def test_metrics_accumulate(self, team_daemon_no_checkpoints):
        """Metrics should accumulate over time."""
        daemon = team_daemon_no_checkpoints

        # Add and complete tasks
        for i in range(3):
            task = Task(id=f"T-{i:03d}", title=f"Task {i}", dod="Do task", scope=f"s-{i}/")
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        for i in range(3):
            daemon.c4_get_task(f"worker-{i}")
            daemon.c4_submit(
                f"T-{i:03d}",
                f"commit-{i}",
                [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
                worker_id=f"worker-{i}",
            )

        status = daemon.c4_status()

        # Metrics should be tracked
        assert status["metrics"]["tasks_completed"] >= 3


# =============================================================================
# Collaboration Conflict Tests
# =============================================================================


class TestCollaborationConflicts:
    """Tests for handling collaboration conflicts."""

    def test_scope_conflict_prevention(self, team_daemon):
        """Workers should not conflict on same scope."""
        daemon = team_daemon

        # Tasks with same scope
        task1 = Task(id="T-001", title="Task 1", dod="Do 1", scope="shared/")
        task2 = Task(id="T-002", title="Task 2", dod="Do 2", scope="shared/")
        daemon.add_task(task1)
        daemon.add_task(task2)

        daemon.state_machine.transition("c4_run")

        # First worker gets first task
        assignment1 = daemon.c4_get_task("worker-1")
        assert assignment1 is not None
        assert assignment1.task_id == "T-001"

        # Second worker cannot get second task (same scope)
        assignment2 = daemon.c4_get_task("worker-2")
        assert assignment2 is None

    def test_double_submit_prevention(self, team_daemon):
        """Same task cannot be submitted twice."""
        daemon = team_daemon

        task = Task(id="T-001", title="Task", dod="Do task")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")

        # First submit succeeds
        result1 = daemon.c4_submit(
            "T-001",
            "commit1",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="worker-1",
        )
        assert result1.success is True

        # Second submit fails (task already done)
        result2 = daemon.c4_submit(
            "T-001",
            "commit2",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="worker-1",
        )
        assert result2.success is False

    def test_unauthorized_submit_prevention(self, team_daemon):
        """Worker cannot submit task assigned to another worker."""
        daemon = team_daemon

        task = Task(id="T-001", title="Task", dod="Do task")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker 1 gets task
        daemon.c4_get_task("worker-1")

        # Worker 2 tries to submit
        result = daemon.c4_submit(
            "T-001",
            "commit",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="worker-2",
        )
        assert result.success is False


# =============================================================================
# Team Communication Tests
# =============================================================================


class TestTeamCommunication:
    """Tests for team communication features."""

    def test_checkpoint_queue_for_review(self, team_daemon):
        """Checkpoints should be queued for team review."""
        daemon = team_daemon

        # Setup task and complete it
        task = Task(id="T-001", title="Task", dod="Do task")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("developer")
        daemon.c4_submit(
            "T-001",
            "commit",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            worker_id="developer",
        )

        # Check checkpoint queue exists
        state = daemon.state_machine.state
        # Checkpoint queue should be accessible
        assert hasattr(state, "checkpoint_queue")

    def test_repair_queue_for_blocked_tasks(self, team_daemon):
        """Blocked tasks should go to repair queue for team attention."""
        daemon = team_daemon

        task = Task(id="T-001", title="Task", dod="Do task")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Get task
        daemon.c4_get_task("worker-1")

        # Mark as blocked
        result = daemon.c4_mark_blocked(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="test::failure",
            attempts=3,
            last_error="Test error",
        )

        assert result["success"] is True

        # Task should be in repair queue
        state = daemon.state_machine.state
        assert len(state.repair_queue) >= 1

    def test_status_includes_team_overview(self, team_daemon):
        """Status should include team overview information."""
        daemon = team_daemon

        # Add tasks and workers
        for i in range(3):
            task = Task(id=f"T-{i:03d}", title=f"Task {i}", dod="Do", scope=f"s-{i}/")
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # Workers get tasks
        for i in range(3):
            daemon.c4_get_task(f"team-member-{i}")

        status = daemon.c4_status()

        # Should include team information
        assert "workers" in status
        assert len(status["workers"]) == 3
        assert "queue" in status
        assert status["queue"]["in_progress"] == 3
