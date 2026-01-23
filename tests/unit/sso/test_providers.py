"""Tests for SSO providers."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from c4.services.sso.models import SSOConfig, SSOProvider
from c4.services.sso.providers.google import GoogleOIDCProvider
from c4.services.sso.providers.microsoft import MicrosoftOIDCProvider


class TestGoogleOIDCProvider:
    """Tests for Google OIDC provider."""

    @pytest.fixture
    def provider(self):
        """Create a Google provider instance."""
        return GoogleOIDCProvider()

    @pytest.fixture
    def config(self):
        """Create a sample SSO config."""
        return SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.GOOGLE,
            client_id="google-client-id",
        )

    def test_provider_type(self, provider):
        """Test provider type."""
        assert provider.provider_type == SSOProvider.GOOGLE

    def test_provider_name(self, provider):
        """Test provider display name."""
        assert provider.name == "Google"

    def test_discovery_url(self, provider):
        """Test discovery URL."""
        assert "accounts.google.com" in provider.discovery_url
        assert ".well-known/openid-configuration" in provider.discovery_url

    def test_default_scopes(self, provider):
        """Test default OIDC scopes."""
        scopes = provider.default_scopes
        assert "openid" in scopes
        assert "email" in scopes
        assert "profile" in scopes

    def test_get_authorization_url(self, provider, config):
        """Test generating authorization URL."""
        url = provider.get_authorization_url(
            config=config,
            redirect_uri="https://app.example.com/callback",
            state="random-state-123",
            nonce="nonce-456",
        )

        assert "accounts.google.com" in url
        assert "client_id=google-client-id" in url
        assert "redirect_uri=" in url
        assert "state=random-state-123" in url
        assert "response_type=code" in url
        assert "scope=" in url

    def test_get_authorization_url_with_hd(self, provider):
        """Test authorization URL with hosted domain."""
        config = SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.GOOGLE,
            client_id="google-client-id",
            allowed_domains=["example.com"],
        )

        url = provider.get_authorization_url(
            config=config,
            redirect_uri="https://app.example.com/callback",
            state="state",
        )

        assert "hd=example.com" in url

    def test_get_authorization_url_additional_scopes(self, provider, config):
        """Test authorization URL with additional scopes."""
        url = provider.get_authorization_url(
            config=config,
            redirect_uri="https://app.example.com/callback",
            state="state",
            scopes=["https://www.googleapis.com/auth/calendar"],
        )

        assert "calendar" in url

    @pytest.mark.asyncio
    async def test_exchange_code_success(self, provider, config):
        """Test successful code exchange."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "access_token": "access-token",
            "refresh_token": "refresh-token",
            "id_token": "id-token",
            "expires_in": 3600,
        }

        mock_user_response = MagicMock()
        mock_user_response.status_code = 200
        mock_user_response.json.return_value = {
            "sub": "google-user-123",
            "email": "user@example.com",
            "name": "John Doe",
            "email_verified": True,
        }
        mock_user_response.raise_for_status = MagicMock()

        with patch("httpx.AsyncClient") as mock_client:
            mock_instance = AsyncMock()
            mock_instance.post = AsyncMock(return_value=mock_response)
            mock_instance.get = AsyncMock(return_value=mock_user_response)
            mock_instance.__aenter__ = AsyncMock(return_value=mock_instance)
            mock_instance.__aexit__ = AsyncMock()
            mock_client.return_value = mock_instance

            result = await provider.exchange_code(
                config=config,
                code="auth-code",
                redirect_uri="https://app.example.com/callback",
            )

        assert result.success is True
        assert result.access_token == "access-token"
        assert result.refresh_token == "refresh-token"
        assert result.user_info is not None
        assert result.user_info.email == "user@example.com"

    @pytest.mark.asyncio
    async def test_exchange_code_failure(self, provider, config):
        """Test failed code exchange."""
        mock_response = MagicMock()
        mock_response.status_code = 400
        mock_response.text = "Invalid grant"

        with patch("httpx.AsyncClient") as mock_client:
            mock_instance = AsyncMock()
            mock_instance.post = AsyncMock(return_value=mock_response)
            mock_instance.__aenter__ = AsyncMock(return_value=mock_instance)
            mock_instance.__aexit__ = AsyncMock()
            mock_client.return_value = mock_instance

            result = await provider.exchange_code(
                config=config,
                code="invalid-code",
                redirect_uri="https://app.example.com/callback",
            )

        assert result.success is False
        assert result.error_code == "token_exchange_failed"

    @pytest.mark.asyncio
    async def test_revoke_token(self, provider, config):
        """Test token revocation."""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch("httpx.AsyncClient") as mock_client:
            mock_instance = AsyncMock()
            mock_instance.post = AsyncMock(return_value=mock_response)
            mock_instance.__aenter__ = AsyncMock(return_value=mock_instance)
            mock_instance.__aexit__ = AsyncMock()
            mock_client.return_value = mock_instance

            result = await provider.revoke_token(config, "access-token")

        assert result is True

    def test_validate_config(self, provider, config):
        """Test config validation."""
        errors = provider.validate_config(config)
        assert len(errors) == 0

    def test_validate_config_missing_client_id(self, provider):
        """Test config validation with missing client_id."""
        config = SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.GOOGLE,
            client_id="",
        )
        errors = provider.validate_config(config)
        assert len(errors) > 0
        assert any("client_id" in e.lower() for e in errors)


class TestMicrosoftOIDCProvider:
    """Tests for Microsoft OIDC provider."""

    @pytest.fixture
    def provider(self):
        """Create a Microsoft provider instance."""
        return MicrosoftOIDCProvider()

    @pytest.fixture
    def config(self):
        """Create a sample SSO config."""
        return SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.MICROSOFT,
            client_id="microsoft-client-id",
        )

    def test_provider_type(self, provider):
        """Test provider type."""
        assert provider.provider_type == SSOProvider.MICROSOFT

    def test_provider_name(self, provider):
        """Test provider display name."""
        assert provider.name == "Microsoft"

    def test_discovery_url(self, provider):
        """Test discovery URL."""
        assert "login.microsoftonline.com" in provider.discovery_url
        assert ".well-known/openid-configuration" in provider.discovery_url

    def test_default_scopes(self, provider):
        """Test default OIDC scopes."""
        scopes = provider.default_scopes
        assert "openid" in scopes
        assert "email" in scopes
        assert "profile" in scopes
        assert "User.Read" in scopes
        assert "offline_access" in scopes

    def test_get_authorization_url(self, provider, config):
        """Test generating authorization URL."""
        url = provider.get_authorization_url(
            config=config,
            redirect_uri="https://app.example.com/callback",
            state="random-state-123",
        )

        assert "login.microsoftonline.com" in url
        assert "client_id=microsoft-client-id" in url
        assert "state=random-state-123" in url
        assert "response_type=code" in url
        assert "prompt=select_account" in url

    def test_get_authorization_url_with_domain_hint(self, provider):
        """Test authorization URL with domain hint."""
        config = SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.MICROSOFT,
            client_id="microsoft-client-id",
            allowed_domains=["contoso.com"],
        )

        url = provider.get_authorization_url(
            config=config,
            redirect_uri="https://app.example.com/callback",
            state="state",
        )

        assert "domain_hint=contoso.com" in url

    def test_get_tenant_url(self, provider, config):
        """Test tenant URL generation."""
        # Default to common
        url = provider._get_tenant_url(config, "authorize")
        assert "common" in url

        # With custom issuer
        config_with_issuer = SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.MICROSOFT,
            client_id="client-id",
            issuer_url="https://login.microsoftonline.com/tenant-id",
        )
        url = provider._get_tenant_url(config_with_issuer, "token")
        assert "tenant-id" in url
        assert "oauth2/v2.0/token" in url

    @pytest.mark.asyncio
    async def test_get_user_info(self, provider, config):
        """Test getting user info from Microsoft Graph."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "id": "ms-user-123",
            "mail": "user@contoso.com",
            "displayName": "John Doe",
            "givenName": "John",
            "surname": "Doe",
        }
        mock_response.raise_for_status = MagicMock()

        with patch("httpx.AsyncClient") as mock_client:
            mock_instance = AsyncMock()
            mock_instance.get = AsyncMock(return_value=mock_response)
            mock_instance.__aenter__ = AsyncMock(return_value=mock_instance)
            mock_instance.__aexit__ = AsyncMock()
            mock_client.return_value = mock_instance

            user_info = await provider.get_user_info(config, "access-token")

        assert user_info.provider_user_id == "ms-user-123"
        assert user_info.email == "user@contoso.com"
        assert user_info.name == "John Doe"
        assert user_info.given_name == "John"
        assert user_info.family_name == "Doe"

    @pytest.mark.asyncio
    async def test_revoke_token_not_supported(self, provider, config):
        """Test that Microsoft token revocation returns True (not supported via API)."""
        result = await provider.revoke_token(config, "access-token")
        assert result is True

    def test_validate_config(self, provider, config):
        """Test config validation."""
        errors = provider.validate_config(config)
        assert len(errors) == 0

    def test_validate_config_missing_client_id(self, provider):
        """Test config validation with missing client_id."""
        config = SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.MICROSOFT,
            client_id="",
        )
        errors = provider.validate_config(config)
        assert len(errors) > 0
        assert any("client_id" in e.lower() for e in errors)
