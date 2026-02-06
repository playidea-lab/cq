"""Tests for metric capture from stdout."""

from c4.tracker.capture import extract_progress, parse_metrics, parse_metrics_from_lines


class TestParseMetrics:
    def test_colon_separator(self):
        result = parse_metrics("loss: 0.234")
        assert result["loss"] == 0.234

    def test_equals_separator(self):
        result = parse_metrics("accuracy = 0.95")
        assert result["accuracy"] == 0.95

    def test_percentage(self):
        result = parse_metrics("accuracy: 92%")
        assert abs(result["accuracy"] - 0.92) < 1e-6

    def test_scientific_notation(self):
        result = parse_metrics("lr: 1e-4")
        assert abs(result["lr"] - 0.0001) < 1e-8

    def test_multiple_metrics(self):
        result = parse_metrics("epoch: 10, loss: 0.234, accuracy: 92%")
        assert result["epoch"] == 10.0
        assert result["loss"] == 0.234
        assert abs(result["accuracy"] - 0.92) < 1e-6

    def test_skip_noise_keys(self):
        result = parse_metrics("pid: 12345, loss: 0.5")
        assert "pid" not in result
        assert result["loss"] == 0.5

    def test_empty_line(self):
        assert parse_metrics("") == {}

    def test_no_metrics(self):
        assert parse_metrics("Training started...") == {}

    def test_negative_value(self):
        result = parse_metrics("delta: -0.05")
        assert result["delta"] == -0.05


class TestParseMetricsFromLines:
    def test_last_value_wins(self):
        lines = ["loss: 0.5", "loss: 0.3", "loss: 0.1"]
        result = parse_metrics_from_lines(lines)
        assert result["loss"] == 0.1

    def test_accumulates_different_keys(self):
        lines = ["loss: 0.5", "accuracy: 0.9"]
        result = parse_metrics_from_lines(lines)
        assert result["loss"] == 0.5
        assert result["accuracy"] == 0.9


class TestExtractProgress:
    def test_epoch(self):
        result = extract_progress("epoch: 10, loss: 0.5")
        assert result["epoch"] == 10

    def test_step(self):
        result = extract_progress("step: 500")
        assert result["step"] == 500
