"""Tests for c4.logging.loki module.

Tests cover:
- LokiHandler initialization and configuration
- Log record formatting
- Loki API communication (mocked)
- Error handling and graceful failures
"""

from __future__ import annotations

import json
import logging
from unittest.mock import MagicMock, patch

import pytest

from c4.logging import LoggerAdapter, LokiHandler, get_logger, setup_logging


class TestLokiHandlerInit:
    """Tests for LokiHandler initialization."""

    def test_default_url_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test URL defaults to LOKI_URL environment variable."""
        monkeypatch.setenv("LOKI_URL", "http://loki.example.com/push")
        handler = LokiHandler()
        assert handler.url == "http://loki.example.com/push"
        handler.close()

    def test_default_url_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test URL falls back to localhost when env var not set."""
        monkeypatch.delenv("LOKI_URL", raising=False)
        handler = LokiHandler()
        assert handler.url == "http://localhost:3100/loki/api/v1/push"
        handler.close()

    def test_explicit_url(self) -> None:
        """Test explicit URL overrides environment variable."""
        handler = LokiHandler(url="http://custom.loki.com/push")
        assert handler.url == "http://custom.loki.com/push"
        handler.close()

    def test_default_labels(self) -> None:
        """Test default labels when none provided."""
        handler = LokiHandler()
        assert handler.labels == {"service": "c4"}
        handler.close()

    def test_custom_labels(self) -> None:
        """Test custom labels are applied."""
        handler = LokiHandler(labels={"service": "c4-api", "env": "prod"})
        assert handler.labels == {"service": "c4-api", "env": "prod"}
        handler.close()

    def test_default_timeout(self) -> None:
        """Test default timeout value."""
        handler = LokiHandler()
        assert handler.timeout == 5.0
        handler.close()

    def test_custom_timeout(self) -> None:
        """Test custom timeout is applied."""
        handler = LokiHandler(timeout=10.0)
        assert handler.timeout == 10.0
        handler.close()


class TestLokiHandlerFormatRecord:
    """Tests for log record formatting."""

    def setup_method(self) -> None:
        """Set up test fixtures."""
        self.handler = LokiHandler()

    def teardown_method(self) -> None:
        """Clean up after tests."""
        self.handler.close()

    def test_format_basic_record(self) -> None:
        """Test formatting a basic log record."""
        record = logging.LogRecord(
            name="test.logger",
            level=logging.INFO,
            pathname="test.py",
            lineno=42,
            msg="Test message",
            args=(),
            exc_info=None,
        )

        formatted = self.handler._format_record(record)

        assert formatted["level"] == "INFO"
        assert formatted["logger"] == "test.logger"
        assert formatted["message"] == "Test message"
        assert formatted["line"] == 42
        assert "timestamp" in formatted

    def test_format_record_with_args(self) -> None:
        """Test formatting record with message arguments."""
        record = logging.LogRecord(
            name="test.logger",
            level=logging.WARNING,
            pathname="test.py",
            lineno=10,
            msg="Value is %d",
            args=(42,),
            exc_info=None,
        )

        formatted = self.handler._format_record(record)
        assert formatted["message"] == "Value is 42"

    def test_format_record_with_context_fields(self) -> None:
        """Test formatting record with extra context fields."""
        record = logging.LogRecord(
            name="test.logger",
            level=logging.INFO,
            pathname="test.py",
            lineno=1,
            msg="Context test",
            args=(),
            exc_info=None,
        )
        record.user_id = "user-123"
        record.workspace_id = "ws-456"
        record.request_id = "req-789"
        record.task_id = "T-001"
        record.worker_id = "worker-1"

        formatted = self.handler._format_record(record)

        assert formatted["user_id"] == "user-123"
        assert formatted["workspace_id"] == "ws-456"
        assert formatted["request_id"] == "req-789"
        assert formatted["task_id"] == "T-001"
        assert formatted["worker_id"] == "worker-1"

    def test_format_record_missing_context_excluded(self) -> None:
        """Test that missing context fields are not included."""
        record = logging.LogRecord(
            name="test.logger",
            level=logging.INFO,
            pathname="test.py",
            lineno=1,
            msg="No context",
            args=(),
            exc_info=None,
        )

        formatted = self.handler._format_record(record)

        assert "user_id" not in formatted
        assert "workspace_id" not in formatted


class TestLokiHandlerEmit:
    """Tests for emitting logs to Loki."""

    def setup_method(self) -> None:
        """Set up test fixtures."""
        self.handler = LokiHandler(url="http://test.loki.com/push")

    def teardown_method(self) -> None:
        """Clean up after tests."""
        self.handler.close()

    @patch("c4.logging.loki.httpx.Client")
    def test_emit_sends_to_loki(self, mock_client_class: MagicMock) -> None:
        """Test that emit sends formatted log to Loki."""
        mock_client = MagicMock()
        mock_client_class.return_value = mock_client

        # Create a fresh handler with mocked client
        handler = LokiHandler(url="http://test.loki.com/push", labels={"service": "test"})

        record = logging.LogRecord(
            name="test.logger",
            level=logging.INFO,
            pathname="test.py",
            lineno=1,
            msg="Test log",
            args=(),
            exc_info=None,
        )

        handler.emit(record)

        # Verify POST was called
        mock_client.post.assert_called_once()
        call_args = mock_client.post.call_args

        # Verify URL
        assert call_args[0][0] == "http://test.loki.com/push"

        # Verify payload structure
        payload = call_args[1]["json"]
        assert "streams" in payload
        assert len(payload["streams"]) == 1
        assert payload["streams"][0]["stream"] == {"service": "test"}

        # Verify log entry
        values = payload["streams"][0]["values"]
        assert len(values) == 1
        timestamp, log_json = values[0]

        # Timestamp should be nanoseconds (19+ digits)
        assert len(timestamp) >= 19

        # Log entry should be valid JSON
        log_entry = json.loads(log_json)
        assert log_entry["message"] == "Test log"
        assert log_entry["level"] == "INFO"

        handler.close()

    @patch("c4.logging.loki.httpx.Client")
    def test_emit_handles_http_error_gracefully(self, mock_client_class: MagicMock) -> None:
        """Test that HTTP errors don't crash the application."""
        import httpx

        mock_client = MagicMock()
        mock_client.post.side_effect = httpx.HTTPError("Connection failed")
        mock_client_class.return_value = mock_client

        handler = LokiHandler(url="http://test.loki.com/push")

        record = logging.LogRecord(
            name="test.logger",
            level=logging.ERROR,
            pathname="test.py",
            lineno=1,
            msg="Error log",
            args=(),
            exc_info=None,
        )

        # Should not raise
        handler.emit(record)

        handler.close()


class TestSetupLogging:
    """Tests for logging setup function."""

    def setup_method(self) -> None:
        """Set up test fixtures."""
        # Store original handlers
        self.root_logger = logging.getLogger()
        self.original_handlers = self.root_logger.handlers.copy()
        self.original_level = self.root_logger.level

    def teardown_method(self) -> None:
        """Clean up after tests."""
        # Restore original state
        self.root_logger.handlers = self.original_handlers
        self.root_logger.setLevel(self.original_level)

    def test_setup_logging_default_level(self) -> None:
        """Test default log level is INFO."""
        setup_logging()

        root = logging.getLogger()
        assert root.level == logging.INFO

    def test_setup_logging_custom_level(self) -> None:
        """Test custom log level is applied."""
        setup_logging(level="DEBUG")

        root = logging.getLogger()
        assert root.level == logging.DEBUG

    def test_setup_logging_adds_console_handler(self) -> None:
        """Test console handler is added."""
        setup_logging()

        root = logging.getLogger()
        console_handlers = [h for h in root.handlers if isinstance(h, logging.StreamHandler)]
        assert len(console_handlers) >= 1

    def test_setup_logging_loki_disabled_by_default(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test Loki is disabled when env var not set."""
        monkeypatch.delenv("LOKI_URL", raising=False)
        setup_logging()

        root = logging.getLogger()
        loki_handlers = [h for h in root.handlers if isinstance(h, LokiHandler)]
        assert len(loki_handlers) == 0

    def test_setup_logging_loki_enabled_by_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test Loki is enabled when LOKI_URL is set."""
        monkeypatch.setenv("LOKI_URL", "http://loki.test.com/push")
        setup_logging()

        root = logging.getLogger()
        loki_handlers = [h for h in root.handlers if isinstance(h, LokiHandler)]
        assert len(loki_handlers) == 1

        # Clean up
        for h in loki_handlers:
            h.close()

    def test_setup_logging_loki_explicit_enable(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test Loki can be explicitly enabled."""
        monkeypatch.delenv("LOKI_URL", raising=False)
        setup_logging(enable_loki=True, loki_url="http://explicit.loki.com/push")

        root = logging.getLogger()
        loki_handlers = [h for h in root.handlers if isinstance(h, LokiHandler)]
        assert len(loki_handlers) == 1
        assert loki_handlers[0].url == "http://explicit.loki.com/push"

        # Clean up
        for h in loki_handlers:
            h.close()

    def test_setup_logging_loki_explicit_disable(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test Loki can be explicitly disabled even with env var."""
        monkeypatch.setenv("LOKI_URL", "http://loki.test.com/push")
        setup_logging(enable_loki=False)

        root = logging.getLogger()
        loki_handlers = [h for h in root.handlers if isinstance(h, LokiHandler)]
        assert len(loki_handlers) == 0

    def test_setup_logging_service_name(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test service name is passed to Loki handler."""
        monkeypatch.delenv("LOKI_URL", raising=False)
        setup_logging(enable_loki=True, service_name="my-service")

        root = logging.getLogger()
        loki_handlers = [h for h in root.handlers if isinstance(h, LokiHandler)]
        assert len(loki_handlers) == 1
        assert loki_handlers[0].labels["service"] == "my-service"

        # Clean up
        for h in loki_handlers:
            h.close()


class TestGetLogger:
    """Tests for get_logger function."""

    def test_get_logger_returns_named_logger(self) -> None:
        """Test get_logger returns a logger with the given name."""
        logger = get_logger("my.module")
        assert logger.name == "my.module"

    def test_get_logger_same_instance(self) -> None:
        """Test get_logger returns same instance for same name."""
        logger1 = get_logger("same.name")
        logger2 = get_logger("same.name")
        assert logger1 is logger2


class TestLoggerAdapter:
    """Tests for LoggerAdapter context support."""

    def test_adapter_adds_context(self) -> None:
        """Test adapter adds extra fields to log records."""
        logger = get_logger("test.adapter")
        adapter = LoggerAdapter(logger, {"user_id": "user-123", "workspace_id": "ws-456"})

        # Process a message
        msg, kwargs = adapter.process("Test message", {})

        assert kwargs["extra"]["user_id"] == "user-123"
        assert kwargs["extra"]["workspace_id"] == "ws-456"

    def test_adapter_merges_with_existing_extra(self) -> None:
        """Test adapter merges its context with existing extra."""
        logger = get_logger("test.adapter")
        adapter = LoggerAdapter(logger, {"user_id": "user-123"})

        msg, kwargs = adapter.process("Test message", {"extra": {"request_id": "req-789"}})

        assert kwargs["extra"]["user_id"] == "user-123"
        assert kwargs["extra"]["request_id"] == "req-789"


class TestLokiHandlerClose:
    """Tests for LokiHandler cleanup."""

    def test_close_releases_client(self) -> None:
        """Test close releases the HTTP client."""
        handler = LokiHandler()

        # Force client initialization
        _ = handler.client

        assert handler._client is not None

        handler.close()

        assert handler._client is None

    def test_close_idempotent(self) -> None:
        """Test close can be called multiple times safely."""
        handler = LokiHandler()
        handler.close()
        handler.close()  # Should not raise


class TestLokiHandlerIntegration:
    """Integration tests for LokiHandler with logging system."""

    def setup_method(self) -> None:
        """Set up test fixtures."""
        self.root_logger = logging.getLogger()
        self.original_handlers = self.root_logger.handlers.copy()

    def teardown_method(self) -> None:
        """Clean up after tests."""
        self.root_logger.handlers = self.original_handlers

    @patch("c4.logging.loki.httpx.Client")
    def test_integration_with_standard_logging(self, mock_client_class: MagicMock) -> None:
        """Test LokiHandler works with standard logging module."""
        mock_client = MagicMock()
        mock_client_class.return_value = mock_client

        # Set up logging
        logger = logging.getLogger("integration.test")
        logger.setLevel(logging.DEBUG)
        logger.handlers.clear()

        handler = LokiHandler(labels={"service": "integration-test"})
        logger.addHandler(handler)

        # Log a message
        logger.info("Integration test message")

        # Verify Loki was called
        mock_client.post.assert_called_once()

        handler.close()

    @patch("c4.logging.loki.httpx.Client")
    def test_integration_with_context_extra(self, mock_client_class: MagicMock) -> None:
        """Test context fields are passed through extra parameter."""
        mock_client = MagicMock()
        mock_client_class.return_value = mock_client

        logger = logging.getLogger("integration.context")
        logger.setLevel(logging.DEBUG)
        logger.handlers.clear()

        handler = LokiHandler()
        logger.addHandler(handler)

        # Log with extra context
        logger.info("Context test", extra={"user_id": "user-xyz", "task_id": "T-999"})

        # Verify the log entry contains context
        call_args = mock_client.post.call_args
        payload = call_args[1]["json"]
        log_json = payload["streams"][0]["values"][0][1]
        log_entry = json.loads(log_json)

        assert log_entry["user_id"] == "user-xyz"
        assert log_entry["task_id"] == "T-999"

        handler.close()
