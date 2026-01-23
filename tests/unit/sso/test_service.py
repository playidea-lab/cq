"""Tests for SSO service."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta

import pytest

from c4.services.sso.models import (
    SSOConfig,
    SSOProvider,
    SSOUserInfo,
)
from c4.services.sso.providers.google import GoogleOIDCProvider
from c4.services.sso.providers.microsoft import MicrosoftOIDCProvider
from c4.services.sso.service import (
    SSOConfigNotFoundError,
    SSODomainNotAllowedError,
    SSOError,
    SSOProviderNotSupportedError,
    SSOService,
)


class TestSSOService:
    """Tests for SSO service."""

    @pytest.fixture
    def service(self):
        """Create SSO service instance."""
        # Register providers
        SSOService.register_provider(SSOProvider.GOOGLE, GoogleOIDCProvider)
        SSOService.register_provider(SSOProvider.MICROSOFT, MicrosoftOIDCProvider)
        return SSOService()

    @pytest.fixture
    def sample_config(self):
        """Create a sample SSO config."""
        return SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.GOOGLE,
            client_id="google-client-id",
            enabled=True,
            verified=True,
            allowed_domains=["example.com"],
        )

    @pytest.fixture
    def sample_user_info(self):
        """Create sample user info."""
        return SSOUserInfo(
            provider_user_id="google-user-123",
            email="user@example.com",
            name="John Doe",
            email_verified=True,
        )

    # Provider Tests
    def test_get_provider_google(self, service):
        """Test getting Google provider."""
        provider = service.get_provider(SSOProvider.GOOGLE)
        assert provider is not None
        assert provider.provider_type == SSOProvider.GOOGLE

    def test_get_provider_microsoft(self, service):
        """Test getting Microsoft provider."""
        provider = service.get_provider(SSOProvider.MICROSOFT)
        assert provider is not None
        assert provider.provider_type == SSOProvider.MICROSOFT

    def test_get_provider_unsupported(self, service):
        """Test getting unsupported provider raises error."""
        with pytest.raises(SSOProviderNotSupportedError):
            service.get_provider(SSOProvider.SAML)

    # Config Tests
    @pytest.mark.asyncio
    async def test_create_config(self, service):
        """Test creating SSO config."""
        config = await service.create_config(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            client_id="client-id",
        )

        assert config is not None
        assert config.team_id == "team-123"
        assert config.provider == SSOProvider.GOOGLE
        assert config.enabled is False  # Disabled by default
        assert config.verified is False  # Not verified by default

    @pytest.mark.asyncio
    async def test_get_config(self, service):
        """Test getting SSO config."""
        # Create config first
        await service.create_config(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            client_id="client-id",
        )

        config = await service.get_config("team-123")

        assert config is not None
        assert config.team_id == "team-123"
        assert config.provider == SSOProvider.GOOGLE

    @pytest.mark.asyncio
    async def test_get_config_not_found(self, service):
        """Test getting non-existent config returns None."""
        config = await service.get_config("non-existent-team")
        assert config is None

    @pytest.mark.asyncio
    async def test_enable_config_requires_verification(self, service):
        """Test enabling config requires verification."""
        await service.create_config(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            client_id="client-id",
        )

        with pytest.raises(SSOError, match="verified"):
            await service.enable_config("team-123")

    @pytest.mark.asyncio
    async def test_enable_config_after_verification(self, service):
        """Test enabling config after verification."""
        await service.create_config(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            client_id="client-id",
        )

        # Manually verify
        config = await service.get_config("team-123")
        config.verified = True
        service._configs["team-123"] = config

        # Now enable
        config = await service.enable_config("team-123")
        assert config.enabled is True

    @pytest.mark.asyncio
    async def test_disable_config(self, service):
        """Test disabling SSO config."""
        await service.create_config(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            client_id="client-id",
        )

        # Verify and enable first
        config = await service.get_config("team-123")
        config.verified = True
        config.enabled = True
        service._configs["team-123"] = config

        # Now disable
        config = await service.disable_config("team-123")
        assert config.enabled is False

    @pytest.mark.asyncio
    async def test_delete_config(self, service):
        """Test deleting SSO config."""
        await service.create_config(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            client_id="client-id",
        )

        result = await service.delete_config("team-123")
        assert result is True

        config = await service.get_config("team-123")
        assert config is None

    @pytest.mark.asyncio
    async def test_delete_config_not_found(self, service):
        """Test deleting non-existent config returns False."""
        result = await service.delete_config("non-existent")
        assert result is False

    # Login Flow Tests
    @pytest.mark.asyncio
    async def test_initiate_login_disabled(self, service):
        """Test initiating login with disabled SSO."""
        await service.create_config(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            client_id="client-id",
        )

        with pytest.raises(SSOError, match="not enabled"):
            await service.initiate_login(
                team_id="team-123",
                redirect_uri="https://app.example.com/callback",
            )

    @pytest.mark.asyncio
    async def test_initiate_login_success(self, service):
        """Test initiating SSO login."""
        await service.create_config(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            client_id="google-client-id",
        )

        # Verify and enable
        config = await service.get_config("team-123")
        config.verified = True
        config.enabled = True
        service._configs["team-123"] = config

        url, state = await service.initiate_login(
            team_id="team-123",
            redirect_uri="https://app.example.com/callback",
        )

        assert url is not None
        assert "accounts.google.com" in url
        assert "client_id=google-client-id" in url
        assert state is not None

    @pytest.mark.asyncio
    async def test_initiate_login_not_configured(self, service):
        """Test initiating login without config."""
        with pytest.raises(SSOConfigNotFoundError):
            await service.initiate_login(
                team_id="non-existent",
                redirect_uri="https://app.example.com/callback",
            )

    # Session Tests
    @pytest.mark.asyncio
    async def test_create_and_get_session(self, service, sample_user_info):
        """Test creating and getting SSO session."""
        session = await service._create_session(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            user_info=sample_user_info,
        )

        assert session is not None
        assert session.team_id == "team-123"
        assert session.provider == SSOProvider.GOOGLE
        assert session.provider_email == sample_user_info.email

        # Get session
        retrieved = await service.get_session(session.id)
        assert retrieved is not None
        assert retrieved.id == session.id

    @pytest.mark.asyncio
    async def test_get_session_expired(self, service, sample_user_info):
        """Test getting expired session returns None."""
        session = await service._create_session(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            user_info=sample_user_info,
        )

        # Expire the session
        session.expires_at = datetime.now(UTC) - timedelta(hours=1)
        service._sessions[session.id] = session

        retrieved = await service.get_session(session.id)
        assert retrieved is None

    @pytest.mark.asyncio
    async def test_get_session_not_found(self, service):
        """Test getting non-existent session returns None."""
        session = await service.get_session("non-existent")
        assert session is None

    @pytest.mark.asyncio
    async def test_revoke_session(self, service, sample_user_info):
        """Test revoking SSO session."""
        session = await service._create_session(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            user_info=sample_user_info,
        )

        result = await service.revoke_session(session.id, reason="logout")
        assert result is True

        # Session should be revoked
        retrieved = await service.get_session(session.id)
        assert retrieved is None

    @pytest.mark.asyncio
    async def test_revoke_session_not_found(self, service):
        """Test revoking non-existent session returns False."""
        result = await service.revoke_session("non-existent")
        assert result is False

    @pytest.mark.asyncio
    async def test_revoke_user_sessions(self, service, sample_user_info):
        """Test revoking all sessions for a user."""
        # Create multiple sessions
        session1 = await service._create_session(
            team_id="team-123",
            provider=SSOProvider.GOOGLE,
            user_info=sample_user_info,
        )
        session2 = await service._create_session(
            team_id="team-456",
            provider=SSOProvider.GOOGLE,
            user_info=sample_user_info,
        )

        count = await service.revoke_user_sessions(
            user_id=sample_user_info.provider_user_id,
            reason="security",
        )

        assert count == 2

    # Domain Verification Tests
    @pytest.mark.asyncio
    async def test_create_domain_verification(self, service):
        """Test creating domain verification."""
        verification = await service.create_domain_verification(
            team_id="team-123",
            domain="example.com",
            method="dns_txt",
        )

        assert verification is not None
        assert verification.domain == "example.com"
        assert verification.verification_method == "dns_txt"
        assert verification.verified is False
        assert "c4-verify=" in verification.verification_token

    @pytest.mark.asyncio
    async def test_verify_domain(self, service):
        """Test verifying domain."""
        await service.create_domain_verification(
            team_id="team-123",
            domain="example.com",
        )

        result = await service.verify_domain("team-123", "example.com")
        assert result is True

    @pytest.mark.asyncio
    async def test_verify_domain_not_found(self, service):
        """Test verifying domain without verification record."""
        result = await service.verify_domain("team-123", "unknown.com")
        assert result is False


class TestSSOExceptions:
    """Tests for SSO exceptions."""

    def test_sso_error(self):
        """Test base SSO error."""
        error = SSOError("Something went wrong")
        assert str(error) == "Something went wrong"
        assert error.code == "sso_error"

    def test_sso_error_with_code(self):
        """Test SSO error with custom code."""
        error = SSOError("Custom error", "custom_code")
        assert str(error) == "Custom error"
        assert error.code == "custom_code"

    def test_config_not_found_error(self):
        """Test config not found error."""
        error = SSOConfigNotFoundError("team-123")
        assert "team-123" in str(error)
        assert error.code == "config_not_found"

    def test_provider_not_supported_error(self):
        """Test provider not supported error."""
        error = SSOProviderNotSupportedError("saml")
        assert "saml" in str(error)
        assert error.code == "provider_not_supported"

    def test_domain_not_allowed_error(self):
        """Test domain not allowed error."""
        error = SSODomainNotAllowedError("user@other.com")
        assert "other.com" in str(error)
        assert error.code == "domain_not_allowed"
