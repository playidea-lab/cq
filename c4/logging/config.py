"""Logging configuration for C4.

Provides utility functions for setting up logging with optional Loki integration.

Usage:
    from c4.logging import setup_logging, get_logger

    # Setup logging with Loki enabled
    setup_logging(level="INFO", enable_loki=True)

    # Get a named logger
    logger = get_logger(__name__)
    logger.info("Application started")
"""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    pass


def setup_logging(
    level: str = "INFO",
    enable_loki: bool | None = None,
    loki_url: str | None = None,
    service_name: str = "c4",
    format_string: str | None = None,
) -> None:
    """Configure the logging system.

    Sets up console logging and optionally enables Loki logging when
    LOKI_URL environment variable is set or enable_loki is True.

    Args:
        level: Log level (DEBUG, INFO, WARNING, ERROR, CRITICAL).
            Defaults to "INFO"
        enable_loki: Explicitly enable/disable Loki logging.
            If None, Loki is enabled when LOKI_URL is set
        loki_url: Loki push API URL. Falls back to LOKI_URL env var
        service_name: Service name label for Loki streams.
            Defaults to "c4"
        format_string: Custom log format string.
            Defaults to "[%(asctime)s] %(levelname)s %(name)s: %(message)s"
    """
    # Default format
    if format_string is None:
        format_string = "[%(asctime)s] %(levelname)s %(name)s: %(message)s"

    formatter = logging.Formatter(format_string)

    # Get or create root logger
    root = logging.getLogger()

    # Clear existing handlers to avoid duplicates
    root.handlers.clear()

    # Set log level
    root.setLevel(getattr(logging, level.upper(), logging.INFO))

    # Console handler
    console_handler = logging.StreamHandler()
    console_handler.setFormatter(formatter)
    root.addHandler(console_handler)

    # Loki handler (enabled by env var or explicit flag)
    should_enable_loki = enable_loki if enable_loki is not None else bool(os.getenv("LOKI_URL"))

    if should_enable_loki:
        from .loki import LokiHandler

        loki_handler = LokiHandler(
            url=loki_url,
            labels={"service": service_name},
        )
        root.addHandler(loki_handler)


def get_logger(name: str) -> logging.Logger:
    """Get a named logger.

    Args:
        name: Logger name, typically __name__ of the calling module

    Returns:
        Configured logger instance
    """
    return logging.getLogger(name)


class LoggerAdapter(logging.LoggerAdapter):
    """Logger adapter with context support for C4.

    Allows adding context fields (user_id, workspace_id, etc.) to log records.

    Usage:
        logger = get_logger(__name__)
        ctx_logger = LoggerAdapter(logger, {"user_id": "user-123"})
        ctx_logger.info("User action")  # Log includes user_id
    """

    def process(
        self,
        msg: str,
        kwargs: dict,
    ) -> tuple[str, dict]:
        """Process log message with extra context.

        Args:
            msg: Log message
            kwargs: Logging kwargs

        Returns:
            Tuple of (message, kwargs) with context added to extra
        """
        extra = kwargs.get("extra", {})
        extra.update(self.extra)
        kwargs["extra"] = extra
        return msg, kwargs
