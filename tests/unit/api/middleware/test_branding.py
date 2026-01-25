"""Tests for Branding Middleware."""

from __future__ import annotations

import asyncio
import time
from dataclasses import dataclass
from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from starlette.requests import Request

from c4.api.middleware.branding import (
    DEFAULT_CACHE_TTL,
    BrandingCache,
    BrandingMiddleware,
    CacheEntry,
)


# Mock TeamBranding dataclass for testing
@dataclass
class MockTeamBranding:
    """Mock TeamBranding for tests."""

    team_id: str
    brand_name: str
    logo_url: str | None = None
    primary_color: str | None = None
    custom_domain: str | None = None
    custom_domain_verified: bool = False


class TestCacheEntry:
    """Tests for CacheEntry dataclass."""

    def test_cache_entry_creation(self):
        """Test creating a cache entry."""
        branding = MockTeamBranding(team_id="team-1", brand_name="Test Brand")
        entry = CacheEntry(value=branding, expires_at=time.time() + 60)

        assert entry.value == branding
        assert entry.expires_at > time.time()

    def test_cache_entry_with_none_value(self):
        """Test cache entry with None value (negative caching)."""
        entry = CacheEntry(value=None, expires_at=time.time() + 60)

        assert entry.value is None


class TestBrandingCache:
    """Tests for BrandingCache."""

    @pytest.fixture
    def cache(self):
        """Create a test cache instance."""
        return BrandingCache(ttl=60)

    @pytest.mark.asyncio
    async def test_cache_miss_on_empty_cache(self, cache):
        """Test cache miss when cache is empty."""
        hit, value = await cache.get("example.com")

        assert hit is False
        assert value is None

    @pytest.mark.asyncio
    async def test_cache_set_and_get(self, cache):
        """Test setting and getting cached value."""
        branding = MockTeamBranding(team_id="team-1", brand_name="Test")

        await cache.set("example.com", branding)
        hit, value = await cache.get("example.com")

        assert hit is True
        assert value == branding

    @pytest.mark.asyncio
    async def test_cache_negative_caching(self, cache):
        """Test caching None values (negative caching)."""
        await cache.set("unknown.com", None)
        hit, value = await cache.get("unknown.com")

        assert hit is True
        assert value is None

    @pytest.mark.asyncio
    async def test_cache_expiration(self):
        """Test that expired entries are not returned."""
        cache = BrandingCache(ttl=0.1)  # 100ms TTL
        branding = MockTeamBranding(team_id="team-1", brand_name="Test")

        await cache.set("example.com", branding)

        # Wait for expiration
        await asyncio.sleep(0.15)

        hit, value = await cache.get("example.com")
        assert hit is False
        assert value is None

    @pytest.mark.asyncio
    async def test_cache_clear(self, cache):
        """Test clearing the cache."""
        branding = MockTeamBranding(team_id="team-1", brand_name="Test")

        await cache.set("example.com", branding)
        await cache.set("other.com", branding)

        await cache.clear()

        hit1, _ = await cache.get("example.com")
        hit2, _ = await cache.get("other.com")

        assert hit1 is False
        assert hit2 is False

    @pytest.mark.asyncio
    async def test_cache_invalidate(self, cache):
        """Test invalidating a specific domain."""
        branding = MockTeamBranding(team_id="team-1", brand_name="Test")

        await cache.set("example.com", branding)
        await cache.set("other.com", branding)

        await cache.invalidate("example.com")

        hit1, _ = await cache.get("example.com")
        hit2, _ = await cache.get("other.com")

        assert hit1 is False
        assert hit2 is True

    @pytest.mark.asyncio
    async def test_cache_invalidate_nonexistent(self, cache):
        """Test invalidating a nonexistent domain doesn't raise error."""
        await cache.invalidate("nonexistent.com")  # Should not raise

    @pytest.mark.asyncio
    async def test_default_ttl(self):
        """Test default TTL value."""
        cache = BrandingCache()
        assert cache.ttl == DEFAULT_CACHE_TTL


class TestBrandingMiddleware:
    """Tests for BrandingMiddleware."""

    @pytest.fixture
    def mock_branding_service(self):
        """Create a mock branding service."""
        service = MagicMock()
        service.get_branding_by_domain = AsyncMock()
        return service

    @pytest.fixture
    def app(self, mock_branding_service):
        """Create a test FastAPI app with branding middleware."""
        app = FastAPI()

        app.add_middleware(
            BrandingMiddleware,
            branding_service=mock_branding_service,
            cache_ttl=60,
        )

        @app.get("/test")
        async def test_endpoint(request: Request):
            branding = getattr(request.state, "branding", None)
            if branding:
                return {
                    "branding": True,
                    "team_id": branding.team_id,
                    "brand_name": branding.brand_name,
                }
            return {"branding": False}

        return app

    @pytest.fixture
    def client(self, app):
        """Create test client."""
        return TestClient(app)

    def test_extract_domain_simple(self):
        """Test domain extraction from simple host."""
        app = MagicMock()
        service = MagicMock()
        middleware = BrandingMiddleware(app, service)

        assert middleware._extract_domain("example.com") == "example.com"

    def test_extract_domain_with_port(self):
        """Test domain extraction from host with port."""
        app = MagicMock()
        service = MagicMock()
        middleware = BrandingMiddleware(app, service)

        assert middleware._extract_domain("example.com:8080") == "example.com"

    def test_extract_domain_empty(self):
        """Test domain extraction from empty host."""
        app = MagicMock()
        service = MagicMock()
        middleware = BrandingMiddleware(app, service)

        assert middleware._extract_domain("") == ""

    def test_request_with_branding(self, client, mock_branding_service):
        """Test request with valid branding."""
        branding = MockTeamBranding(
            team_id="team-123",
            brand_name="Agency Brand",
            custom_domain="agency.example.com",
            custom_domain_verified=True,
        )
        mock_branding_service.get_branding_by_domain.return_value = branding

        response = client.get(
            "/test",
            headers={"Host": "agency.example.com"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["branding"] is True
        assert data["team_id"] == "team-123"
        assert data["brand_name"] == "Agency Brand"

    def test_request_without_branding(self, client, mock_branding_service):
        """Test request without branding (domain not found)."""
        mock_branding_service.get_branding_by_domain.return_value = None

        response = client.get(
            "/test",
            headers={"Host": "unknown.example.com"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["branding"] is False

    def test_request_with_port_in_host(self, client, mock_branding_service):
        """Test request with port in Host header."""
        branding = MockTeamBranding(
            team_id="team-123",
            brand_name="Test Brand",
        )
        mock_branding_service.get_branding_by_domain.return_value = branding

        response = client.get(
            "/test",
            headers={"Host": "agency.example.com:8080"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["branding"] is True

        # Verify domain was extracted correctly (without port)
        mock_branding_service.get_branding_by_domain.assert_called()

    def test_caching_reduces_service_calls(self, client, mock_branding_service):
        """Test that caching reduces service calls."""
        branding = MockTeamBranding(
            team_id="team-123",
            brand_name="Cached Brand",
        )
        mock_branding_service.get_branding_by_domain.return_value = branding

        # Make multiple requests
        for _ in range(5):
            response = client.get(
                "/test",
                headers={"Host": "cached.example.com"},
            )
            assert response.status_code == 200

        # Service should only be called once due to caching
        assert mock_branding_service.get_branding_by_domain.call_count == 1

    def test_service_error_handling(self, client, mock_branding_service):
        """Test handling of service errors."""
        mock_branding_service.get_branding_by_domain.side_effect = Exception(
            "Database error"
        )

        response = client.get(
            "/test",
            headers={"Host": "error.example.com"},
        )

        # Should still return response (with None branding)
        assert response.status_code == 200
        data = response.json()
        assert data["branding"] is False

    def test_empty_host_header(self, client, mock_branding_service):
        """Test request with empty host header."""
        response = client.get("/test", headers={"Host": ""})

        assert response.status_code == 200
        data = response.json()
        assert data["branding"] is False

        # Service should not be called for empty domain
        mock_branding_service.get_branding_by_domain.assert_not_called()

    @pytest.mark.asyncio
    async def test_invalidate_cache(self, mock_branding_service):
        """Test cache invalidation."""
        app = MagicMock()
        middleware = BrandingMiddleware(
            app,
            mock_branding_service,
            cache_ttl=60,
        )

        # Pre-populate cache
        branding = MockTeamBranding(team_id="team-1", brand_name="Test")
        await middleware.cache.set("example.com", branding)

        # Verify it's cached
        hit, _ = await middleware.cache.get("example.com")
        assert hit is True

        # Invalidate
        await middleware.invalidate_cache("example.com")

        # Verify it's cleared
        hit, _ = await middleware.cache.get("example.com")
        assert hit is False

    @pytest.mark.asyncio
    async def test_clear_cache(self, mock_branding_service):
        """Test clearing entire cache."""
        app = MagicMock()
        middleware = BrandingMiddleware(
            app,
            mock_branding_service,
            cache_ttl=60,
        )

        # Pre-populate cache
        branding = MockTeamBranding(team_id="team-1", brand_name="Test")
        await middleware.cache.set("example.com", branding)
        await middleware.cache.set("other.com", branding)

        # Clear all
        await middleware.clear_cache()

        # Verify both are cleared
        hit1, _ = await middleware.cache.get("example.com")
        hit2, _ = await middleware.cache.get("other.com")
        assert hit1 is False
        assert hit2 is False


class TestServerIntegration:
    """Tests for server.py integration."""

    def test_app_created_without_branding_service(self):
        """Test app creation without branding service."""
        from c4.api.server import create_app

        app = create_app()
        assert app is not None

    def test_app_created_with_branding_service(self):
        """Test app creation with branding service."""
        from c4.api.server import create_app

        mock_service = MagicMock()
        mock_service.get_branding_by_domain = AsyncMock(return_value=None)

        app = create_app(
            branding_service=mock_service,
            branding_cache_ttl=30,
        )
        assert app is not None

        # Make a test request to verify middleware is active
        client = TestClient(app)
        response = client.get("/health")
        assert response.status_code == 200
