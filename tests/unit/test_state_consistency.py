"""Tests for state consistency and complex task ID parsing.

These tests cover:
- _parse_task_id with complex ID patterns (T-SBX-001-0, T-DEP-001-0-0)
- _sync_state_consistency for auto-fixing state inconsistencies
- _get_all_done_impl_tasks helper method
"""

from datetime import datetime
from unittest.mock import MagicMock, patch

import pytest

from c4.models import (
    C4Config,
    C4State,
    Task,
    TaskQueue,
    TaskStatus,
    TaskType,
    WorkerInfo,
)


class TestParseTaskIdComplex:
    """Test _parse_task_id with complex ID patterns (project prefixes).

    Tests the improved _parse_task_id in c4/daemon/c4_daemon.py which supports:
    - Simple numeric IDs: T-001, T-001-0
    - Complex IDs with project prefix: T-SBX-001-0, T-DEP-001-0-0
    """

    @pytest.fixture
    def daemon(self, tmp_path):
        """Create C4Daemon for testing (from daemon module with fix)."""
        from c4.daemon.c4_daemon import C4Daemon

        daemon = C4Daemon(project_root=tmp_path)
        return daemon

    def test_parse_complex_id_with_prefix(self, daemon):
        """Parse T-SBX-001-0 correctly (project prefix)."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-SBX-001-0")
        assert normalized == "T-SBX-001-0"
        assert base_id == "SBX-001"
        assert version == 0
        assert task_type == TaskType.IMPLEMENTATION

    def test_parse_complex_id_with_double_prefix(self, daemon):
        """Parse T-SBX-001-0-0 correctly (extra version segment)."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-SBX-001-0-0")
        assert normalized == "T-SBX-001-0-0"
        assert base_id == "SBX-001-0"
        assert version == 0
        assert task_type == TaskType.IMPLEMENTATION

    def test_parse_dep_prefix(self, daemon):
        """Parse T-DEP-001-0-0 correctly."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-DEP-001-0-0")
        assert normalized == "T-DEP-001-0-0"
        assert base_id == "DEP-001-0"
        assert version == 0
        assert task_type == TaskType.IMPLEMENTATION

    def test_parse_complex_review_id(self, daemon):
        """Parse R-SBX-001-0 correctly."""
        normalized, base_id, version, task_type = daemon._parse_task_id("R-SBX-001-0")
        assert normalized == "R-SBX-001-0"
        assert base_id == "SBX-001"
        assert version == 0
        assert task_type == TaskType.REVIEW

    def test_parse_complex_id_higher_version(self, daemon):
        """Parse T-SBX-001-2 correctly (version 2)."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-SBX-001-2")
        assert normalized == "T-SBX-001-2"
        assert base_id == "SBX-001"
        assert version == 2
        assert task_type == TaskType.IMPLEMENTATION

    def test_parse_complex_id_without_version(self, daemon):
        """Parse T-SBX-001 -> T-SBX-001-0 (auto-append)."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-SBX-001")
        assert normalized == "T-SBX-001-0"
        assert base_id == "SBX-001"
        assert version == 0
        assert task_type == TaskType.IMPLEMENTATION

    def test_parse_long_prefix_id(self, daemon):
        """Parse T-CLOUD-API-001-0 correctly."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-CLOUD-API-001-0")
        assert normalized == "T-CLOUD-API-001-0"
        assert base_id == "CLOUD-API-001"
        assert version == 0
        assert task_type == TaskType.IMPLEMENTATION

    def test_parse_version_9(self, daemon):
        """Parse T-SBX-001-9 correctly (single-digit max)."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-SBX-001-9")
        assert normalized == "T-SBX-001-9"
        assert base_id == "SBX-001"
        assert version == 9
        assert task_type == TaskType.IMPLEMENTATION


class TestSyncStateConsistency:
    """Test _sync_state_consistency method."""

    @pytest.fixture
    def daemon_with_inconsistent_state(self, tmp_path):
        """Create daemon with inconsistent state for testing."""
        from c4.daemon.c4_daemon import C4Daemon

        now = datetime.now()

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path

            # Config
            d._config = C4Config(project_id="test")

            # Tasks - T-001-0 is DONE in c4_tasks
            d._tasks = {
                "T-001-0": Task(
                    id="T-001-0",
                    title="Task 1",
                    dod="...",
                    status=TaskStatus.DONE,  # DONE in c4_tasks
                    type=TaskType.IMPLEMENTATION,
                ),
                "T-002-0": Task(
                    id="T-002-0",
                    title="Task 2",
                    dod="...",
                    status=TaskStatus.IN_PROGRESS,
                    assigned_to="worker-1",
                    type=TaskType.IMPLEMENTATION,
                ),
            }

            # State machine - T-001-0 is still in in_progress (inconsistent!)
            d.state_machine = MagicMock()
            d.state_machine.state = C4State(
                project_id="test",
                queue=TaskQueue(
                    pending=["T-003-0"],
                    in_progress={
                        "T-001-0": "worker-old",  # Should be in done!
                        "T-002-0": "worker-1",
                    },
                    done=[],
                ),
                workers={
                    "worker-old": WorkerInfo(
                        worker_id="worker-old",
                        state="busy",  # Valid state
                        task_id="T-001-0",
                        scope="src/",
                        joined_at=now,  # Required field
                    ),
                    "worker-1": WorkerInfo(
                        worker_id="worker-1",
                        state="busy",  # Valid state
                        task_id="T-002-0",
                        scope="tests/",
                        joined_at=now,  # Required field
                    ),
                },
            )
            d.state_machine.store = MagicMock()
            d.state_machine._state = d.state_machine.state

            # Mock atomic_modify context manager
            from contextlib import contextmanager

            @contextmanager
            def mock_atomic_modify(project_id):
                yield d.state_machine.state
            d.state_machine.store.atomic_modify = mock_atomic_modify

            d.get_task = MagicMock(side_effect=lambda tid: d._tasks.get(tid))
            d.get_all_tasks = MagicMock(return_value=d._tasks)

            # Bind method
            from c4.daemon.c4_daemon import C4Daemon as RealDaemon
            d._sync_state_consistency = RealDaemon._sync_state_consistency.__get__(d)

        return d

    def test_fixes_done_task_in_progress(self, daemon_with_inconsistent_state):
        """Fix task that is done in c4_tasks but in_progress in c4_state."""
        result = daemon_with_inconsistent_state._sync_state_consistency()

        assert "T-001-0" in result["fixed"]
        assert len(result["errors"]) == 0

        # Verify state was updated
        state = daemon_with_inconsistent_state.state_machine.state
        assert "T-001-0" not in state.queue.in_progress
        assert "T-001-0" in state.queue.done

        # Worker should be reset
        worker = state.workers["worker-old"]
        assert worker.state == "idle"
        assert worker.task_id is None
        assert worker.scope is None

    def test_keeps_valid_in_progress(self, daemon_with_inconsistent_state):
        """Keep valid in_progress tasks unchanged."""
        daemon_with_inconsistent_state._sync_state_consistency()

        # T-002-0 should still be in progress
        state = daemon_with_inconsistent_state.state_machine.state
        assert "T-002-0" in state.queue.in_progress
        assert state.queue.in_progress["T-002-0"] == "worker-1"

    def test_handles_missing_task(self, tmp_path):
        """Fix task that doesn't exist in c4_tasks."""
        from c4.daemon.c4_daemon import C4Daemon

        now = datetime.now()

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path
            d._config = C4Config(project_id="test")
            d._tasks = {}  # No tasks!

            d.state_machine = MagicMock()
            d.state_machine.state = C4State(
                project_id="test",
                queue=TaskQueue(
                    pending=[],
                    in_progress={"T-GHOST-0": "worker-1"},  # Doesn't exist!
                    done=[],
                ),
                workers={
                    "worker-1": WorkerInfo(
                        worker_id="worker-1",
                        state="busy",
                        task_id="T-GHOST-0",
                        joined_at=now,
                    ),
                },
            )
            d.state_machine.store = MagicMock()
            d.state_machine._state = d.state_machine.state

            from contextlib import contextmanager
            @contextmanager
            def mock_atomic_modify(project_id):
                yield d.state_machine.state
            d.state_machine.store.atomic_modify = mock_atomic_modify

            d.get_task = MagicMock(return_value=None)

            from c4.daemon.c4_daemon import C4Daemon as RealDaemon
            d._sync_state_consistency = RealDaemon._sync_state_consistency.__get__(d)

            result = d._sync_state_consistency()

            assert "T-GHOST-0" in result["fixed"]
            assert "T-GHOST-0" not in d.state_machine.state.queue.in_progress
            # Ghost task should NOT be added to done
            assert "T-GHOST-0" not in d.state_machine.state.queue.done

    def test_no_state_machine_returns_empty(self, tmp_path):
        """Return empty result if no state machine."""
        from c4.daemon.c4_daemon import C4Daemon

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path
            d.state_machine = None

            from c4.daemon.c4_daemon import C4Daemon as RealDaemon
            d._sync_state_consistency = RealDaemon._sync_state_consistency.__get__(d)

            result = d._sync_state_consistency()

            assert result == {"fixed": [], "errors": []}


class TestGetAllDoneImplTasks:
    """Test _get_all_done_impl_tasks helper method."""

    @pytest.fixture
    def daemon_with_tasks(self, tmp_path):
        """Create daemon with various tasks."""
        from c4.daemon.c4_daemon import C4Daemon

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path

            # Tasks
            d._tasks = {
                "T-001-0": Task(
                    id="T-001-0",
                    title="Task 1",
                    dod="...",
                    type=TaskType.IMPLEMENTATION,
                ),
                "T-002-0": Task(
                    id="T-002-0",
                    title="Task 2",
                    dod="...",
                    type=TaskType.IMPLEMENTATION,
                ),
                "R-001-0": Task(
                    id="R-001-0",
                    title="Review 1",
                    dod="...",
                    type=TaskType.REVIEW,
                ),
                "T-003-0": Task(
                    id="T-003-0",
                    title="Task 3 (pending)",
                    dod="...",
                    type=TaskType.IMPLEMENTATION,
                ),
            }

            # State - T-001-0 and T-002-0 are done
            d.state_machine = MagicMock()
            d.state_machine.state = C4State(
                project_id="test",
                queue=TaskQueue(
                    pending=["T-003-0"],
                    in_progress={},
                    done=["T-001-0", "T-002-0", "R-001-0"],
                ),
            )

            d.get_all_tasks = MagicMock(return_value=d._tasks)

            from c4.daemon.c4_daemon import C4Daemon as RealDaemon
            d._get_all_done_impl_tasks = RealDaemon._get_all_done_impl_tasks.__get__(d)

        return d

    def test_returns_only_done_impl_tasks(self, daemon_with_tasks):
        """Return only implementation tasks that are done."""
        result = daemon_with_tasks._get_all_done_impl_tasks()

        assert len(result) == 2
        task_ids = [t.id for t in result]
        assert "T-001-0" in task_ids
        assert "T-002-0" in task_ids
        # Review and pending should not be included
        assert "R-001-0" not in task_ids
        assert "T-003-0" not in task_ids

    def test_excludes_review_tasks(self, daemon_with_tasks):
        """Exclude review tasks even if done."""
        result = daemon_with_tasks._get_all_done_impl_tasks()

        for task in result:
            assert task.type == TaskType.IMPLEMENTATION

    def test_excludes_pending_impl_tasks(self, daemon_with_tasks):
        """Exclude implementation tasks that are not done."""
        result = daemon_with_tasks._get_all_done_impl_tasks()

        task_ids = [t.id for t in result]
        assert "T-003-0" not in task_ids

    def test_empty_when_no_done_tasks(self, tmp_path):
        """Return empty list when no tasks are done."""
        from c4.daemon.c4_daemon import C4Daemon

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path

            d._tasks = {
                "T-001-0": Task(id="T-001-0", title="Task 1", dod="...", type=TaskType.IMPLEMENTATION),
            }

            d.state_machine = MagicMock()
            d.state_machine.state = C4State(
                project_id="test",
                queue=TaskQueue(pending=["T-001-0"], in_progress={}, done=[]),
            )

            d.get_all_tasks = MagicMock(return_value=d._tasks)

            from c4.daemon.c4_daemon import C4Daemon as RealDaemon
            d._get_all_done_impl_tasks = RealDaemon._get_all_done_impl_tasks.__get__(d)

            result = d._get_all_done_impl_tasks()
            assert result == []

    def test_empty_when_no_state_machine(self, tmp_path):
        """Return empty list when no state machine."""
        from c4.daemon.c4_daemon import C4Daemon

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path
            d.state_machine = None

            from c4.daemon.c4_daemon import C4Daemon as RealDaemon
            d._get_all_done_impl_tasks = RealDaemon._get_all_done_impl_tasks.__get__(d)

            result = d._get_all_done_impl_tasks()
            assert result == []


class TestCheckpointWithEmptyRequiredTasks:
    """Test checkpoint creation when required_tasks is empty (applies to ALL tasks)."""

    @pytest.fixture
    def daemon_empty_required(self, tmp_path):
        """Create daemon with checkpoint that has empty required_tasks."""
        from c4.daemon.c4_daemon import C4Daemon
        from c4.models import CheckpointConfig

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path

            # Config with empty required_tasks (applies to ALL)
            d._config = C4Config(
                project_id="test",
                checkpoints=[
                    CheckpointConfig(
                        id="001",
                        description="Global checkpoint",
                        required_tasks=[],  # Empty = ALL tasks
                        required_validations=["lint"],
                        auto_approve=True,
                    )
                ],
                checkpoint_as_task=True,
            )

            # Tasks - all done
            d._tasks = {
                "T-001-0": Task(
                    id="T-001-0",
                    title="Task 1",
                    dod="...",
                    base_id="001",
                    version=0,
                    type=TaskType.IMPLEMENTATION,
                ),
                "T-002-0": Task(
                    id="T-002-0",
                    title="Task 2",
                    dod="...",
                    base_id="002",
                    version=0,
                    type=TaskType.IMPLEMENTATION,
                ),
                "R-001-0": Task(
                    id="R-001-0",
                    title="Review 1",
                    dod="...",
                    base_id="001",
                    version=0,
                    type=TaskType.REVIEW,
                    parent_id="T-001-0",
                    review_decision="APPROVE",
                ),
                "R-002-0": Task(
                    id="R-002-0",
                    title="Review 2",
                    dod="...",
                    base_id="002",
                    version=0,
                    type=TaskType.REVIEW,
                    parent_id="T-002-0",
                    review_decision="APPROVE",
                ),
            }

            d.state_machine = MagicMock()
            d.state_machine.state = C4State(
                project_id="test",
                queue=TaskQueue(
                    pending=[],
                    in_progress={},
                    done=["T-001-0", "T-002-0", "R-001-0", "R-002-0"],
                ),
                passed_checkpoints=[],
            )

            d.get_all_tasks = MagicMock(return_value=d._tasks)
            d.get_task = MagicMock(side_effect=lambda tid: d._tasks.get(tid))
            d.add_task = MagicMock()

            from c4.daemon.c4_daemon import C4Daemon as RealDaemon
            d._check_and_create_checkpoint_task = RealDaemon._check_and_create_checkpoint_task.__get__(d)
            d._find_tasks_by_pattern = RealDaemon._find_tasks_by_pattern.__get__(d)
            d._get_latest_review_for_impl = RealDaemon._get_latest_review_for_impl.__get__(d)
            d._get_all_done_impl_tasks = RealDaemon._get_all_done_impl_tasks.__get__(d)
            d._build_checkpoint_dod = RealDaemon._build_checkpoint_dod.__get__(d)
            d._task_exists = RealDaemon._task_exists.__get__(d)

        return d

    def test_creates_checkpoint_for_all_tasks(self, daemon_empty_required):
        """Create checkpoint when required_tasks is empty (matches ALL)."""
        completed_review = daemon_empty_required._tasks["R-002-0"]

        result = daemon_empty_required._check_and_create_checkpoint_task(completed_review)

        assert result is not None
        assert result.id == "CP-001"
        assert result.type == TaskType.CHECKPOINT
        # Should include ALL implementation tasks
        assert "T-001-0" in result.required_tasks
        assert "T-002-0" in result.required_tasks
        daemon_empty_required.add_task.assert_called_once()

    def test_no_checkpoint_if_not_all_approved(self, daemon_empty_required):
        """Don't create checkpoint if not all reviews are approved."""
        # Change one review to not approved
        daemon_empty_required._tasks["R-001-0"].review_decision = "REQUEST_CHANGES"

        completed_review = daemon_empty_required._tasks["R-002-0"]
        result = daemon_empty_required._check_and_create_checkpoint_task(completed_review)

        assert result is None
        daemon_empty_required.add_task.assert_not_called()
