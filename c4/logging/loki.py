"""Loki logging handler for C4.

Provides a logging handler that sends structured JSON logs to Loki.

Usage:
    import logging
    from c4.logging import LokiHandler

    logger = logging.getLogger(__name__)
    handler = LokiHandler(
        url="http://localhost:3100/loki/api/v1/push",
        labels={"service": "c4-api"}
    )
    logger.addHandler(handler)
    logger.info("Hello from C4")
"""

from __future__ import annotations

import json
import logging
import os
from datetime import datetime, timezone
from typing import Any

import httpx


class LokiHandler(logging.Handler):
    """Loki logging handler that sends structured JSON logs.

    This handler formats log records as JSON and sends them to Loki's
    push API endpoint. Each log entry includes timestamp, level, logger name,
    message, and optional context fields.

    Attributes:
        url: Loki push API URL
        labels: Stream labels for log grouping
        timeout: HTTP request timeout in seconds
    """

    def __init__(
        self,
        url: str | None = None,
        labels: dict[str, str] | None = None,
        timeout: float = 5.0,
    ) -> None:
        """Initialize the Loki handler.

        Args:
            url: Loki push API URL. Defaults to LOKI_URL environment variable
                or http://localhost:3100/loki/api/v1/push
            labels: Log stream labels for grouping (e.g., {"service": "c4-api"}).
                Defaults to {"service": "c4"}
            timeout: HTTP request timeout in seconds. Defaults to 5.0
        """
        super().__init__()
        self.url = url or os.getenv("LOKI_URL", "http://localhost:3100/loki/api/v1/push")
        self.labels = labels or {"service": "c4"}
        self.timeout = timeout
        self._client: httpx.Client | None = None

    @property
    def client(self) -> httpx.Client:
        """Lazy-initialize HTTP client."""
        if self._client is None:
            self._client = httpx.Client(timeout=self.timeout)
        return self._client

    def emit(self, record: logging.LogRecord) -> None:
        """Emit a log record to Loki.

        Args:
            record: The log record to emit
        """
        try:
            log_entry = self._format_record(record)
            self._send_to_loki(log_entry)
        except Exception:
            self.handleError(record)

    def _format_record(self, record: logging.LogRecord) -> dict[str, Any]:
        """Format a LogRecord as a JSON-serializable dictionary.

        Args:
            record: The log record to format

        Returns:
            Dictionary containing structured log data
        """
        # Build base log entry
        entry: dict[str, Any] = {
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
            "module": record.module,
            "function": record.funcName,
            "line": record.lineno,
        }

        # Add optional context fields if present
        for field in ("user_id", "workspace_id", "request_id", "task_id", "worker_id"):
            value = getattr(record, field, None)
            if value is not None:
                entry[field] = value

        # Add exception info if present
        if record.exc_info and record.exc_info[0] is not None:
            entry["exception"] = self.formatter.formatException(record.exc_info) if self.formatter else str(record.exc_info[1])

        return entry

    def _send_to_loki(self, log_entry: dict[str, Any]) -> None:
        """Send a log entry to Loki's push API.

        Args:
            log_entry: The formatted log entry to send
        """
        # Loki expects timestamps in nanoseconds
        timestamp_ns = str(int(datetime.now(timezone.utc).timestamp() * 1_000_000_000))

        payload = {
            "streams": [
                {
                    "stream": self.labels,
                    "values": [[timestamp_ns, json.dumps(log_entry)]],
                }
            ]
        }

        try:
            self.client.post(self.url, json=payload)
        except httpx.HTTPError:
            # Silently fail on HTTP errors to avoid disrupting application
            pass

    def close(self) -> None:
        """Close the HTTP client and release resources."""
        if self._client is not None:
            self._client.close()
            self._client = None
        super().close()
