"""C4 Sentry Integration - Error tracking and performance monitoring."""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Callable

logger = logging.getLogger(__name__)


class SentryEnvironment(str, Enum):
    """Sentry environment types."""

    DEVELOPMENT = "development"
    STAGING = "staging"
    PRODUCTION = "production"


@dataclass
class SentryConfig:
    """Sentry configuration."""

    dsn: str | None = None
    environment: SentryEnvironment = SentryEnvironment.DEVELOPMENT
    release: str | None = None
    sample_rate: float = 1.0
    traces_sample_rate: float = 0.1
    profiles_sample_rate: float = 0.1
    send_default_pii: bool = False
    attach_stacktrace: bool = True
    max_breadcrumbs: int = 100
    debug: bool = False
    enabled: bool = True
    tags: dict[str, str] = field(default_factory=dict)
    integrations: list[str] = field(
        default_factory=lambda: ["logging", "threading", "stdlib"]
    )

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for Sentry SDK."""
        return {
            "dsn": self.dsn,
            "environment": self.environment.value,
            "release": self.release,
            "sample_rate": self.sample_rate,
            "traces_sample_rate": self.traces_sample_rate,
            "profiles_sample_rate": self.profiles_sample_rate,
            "send_default_pii": self.send_default_pii,
            "attach_stacktrace": self.attach_stacktrace,
            "max_breadcrumbs": self.max_breadcrumbs,
            "debug": self.debug,
        }


# Global Sentry state
_sentry_initialized: bool = False
_sentry_config: SentryConfig | None = None
_sentry_sdk: Any = None


def init_sentry(config: SentryConfig | None = None) -> bool:
    """Initialize Sentry SDK.

    Args:
        config: Sentry configuration. If None, uses defaults.

    Returns:
        True if initialization succeeded, False otherwise.
    """
    global _sentry_initialized, _sentry_config, _sentry_sdk

    if _sentry_initialized:
        logger.warning("Sentry already initialized")
        return True

    config = config or SentryConfig()
    _sentry_config = config

    if not config.enabled:
        logger.info("Sentry disabled by configuration")
        return False

    if not config.dsn:
        logger.info("Sentry DSN not configured, running in mock mode")
        _sentry_initialized = True
        return True

    try:
        import sentry_sdk

        _sentry_sdk = sentry_sdk

        # Build integrations list
        integrations = []
        if "logging" in config.integrations:
            from sentry_sdk.integrations.logging import LoggingIntegration

            integrations.append(
                LoggingIntegration(
                    level=logging.INFO,
                    event_level=logging.ERROR,
                )
            )
        if "threading" in config.integrations:
            from sentry_sdk.integrations.threading import ThreadingIntegration

            integrations.append(ThreadingIntegration())

        sentry_sdk.init(
            dsn=config.dsn,
            environment=config.environment.value,
            release=config.release,
            sample_rate=config.sample_rate,
            traces_sample_rate=config.traces_sample_rate,
            profiles_sample_rate=config.profiles_sample_rate,
            send_default_pii=config.send_default_pii,
            attach_stacktrace=config.attach_stacktrace,
            max_breadcrumbs=config.max_breadcrumbs,
            debug=config.debug,
            integrations=integrations,
        )

        # Set default tags
        for key, value in config.tags.items():
            sentry_sdk.set_tag(key, value)

        _sentry_initialized = True
        logger.info(f"Sentry initialized: env={config.environment.value}")
        return True

    except ImportError:
        logger.warning("sentry-sdk not installed, running in mock mode")
        _sentry_initialized = True
        return True
    except Exception as e:
        logger.error(f"Failed to initialize Sentry: {e}")
        return False


def capture_exception(
    exception: Exception,
    context: dict[str, Any] | None = None,
    tags: dict[str, str] | None = None,
    level: str = "error",
) -> str | None:
    """Capture an exception to Sentry.

    Args:
        exception: Exception to capture
        context: Additional context data
        tags: Tags to attach
        level: Error level (error, warning, info)

    Returns:
        Event ID if captured, None otherwise
    """
    if not _sentry_initialized or _sentry_sdk is None:
        logger.debug(f"Sentry not available, logging exception: {exception}")
        return None

    try:
        with _sentry_sdk.push_scope() as scope:
            if context:
                for key, value in context.items():
                    scope.set_context(key, value)
            if tags:
                for key, value in tags.items():
                    scope.set_tag(key, value)
            scope.level = level

            return _sentry_sdk.capture_exception(exception)
    except Exception as e:
        logger.error(f"Failed to capture exception to Sentry: {e}")
        return None


def capture_message(
    message: str,
    level: str = "info",
    context: dict[str, Any] | None = None,
    tags: dict[str, str] | None = None,
) -> str | None:
    """Capture a message to Sentry.

    Args:
        message: Message to capture
        level: Message level (error, warning, info, debug)
        context: Additional context data
        tags: Tags to attach

    Returns:
        Event ID if captured, None otherwise
    """
    if not _sentry_initialized or _sentry_sdk is None:
        logger.debug(f"Sentry not available, logging message: {message}")
        return None

    try:
        with _sentry_sdk.push_scope() as scope:
            if context:
                for key, value in context.items():
                    scope.set_context(key, value)
            if tags:
                for key, value in tags.items():
                    scope.set_tag(key, value)

            return _sentry_sdk.capture_message(message, level=level)
    except Exception as e:
        logger.error(f"Failed to capture message to Sentry: {e}")
        return None


def add_breadcrumb(
    category: str,
    message: str,
    level: str = "info",
    data: dict[str, Any] | None = None,
) -> None:
    """Add a breadcrumb for debugging.

    Args:
        category: Breadcrumb category
        message: Breadcrumb message
        level: Breadcrumb level
        data: Additional data
    """
    if not _sentry_initialized or _sentry_sdk is None:
        return

    try:
        _sentry_sdk.add_breadcrumb(
            category=category,
            message=message,
            level=level,
            data=data or {},
        )
    except Exception as e:
        logger.debug(f"Failed to add breadcrumb: {e}")


def set_user(user_id: str, email: str | None = None, username: str | None = None) -> None:
    """Set the current user context.

    Args:
        user_id: User identifier
        email: User email
        username: Username
    """
    if not _sentry_initialized or _sentry_sdk is None:
        return

    try:
        _sentry_sdk.set_user(
            {
                "id": user_id,
                "email": email,
                "username": username,
            }
        )
    except Exception as e:
        logger.debug(f"Failed to set user: {e}")


def set_tag(key: str, value: str) -> None:
    """Set a global tag.

    Args:
        key: Tag key
        value: Tag value
    """
    if not _sentry_initialized or _sentry_sdk is None:
        return

    try:
        _sentry_sdk.set_tag(key, value)
    except Exception as e:
        logger.debug(f"Failed to set tag: {e}")


def start_transaction(name: str, op: str = "task") -> Any:
    """Start a performance transaction.

    Args:
        name: Transaction name
        op: Operation type

    Returns:
        Transaction object or None
    """
    if not _sentry_initialized or _sentry_sdk is None:
        return None

    try:
        return _sentry_sdk.start_transaction(name=name, op=op)
    except Exception as e:
        logger.debug(f"Failed to start transaction: {e}")
        return None


class SentryMiddleware:
    """FastAPI middleware for Sentry integration."""

    def __init__(self, app: Any):
        """Initialize middleware.

        Args:
            app: FastAPI application
        """
        self.app = app

    async def __call__(self, scope: dict, receive: Callable, send: Callable) -> None:
        """Handle request.

        Args:
            scope: ASGI scope
            receive: Receive callable
            send: Send callable
        """
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        # Add request breadcrumb
        add_breadcrumb(
            category="http",
            message=f"{scope.get('method', 'UNKNOWN')} {scope.get('path', '/')}",
            level="info",
            data={
                "method": scope.get("method"),
                "path": scope.get("path"),
                "query_string": scope.get("query_string", b"").decode(),
            },
        )

        try:
            await self.app(scope, receive, send)
        except Exception as e:
            # Capture exception with request context
            capture_exception(
                e,
                context={
                    "request": {
                        "method": scope.get("method"),
                        "path": scope.get("path"),
                        "query_string": scope.get("query_string", b"").decode(),
                    }
                },
            )
            raise


def sentry_middleware(app: Any) -> SentryMiddleware:
    """Create Sentry middleware for FastAPI.

    Args:
        app: FastAPI application

    Returns:
        Configured middleware
    """
    return SentryMiddleware(app)


def get_sentry_status() -> dict[str, Any]:
    """Get current Sentry status.

    Returns:
        Status information
    """
    return {
        "initialized": _sentry_initialized,
        "enabled": _sentry_config.enabled if _sentry_config else False,
        "environment": _sentry_config.environment.value if _sentry_config else None,
        "has_dsn": bool(_sentry_config.dsn) if _sentry_config else False,
        "sdk_available": _sentry_sdk is not None,
    }
