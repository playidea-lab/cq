"""Unit tests for Claude API Error Handler."""

import time
from unittest.mock import MagicMock, patch

import pytest

from c4.supervisor.error_handler import (
    ClaudeErrorHandler,
    ClaudeErrorType,
    ErrorAction,
    RateLimitTracker,
    retry_with_backoff,
)


class TestClaudeErrorType:
    """Tests for ClaudeErrorType enum."""

    def test_error_types_exist(self):
        """All expected error types exist."""
        assert ClaudeErrorType.RATE_LIMIT == "rate_limit"
        assert ClaudeErrorType.QUOTA_EXCEEDED == "quota_exceeded"
        assert ClaudeErrorType.OVERLOADED == "overloaded"
        assert ClaudeErrorType.INVALID_REQUEST == "invalid_request"
        assert ClaudeErrorType.AUTHENTICATION == "authentication"
        assert ClaudeErrorType.PERMISSION == "permission"
        assert ClaudeErrorType.NOT_FOUND == "not_found"
        assert ClaudeErrorType.TIMEOUT == "timeout"
        assert ClaudeErrorType.CONNECTION == "connection"
        assert ClaudeErrorType.SERVER == "server"
        assert ClaudeErrorType.UNKNOWN == "unknown"


class TestErrorAction:
    """Tests for ErrorAction dataclass."""

    def test_create_action(self):
        """Test creating an error action."""
        action = ErrorAction(
            error_type=ClaudeErrorType.RATE_LIMIT,
            should_retry=True,
            retry_after=5.0,
            max_retries=5,
            user_message="Rate limit exceeded",
            original_error="429 Too Many Requests",
            is_transient=True,
        )
        assert action.error_type == ClaudeErrorType.RATE_LIMIT
        assert action.should_retry is True
        assert action.retry_after == 5.0
        assert action.max_retries == 5
        assert action.is_transient is True


class TestClaudeErrorHandler:
    """Tests for ClaudeErrorHandler."""

    def test_default_initialization(self):
        """Test default handler initialization."""
        handler = ClaudeErrorHandler()
        assert handler.base_delay == 1.0
        assert handler.max_delay == 60.0
        assert handler.max_retries == 3
        assert handler.jitter is True

    def test_custom_initialization(self):
        """Test custom handler initialization."""
        handler = ClaudeErrorHandler(
            base_delay=2.0,
            max_delay=120.0,
            max_retries=5,
            jitter=False,
        )
        assert handler.base_delay == 2.0
        assert handler.max_delay == 120.0
        assert handler.max_retries == 5
        assert handler.jitter is False

    # Error classification tests
    def test_classify_rate_limit_429(self):
        """Classify 429 status code as rate limit."""
        handler = ClaudeErrorHandler()
        error = Exception("Error 429: Too many requests")
        assert handler.classify_error(error) == ClaudeErrorType.RATE_LIMIT

    def test_classify_rate_limit_text(self):
        """Classify rate limit text."""
        handler = ClaudeErrorHandler()
        error = Exception("rate limit exceeded")
        assert handler.classify_error(error) == ClaudeErrorType.RATE_LIMIT

    def test_classify_quota_exceeded(self):
        """Classify quota exceeded error."""
        handler = ClaudeErrorHandler()
        error = Exception("402 Payment Required: quota exceeded")
        assert handler.classify_error(error) == ClaudeErrorType.QUOTA_EXCEEDED

    def test_classify_overloaded(self):
        """Classify API overloaded error."""
        handler = ClaudeErrorHandler()
        error = Exception("529: API is overloaded")
        assert handler.classify_error(error) == ClaudeErrorType.OVERLOADED

    def test_classify_authentication(self):
        """Classify authentication error."""
        handler = ClaudeErrorHandler()
        error = Exception("401 Unauthorized")
        assert handler.classify_error(error) == ClaudeErrorType.AUTHENTICATION

    def test_classify_permission(self):
        """Classify permission error."""
        handler = ClaudeErrorHandler()
        error = Exception("403 Forbidden")
        assert handler.classify_error(error) == ClaudeErrorType.PERMISSION

    def test_classify_not_found(self):
        """Classify not found error."""
        handler = ClaudeErrorHandler()
        error = Exception("404 Model not found")
        assert handler.classify_error(error) == ClaudeErrorType.NOT_FOUND

    def test_classify_invalid_request(self):
        """Classify invalid request error."""
        handler = ClaudeErrorHandler()
        error = Exception("400 Bad Request: invalid parameter")
        assert handler.classify_error(error) == ClaudeErrorType.INVALID_REQUEST

    def test_classify_timeout(self):
        """Classify timeout error."""
        handler = ClaudeErrorHandler()
        error = TimeoutError("Request timed out")
        assert handler.classify_error(error) == ClaudeErrorType.TIMEOUT

    def test_classify_connection(self):
        """Classify connection error."""
        handler = ClaudeErrorHandler()
        error = ConnectionError("Connection refused")
        assert handler.classify_error(error) == ClaudeErrorType.CONNECTION

    def test_classify_server_error(self):
        """Classify server error."""
        handler = ClaudeErrorHandler()
        error = Exception("500 Internal Server Error")
        assert handler.classify_error(error) == ClaudeErrorType.SERVER

    def test_classify_unknown(self):
        """Classify unknown error."""
        handler = ClaudeErrorHandler()
        error = Exception("Some random error")
        assert handler.classify_error(error) == ClaudeErrorType.UNKNOWN

    def test_classify_from_status_code_attribute(self):
        """Classify error from status_code attribute."""
        handler = ClaudeErrorHandler()
        error = MagicMock()
        error.status_code = 429
        error.__str__ = lambda self: "Error"
        assert handler.classify_error(error) == ClaudeErrorType.RATE_LIMIT

    # Handle error tests
    def test_handle_rate_limit_error(self):
        """Handle rate limit error."""
        handler = ClaudeErrorHandler()
        error = Exception("429 Too many requests")
        action = handler.handle_error(error, attempt=0)

        assert action.error_type == ClaudeErrorType.RATE_LIMIT
        assert action.should_retry is True
        assert action.retry_after >= 5.0  # Min for rate limits
        assert action.max_retries == 5
        assert action.is_transient is True

    def test_handle_quota_error_no_retry(self):
        """Handle quota error - no retry."""
        handler = ClaudeErrorHandler()
        error = Exception("402 Quota exceeded")
        action = handler.handle_error(error, attempt=0)

        assert action.error_type == ClaudeErrorType.QUOTA_EXCEEDED
        assert action.should_retry is False
        assert action.max_retries == 0
        assert action.is_transient is False

    def test_handle_auth_error_no_retry(self):
        """Handle auth error - no retry."""
        handler = ClaudeErrorHandler()
        error = Exception("401 Invalid API key")
        action = handler.handle_error(error, attempt=0)

        assert action.error_type == ClaudeErrorType.AUTHENTICATION
        assert action.should_retry is False
        assert action.is_transient is False

    def test_handle_server_error_retry(self):
        """Handle server error - should retry."""
        handler = ClaudeErrorHandler()
        error = Exception("500 Server error")
        action = handler.handle_error(error, attempt=0)

        assert action.error_type == ClaudeErrorType.SERVER
        assert action.should_retry is True
        assert action.max_retries == 3
        assert action.is_transient is True

    def test_handle_error_max_retries_exceeded(self):
        """Stop retrying after max attempts."""
        handler = ClaudeErrorHandler()
        error = Exception("429 Rate limit")
        action = handler.handle_error(error, attempt=5)  # Already at max

        assert action.error_type == ClaudeErrorType.RATE_LIMIT
        assert action.should_retry is False  # Max reached

    def test_handle_error_extracts_retry_after_header(self):
        """Extract Retry-After from response headers."""
        handler = ClaudeErrorHandler()
        error = MagicMock()
        error.__str__ = lambda self: "429 Rate limit"
        error.retry_after = 30.0

        action = handler.handle_error(error, attempt=0)
        assert action.retry_after == 30.0

    def test_handle_error_caps_retry_after(self):
        """Cap retry_after at max_delay."""
        handler = ClaudeErrorHandler(max_delay=10.0)
        error = MagicMock()
        error.__str__ = lambda self: "429 Rate limit"
        error.retry_after = 100.0

        action = handler.handle_error(error, attempt=0)
        assert action.retry_after == 10.0

    # Retry delay calculation tests
    def test_exponential_backoff(self):
        """Verify exponential backoff calculation."""
        handler = ClaudeErrorHandler(base_delay=1.0, jitter=False)
        error = Exception("500 Server error")

        action0 = handler.handle_error(error, attempt=0)
        action1 = handler.handle_error(error, attempt=1)
        action2 = handler.handle_error(error, attempt=2)

        # Without jitter: 1, 2, 4 seconds
        assert action0.retry_after == 1.0
        assert action1.retry_after == 2.0
        assert action2.retry_after == 4.0

    def test_retry_delay_with_jitter(self):
        """Verify jitter adds randomness."""
        handler = ClaudeErrorHandler(base_delay=10.0, jitter=True)
        error = Exception("500 Server error")

        # With jitter, should be 50-150% of base
        action = handler.handle_error(error, attempt=0)
        assert 5.0 <= action.retry_after <= 15.0

    def test_retry_delay_max_cap(self):
        """Verify delay is capped at max_delay."""
        handler = ClaudeErrorHandler(base_delay=1.0, max_delay=5.0, jitter=False)
        error = Exception("500 Server error")

        # attempt=2 means 1.0 * 2^2 = 4.0, still under max
        action2 = handler.handle_error(error, attempt=2)
        assert action2.retry_after == 4.0
        assert action2.should_retry is True

        # Use rate limit error which has max_retries=5
        # attempt=4 means 1.0 * 2^4 = 16.0, should be capped at 5.0
        rate_error = Exception("429 Rate limit")
        action4 = handler.handle_error(rate_error, attempt=4)
        assert action4.retry_after == 5.0  # Capped at max_delay
        assert action4.should_retry is True

    # User message tests
    def test_user_message_for_rate_limit(self):
        """Verify user-friendly message for rate limit."""
        handler = ClaudeErrorHandler()
        error = Exception("429")
        action = handler.handle_error(error)

        assert "rate limit" in action.user_message.lower()

    def test_user_message_for_quota(self):
        """Verify user-friendly message for quota."""
        handler = ClaudeErrorHandler()
        error = Exception("402 quota")
        action = handler.handle_error(error)

        assert "quota" in action.user_message.lower()

    def test_original_error_truncated(self):
        """Verify long errors are truncated."""
        handler = ClaudeErrorHandler()
        long_error = "x" * 1000
        error = Exception(long_error)
        action = handler.handle_error(error)

        assert len(action.original_error) <= 500


class TestRetryWithBackoff:
    """Tests for retry_with_backoff function."""

    def test_success_first_try(self):
        """Return immediately on success."""
        func = MagicMock(return_value="success")
        result = retry_with_backoff(func)

        assert result == "success"
        assert func.call_count == 1

    def test_retry_on_transient_error(self):
        """Retry on transient error."""
        func = MagicMock(
            side_effect=[
                Exception("500 Server error"),
                "success",
            ]
        )

        with patch("time.sleep"):  # Skip actual sleep
            result = retry_with_backoff(func)

        assert result == "success"
        assert func.call_count == 2

    def test_raise_on_permanent_error(self):
        """Raise immediately on permanent error."""
        func = MagicMock(side_effect=Exception("401 Invalid API key"))

        with pytest.raises(Exception, match="401"):
            retry_with_backoff(func)

        assert func.call_count == 1

    def test_raise_after_max_retries(self):
        """Raise after exhausting retries."""
        func = MagicMock(side_effect=Exception("500 Server error"))

        with patch("time.sleep"):
            with pytest.raises(Exception, match="500"):
                retry_with_backoff(func, max_retries=2)

        assert func.call_count == 3  # Initial + 2 retries

    def test_on_retry_callback(self):
        """Call on_retry callback before each retry."""
        func = MagicMock(
            side_effect=[
                Exception("500 Server error"),
                Exception("500 Server error"),
                "success",
            ]
        )
        callback = MagicMock()

        with patch("time.sleep"):
            retry_with_backoff(func, on_retry=callback)

        assert callback.call_count == 2

    def test_custom_handler(self):
        """Use custom error handler."""
        handler = ClaudeErrorHandler(max_retries=1)
        func = MagicMock(side_effect=Exception("500 Server error"))

        with patch("time.sleep"):
            with pytest.raises(Exception):
                retry_with_backoff(func, handler=handler, max_retries=1)

        # Initial + 1 retry = 2 calls
        assert func.call_count == 2


class TestRateLimitTracker:
    """Tests for RateLimitTracker."""

    def test_default_initialization(self):
        """Test default tracker initialization."""
        tracker = RateLimitTracker()
        assert tracker.requests_per_minute == 60
        assert tracker.tokens_per_minute == 100000

    def test_custom_initialization(self):
        """Test custom tracker initialization."""
        tracker = RateLimitTracker(
            requests_per_minute=10,
            tokens_per_minute=50000,
        )
        assert tracker.requests_per_minute == 10
        assert tracker.tokens_per_minute == 50000

    def test_record_request(self):
        """Record a request."""
        tracker = RateLimitTracker()
        tracker.record_request(tokens=1000)

        assert len(tracker._request_times) == 1
        assert len(tracker._token_counts) == 1
        assert tracker._token_counts[0][1] == 1000

    def test_should_wait_no_requests(self):
        """No wait needed with no requests."""
        tracker = RateLimitTracker()
        assert tracker.should_wait() == 0

    def test_should_wait_under_limit(self):
        """No wait needed under request limit."""
        tracker = RateLimitTracker(requests_per_minute=10)

        for _ in range(5):
            tracker.record_request()

        assert tracker.should_wait() == 0

    def test_should_wait_at_request_limit(self):
        """Wait needed at request limit."""
        tracker = RateLimitTracker(requests_per_minute=3)

        for _ in range(3):
            tracker.record_request()

        wait_time = tracker.should_wait()
        assert wait_time > 0
        assert wait_time <= 60

    def test_should_wait_at_token_limit(self):
        """Wait needed at token limit."""
        tracker = RateLimitTracker(tokens_per_minute=1000)

        tracker.record_request(tokens=1000)

        wait_time = tracker.should_wait()
        assert wait_time > 0
        assert wait_time <= 60

    def test_cleanup_old_requests(self):
        """Old requests are cleaned up."""
        tracker = RateLimitTracker(requests_per_minute=3)

        # Manually add old timestamps
        old_time = time.time() - 70  # 70 seconds ago
        tracker._request_times = [old_time, old_time]
        tracker._token_counts = [(old_time, 1000)]

        # Cleanup happens on should_wait
        tracker.should_wait()

        assert len(tracker._request_times) == 0
        assert len(tracker._token_counts) == 0

    def test_wait_if_needed(self):
        """wait_if_needed sleeps when over limit."""
        tracker = RateLimitTracker(requests_per_minute=1)
        tracker.record_request()

        with patch("time.sleep") as mock_sleep:
            tracker.wait_if_needed()
            mock_sleep.assert_called_once()

    def test_wait_if_needed_no_wait(self):
        """wait_if_needed doesn't sleep when under limit."""
        tracker = RateLimitTracker()

        with patch("time.sleep") as mock_sleep:
            tracker.wait_if_needed()
            mock_sleep.assert_not_called()


class TestErrorHandlerIntegration:
    """Integration tests for error handling."""

    def test_rate_limit_with_tracker(self):
        """Combine error handler with rate limit tracker."""
        tracker = RateLimitTracker(requests_per_minute=60)
        handler = ClaudeErrorHandler()

        # Record some usage
        for _ in range(10):
            tracker.record_request(tokens=1000)

        # Simulate error
        error = Exception("429 Rate limit")
        action = handler.handle_error(error)

        assert action.should_retry is True
        assert tracker.should_wait() == 0  # Not at limit yet

    def test_full_retry_flow(self):
        """Test complete retry flow with mock API."""
        call_count = [0]

        def mock_api():
            call_count[0] += 1
            if call_count[0] < 3:
                raise Exception("500 Server error")
            return {"result": "success"}

        with patch("time.sleep"):
            result = retry_with_backoff(mock_api, max_retries=3)

        assert result == {"result": "success"}
        assert call_count[0] == 3
