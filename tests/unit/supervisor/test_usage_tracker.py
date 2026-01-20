"""Unit tests for UsageTracker."""

import json
from datetime import datetime
from pathlib import Path

from c4.supervisor.usage_tracker import (
    UsageRecord,
    UsageSummary,
    UsageTracker,
    create_usage_tracker,
)


class TestUsageRecord:
    """Tests for UsageRecord dataclass."""

    def test_create_record(self):
        """Test creating a usage record."""
        record = UsageRecord(
            timestamp=datetime(2025, 1, 20, 10, 0, 0),
            model="claude-sonnet-4-20250514",
            input_tokens=1000,
            output_tokens=500,
            total_tokens=1500,
            cost=0.0105,
        )
        assert record.model == "claude-sonnet-4-20250514"
        assert record.input_tokens == 1000
        assert record.output_tokens == 500
        assert record.total_tokens == 1500
        assert record.cost == 0.0105

    def test_to_dict(self):
        """Test serialization to dict."""
        record = UsageRecord(
            timestamp=datetime(2025, 1, 20, 10, 0, 0),
            model="claude-sonnet-4-20250514",
            input_tokens=1000,
            output_tokens=500,
            total_tokens=1500,
            cost=0.0105,
            request_id="req-123",
            metadata={"task": "review"},
        )
        data = record.to_dict()
        assert data["model"] == "claude-sonnet-4-20250514"
        assert data["input_tokens"] == 1000
        assert data["request_id"] == "req-123"
        assert data["metadata"] == {"task": "review"}

    def test_from_dict(self):
        """Test deserialization from dict."""
        data = {
            "timestamp": "2025-01-20T10:00:00",
            "model": "claude-sonnet-4-20250514",
            "input_tokens": 1000,
            "output_tokens": 500,
            "total_tokens": 1500,
            "cost": 0.0105,
        }
        record = UsageRecord.from_dict(data)
        assert record.model == "claude-sonnet-4-20250514"
        assert record.input_tokens == 1000
        assert record.timestamp == datetime(2025, 1, 20, 10, 0, 0)

    def test_roundtrip(self):
        """Test serialization roundtrip."""
        original = UsageRecord(
            timestamp=datetime(2025, 1, 20, 10, 0, 0),
            model="claude-sonnet-4-20250514",
            input_tokens=1000,
            output_tokens=500,
            total_tokens=1500,
            cost=0.0105,
            request_id="req-123",
            metadata={"key": "value"},
        )
        data = original.to_dict()
        restored = UsageRecord.from_dict(data)
        assert restored.model == original.model
        assert restored.input_tokens == original.input_tokens
        assert restored.cost == original.cost


class TestUsageSummary:
    """Tests for UsageSummary dataclass."""

    def test_to_dict(self):
        """Test serialization to dict."""
        summary = UsageSummary(
            total_requests=5,
            total_input_tokens=5000,
            total_output_tokens=2500,
            total_tokens=7500,
            total_cost=0.05,
            by_model={"claude-sonnet-4-20250514": {"requests": 5}},
            first_request=datetime(2025, 1, 20, 10, 0, 0),
            last_request=datetime(2025, 1, 20, 12, 0, 0),
        )
        data = summary.to_dict()
        assert data["total_requests"] == 5
        assert data["total_tokens"] == 7500
        assert data["total_cost"] == 0.05

    def test_empty_summary(self):
        """Test empty summary serialization."""
        summary = UsageSummary(
            total_requests=0,
            total_input_tokens=0,
            total_output_tokens=0,
            total_tokens=0,
            total_cost=0.0,
            by_model={},
            first_request=None,
            last_request=None,
        )
        data = summary.to_dict()
        assert data["first_request"] is None
        assert data["last_request"] is None


class TestUsageTracker:
    """Tests for UsageTracker class."""

    def test_record_usage(self):
        """Test recording usage."""
        tracker = UsageTracker()
        record = tracker.record_usage(
            model="claude-sonnet-4-20250514",
            input_tokens=1000,
            output_tokens=500,
        )
        assert record.model == "claude-sonnet-4-20250514"
        assert record.input_tokens == 1000
        assert record.output_tokens == 500
        assert record.total_tokens == 1500
        # Cost should be estimated
        assert record.cost is not None
        assert record.cost > 0

    def test_record_usage_with_cost(self):
        """Test recording usage with explicit cost."""
        tracker = UsageTracker()
        record = tracker.record_usage(
            model="claude-sonnet-4-20250514",
            input_tokens=1000,
            output_tokens=500,
            cost=0.05,  # Explicit cost
        )
        assert record.cost == 0.05

    def test_record_usage_with_metadata(self):
        """Test recording usage with metadata."""
        tracker = UsageTracker()
        record = tracker.record_usage(
            model="claude-sonnet-4-20250514",
            input_tokens=1000,
            output_tokens=500,
            request_id="req-123",
            metadata={"task": "review"},
        )
        assert record.request_id == "req-123"
        assert record.metadata == {"task": "review"}

    def test_session_records(self):
        """Test session records tracking."""
        tracker = UsageTracker()
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500)
        tracker.record_usage("claude-sonnet-4-20250514", 2000, 1000)

        records = tracker.session_records
        assert len(records) == 2
        assert records[0].input_tokens == 1000
        assert records[1].input_tokens == 2000

    def test_session_cost(self):
        """Test session cost calculation."""
        tracker = UsageTracker()
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.01)
        tracker.record_usage("claude-sonnet-4-20250514", 2000, 1000, cost=0.02)

        assert tracker.session_cost == 0.03

    def test_session_tokens(self):
        """Test session token calculation."""
        tracker = UsageTracker()
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500)
        tracker.record_usage("claude-sonnet-4-20250514", 2000, 1000)

        assert tracker.session_tokens == 4500  # 1500 + 3000

    def test_get_session_summary(self):
        """Test getting session summary."""
        tracker = UsageTracker()
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.01)
        tracker.record_usage("claude-3-5-haiku-20241022", 2000, 1000, cost=0.02)

        summary = tracker.get_session_summary()
        assert summary.total_requests == 2
        assert summary.total_tokens == 4500
        assert summary.total_cost == 0.03
        assert "claude-sonnet-4-20250514" in summary.by_model
        assert "claude-3-5-haiku-20241022" in summary.by_model

    def test_empty_session_summary(self):
        """Test empty session summary."""
        tracker = UsageTracker()
        summary = tracker.get_session_summary()
        assert summary.total_requests == 0
        assert summary.total_tokens == 0
        assert summary.total_cost == 0.0

    def test_reset_session(self):
        """Test resetting session."""
        tracker = UsageTracker()
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500)
        tracker.record_usage("claude-sonnet-4-20250514", 2000, 1000)

        # Reset and get summary
        summary = tracker.reset_session()
        assert summary.total_requests == 2
        assert len(tracker.session_records) == 0
        assert tracker.session_cost == 0.0

    def test_budget_tracking(self):
        """Test budget tracking."""
        callback_called = []

        def budget_callback(current, budget, percentage):
            callback_called.append((current, budget, percentage))

        tracker = UsageTracker(budget=0.05, budget_warning_threshold=0.5)
        tracker.set_budget_callback(budget_callback)

        # Below threshold - no callback
        tracker.record_usage("claude-sonnet-4-20250514", 100, 50, cost=0.01)
        assert len(callback_called) == 0

        # Above threshold - callback triggered
        tracker.record_usage("claude-sonnet-4-20250514", 100, 50, cost=0.02)
        assert len(callback_called) == 1
        assert callback_called[0][2] >= 0.5  # percentage >= 50%

    def test_budget_exceeded(self):
        """Test budget exceeded notification."""
        callback_called = []

        def budget_callback(current, budget, percentage):
            callback_called.append(percentage)

        tracker = UsageTracker(budget=0.02, budget_warning_threshold=0.8)
        tracker.set_budget_callback(budget_callback)

        # Exceed budget
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.03)
        assert len(callback_called) == 1
        assert callback_called[0] >= 1.0  # 150%

    def test_format_summary(self):
        """Test formatting summary as string."""
        tracker = UsageTracker(budget=1.0)
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.01)

        summary_str = tracker.format_summary()
        assert "Usage Summary" in summary_str
        assert "Total Requests: 1" in summary_str
        assert "Budget:" in summary_str


class TestUsageTrackerPersistence:
    """Tests for UsageTracker persistence."""

    def test_save_report(self, tmp_path: Path):
        """Test saving usage report."""
        tracker = UsageTracker()
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.01)

        report_path = tmp_path / "report.json"
        tracker.save_report(report_path)

        assert report_path.exists()
        data = json.loads(report_path.read_text())
        assert "summary" in data
        assert "records" in data
        assert len(data["records"]) == 1

    def test_save_report_session_only(self, tmp_path: Path):
        """Test saving session-only report."""
        tracker = UsageTracker()
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.01)

        report_path = tmp_path / "report.json"
        tracker.save_report(report_path, session_only=True)

        data = json.loads(report_path.read_text())
        assert data["summary"]["total_requests"] == 1

    def test_save_report_without_records(self, tmp_path: Path):
        """Test saving report without individual records."""
        tracker = UsageTracker()
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.01)

        report_path = tmp_path / "report.json"
        tracker.save_report(report_path, include_records=False)

        data = json.loads(report_path.read_text())
        assert "records" not in data

    def test_persistent_file(self, tmp_path: Path):
        """Test persistent file storage."""
        persistent_file = tmp_path / "usage.json"

        # Create tracker and record usage
        tracker1 = UsageTracker(persistent_file=persistent_file)
        tracker1.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.01)

        # File should be created
        assert persistent_file.exists()

        # Create new tracker with same file
        tracker2 = UsageTracker(persistent_file=persistent_file)
        # Should load persistent records
        assert tracker2.total_tokens == 1500
        assert tracker2.total_cost == 0.01
        # Session should be empty
        assert tracker2.session_tokens == 0

    def test_persistent_accumulation(self, tmp_path: Path):
        """Test persistent accumulation across sessions."""
        persistent_file = tmp_path / "usage.json"

        # Session 1
        tracker1 = UsageTracker(persistent_file=persistent_file)
        tracker1.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.01)

        # Session 2
        tracker2 = UsageTracker(persistent_file=persistent_file)
        tracker2.record_usage("claude-sonnet-4-20250514", 2000, 1000, cost=0.02)

        # Total should accumulate
        assert tracker2.total_cost == 0.03
        assert tracker2.total_tokens == 4500


class TestCreateUsageTracker:
    """Tests for create_usage_tracker factory function."""

    def test_create_basic(self):
        """Test creating basic tracker."""
        tracker = create_usage_tracker()
        assert tracker.budget is None
        assert tracker.persistent_file is None

    def test_create_with_budget(self):
        """Test creating tracker with budget."""
        tracker = create_usage_tracker(budget=10.0)
        assert tracker.budget == 10.0

    def test_create_with_persistent_file(self, tmp_path: Path):
        """Test creating tracker with persistent file."""
        file_path = tmp_path / "usage.json"
        tracker = create_usage_tracker(persistent_file=file_path)
        assert tracker.persistent_file == file_path

    def test_create_with_string_path(self, tmp_path: Path):
        """Test creating tracker with string path."""
        file_path = str(tmp_path / "usage.json")
        tracker = create_usage_tracker(persistent_file=file_path)
        assert tracker.persistent_file == Path(file_path)


class TestUsageTrackerCostEstimation:
    """Tests for automatic cost estimation."""

    def test_sonnet_cost_estimation(self):
        """Test cost estimation for Sonnet model."""
        tracker = UsageTracker()
        record = tracker.record_usage(
            "claude-sonnet-4-20250514",
            input_tokens=1_000_000,  # 1M tokens
            output_tokens=500_000,  # 500K tokens
        )
        # $3/1M input + $15/1M output = $3 + $7.5 = $10.5
        assert record.cost is not None
        assert abs(record.cost - 10.5) < 0.01

    def test_haiku_cost_estimation(self):
        """Test cost estimation for Haiku model."""
        tracker = UsageTracker()
        record = tracker.record_usage(
            "claude-3-5-haiku-20241022",
            input_tokens=1_000_000,
            output_tokens=500_000,
        )
        # $0.80/1M input + $4/1M output = $0.80 + $2 = $2.80
        assert record.cost is not None
        assert abs(record.cost - 2.80) < 0.01

    def test_unknown_model_no_cost(self):
        """Test unknown model returns no cost estimate."""
        tracker = UsageTracker()
        record = tracker.record_usage(
            "unknown-model",
            input_tokens=1000,
            output_tokens=500,
        )
        assert record.cost is None
