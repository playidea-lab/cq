"""Branding Middleware.

Custom domain-based branding middleware for white-label support.
Extracts domain from Host header and applies team branding to requests.
"""

from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import Response

if TYPE_CHECKING:
    from starlette.types import ASGIApp

    from c4.services.branding import BrandingService, TeamBranding

logger = logging.getLogger(__name__)

# Default cache TTL in seconds
DEFAULT_CACHE_TTL = 60


@dataclass
class CacheEntry:
    """Cache entry with value and expiration time."""

    value: TeamBranding | None
    expires_at: float


@dataclass
class BrandingCache:
    """TTL-based cache for branding configurations.

    Thread-safe cache with automatic expiration.
    """

    ttl: float = DEFAULT_CACHE_TTL
    _cache: dict[str, CacheEntry] = field(default_factory=dict)
    _lock: asyncio.Lock = field(default_factory=asyncio.Lock)

    async def get(self, domain: str) -> tuple[bool, TeamBranding | None]:
        """Get cached branding for domain.

        Args:
            domain: Custom domain to lookup

        Returns:
            Tuple of (hit, value) where hit indicates cache hit/miss
        """
        async with self._lock:
            entry = self._cache.get(domain)
            if entry is None:
                return False, None

            # Check if expired
            if time.time() > entry.expires_at:
                del self._cache[domain]
                return False, None

            return True, entry.value

    async def set(self, domain: str, value: TeamBranding | None) -> None:
        """Cache branding for domain.

        Args:
            domain: Custom domain
            value: Branding configuration (or None if not found)
        """
        async with self._lock:
            self._cache[domain] = CacheEntry(
                value=value,
                expires_at=time.time() + self.ttl,
            )

    async def clear(self) -> None:
        """Clear all cached entries."""
        async with self._lock:
            self._cache.clear()

    async def invalidate(self, domain: str) -> None:
        """Invalidate cache for specific domain.

        Args:
            domain: Custom domain to invalidate
        """
        async with self._lock:
            self._cache.pop(domain, None)


class BrandingMiddleware(BaseHTTPMiddleware):
    """Middleware that applies team branding based on custom domain.

    Extracts the Host header from incoming requests, looks up the
    corresponding branding configuration, and stores it in request.state.branding.

    Uses a TTL cache (default 60 seconds) to minimize database queries.

    Usage:
        app.add_middleware(
            BrandingMiddleware,
            branding_service=branding_service,
            cache_ttl=60,
        )

    Access branding in route handlers:
        branding = request.state.branding  # TeamBranding | None
    """

    def __init__(
        self,
        app: ASGIApp,
        branding_service: BrandingService,
        cache_ttl: float = DEFAULT_CACHE_TTL,
    ) -> None:
        """Initialize branding middleware.

        Args:
            app: ASGI application
            branding_service: Service for branding lookups
            cache_ttl: Cache TTL in seconds (default 60)
        """
        super().__init__(app)
        self.branding_service = branding_service
        self.cache = BrandingCache(ttl=cache_ttl)

    async def dispatch(self, request: Request, call_next) -> Response:
        """Process request and apply branding.

        Args:
            request: Incoming request
            call_next: Next middleware/handler in chain

        Returns:
            Response from downstream handler
        """
        # Extract domain from Host header
        host = request.headers.get("host", "")
        domain = self._extract_domain(host)

        # Lookup branding (with caching)
        branding = await self._get_branding(domain)

        # Store in request state for access in handlers
        request.state.branding = branding

        # Log for debugging
        if branding:
            logger.debug(
                "Applied branding for domain: %s (team: %s)",
                domain,
                branding.team_id,
            )
        else:
            logger.debug("No branding found for domain: %s", domain)

        # Continue to next handler
        return await call_next(request)

    def _extract_domain(self, host: str) -> str:
        """Extract domain from Host header.

        Strips port number if present.

        Args:
            host: Host header value (e.g., 'example.com:8080')

        Returns:
            Domain without port (e.g., 'example.com')
        """
        if not host:
            return ""

        # Remove port if present
        if ":" in host:
            return host.split(":")[0]

        return host

    async def _get_branding(self, domain: str) -> TeamBranding | None:
        """Get branding for domain with caching.

        Args:
            domain: Custom domain to lookup

        Returns:
            TeamBranding if found and verified, None otherwise
        """
        if not domain:
            return None

        # Check cache first
        hit, cached_value = await self.cache.get(domain)
        if hit:
            logger.debug("Cache hit for domain: %s", domain)
            return cached_value

        # Cache miss - lookup from service
        logger.debug("Cache miss for domain: %s, querying service", domain)
        try:
            branding = await self.branding_service.get_branding_by_domain(domain)
        except Exception as e:
            logger.warning("Error looking up branding for domain %s: %s", domain, e)
            branding = None

        # Cache the result (including None for negative caching)
        await self.cache.set(domain, branding)

        return branding

    async def invalidate_cache(self, domain: str) -> None:
        """Invalidate cached branding for domain.

        Call this when branding configuration changes.

        Args:
            domain: Custom domain to invalidate
        """
        await self.cache.invalidate(domain)

    async def clear_cache(self) -> None:
        """Clear entire branding cache."""
        await self.cache.clear()
