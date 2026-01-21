"""Metrics middleware for FastAPI.

Provides automatic request metrics collection.
"""

import time

from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import Response

from .prometheus import record_api_request, record_request_duration


class MetricsMiddleware(BaseHTTPMiddleware):
    """Middleware that records API request metrics.

    Records:
    - Endpoint path
    - HTTP method
    - Response status code
    - Request duration (histogram)
    """

    async def dispatch(self, request: Request, call_next) -> Response:
        """Process request and record metrics.

        Args:
            request: Incoming HTTP request
            call_next: Next middleware/handler in chain

        Returns:
            HTTP response
        """
        start_time = time.time()

        response = await call_next(request)

        # Calculate duration
        duration = time.time() - start_time

        # Record metrics
        record_api_request(
            endpoint=request.url.path,
            method=request.method,
            status=response.status_code,
        )
        record_request_duration(
            endpoint=request.url.path,
            method=request.method,
            duration_seconds=duration,
        )

        return response
