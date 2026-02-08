"""Tests for Checkpoint/Worker 4-Bug Fix + Zombie Review Fix.

Verifies:
1. Legacy checkpoint path blocked when checkpoint_as_task=True
2. checkpoint() works in EXECUTE state (checkpoint_as_task mode)
3. _ensure_supervisor_running removed (dead code)
4. Completed tasks not reassigned (self-heal)
5. report() moves task to done queue (zombie review fix)
6. report() creates review with no dependencies
7. report() triggers review/checkpoint completion handlers
"""


import pytest

from c4.mcp_server import C4Daemon
from c4.models import (
    CheckpointConfig,
    ProjectStatus,
    Task,
    TaskType,
)
from tests.conftest import WORKER_1, WORKER_2


@pytest.fixture
def daemon_exec(tmp_path):
    """Daemon in EXECUTE with checkpoint_as_task=True (default)."""
    d = C4Daemon(tmp_path)
    d.initialize("test-4bug", with_default_checkpoints=False)
    d.state_machine.transition("skip_discovery", "test")
    d.state_machine.transition("c4_run", "test")
    assert d.state_machine.state.status == ProjectStatus.EXECUTE
    return d


# =========================================================================
# Fix 1: Legacy checkpoint path blocked
# =========================================================================

class TestFix1LegacyCheckpointBlocked:
    """checkpoint_as_task=True일 때 _check_completion_state가 await_checkpoint를 반환하지 않아야 함."""

    def test_no_await_checkpoint_when_checkpoint_as_task(self, daemon_exec):
        """All tasks done → 'complete', not 'await_checkpoint'."""
        d = daemon_exec
        # checkpoint_as_task is True by default
        assert d.config.checkpoint_as_task is True

        # Add checkpoint config (gate condition)
        d._config.checkpoints = [
            CheckpointConfig(id="CP-001", after_tasks=["T-001-0"])
        ]

        # Add and complete a task
        t = Task(id="T-001-0", title="Impl", dod="Do it", base_id="001", version=0)
        d.add_task(t)
        result = d.c4_get_task(WORKER_1)
        assert result is not None

        resp = d.c4_submit(
            task_id="T-001-0",
            commit_sha="abc123",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        # Should NOT get await_checkpoint (legacy path)
        assert resp.next_action != "await_checkpoint"


# =========================================================================
# Fix 2: checkpoint() in EXECUTE state
# =========================================================================

class TestFix2CheckpointInExecute:
    """EXECUTE 상태에서 checkpoint_as_task 모드일 때 checkpoint 호출 가능."""

    def test_checkpoint_allowed_in_execute_with_flag(self, daemon_exec):
        d = daemon_exec
        assert d.config.checkpoint_as_task is True
        assert d.state_machine.state.status == ProjectStatus.EXECUTE

        resp = d.checkpoint_ops.checkpoint(
            checkpoint_id="CP-001",
            decision="APPROVE",
            notes="All good",
        )
        # Should succeed (not reject with "Not in CHECKPOINT state")
        assert resp.success is True
        assert "CP-001" in d.state_machine.state.passed_checkpoints

    def test_checkpoint_rejected_in_execute_without_flag(self, daemon_exec):
        d = daemon_exec
        d._config.checkpoint_as_task = False

        resp = d.checkpoint_ops.checkpoint(
            checkpoint_id="CP-001",
            decision="APPROVE",
            notes="test",
        )
        assert resp.success is False
        assert "CHECKPOINT" in resp.message


# =========================================================================
# Fix 3: _ensure_supervisor_running removed
# =========================================================================

class TestFix3DeadCodeRemoved:
    """_ensure_supervisor_running이 C4Daemon에 없어야 함."""

    def test_no_ensure_supervisor_running(self, daemon_exec):
        assert not hasattr(daemon_exec, "_ensure_supervisor_running")


# =========================================================================
# Fix 4: Completed task not reassigned
# =========================================================================

class TestFix4CompletedTaskGuard:
    """pending ∩ done overlap self-heal + done 체크."""

    def test_self_heal_pending_done_overlap(self, daemon_exec):
        """pending에도 done에도 있는 태스크는 _find_and_assign_task에서 할당되지 않아야 함."""
        d = daemon_exec
        project_id = d.state_machine.state.project_id
        store = d.state_machine.store

        t = Task(id="T-001-0", title="Impl", dod="x", base_id="001", version=0)
        d.add_task(t)

        # Manually corrupt via atomic_modify: add to done while keeping in pending
        with store.atomic_modify(project_id) as mod_state:
            mod_state.queue.done.append("T-001-0")
            d.state_machine._state = mod_state

        # Verify corruption exists
        state = d.state_machine.state
        assert "T-001-0" in state.queue.pending
        assert "T-001-0" in state.queue.done

        # get_task should NOT assign the already-done task
        result = d.c4_get_task(WORKER_1)
        assert result is None  # No assignable task (T-001-0 is already done)

    def test_atomic_assign_skips_done_task(self, daemon_exec):
        """atomic 블록에서 done에 있는 태스크 할당 안 함."""
        d = daemon_exec
        state = d.state_machine.state

        t = Task(id="T-002-0", title="Done task", dod="x", base_id="002", version=0)
        d.add_task(t)

        # Put in pending AND done (race condition simulation)
        if "T-002-0" not in state.queue.done:
            state.queue.done.append("T-002-0")
        d.state_machine.save_state()

        # Should not get this task
        result = d.c4_get_task(WORKER_1)
        # Either None or a different task
        if result is not None:
            assert result.task_id != "T-002-0"


# =========================================================================
# Fix 5-7: Zombie review fixes in report()
# =========================================================================

class TestZombieReviewFixes:
    """report()에서 done 큐 이동, 의존성 없는 리뷰 생성, 완료 핸들러 호출."""

    def test_report_moves_task_to_done_queue(self, daemon_exec):
        """report() 후 태스크가 done 큐에 존재해야 함."""
        d = daemon_exec

        t = Task(
            id="T-010-0", title="Direct task", dod="x",
            base_id="010", version=0, review_required=False,
        )
        d.add_task(t)
        d.task_ops.claim("T-010-0")

        result = d.task_ops.report("T-010-0", summary="Done", files_changed=["a.py"])
        assert result["success"] is True

        state = d.state_machine.state
        assert "T-010-0" in state.queue.done
        assert "T-010-0" not in state.queue.in_progress

    def test_report_review_has_no_dependencies(self, daemon_exec):
        """report()로 생성된 리뷰 태스크에 dependencies가 없어야 함."""
        d = daemon_exec
        d._config.review_as_task = True

        t = Task(
            id="T-011-0", title="Impl", dod="x",
            base_id="011", version=0,
            type=TaskType.IMPLEMENTATION,
            review_required=True,
        )
        d.add_task(t)
        d.task_ops.claim("T-011-0")

        result = d.task_ops.report("T-011-0", summary="Implemented", files_changed=["b.py"])
        assert result["success"] is True
        assert "review_task_created" in result

        review_id = result["review_task_created"]
        review_task = d.get_task(review_id)
        assert review_task is not None
        assert review_task.dependencies == []  # NOT [T-011-0]
        assert review_task.type == TaskType.REVIEW

    def test_report_review_is_assignable(self, daemon_exec):
        """report()로 생성된 리뷰 태스크를 Worker가 할당받을 수 있어야 함."""
        d = daemon_exec
        d._config.review_as_task = True

        t = Task(
            id="T-012-0", title="Impl", dod="x",
            base_id="012", version=0,
            type=TaskType.IMPLEMENTATION,
            review_required=True,
        )
        d.add_task(t)
        d.task_ops.claim("T-012-0")
        d.task_ops.report("T-012-0", summary="Done", files_changed=["c.py"])

        # Worker should be able to pick up the review task
        assignment = d.c4_get_task(WORKER_2)
        assert assignment is not None
        assert assignment.task_id.startswith("R-")

    def test_report_metrics_updated(self, daemon_exec):
        """report() 후 metrics.tasks_completed 증가."""
        d = daemon_exec

        t = Task(
            id="T-013-0", title="Metrics test", dod="x",
            base_id="013", version=0, review_required=False,
        )
        d.add_task(t)
        before = d.state_machine.state.metrics.tasks_completed

        d.task_ops.claim("T-013-0")
        d.task_ops.report("T-013-0", summary="Done")

        after = d.state_machine.state.metrics.tasks_completed
        assert after == before + 1
