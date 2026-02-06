"""Tests for @c4_track decorator and LocalTracker."""

import os

import pytest

from c4.models.task import ExecutionStats
from c4.tracker.decorator import LocalTracker, _StdoutCapture, c4_track
from c4.tracker.reviewer import _build_review_prompt, review_experiment
from c4.tracker.snapshot import SnapshotManager


class TestC4TrackDecorator:
    def test_basic_decoration(self):
        @c4_track(task_id="T-001-0", capture_git=False)
        def add(a, b):
            return a + b

        result = add(1, 2)
        assert result == 3
        assert add._last_stats is not None
        assert add._last_task_id == "T-001-0"
        assert add._last_stats.run_time_sec >= 0

    def test_stdout_capture(self):
        @c4_track(task_id="T-002-0", capture_git=False, capture_code=False)
        def train():
            print("epoch: 1, loss: 0.5")
            print("epoch: 2, loss: 0.3")
            return "done"

        result = train()
        assert result == "done"
        stats = train._last_stats
        assert "loss" in stats.metrics
        assert stats.metrics["loss"] == 0.3  # Last value wins

    def test_no_stdout_capture(self):
        @c4_track(task_id="T-003-0", capture_stdout=False, capture_git=False, capture_code=False)
        def func():
            print("loss: 0.5")
            return 42

        result = func()
        assert result == 42
        assert func._last_stats.metrics == {}

    def test_code_analysis(self):
        @c4_track(task_id="T-004-0", capture_git=False, capture_stdout=False)
        def ml_func():
            x = [1, 2, 3]
            return sum(x)

        ml_func()
        features = ml_func._code_features
        assert isinstance(features, dict)

    def test_env_var_task_id(self, monkeypatch):
        monkeypatch.setenv("C4_TASK_ID", "T-ENV-001")

        @c4_track(capture_git=False, capture_code=False, capture_stdout=False)
        def func():
            return True

        func()
        assert func._last_task_id == "T-ENV-001"

    def test_default_task_id(self):
        @c4_track(capture_git=False, capture_code=False, capture_stdout=False)
        def func():
            pass

        # Remove env var if present
        os.environ.pop("C4_TASK_ID", None)
        func()
        assert func._last_task_id == "unknown"

    def test_execution_stats_type(self):
        @c4_track(task_id="T-005-0", capture_git=False, capture_code=False, capture_stdout=False)
        def func():
            return 1

        func()
        assert isinstance(func._last_stats, ExecutionStats)

    def test_exception_still_captures(self):
        @c4_track(task_id="T-006-0", capture_git=False, capture_code=False, capture_stdout=False)
        def bad_func():
            raise ValueError("test error")

        with pytest.raises(ValueError, match="test error"):
            bad_func()

        assert bad_func._last_stats is not None
        assert bad_func._last_stats.run_time_sec >= 0

    def test_env_context_captured(self):
        @c4_track(task_id="T-007-0", capture_git=False, capture_code=False, capture_stdout=False)
        def func():
            return True

        func()
        env = func._last_stats.env_context
        assert "python_version" in env
        assert "platform" in env


class TestStdoutCapture:
    def test_captures_lines(self, capsys):
        import io

        original = io.StringIO()
        capture = _StdoutCapture(original)
        capture.write("hello\n")
        capture.write("world\n")
        lines = capture.captured_lines
        assert len(lines) == 2
        assert lines[0] == "hello"

    def test_passthrough(self):
        import io

        original = io.StringIO()
        capture = _StdoutCapture(original)
        capture.write("test output\n")
        assert "test output" in original.getvalue()

    def test_partial_line(self):
        import io

        original = io.StringIO()
        capture = _StdoutCapture(original)
        capture.write("partial")
        capture.write(" line\n")
        lines = capture.captured_lines
        assert len(lines) == 1
        assert lines[0] == "partial line"


class TestLocalTracker:
    def test_start_and_end_run(self):
        tracker = LocalTracker()
        run_id = tracker.start_run("T-001-0", {"algorithm": "rf"})
        assert run_id.startswith("run-T-001-0-")

        tracker.log_metrics(run_id, {"loss": 0.5})
        tracker.log_metrics(run_id, {"loss": 0.3, "accuracy": 0.9})

        run = tracker.get_run(run_id)
        assert run is not None
        assert run["metrics"]["loss"] == 0.3
        assert run["metrics"]["accuracy"] == 0.9

        tracker.end_run(run_id)
        run = tracker.get_run(run_id)
        assert "end_time" in run

    def test_get_nonexistent_run(self):
        tracker = LocalTracker()
        assert tracker.get_run("nonexistent") is None


class TestReviewer:
    def test_build_review_prompt(self):
        prompt = _build_review_prompt(
            code_features={"imports": ["sklearn", "numpy"], "algorithm": "RandomForest"},
            metrics={"accuracy": 0.92, "loss": 0.08},
            data_profile={"X_train": {"shape": [1000, 10], "dtype": "float64"}},
            git_context={"commit_sha": "abc12345", "branch": "main", "dirty": False},
        )
        assert "RandomForest" in prompt
        assert "accuracy" in prompt
        assert "X_train" in prompt
        assert "main" in prompt

    def test_review_experiment_returns_none(self):
        # LLM integration deferred - should return None
        result = review_experiment(
            code_features={},
            metrics={"loss": 0.5},
            data_profile={},
            git_context={},
        )
        assert result is None


class TestSnapshotManager:
    def test_disabled_by_default(self, tmp_path):
        sm = SnapshotManager(snapshot_dir=tmp_path / "snap", interval=0)
        assert sm.enabled is False
        assert sm.maybe_snapshot(100, {"loss": 0.5}) is False

    def test_snapshot_at_interval(self, tmp_path):
        sm = SnapshotManager(snapshot_dir=tmp_path / "snap", interval=10)
        assert sm.enabled is True

        assert sm.maybe_snapshot(0, {"loss": 1.0}) is True
        assert sm.maybe_snapshot(5, {"loss": 0.8}) is False
        assert sm.maybe_snapshot(10, {"loss": 0.5}) is True
        assert sm.maybe_snapshot(15, {"loss": 0.4}) is False
        assert sm.maybe_snapshot(20, {"loss": 0.3}) is True

        snapshots = sm.list_snapshots()
        assert len(snapshots) == 3
        assert snapshots[0]["step"] == 0
        assert snapshots[2]["metrics"]["loss"] == 0.3

    def test_snapshot_persistence(self, tmp_path):
        snap_dir = tmp_path / "snap"
        sm = SnapshotManager(snapshot_dir=snap_dir, interval=1)
        sm.maybe_snapshot(0, {"loss": 1.0})
        sm.maybe_snapshot(1, {"loss": 0.5})

        # Load from disk
        sm2 = SnapshotManager(snapshot_dir=snap_dir, interval=1)
        loaded = sm2.load_snapshots()
        assert len(loaded) == 2
        assert loaded[0]["metrics"]["loss"] == 1.0
        assert loaded[1]["metrics"]["loss"] == 0.5

    def test_load_empty_dir(self, tmp_path):
        sm = SnapshotManager(snapshot_dir=tmp_path / "nonexistent", interval=1)
        assert sm.load_snapshots() == []
