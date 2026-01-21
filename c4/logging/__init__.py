"""C4 Logging package.

Provides structured logging with optional Loki integration for observability.

Features:
- LokiHandler: Send structured JSON logs to Loki
- setup_logging: Configure logging with console and Loki outputs
- get_logger: Get named loggers for modules
- LoggerAdapter: Add context fields to log records

Environment Variables:
- LOKI_URL: Loki push API URL (enables Loki logging when set)

Usage:
    from c4.logging import setup_logging, get_logger

    # Initialize logging (Loki enabled if LOKI_URL is set)
    setup_logging(level="INFO", service_name="c4-api")

    # Get a logger for your module
    logger = get_logger(__name__)
    logger.info("Application started")

    # Log with context
    logger.info("Task completed", extra={"task_id": "T-001"})
"""

from .config import LoggerAdapter, get_logger, setup_logging
from .loki import LokiHandler

__all__ = [
    "LokiHandler",
    "setup_logging",
    "get_logger",
    "LoggerAdapter",
]
