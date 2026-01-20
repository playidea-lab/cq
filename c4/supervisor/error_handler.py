"""Claude API Error Handler - Improved error handling and retry logic.

Provides specialized error handling for Anthropic API errors including
rate limiting, quota management, and automatic retries with backoff.

Example:
    >>> handler = ClaudeErrorHandler()
    >>> try:
    ...     response = api_call()
    ... except Exception as e:
    ...     action = handler.handle_error(e)
    ...     if action.should_retry:
    ...         time.sleep(action.retry_after)
"""

import logging
import random
import time
from dataclasses import dataclass
from enum import Enum
from typing import Callable, TypeVar

logger = logging.getLogger(__name__)


class ClaudeErrorType(str, Enum):
    """Types of Claude API errors."""

    RATE_LIMIT = "rate_limit"  # 429: Too many requests
    QUOTA_EXCEEDED = "quota_exceeded"  # 402: Payment required
    OVERLOADED = "overloaded"  # 529: API overloaded
    INVALID_REQUEST = "invalid_request"  # 400: Bad request
    AUTHENTICATION = "authentication"  # 401: Invalid API key
    PERMISSION = "permission"  # 403: Permission denied
    NOT_FOUND = "not_found"  # 404: Model not found
    TIMEOUT = "timeout"  # Request timeout
    CONNECTION = "connection"  # Network error
    SERVER = "server"  # 500+: Server error
    UNKNOWN = "unknown"  # Unknown error


@dataclass
class ErrorAction:
    """Recommended action for handling an error."""

    error_type: ClaudeErrorType
    should_retry: bool
    retry_after: float  # seconds to wait before retry
    max_retries: int  # recommended max retries for this error
    user_message: str  # user-friendly error message
    original_error: str  # original error message
    is_transient: bool  # whether error is temporary


# Error type to HTTP status code mapping
ERROR_STATUS_CODES = {
    400: ClaudeErrorType.INVALID_REQUEST,
    401: ClaudeErrorType.AUTHENTICATION,
    402: ClaudeErrorType.QUOTA_EXCEEDED,
    403: ClaudeErrorType.PERMISSION,
    404: ClaudeErrorType.NOT_FOUND,
    429: ClaudeErrorType.RATE_LIMIT,
    500: ClaudeErrorType.SERVER,
    502: ClaudeErrorType.SERVER,
    503: ClaudeErrorType.SERVER,
    529: ClaudeErrorType.OVERLOADED,
}

# User-friendly error messages
ERROR_MESSAGES = {
    ClaudeErrorType.RATE_LIMIT: (
        "API rate limit exceeded. Please wait before making more requests. "
        "Consider reducing request frequency or upgrading your plan."
    ),
    ClaudeErrorType.QUOTA_EXCEEDED: (
        "API quota exceeded. Your account has reached its usage limit. "
        "Please check your billing settings or upgrade your plan."
    ),
    ClaudeErrorType.OVERLOADED: (
        "The Claude API is temporarily overloaded. Please wait a moment and try again."
    ),
    ClaudeErrorType.INVALID_REQUEST: ("Invalid request. Please check your request parameters."),
    ClaudeErrorType.AUTHENTICATION: (
        "Invalid API key. Please check your ANTHROPIC_API_KEY environment variable."
    ),
    ClaudeErrorType.PERMISSION: (
        "Permission denied. Your API key doesn't have access to this resource."
    ),
    ClaudeErrorType.NOT_FOUND: ("Model not found. Please check the model ID is correct."),
    ClaudeErrorType.TIMEOUT: (
        "Request timed out. The API took too long to respond. "
        "Consider increasing the timeout or simplifying your request."
    ),
    ClaudeErrorType.CONNECTION: ("Connection error. Please check your network connection."),
    ClaudeErrorType.SERVER: (
        "Server error. The Claude API is experiencing issues. Please try again later."
    ),
    ClaudeErrorType.UNKNOWN: ("An unexpected error occurred. Please try again."),
}


class ClaudeErrorHandler:
    """Handler for Claude API errors with automatic retry logic.

    Provides intelligent error classification, retry recommendations,
    and exponential backoff with jitter.

    Attributes:
        base_delay: Base delay for exponential backoff
        max_delay: Maximum delay between retries
        max_retries: Default maximum retry attempts
        jitter: Add randomness to delays (recommended)
    """

    def __init__(
        self,
        base_delay: float = 1.0,
        max_delay: float = 60.0,
        max_retries: int = 3,
        jitter: bool = True,
    ):
        """Initialize error handler.

        Args:
            base_delay: Base delay in seconds for exponential backoff
            max_delay: Maximum delay in seconds
            max_retries: Default maximum retry attempts
            jitter: Add randomness to delays
        """
        self.base_delay = base_delay
        self.max_delay = max_delay
        self.max_retries = max_retries
        self.jitter = jitter

    def classify_error(self, error: Exception) -> ClaudeErrorType:
        """Classify an exception into error type.

        Args:
            error: Exception to classify

        Returns:
            Classified error type
        """
        error_str = str(error).lower()
        error_type = type(error).__name__

        # Check for timeout errors
        if "timeout" in error_str or error_type in ("TimeoutError", "ReadTimeout"):
            return ClaudeErrorType.TIMEOUT

        # Check for connection errors
        if any(x in error_str for x in ("connection", "network", "dns", "socket", "refused")):
            return ClaudeErrorType.CONNECTION

        # Check for rate limit indicators
        if "rate" in error_str and "limit" in error_str:
            return ClaudeErrorType.RATE_LIMIT
        if "429" in error_str or "too many" in error_str:
            return ClaudeErrorType.RATE_LIMIT

        # Check for quota/billing errors
        if any(x in error_str for x in ("quota", "402", "payment", "billing")):
            return ClaudeErrorType.QUOTA_EXCEEDED

        # Check for overload
        if "529" in error_str or "overloaded" in error_str:
            return ClaudeErrorType.OVERLOADED

        # Check for auth errors
        if any(x in error_str for x in ("401", "unauthorized", "invalid.*key")):
            return ClaudeErrorType.AUTHENTICATION

        # Check for permission errors
        if "403" in error_str or "forbidden" in error_str:
            return ClaudeErrorType.PERMISSION

        # Check for not found
        if "404" in error_str or "not found" in error_str:
            return ClaudeErrorType.NOT_FOUND

        # Check for invalid request
        if "400" in error_str or "invalid" in error_str:
            return ClaudeErrorType.INVALID_REQUEST

        # Check for server errors
        if any(x in error_str for x in ("500", "502", "503", "server error")):
            return ClaudeErrorType.SERVER

        # Try to extract status code from error
        status_code = self._extract_status_code(error)
        if status_code and status_code in ERROR_STATUS_CODES:
            return ERROR_STATUS_CODES[status_code]

        return ClaudeErrorType.UNKNOWN

    def _extract_status_code(self, error: Exception) -> int | None:
        """Extract HTTP status code from error if available."""
        # Try common attributes
        for attr in ("status_code", "status", "code"):
            code = getattr(error, attr, None)
            if isinstance(code, int):
                return code

        # Try response attribute
        response = getattr(error, "response", None)
        if response:
            code = getattr(response, "status_code", None)
            if isinstance(code, int):
                return code

        return None

    def _extract_retry_after(self, error: Exception) -> float | None:
        """Extract Retry-After header value from error if available."""
        # Check for retry_after attribute
        retry_after = getattr(error, "retry_after", None)
        if retry_after is not None:
            return float(retry_after)

        # Check response headers
        response = getattr(error, "response", None)
        if response:
            headers = getattr(response, "headers", {})
            if isinstance(headers, dict):
                retry_after = headers.get("Retry-After") or headers.get("retry-after")
                if retry_after:
                    try:
                        return float(retry_after)
                    except ValueError:
                        pass

        return None

    def handle_error(
        self,
        error: Exception,
        attempt: int = 0,
    ) -> ErrorAction:
        """Handle an error and return recommended action.

        Args:
            error: Exception to handle
            attempt: Current attempt number (0-based)

        Returns:
            ErrorAction with retry recommendations
        """
        error_type = self.classify_error(error)
        original_error = str(error)[:500]  # Truncate long errors

        # Determine if error is transient (worth retrying)
        is_transient = error_type in (
            ClaudeErrorType.RATE_LIMIT,
            ClaudeErrorType.OVERLOADED,
            ClaudeErrorType.TIMEOUT,
            ClaudeErrorType.CONNECTION,
            ClaudeErrorType.SERVER,
        )

        # Calculate retry delay
        retry_after = self._calculate_retry_delay(error, error_type, attempt)

        # Determine max retries for this error type
        if error_type == ClaudeErrorType.RATE_LIMIT:
            max_retries = 5  # More patient with rate limits
        elif error_type == ClaudeErrorType.OVERLOADED:
            max_retries = 5  # More patient with overload
        elif error_type in (ClaudeErrorType.TIMEOUT, ClaudeErrorType.CONNECTION):
            max_retries = 3  # Standard for network issues
        elif error_type == ClaudeErrorType.SERVER:
            max_retries = 3  # Standard for server issues
        else:
            max_retries = 0  # Don't retry permanent errors

        # Should we retry?
        should_retry = is_transient and attempt < max_retries

        return ErrorAction(
            error_type=error_type,
            should_retry=should_retry,
            retry_after=retry_after if should_retry else 0,
            max_retries=max_retries,
            user_message=ERROR_MESSAGES.get(error_type, ERROR_MESSAGES[ClaudeErrorType.UNKNOWN]),
            original_error=original_error,
            is_transient=is_transient,
        )

    def _calculate_retry_delay(
        self,
        error: Exception,
        error_type: ClaudeErrorType,
        attempt: int,
    ) -> float:
        """Calculate retry delay with exponential backoff and jitter."""
        # Check for Retry-After header
        retry_after = self._extract_retry_after(error)
        if retry_after is not None:
            return min(retry_after, self.max_delay)

        # Calculate exponential backoff: base_delay * 2^attempt
        delay = self.base_delay * (2**attempt)

        # Cap at max delay
        delay = min(delay, self.max_delay)

        # Add jitter to prevent thundering herd
        if self.jitter:
            delay = delay * (0.5 + random.random())  # 50-150% of calculated delay

        # Special handling for rate limits - be more conservative
        if error_type == ClaudeErrorType.RATE_LIMIT:
            delay = max(delay, 5.0)  # At least 5 seconds for rate limits

        return delay

    def log_error(
        self,
        error: Exception,
        action: ErrorAction,
        attempt: int,
        max_attempts: int,
    ) -> None:
        """Log error with appropriate level and details.

        Args:
            error: Original exception
            action: Error action with recommendations
            attempt: Current attempt number
            max_attempts: Maximum attempts configured
        """
        if action.should_retry:
            logger.warning(
                f"[{action.error_type.value}] Attempt {attempt + 1}/{max_attempts} failed: "
                f"{action.original_error[:100]}. "
                f"Retrying in {action.retry_after:.1f}s..."
            )
        else:
            if action.is_transient:
                logger.error(
                    f"[{action.error_type.value}] Max retries exceeded. {action.user_message}"
                )
            else:
                logger.error(
                    f"[{action.error_type.value}] Permanent error (no retry): {action.user_message}"
                )


T = TypeVar("T")


def retry_with_backoff(
    func: Callable[[], T],
    handler: ClaudeErrorHandler | None = None,
    max_retries: int | None = None,
    on_retry: Callable[[ErrorAction, int], None] | None = None,
) -> T:
    """Execute function with automatic retry on transient errors.

    Args:
        func: Function to execute (should raise exception on failure)
        handler: Error handler instance (creates default if None)
        max_retries: Override max retries (uses handler default if None)
        on_retry: Optional callback before each retry

    Returns:
        Result of successful function call

    Raises:
        Exception: Original exception if all retries fail or error is permanent

    Example:
        >>> def api_call():
        ...     return litellm.completion(...)
        >>> result = retry_with_backoff(api_call, max_retries=3)
    """
    if handler is None:
        handler = ClaudeErrorHandler()

    effective_max_retries = max_retries or handler.max_retries

    last_error: Exception | None = None

    for attempt in range(effective_max_retries + 1):  # +1 for initial attempt
        try:
            return func()
        except Exception as e:
            last_error = e
            action = handler.handle_error(e, attempt)
            handler.log_error(e, action, attempt, effective_max_retries + 1)

            if not action.should_retry:
                raise

            if on_retry:
                on_retry(action, attempt)

            time.sleep(action.retry_after)

    # Should not reach here, but just in case
    if last_error:
        raise last_error
    raise RuntimeError("Unexpected state in retry_with_backoff")


class RateLimitTracker:
    """Track and manage rate limiting state.

    Helps prevent hitting rate limits by tracking request timing
    and providing wait recommendations.
    """

    def __init__(
        self,
        requests_per_minute: int = 60,
        tokens_per_minute: int = 100000,
    ):
        """Initialize rate limit tracker.

        Args:
            requests_per_minute: Maximum requests per minute
            tokens_per_minute: Maximum tokens per minute
        """
        self.requests_per_minute = requests_per_minute
        self.tokens_per_minute = tokens_per_minute
        self._request_times: list[float] = []
        self._token_counts: list[tuple[float, int]] = []

    def record_request(self, tokens: int = 0) -> None:
        """Record a request.

        Args:
            tokens: Number of tokens in request
        """
        now = time.time()
        self._request_times.append(now)
        if tokens > 0:
            self._token_counts.append((now, tokens))
        self._cleanup()

    def _cleanup(self) -> None:
        """Remove entries older than 1 minute."""
        cutoff = time.time() - 60
        self._request_times = [t for t in self._request_times if t > cutoff]
        self._token_counts = [(t, c) for t, c in self._token_counts if t > cutoff]

    def should_wait(self) -> float:
        """Check if we should wait before next request.

        Returns:
            Seconds to wait (0 if OK to proceed)
        """
        self._cleanup()

        # Check request rate
        if len(self._request_times) >= self.requests_per_minute:
            oldest = self._request_times[0]
            wait_for_requests = 60 - (time.time() - oldest)
            if wait_for_requests > 0:
                return wait_for_requests

        # Check token rate
        total_tokens = sum(c for _, c in self._token_counts)
        if total_tokens >= self.tokens_per_minute:
            oldest = self._token_counts[0][0]
            wait_for_tokens = 60 - (time.time() - oldest)
            if wait_for_tokens > 0:
                return wait_for_tokens

        return 0

    def wait_if_needed(self) -> None:
        """Wait if rate limit would be exceeded."""
        wait_time = self.should_wait()
        if wait_time > 0:
            logger.info(f"Rate limit prevention: waiting {wait_time:.1f}s")
            time.sleep(wait_time)
