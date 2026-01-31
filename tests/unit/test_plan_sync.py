"""Tests for PlanFileSync - Claude Plan ↔ C4 Task synchronization."""

from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

import pytest

from c4.daemon.plan_sync import PlanFileSync
from c4.models import Task, TaskStatus

if TYPE_CHECKING:
    pass


@pytest.fixture
def temp_plan_dir(tmp_path: Path) -> Path:
    """Create a temporary plan directory."""
    plan_dir = tmp_path / "plans"
    plan_dir.mkdir(parents=True)
    return plan_dir


@pytest.fixture
def plan_sync(temp_plan_dir: Path) -> PlanFileSync:
    """Create PlanFileSync with temporary directory."""
    return PlanFileSync(plan_dir=temp_plan_dir)


@pytest.fixture
def sample_tasks() -> list[Task]:
    """Create sample tasks for testing."""
    return [
        Task(
            id="T-001-0",
            title="Implement feature A",
            dod="Feature A complete",
            status=TaskStatus.DONE,
        ),
        Task(
            id="T-002-0",
            title="Implement feature B",
            dod="Feature B complete",
            status=TaskStatus.PENDING,
        ),
        Task(
            id="T-003-0",
            title="Implement feature C",
            dod="Feature C complete",
            status=TaskStatus.IN_PROGRESS,
        ),
    ]


class TestPlanFileSync:
    """Test suite for PlanFileSync."""

    def test_get_plan_file_path(self, plan_sync: PlanFileSync) -> None:
        """Test plan file path generation."""
        path = plan_sync.get_plan_file("my-project")
        assert path.name == "c4-my-project.md"
        assert path.parent == plan_sync.plan_dir

    def test_generate_plan_file(
        self, plan_sync: PlanFileSync, sample_tasks: list[Task]
    ) -> None:
        """Test generating plan file from tasks."""
        plan_file = plan_sync.generate_plan_file("test-project", sample_tasks)

        assert plan_file.exists()
        content = plan_file.read_text()

        # Check header
        assert "# C4 Project: test-project" in content
        assert "Progress: 1/3 (33%)" in content

        # Check task checkboxes
        assert "- [x] T-001-0: Implement feature A" in content
        assert "- [ ] T-002-0: Implement feature B" in content
        assert "- [ ] T-003-0: Implement feature C" in content

    def test_generate_plan_file_empty_tasks(
        self, plan_sync: PlanFileSync
    ) -> None:
        """Test generating plan file with no tasks."""
        plan_file = plan_sync.generate_plan_file("empty-project", [])

        assert plan_file.exists()
        content = plan_file.read_text()

        assert "# C4 Project: empty-project" in content
        assert "Progress: 0/0 (0%)" in content

    def test_update_task_status_done(
        self, plan_sync: PlanFileSync, sample_tasks: list[Task]
    ) -> None:
        """Test updating task status to done."""
        plan_sync.generate_plan_file("test-project", sample_tasks)

        # Update T-002-0 to done
        result = plan_sync.update_task_status("test-project", "T-002-0", "done")

        assert result is True
        content = plan_sync.get_plan_file("test-project").read_text()
        assert "- [x] T-002-0: Implement feature B" in content

    def test_update_task_status_pending(
        self, plan_sync: PlanFileSync, sample_tasks: list[Task]
    ) -> None:
        """Test updating task status back to pending."""
        plan_sync.generate_plan_file("test-project", sample_tasks)

        # Update T-001-0 to pending (was done)
        result = plan_sync.update_task_status("test-project", "T-001-0", "pending")

        assert result is True
        content = plan_sync.get_plan_file("test-project").read_text()
        assert "- [ ] T-001-0: Implement feature A" in content

    def test_update_task_status_not_found(
        self, plan_sync: PlanFileSync, sample_tasks: list[Task]
    ) -> None:
        """Test updating non-existent task."""
        plan_sync.generate_plan_file("test-project", sample_tasks)

        result = plan_sync.update_task_status("test-project", "T-999-0", "done")

        assert result is False

    def test_update_task_status_no_file(
        self, plan_sync: PlanFileSync
    ) -> None:
        """Test updating task when plan file doesn't exist."""
        result = plan_sync.update_task_status("nonexistent", "T-001-0", "done")

        assert result is False

    def test_sync_from_plan_file_status_updates(
        self, plan_sync: PlanFileSync
    ) -> None:
        """Test detecting status changes from plan file."""
        # Create plan file with T-001-0 checked
        plan_file = plan_sync.get_plan_file("test-project")
        plan_file.parent.mkdir(parents=True, exist_ok=True)
        plan_file.write_text("""# C4 Project: test-project

## Tasks

- [x] T-001-0: Implement feature A
- [ ] T-002-0: Implement feature B
""")

        # C4 has T-001-0 as pending
        c4_tasks = {
            "T-001-0": Task(
                id="T-001-0",
                title="Implement feature A",
                dod="Done",
                status=TaskStatus.PENDING,
            ),
            "T-002-0": Task(
                id="T-002-0",
                title="Implement feature B",
                dod="Done",
                status=TaskStatus.PENDING,
            ),
        }

        result = plan_sync.sync_from_plan_file("test-project", c4_tasks)

        # Should detect T-001-0 needs update in C4
        assert len(result["status_updates"]) == 1
        assert result["status_updates"][0]["task_id"] == "T-001-0"
        assert result["status_updates"][0]["new_status"] == "done"
        assert result["status_updates"][0]["source"] == "plan"

    def test_sync_from_plan_file_plan_updates(
        self, plan_sync: PlanFileSync
    ) -> None:
        """Test detecting updates needed in plan file."""
        # Create plan file with T-001-0 unchecked
        plan_file = plan_sync.get_plan_file("test-project")
        plan_file.parent.mkdir(parents=True, exist_ok=True)
        plan_file.write_text("""# C4 Project: test-project

## Tasks

- [ ] T-001-0: Implement feature A
""")

        # C4 has T-001-0 as done
        c4_tasks = {
            "T-001-0": Task(
                id="T-001-0",
                title="Implement feature A",
                dod="Done",
                status=TaskStatus.DONE,
            ),
        }

        result = plan_sync.sync_from_plan_file("test-project", c4_tasks)

        # Should detect plan file needs update
        assert len(result["plan_updates"]) == 1
        assert result["plan_updates"][0]["task_id"] == "T-001-0"
        assert result["plan_updates"][0]["new_status"] == "done"

    def test_sync_from_plan_file_new_tasks(
        self, plan_sync: PlanFileSync
    ) -> None:
        """Test detecting new tasks in plan file."""
        # Create plan file with a task without ID
        plan_file = plan_sync.get_plan_file("test-project")
        plan_file.parent.mkdir(parents=True, exist_ok=True)
        plan_file.write_text("""# C4 Project: test-project

## Tasks

- [x] T-001-0: Existing task
- [ ] New feature implementation
- [x] Another new completed task
""")

        c4_tasks = {
            "T-001-0": Task(
                id="T-001-0",
                title="Existing task",
                dod="Done",
                status=TaskStatus.DONE,
            ),
        }

        result = plan_sync.sync_from_plan_file("test-project", c4_tasks)

        # Should detect 2 new tasks
        assert len(result["new_tasks"]) == 2
        assert result["new_tasks"][0]["title"] == "New feature implementation"
        assert result["new_tasks"][0]["status"] == "pending"
        assert result["new_tasks"][1]["title"] == "Another new completed task"
        assert result["new_tasks"][1]["status"] == "done"

    def test_sync_from_plan_file_no_file(
        self, plan_sync: PlanFileSync
    ) -> None:
        """Test sync when plan file doesn't exist."""
        result = plan_sync.sync_from_plan_file("nonexistent", {})

        assert result["new_tasks"] == []
        assert result["status_updates"] == []
        assert result["plan_updates"] == []

    def test_sync_bidirectional(
        self, plan_sync: PlanFileSync, sample_tasks: list[Task]
    ) -> None:
        """Test full bidirectional sync scenario."""
        # Generate initial plan file
        plan_sync.generate_plan_file("test-project", sample_tasks)

        # Manually modify plan file - mark T-002-0 as done
        plan_file = plan_sync.get_plan_file("test-project")
        content = plan_file.read_text()
        content = content.replace(
            "- [ ] T-002-0: Implement feature B",
            "- [x] T-002-0: Implement feature B"
        )
        # Add a new task
        content = content.replace(
            "- [ ] T-003-0: Implement feature C",
            "- [ ] T-003-0: Implement feature C\n- [ ] New manual task"
        )
        plan_file.write_text(content)

        # Create C4 tasks dict
        c4_tasks = {t.id: t for t in sample_tasks}

        # Sync from plan file
        result = plan_sync.sync_from_plan_file("test-project", c4_tasks)

        # T-002-0 should need C4 update (plan: done, C4: pending)
        assert any(
            u["task_id"] == "T-002-0" and u["new_status"] == "done"
            for u in result["status_updates"]
        )

        # New task should be detected
        assert any(t["title"] == "New manual task" for t in result["new_tasks"])

    def test_has_plan_file(
        self, plan_sync: PlanFileSync, sample_tasks: list[Task]
    ) -> None:
        """Test checking plan file existence."""
        assert plan_sync.has_plan_file("test-project") is False

        plan_sync.generate_plan_file("test-project", sample_tasks)

        assert plan_sync.has_plan_file("test-project") is True

    def test_delete_plan_file(
        self, plan_sync: PlanFileSync, sample_tasks: list[Task]
    ) -> None:
        """Test deleting plan file."""
        plan_sync.generate_plan_file("test-project", sample_tasks)
        assert plan_sync.has_plan_file("test-project") is True

        result = plan_sync.delete_plan_file("test-project")

        assert result is True
        assert plan_sync.has_plan_file("test-project") is False

    def test_delete_plan_file_not_exists(
        self, plan_sync: PlanFileSync
    ) -> None:
        """Test deleting non-existent plan file."""
        result = plan_sync.delete_plan_file("nonexistent")

        assert result is False
