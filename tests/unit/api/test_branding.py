"""Unit tests for branding API routes."""

from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock, patch
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

from c4.api.auth import User, get_current_user
from c4.api.routes.branding import get_branding_service, get_team_service
from c4.api.server import create_app


@pytest.fixture
def mock_user():
    """Create mock user."""
    return User(user_id="user-123", email="test@example.com", is_api_key_user=False)


@pytest.fixture
def mock_branding():
    """Create mock branding data."""
    return MagicMock(
        id=str(uuid4()),
        team_id="team-123",
        logo_url="https://example.com/logo.png",
        logo_dark_url="https://example.com/logo-dark.png",
        favicon_url="https://example.com/favicon.ico",
        brand_name="Test Brand",
        primary_color="#2563EB",
        secondary_color="#64748B",
        accent_color="#F59E0B",
        background_color="#FFFFFF",
        text_color="#1F2937",
        heading_font="Inter",
        body_font="Open Sans",
        font_scale=1.0,
        custom_domain="app.testbrand.com",
        custom_domain_verified=True,
        custom_domain_verified_at=datetime.now(timezone.utc),
        email_from_name="Test Brand",
        email_footer_text="© 2025 Test Brand",
        meta_description="Test Brand Description",
        social_preview_image_url="https://example.com/preview.png",
        custom_login_background_url="https://example.com/login-bg.png",
        hide_powered_by=False,
        created_at=datetime.now(timezone.utc),
        updated_at=datetime.now(timezone.utc),
    )


@pytest.fixture
def mock_branding_service():
    """Create mock branding service."""
    return AsyncMock()


@pytest.fixture
def mock_team_service():
    """Create mock team service."""
    service = AsyncMock()
    service.require_permission.return_value = None
    return service


@pytest.fixture
def app(mock_user, mock_branding_service, mock_team_service):
    """Create test application with overridden dependencies."""
    app = create_app()

    # Override dependencies
    app.dependency_overrides[get_current_user] = lambda: mock_user
    app.dependency_overrides[get_branding_service] = lambda: mock_branding_service
    app.dependency_overrides[get_team_service] = lambda: mock_team_service

    yield app

    # Clean up
    app.dependency_overrides.clear()


@pytest.fixture
def client(app):
    """Create test client."""
    return TestClient(app)


class TestGetBranding:
    """Tests for GET /api/teams/{team_id}/branding endpoint."""

    def test_get_branding_success(
        self, client, mock_branding_service, mock_branding
    ):
        """Test successful branding retrieval."""
        mock_branding_service.get_or_create_branding.return_value = mock_branding

        response = client.get("/api/teams/team-123/branding")

        assert response.status_code == 200
        data = response.json()
        assert data["brand_name"] == "Test Brand"
        assert data["primary_color"] == "#2563EB"
        assert data["custom_domain"] == "app.testbrand.com"

    def test_get_branding_permission_denied(
        self, client, mock_team_service
    ):
        """Test branding retrieval with permission denied."""
        mock_team_service.require_permission.side_effect = Exception("Not authorized")

        response = client.get("/api/teams/team-123/branding")

        assert response.status_code == 403
        assert "Not authorized" in response.json()["detail"]

    def test_get_branding_team_not_found(
        self, client, mock_team_service
    ):
        """Test branding retrieval when team not found."""
        mock_team_service.require_permission.side_effect = Exception("Team not found")

        response = client.get("/api/teams/nonexistent/branding")

        assert response.status_code == 404
        assert "Team not found" in response.json()["detail"]


class TestUpdateBranding:
    """Tests for PATCH /api/teams/{team_id}/branding endpoint."""

    def test_update_branding_success(
        self, client, mock_branding_service, mock_branding
    ):
        """Test successful branding update."""
        mock_branding.brand_name = "Updated Brand"
        mock_branding.primary_color = "#FF0000"
        mock_branding_service.update_branding.return_value = mock_branding

        response = client.patch(
            "/api/teams/team-123/branding",
            json={"brand_name": "Updated Brand", "primary_color": "#FF0000"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["brand_name"] == "Updated Brand"
        assert data["primary_color"] == "#FF0000"

    def test_update_branding_permission_denied(
        self, client, mock_team_service
    ):
        """Test branding update with permission denied."""
        mock_team_service.require_permission.side_effect = Exception("Not authorized")

        response = client.patch(
            "/api/teams/team-123/branding",
            json={"brand_name": "New Name"},
        )

        assert response.status_code == 403

    def test_update_branding_partial_update(
        self, client, mock_branding_service, mock_branding
    ):
        """Test partial branding update (only some fields)."""
        mock_branding_service.update_branding.return_value = mock_branding

        # Only update one field
        response = client.patch(
            "/api/teams/team-123/branding",
            json={"primary_color": "#00FF00"},
        )

        assert response.status_code == 200
        # Verify update_branding was called
        mock_branding_service.update_branding.assert_called_once()
        call_kwargs = mock_branding_service.update_branding.call_args.kwargs
        assert "primary_color" in call_kwargs["updates"]


class TestInitiateDomainVerification:
    """Tests for POST /api/teams/{team_id}/branding/domain endpoint."""

    def test_initiate_domain_verification_success(
        self, client, mock_branding_service
    ):
        """Test successful domain verification initiation."""
        mock_result = MagicMock(
            success=True,
            verification_token="c4-verify-abc123",
            instructions={
                "type": "TXT",
                "name": "_c4-verification.example.com",
                "value": "c4-verify-abc123",
            },
            error=None,
        )
        mock_branding_service.initiate_domain_verification.return_value = mock_result

        response = client.post(
            "/api/teams/team-123/branding/domain",
            json={"domain": "app.example.com"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert "c4-verify" in data["verification_token"]

    def test_initiate_domain_verification_permission_denied(
        self, client, mock_team_service
    ):
        """Test domain verification initiation with permission denied."""
        mock_team_service.require_permission.side_effect = Exception("Not authorized")

        response = client.post(
            "/api/teams/team-123/branding/domain",
            json={"domain": "app.example.com"},
        )

        assert response.status_code == 403


class TestVerifyDomain:
    """Tests for POST /api/teams/{team_id}/branding/domain/verify endpoint."""

    def test_verify_domain_success(
        self, client, mock_branding_service, mock_branding
    ):
        """Test successful domain verification."""
        mock_branding_service.verify_domain.return_value = None
        mock_branding_service.get_branding.return_value = mock_branding

        response = client.post("/api/teams/team-123/branding/domain/verify")

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["domain"] == "app.testbrand.com"

    def test_verify_domain_not_found(self, client, mock_branding_service):
        """Test domain verification when branding not found."""
        from c4.services.branding import BrandingNotFoundError

        mock_branding_service.verify_domain.side_effect = BrandingNotFoundError(
            "No branding found"
        )

        response = client.post("/api/teams/team-123/branding/domain/verify")

        assert response.status_code == 404

    def test_verify_domain_verification_failed(self, client, mock_branding_service):
        """Test domain verification when DNS check fails."""
        from c4.services.branding import DomainVerificationError

        mock_branding_service.verify_domain.side_effect = DomainVerificationError(
            "DNS record not found"
        )

        response = client.post("/api/teams/team-123/branding/domain/verify")

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "DNS record not found" in data["error"]


class TestRemoveCustomDomain:
    """Tests for DELETE /api/teams/{team_id}/branding/domain endpoint."""

    def test_remove_custom_domain_success(self, client, mock_branding_service):
        """Test successful custom domain removal."""
        mock_branding_service.remove_custom_domain.return_value = None

        response = client.delete("/api/teams/team-123/branding/domain")

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert "removed" in data["message"].lower()

    def test_remove_custom_domain_permission_denied(self, client, mock_team_service):
        """Test custom domain removal with permission denied."""
        mock_team_service.require_permission.side_effect = Exception("Not authorized")

        response = client.delete("/api/teams/team-123/branding/domain")

        assert response.status_code == 403


class TestPublicBrandingByDomain:
    """Tests for GET /api/branding/by-domain/{domain} endpoint (public)."""

    def test_get_branding_by_domain_success(
        self, client, mock_branding_service, mock_branding
    ):
        """Test successful public branding retrieval by domain."""
        mock_branding_service.get_branding_by_domain.return_value = mock_branding

        response = client.get("/api/branding/by-domain/app.testbrand.com")

        assert response.status_code == 200
        data = response.json()
        assert data["brand_name"] == "Test Brand"
        assert data["logo_url"] == "https://example.com/logo.png"
        assert data["primary_color"] == "#2563EB"
        # Public endpoint should not return all fields
        assert "email_from_name" not in data
        assert "hide_powered_by" not in data

    def test_get_branding_by_domain_not_found(self, client, mock_branding_service):
        """Test public branding retrieval when domain not found."""
        mock_branding_service.get_branding_by_domain.return_value = None

        response = client.get("/api/branding/by-domain/unknown.example.com")

        assert response.status_code == 404
        assert "No branding found" in response.json()["detail"]


class TestPublicBrandingNoAuth:
    """Test that public endpoint works without auth."""

    def test_get_branding_by_domain_no_auth_required(self, mock_branding):
        """Test that public endpoint does not require authentication."""
        # Create app without auth override
        app = create_app()
        mock_branding_service = AsyncMock()
        mock_branding_service.get_branding_by_domain.return_value = mock_branding
        app.dependency_overrides[get_branding_service] = lambda: mock_branding_service

        client = TestClient(app)

        # No auth header provided - should still work for public endpoint
        response = client.get("/api/branding/by-domain/app.testbrand.com")

        assert response.status_code == 200
        app.dependency_overrides.clear()


class TestBrandingEdgeCases:
    """Edge case tests for branding API."""

    def test_branding_with_null_optional_fields(
        self, client, mock_branding_service
    ):
        """Test branding response with null optional fields."""
        # Create branding with many null optional fields
        # Note: secondary_color and accent_color have defaults in the schema
        minimal_branding = MagicMock(
            id=str(uuid4()),
            team_id="team-123",
            logo_url=None,
            logo_dark_url=None,
            favicon_url=None,
            brand_name="Minimal Brand",
            primary_color="#2563EB",
            secondary_color="#64748B",  # Default value (required field)
            accent_color="#F59E0B",  # Default value (required field)
            background_color="#FFFFFF",
            text_color="#1F2937",
            heading_font=None,
            body_font=None,
            font_scale=1.0,
            custom_domain=None,
            custom_domain_verified=False,
            custom_domain_verified_at=None,
            email_from_name=None,
            email_footer_text=None,
            meta_description=None,
            social_preview_image_url=None,
            custom_login_background_url=None,
            hide_powered_by=False,
            created_at=datetime.now(timezone.utc),
            updated_at=datetime.now(timezone.utc),
        )

        mock_branding_service.get_or_create_branding.return_value = minimal_branding

        response = client.get("/api/teams/team-123/branding")

        assert response.status_code == 200
        data = response.json()
        assert data["brand_name"] == "Minimal Brand"
        assert data["logo_url"] is None
        assert data["custom_domain"] is None

    def test_update_branding_with_empty_body(
        self, client, mock_branding_service, mock_branding
    ):
        """Test branding update with empty request body."""
        mock_branding_service.update_branding.return_value = mock_branding

        response = client.patch(
            "/api/teams/team-123/branding",
            json={},
        )

        assert response.status_code == 200

    def test_special_characters_in_domain(self, client, mock_branding_service):
        """Test domain lookup with special characters."""
        mock_branding_service.get_branding_by_domain.return_value = None

        # URL-encoded special characters
        response = client.get("/api/branding/by-domain/test%2Bdomain.com")

        # Should handle gracefully (404 is expected for non-existent domain)
        assert response.status_code == 404
