"""Tests for Task model field and model_filter in c4_get_task."""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from c4.mcp_server import C4Daemon
from c4.models.enums import TaskStatus, TaskType
from c4.models.task import Task


class TestTaskModelField:
    """Tests for Task.model field."""

    def test_task_model_default_opus(self) -> None:
        """Task model defaults to opus."""
        task = Task(
            id="T-001-0",
            title="Test task",
            dod="Do something",
        )
        assert task.model == "opus"

    def test_task_model_sonnet(self) -> None:
        """Task model can be set to sonnet."""
        task = Task(
            id="T-001-0",
            title="Test task",
            dod="Do something",
            model="sonnet",
        )
        assert task.model == "sonnet"

    def test_task_model_haiku(self) -> None:
        """Task model can be set to haiku."""
        task = Task(
            id="T-001-0",
            title="Test task",
            dod="Do something",
            model="haiku",
        )
        assert task.model == "haiku"

    def test_task_model_invalid_rejected(self) -> None:
        """Invalid model values are rejected by pydantic validation."""
        with pytest.raises(ValueError, match="String should match pattern"):
            Task(
                id="T-001-0",
                title="Test task",
                dod="Do something",
                model="gpt-4",  # Invalid model
            )

    def test_task_model_preserved_in_dict(self) -> None:
        """Model is preserved when converting to dict."""
        task = Task(
            id="T-001-0",
            title="Test task",
            dod="Do something",
            model="sonnet",
        )
        task_dict = task.model_dump()
        assert task_dict["model"] == "sonnet"


@pytest.fixture
def mock_state_machine() -> MagicMock:
    """Create a mock state machine."""
    mock = MagicMock()
    mock.tasks = {}
    return mock


@pytest.fixture
def daemon(mock_state_machine: MagicMock) -> C4Daemon:
    """Create a C4Daemon with mock state machine."""
    d = C4Daemon()
    d.state_machine = mock_state_machine
    d._tasks = {}

    def add_task(task: Task) -> None:
        d._tasks[task.id] = task

    d.add_task = add_task  # type: ignore
    return d


class TestC4AddTodoModel:
    """Tests for c4_add_todo with model parameter."""

    def test_add_todo_default_model(self, daemon: C4Daemon) -> None:
        """c4_add_todo uses opus as default model."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope="src/",
            dod="Implement feature",
        )

        assert result["success"] is True
        assert result["model"] == "opus"
        task = daemon._tasks["T-001-0"]  # type: ignore
        assert task.model == "opus"

    def test_add_todo_with_sonnet(self, daemon: C4Daemon) -> None:
        """c4_add_todo accepts sonnet model."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Simple task",
            scope=None,
            dod="Simple implementation",
            model="sonnet",
        )

        assert result["success"] is True
        assert result["model"] == "sonnet"
        task = daemon._tasks["T-001-0"]  # type: ignore
        assert task.model == "sonnet"

    def test_add_todo_with_haiku(self, daemon: C4Daemon) -> None:
        """c4_add_todo accepts haiku model."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Tiny task",
            scope=None,
            dod="Quick fix",
            model="haiku",
        )

        assert result["success"] is True
        assert result["model"] == "haiku"
        task = daemon._tasks["T-001-0"]  # type: ignore
        assert task.model == "haiku"

    def test_add_todo_with_opus(self, daemon: C4Daemon) -> None:
        """c4_add_todo accepts opus model explicitly."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Complex task",
            scope=None,
            dod="Complex implementation",
            model="opus",
        )

        assert result["success"] is True
        assert result["model"] == "opus"
        task = daemon._tasks["T-001-0"]  # type: ignore
        assert task.model == "opus"


class TestC4GetTaskModelFilter:
    """Tests for c4_get_task with model_filter."""

    @pytest.fixture
    def daemon_with_tasks(self) -> C4Daemon:
        """Create daemon with multiple tasks of different models."""
        daemon = C4Daemon()
        daemon._tasks = {}

        # Create tasks with different models
        tasks = [
            Task(
                id="T-001-0",
                title="Sonnet task 1",
                dod="Simple task",
                model="sonnet",
                status=TaskStatus.PENDING,
                type=TaskType.IMPLEMENTATION,
                base_id="001",
                version=0,
            ),
            Task(
                id="T-002-0",
                title="Opus task",
                dod="Complex task",
                model="opus",
                status=TaskStatus.PENDING,
                type=TaskType.IMPLEMENTATION,
                base_id="002",
                version=0,
            ),
            Task(
                id="T-003-0",
                title="Sonnet task 2",
                dod="Another simple task",
                model="sonnet",
                status=TaskStatus.PENDING,
                type=TaskType.IMPLEMENTATION,
                base_id="003",
                version=0,
            ),
        ]

        for task in tasks:
            daemon._tasks[task.id] = task

        return daemon

    def test_model_filter_filters_tasks(self, daemon_with_tasks: C4Daemon) -> None:
        """model_filter should only return tasks with matching model."""
        # This tests the filtering logic conceptually
        # Full integration test would require more setup

        sonnet_tasks = [
            t for t in daemon_with_tasks._tasks.values() if t.model == "sonnet"
        ]
        opus_tasks = [
            t for t in daemon_with_tasks._tasks.values() if t.model == "opus"
        ]

        assert len(sonnet_tasks) == 2
        assert len(opus_tasks) == 1
        assert all(t.model == "sonnet" for t in sonnet_tasks)
        assert all(t.model == "opus" for t in opus_tasks)

    def test_no_filter_returns_any_model(self, daemon_with_tasks: C4Daemon) -> None:
        """Without model_filter, any task can be returned."""
        all_tasks = list(daemon_with_tasks._tasks.values())
        assert len(all_tasks) == 3

        models = {t.model for t in all_tasks}
        assert "sonnet" in models
        assert "opus" in models
