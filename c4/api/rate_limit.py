"""C4 Rate Limiting - Token bucket rate limiter for API requests."""

from __future__ import annotations

import asyncio
import time
from dataclasses import dataclass, field
from typing import Callable

from fastapi import Request
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.responses import JSONResponse


@dataclass
class RateLimitConfig:
    """Rate limit configuration."""

    requests_per_minute: int = 60
    requests_per_hour: int = 1000
    tokens_per_minute: int = 100000
    tokens_per_hour: int = 1000000
    burst_multiplier: float = 1.5


@dataclass
class TokenBucket:
    """Token bucket for rate limiting."""

    capacity: float
    fill_rate: float  # tokens per second
    tokens: float = field(default=0.0)
    last_update: float = field(default_factory=time.monotonic)

    def __post_init__(self) -> None:
        """Initialize bucket to full capacity."""
        self.tokens = self.capacity

    def consume(self, amount: float = 1.0) -> bool:
        """Try to consume tokens from bucket.

        Args:
            amount: Number of tokens to consume

        Returns:
            True if tokens were consumed, False if insufficient
        """
        self._refill()

        if self.tokens >= amount:
            self.tokens -= amount
            return True
        return False

    def _refill(self) -> None:
        """Refill bucket based on elapsed time."""
        now = time.monotonic()
        elapsed = now - self.last_update
        self.tokens = min(self.capacity, self.tokens + elapsed * self.fill_rate)
        self.last_update = now

    @property
    def available(self) -> float:
        """Get available tokens."""
        self._refill()
        return self.tokens


@dataclass
class RateLimiter:
    """Rate limiter with multiple buckets."""

    config: RateLimitConfig = field(default_factory=RateLimitConfig)
    minute_bucket: TokenBucket | None = None
    hour_bucket: TokenBucket | None = None
    token_minute_bucket: TokenBucket | None = None
    token_hour_bucket: TokenBucket | None = None

    def __post_init__(self) -> None:
        """Initialize buckets."""
        burst = self.config.burst_multiplier

        # Request rate buckets
        self.minute_bucket = TokenBucket(
            capacity=self.config.requests_per_minute * burst,
            fill_rate=self.config.requests_per_minute / 60,
        )
        self.hour_bucket = TokenBucket(
            capacity=self.config.requests_per_hour * burst,
            fill_rate=self.config.requests_per_hour / 3600,
        )

        # Token rate buckets
        self.token_minute_bucket = TokenBucket(
            capacity=self.config.tokens_per_minute * burst,
            fill_rate=self.config.tokens_per_minute / 60,
        )
        self.token_hour_bucket = TokenBucket(
            capacity=self.config.tokens_per_hour * burst,
            fill_rate=self.config.tokens_per_hour / 3600,
        )

    def check_request_limit(self) -> tuple[bool, str | None]:
        """Check if request is within rate limits.

        Returns:
            Tuple of (allowed, reason_if_denied)
        """
        if self.minute_bucket and not self.minute_bucket.consume():
            return False, "Rate limit exceeded: too many requests per minute"

        if self.hour_bucket and not self.hour_bucket.consume():
            # Refund minute bucket
            if self.minute_bucket:
                self.minute_bucket.tokens += 1
            return False, "Rate limit exceeded: too many requests per hour"

        return True, None

    def check_token_limit(self, tokens: int) -> tuple[bool, str | None]:
        """Check if token usage is within rate limits.

        Args:
            tokens: Number of tokens to consume

        Returns:
            Tuple of (allowed, reason_if_denied)
        """
        if self.token_minute_bucket and not self.token_minute_bucket.consume(tokens):
            return False, "Rate limit exceeded: too many tokens per minute"

        if self.token_hour_bucket and not self.token_hour_bucket.consume(tokens):
            # Refund minute bucket
            if self.token_minute_bucket:
                self.token_minute_bucket.tokens += tokens
            return False, "Rate limit exceeded: too many tokens per hour"

        return True, None

    def get_status(self) -> dict[str, float]:
        """Get current rate limit status.

        Returns:
            Dict with available limits
        """
        return {
            "requests_per_minute_available": (
                self.minute_bucket.available if self.minute_bucket else 0
            ),
            "requests_per_hour_available": (self.hour_bucket.available if self.hour_bucket else 0),
            "tokens_per_minute_available": (
                self.token_minute_bucket.available if self.token_minute_bucket else 0
            ),
            "tokens_per_hour_available": (
                self.token_hour_bucket.available if self.token_hour_bucket else 0
            ),
        }


class RateLimitStore:
    """Storage for per-user/per-key rate limiters."""

    def __init__(self, config: RateLimitConfig | None = None):
        """Initialize store.

        Args:
            config: Default rate limit config
        """
        self.config = config or RateLimitConfig()
        self._limiters: dict[str, RateLimiter] = {}
        self._lock = asyncio.Lock()

    async def get_limiter(self, key: str) -> RateLimiter:
        """Get or create rate limiter for key.

        Args:
            key: Identifier (user ID, API key, IP address)

        Returns:
            Rate limiter for the key
        """
        if key not in self._limiters:
            async with self._lock:
                if key not in self._limiters:
                    self._limiters[key] = RateLimiter(config=self.config)
        return self._limiters[key]

    def cleanup_expired(self, max_age_seconds: float = 3600) -> int:
        """Remove limiters that haven't been used recently.

        Args:
            max_age_seconds: Maximum age before cleanup

        Returns:
            Number of limiters removed
        """
        now = time.monotonic()
        expired = []

        for key, limiter in self._limiters.items():
            if limiter.minute_bucket:
                age = now - limiter.minute_bucket.last_update
                if age > max_age_seconds:
                    expired.append(key)

        for key in expired:
            del self._limiters[key]

        return len(expired)


class RateLimitMiddleware(BaseHTTPMiddleware):
    """FastAPI middleware for rate limiting."""

    def __init__(
        self,
        app,
        store: RateLimitStore | None = None,
        key_func: Callable[[Request], str] | None = None,
        exclude_paths: list[str] | None = None,
    ):
        """Initialize middleware.

        Args:
            app: FastAPI application
            store: Rate limit store
            key_func: Function to extract key from request
            exclude_paths: Paths to exclude from rate limiting
        """
        super().__init__(app)
        self.store = store or RateLimitStore()
        self.key_func = key_func or self._default_key_func
        self.exclude_paths = exclude_paths or ["/api/health", "/api/docs"]

    def _default_key_func(self, request: Request) -> str:
        """Default key function - uses client IP."""
        forwarded = request.headers.get("x-forwarded-for")
        if forwarded:
            return forwarded.split(",")[0].strip()
        return request.client.host if request.client else "unknown"

    async def dispatch(self, request: Request, call_next):
        """Process request through rate limiter.

        Args:
            request: Incoming request
            call_next: Next middleware/handler

        Returns:
            Response or rate limit error
        """
        # Skip excluded paths
        if any(request.url.path.startswith(p) for p in self.exclude_paths):
            return await call_next(request)

        # Get limiter for this key
        key = self.key_func(request)
        limiter = await self.store.get_limiter(key)

        # Check rate limit
        allowed, reason = limiter.check_request_limit()
        if not allowed:
            status = limiter.get_status()
            return JSONResponse(
                status_code=429,
                content={
                    "error": "rate_limit_exceeded",
                    "message": reason,
                    "retry_after_seconds": 60,
                    "limits": status,
                },
                headers={
                    "Retry-After": "60",
                    "X-RateLimit-Remaining": str(int(status["requests_per_minute_available"])),
                },
            )

        # Add rate limit headers to response
        response = await call_next(request)
        status = limiter.get_status()
        response.headers["X-RateLimit-Remaining"] = str(
            int(status["requests_per_minute_available"])
        )
        response.headers["X-RateLimit-Limit"] = str(self.store.config.requests_per_minute)

        return response
