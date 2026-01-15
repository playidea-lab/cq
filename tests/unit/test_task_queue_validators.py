"""Tests for TaskQueue defensive validators.

These validators handle corrupt data formats where full task objects
were stored instead of just task IDs, providing self-healing behavior.
"""

from c4.models.state import TaskQueue, _extract_task_id, _extract_worker_id


class TestExtractTaskId:
    """Test _extract_task_id helper function."""

    def test_string_passthrough(self) -> None:
        """Strings should pass through unchanged."""
        assert _extract_task_id("T-001") == "T-001"
        assert _extract_task_id("task-abc") == "task-abc"

    def test_dict_with_id(self) -> None:
        """Dicts with 'id' key should extract the ID."""
        assert _extract_task_id({"id": "T-001", "title": "Test"}) == "T-001"
        assert _extract_task_id({"id": "task-abc", "status": "pending"}) == "task-abc"

    def test_dict_without_id(self) -> None:
        """Dicts without 'id' key should convert to string."""
        result = _extract_task_id({"title": "Test"})
        assert isinstance(result, str)

    def test_other_types(self) -> None:
        """Other types should convert to string."""
        assert _extract_task_id(123) == "123"
        assert _extract_task_id(None) == "None"


class TestExtractWorkerId:
    """Test _extract_worker_id helper function."""

    def test_string_passthrough(self) -> None:
        """Strings should pass through unchanged."""
        assert _extract_worker_id("worker-123") == "worker-123"

    def test_dict_with_worker_id(self) -> None:
        """Dicts with worker ID fields should extract the ID."""
        assert _extract_worker_id({"worker_id": "w-1"}) == "w-1"
        assert _extract_worker_id({"assigned_to": "w-2"}) == "w-2"
        assert _extract_worker_id({"owner": "w-3"}) == "w-3"

    def test_dict_fallback(self) -> None:
        """Dicts without known keys should find first string value."""
        result = _extract_worker_id({"name": "worker-x", "count": 5})
        assert result == "worker-x"


class TestTaskQueueValidators:
    """Test TaskQueue field validators."""

    def test_normal_format(self) -> None:
        """Normal format (string IDs) should work."""
        queue = TaskQueue(
            pending=["T-001", "T-002"],
            in_progress={"T-003": "worker-1"},
            done=["T-004"],
        )
        assert queue.pending == ["T-001", "T-002"]
        assert queue.in_progress == {"T-003": "worker-1"}
        assert queue.done == ["T-004"]

    def test_corrupt_pending_with_dict_tasks(self) -> None:
        """Corrupt pending (dict tasks) should extract IDs."""
        queue = TaskQueue(
            pending=[
                {"id": "T-001", "title": "First task", "status": "pending"},
                {"id": "T-002", "title": "Second task"},
                "T-003",  # Mixed with normal string
            ],
            in_progress={},
            done=[],
        )
        assert queue.pending == ["T-001", "T-002", "T-003"]

    def test_corrupt_done_with_dict_tasks(self) -> None:
        """Corrupt done (dict tasks with commit_sha) should extract IDs."""
        queue = TaskQueue(
            pending=[],
            in_progress={},
            done=[
                {"id": "T-001", "commit_sha": "abc123", "status": "done"},
                {"id": "T-002", "commit_sha": "def456"},
            ],
        )
        assert queue.done == ["T-001", "T-002"]

    def test_corrupt_in_progress_with_dict_values(self) -> None:
        """Corrupt in_progress (dict values) should extract worker IDs."""
        queue = TaskQueue(
            pending=[],
            in_progress={
                "T-001": {"worker_id": "worker-1", "started_at": "2024-01-01"},
                "T-002": {"assigned_to": "worker-2"},
                "T-003": "worker-3",  # Normal string
            },
            done=[],
        )
        assert queue.in_progress == {
            "T-001": "worker-1",
            "T-002": "worker-2",
            "T-003": "worker-3",
        }

    def test_corrupt_in_progress_with_dict_keys(self) -> None:
        """Corrupt in_progress (dict keys) should extract task IDs."""
        # This is a more extreme corruption case
        queue_data = {
            "pending": [],
            "in_progress": {
                "T-001": "worker-1",  # Normal
            },
            "done": [],
        }
        queue = TaskQueue.model_validate(queue_data)
        assert "T-001" in queue.in_progress

    def test_empty_queue(self) -> None:
        """Empty queue should work."""
        queue = TaskQueue()
        assert queue.pending == []
        assert queue.in_progress == {}
        assert queue.done == []

    def test_invalid_types_handled_gracefully(self) -> None:
        """Invalid types should be handled gracefully."""
        # None values
        queue = TaskQueue(
            pending=None,  # type: ignore
            in_progress=None,  # type: ignore
            done=None,  # type: ignore
        )
        assert queue.pending == []
        assert queue.in_progress == {}
        assert queue.done == []


class TestTaskQueueFromJson:
    """Test TaskQueue validation from JSON-like data (simulating DB load)."""

    def test_realistic_corrupt_data(self) -> None:
        """Test with realistic corrupt data format from bug report."""
        # Simulating the actual corrupt data structure from the bug report
        corrupt_data = {
            "pending": [
                {
                    "id": "expert-integration",
                    "description": "Expert integration task",
                    "category": "interview-agent",
                },
                {
                    "id": "step-handlers",
                    "description": "Step handlers",
                },
            ],
            "in_progress": {
                "step-handlers": {
                    "id": "step-handlers",
                    "assigned_to": "worker-abc",
                    "started_at": "2025-01-15T20:55:45.100064",
                },
            },
            "done": [
                {
                    "id": "seed-schema",
                    "description": "Seed schema",
                    "commit_sha": "e88fb0e",
                },
            ],
        }

        # This should NOT raise ValidationError anymore
        queue = TaskQueue.model_validate(corrupt_data)

        # Check that IDs were extracted correctly
        assert "expert-integration" in queue.pending
        assert "step-handlers" in queue.pending
        assert "seed-schema" in queue.done

        # in_progress should have extracted worker_id
        assert "step-handlers" in queue.in_progress
        assert queue.in_progress["step-handlers"] == "worker-abc"
