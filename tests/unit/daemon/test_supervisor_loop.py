"""Tests for SupervisorLoop git event processing."""

import json
from unittest.mock import MagicMock

import pytest

from c4.daemon.supervisor_loop import GitCommitEvent, SupervisorLoop


class TestGitCommitEvent:
    """Tests for GitCommitEvent class."""

    def test_parse_complete_event(self):
        """Should parse a complete git commit event."""
        data = {
            "type": "git_commit",
            "sha": "abc123def456",
            "task_id": "T-001-0",
            "files": "src/main.py,tests/test_main.py",
            "timestamp": "2026-01-30T10:00:00Z",
        }

        event = GitCommitEvent.model_validate(data)

        assert event.type == "git_commit"
        assert event.sha == "abc123def456"
        assert event.task_id == "T-001-0"
        assert event.files == ["src/main.py", "tests/test_main.py"]
        assert event.timestamp == "2026-01-30T10:00:00Z"

    def test_parse_event_with_null_task_id(self):
        """Should handle null task_id gracefully."""
        data = {
            "type": "git_commit",
            "sha": "abc123",
            "task_id": None,
            "files": "README.md",
            "timestamp": "2026-01-30T10:00:00Z",
        }

        event = GitCommitEvent.model_validate(data)

        assert event.task_id is None
        assert event.files == ["README.md"]

    def test_parse_event_with_empty_files(self):
        """Should handle empty files string."""
        data = {
            "type": "git_commit",
            "sha": "abc123",
            "task_id": None,
            "files": "",
            "timestamp": "2026-01-30T10:00:00Z",
        }

        event = GitCommitEvent.model_validate(data)

        assert event.files == []

    def test_parse_minimal_event(self):
        """Should handle minimal event data with defaults."""
        data = {}

        event = GitCommitEvent.model_validate(data)

        assert event.type == "git_commit"
        assert event.sha == ""
        assert event.task_id is None
        assert event.files == []
        assert event.timestamp == ""


class TestSupervisorLoopGitEvents:
    """Tests for SupervisorLoop._process_git_events method."""

    @pytest.fixture
    def mock_daemon(self, tmp_path):
        """Create a mock C4Daemon."""
        daemon = MagicMock()
        daemon.root = tmp_path
        daemon.state_machine = MagicMock()
        daemon.state_machine.emit_event = MagicMock()
        daemon.trigger_lsp_reindex = MagicMock()
        return daemon

    @pytest.fixture
    def events_dir(self, tmp_path):
        """Create events directory."""
        events = tmp_path / ".c4" / "events"
        events.mkdir(parents=True)
        return events

    @pytest.fixture
    def supervisor_loop(self, mock_daemon):
        """Create SupervisorLoop instance."""
        return SupervisorLoop(mock_daemon)

    @pytest.mark.asyncio
    async def test_process_git_events_no_events_dir(self, mock_daemon):
        """Should return False when events directory doesn't exist."""
        loop = SupervisorLoop(mock_daemon)

        result = await loop._process_git_events()

        assert result is False

    @pytest.mark.asyncio
    async def test_process_git_events_empty_dir(self, supervisor_loop, events_dir):
        """Should return False when no event files exist."""
        result = await supervisor_loop._process_git_events()

        assert result is False

    @pytest.mark.asyncio
    async def test_process_git_events_single_event(
        self, supervisor_loop, events_dir, mock_daemon
    ):
        """Should process a single git event file."""
        event_data = {
            "type": "git_commit",
            "sha": "abc123def",
            "task_id": "T-001-0",
            "files": "src/main.py",
            "timestamp": "2026-01-30T10:00:00Z",
        }
        event_file = events_dir / "git-abc123d.json"
        event_file.write_text(json.dumps(event_data))

        result = await supervisor_loop._process_git_events()

        assert result is True
        # Event file should be deleted after processing
        assert not event_file.exists()
        # emit_event should be called
        mock_daemon.state_machine.emit_event.assert_called_once()

    @pytest.mark.asyncio
    async def test_process_git_events_triggers_lsp_reindex(
        self, supervisor_loop, events_dir, mock_daemon
    ):
        """Should trigger LSP reindex for Python files."""
        event_data = {
            "type": "git_commit",
            "sha": "abc123def",
            "task_id": None,
            "files": "src/main.py,src/utils.py,README.md",
            "timestamp": "2026-01-30T10:00:00Z",
        }
        event_file = events_dir / "git-abc123d.json"
        event_file.write_text(json.dumps(event_data))

        await supervisor_loop._process_git_events()

        # Should call trigger_lsp_reindex with only Python files
        mock_daemon.trigger_lsp_reindex.assert_called_once_with(
            ["src/main.py", "src/utils.py"]
        )

    @pytest.mark.asyncio
    async def test_process_git_events_no_lsp_for_non_python(
        self, supervisor_loop, events_dir, mock_daemon
    ):
        """Should not trigger LSP reindex when no Python files changed."""
        event_data = {
            "type": "git_commit",
            "sha": "abc123def",
            "task_id": None,
            "files": "README.md,package.json",
            "timestamp": "2026-01-30T10:00:00Z",
        }
        event_file = events_dir / "git-abc123d.json"
        event_file.write_text(json.dumps(event_data))

        await supervisor_loop._process_git_events()

        # Should not call trigger_lsp_reindex
        mock_daemon.trigger_lsp_reindex.assert_not_called()

    @pytest.mark.asyncio
    async def test_process_git_events_invalid_json(
        self, supervisor_loop, events_dir, mock_daemon
    ):
        """Should handle invalid JSON gracefully and delete file."""
        event_file = events_dir / "git-invalid.json"
        event_file.write_text("not valid json")

        result = await supervisor_loop._process_git_events()

        # Should still return False (no valid events processed)
        assert result is False
        # Corrupted file should be deleted
        assert not event_file.exists()

    @pytest.mark.asyncio
    async def test_process_git_events_multiple_events(
        self, supervisor_loop, events_dir, mock_daemon
    ):
        """Should process multiple event files in order."""
        for i, sha in enumerate(["aaa", "bbb", "ccc"]):
            event_data = {
                "type": "git_commit",
                "sha": sha * 3,
                "task_id": f"T-00{i}-0",
                "files": f"file{i}.py",
                "timestamp": "2026-01-30T10:00:00Z",
            }
            event_file = events_dir / f"git-{sha}.json"
            event_file.write_text(json.dumps(event_data))

        result = await supervisor_loop._process_git_events()

        assert result is True
        # All event files should be deleted
        assert list(events_dir.glob("git-*.json")) == []
        # emit_event should be called 3 times
        assert mock_daemon.state_machine.emit_event.call_count == 3

    @pytest.mark.asyncio
    async def test_process_git_events_no_state_machine(self, mock_daemon, events_dir):
        """Should return False when state_machine is None."""
        mock_daemon.state_machine = None
        loop = SupervisorLoop(mock_daemon)

        # Create an event file
        event_file = events_dir / "git-test.json"
        event_file.write_text('{"type": "git_commit", "sha": "abc"}')

        result = await loop._process_git_events()

        assert result is False
        # File should NOT be deleted (no processing occurred)
        assert event_file.exists()
