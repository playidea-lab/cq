"""C4 API Middleware.

Custom middleware components for request processing.
"""

from .branding import BrandingMiddleware

__all__ = ["BrandingMiddleware"]
