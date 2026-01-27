"""Tests for SSO models."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta

from c4.services.sso.models import (
    SSOAuthResult,
    SSOConfig,
    SSODomainVerification,
    SSOProvider,
    SSOSession,
    SSOStatus,
    SSOUserInfo,
)


class TestSSOProvider:
    """Tests for SSOProvider enum."""

    def test_provider_values(self):
        """Test provider enum values."""
        assert SSOProvider.GOOGLE.value == "google"
        assert SSOProvider.MICROSOFT.value == "microsoft"
        assert SSOProvider.OKTA.value == "okta"
        assert SSOProvider.SAML.value == "saml"

    def test_provider_from_string(self):
        """Test creating provider from string."""
        assert SSOProvider("google") == SSOProvider.GOOGLE
        assert SSOProvider("microsoft") == SSOProvider.MICROSOFT


class TestSSOStatus:
    """Tests for SSOStatus enum."""

    def test_status_values(self):
        """Test status enum values."""
        assert SSOStatus.ACTIVE.value == "active"
        assert SSOStatus.PENDING.value == "pending"
        assert SSOStatus.DISABLED.value == "disabled"
        assert SSOStatus.ERROR.value == "error"


class TestSSOConfig:
    """Tests for SSOConfig model."""

    def test_minimal_config(self):
        """Test creating config with minimal required fields."""
        config = SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.GOOGLE,
            client_id="client-id",
        )

        assert config.id == "config-123"
        assert config.team_id == "team-456"
        assert config.provider == SSOProvider.GOOGLE
        assert config.client_id == "client-id"
        assert config.enabled is False
        assert config.verified is False

    def test_full_config(self):
        """Test creating config with all fields."""
        config = SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.MICROSOFT,
            client_id="client-id",
            issuer_url="https://login.microsoftonline.com/tenant",
            allowed_domains=["example.com", "corp.example.com"],
            auto_provision=True,
            default_role="member",
            enabled=True,
            verified=True,
        )

        assert config.issuer_url == "https://login.microsoftonline.com/tenant"
        assert "example.com" in config.allowed_domains
        assert config.auto_provision is True
        assert config.enabled is True

    def test_saml_config(self):
        """Test SAML-specific config fields."""
        config = SSOConfig(
            id="config-123",
            team_id="team-456",
            provider=SSOProvider.SAML,
            client_id="entity-id",
            entity_id="https://example.com/saml",
            sso_url="https://idp.example.com/sso",
            certificate="-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
        )

        assert config.provider == SSOProvider.SAML
        assert config.entity_id == "https://example.com/saml"
        assert config.sso_url == "https://idp.example.com/sso"
        assert "BEGIN CERTIFICATE" in config.certificate

    def test_is_oidc(self):
        """Test OIDC provider detection."""
        google_config = SSOConfig(
            id="1", team_id="t1", provider=SSOProvider.GOOGLE, client_id="c"
        )
        assert google_config.is_oidc() is True

        microsoft_config = SSOConfig(
            id="2", team_id="t2", provider=SSOProvider.MICROSOFT, client_id="c"
        )
        assert microsoft_config.is_oidc() is True

        saml_config = SSOConfig(
            id="3", team_id="t3", provider=SSOProvider.SAML, client_id="c"
        )
        assert saml_config.is_oidc() is False

    def test_is_saml(self):
        """Test SAML provider detection."""
        saml_config = SSOConfig(
            id="1", team_id="t1", provider=SSOProvider.SAML, client_id="c"
        )
        assert saml_config.is_saml() is True

        okta_config = SSOConfig(
            id="2", team_id="t2", provider=SSOProvider.OKTA, client_id="c"
        )
        assert okta_config.is_saml() is True

        google_config = SSOConfig(
            id="3", team_id="t3", provider=SSOProvider.GOOGLE, client_id="c"
        )
        assert google_config.is_saml() is False


class TestSSOUserInfo:
    """Tests for SSOUserInfo model."""

    def test_minimal_user_info(self):
        """Test creating user info with minimal fields."""
        user_info = SSOUserInfo(
            provider_user_id="user-123",
            email="user@example.com",
        )

        assert user_info.provider_user_id == "user-123"
        assert user_info.email == "user@example.com"
        assert user_info.email_verified is False

    def test_full_user_info(self):
        """Test creating user info with all fields."""
        user_info = SSOUserInfo(
            provider_user_id="user-123",
            email="john.doe@example.com",
            name="John Doe",
            given_name="John",
            family_name="Doe",
            picture="https://example.com/photo.jpg",
            email_verified=True,
            locale="en-US",
            raw_claims={"sub": "user-123", "custom_claim": "value"},
        )

        assert user_info.name == "John Doe"
        assert user_info.given_name == "John"
        assert user_info.email_verified is True
        assert user_info.raw_claims["custom_claim"] == "value"


class TestSSOSession:
    """Tests for SSOSession model."""

    def test_create_session(self):
        """Test creating an SSO session."""
        expires_at = datetime.now(UTC) + timedelta(hours=1)
        session = SSOSession(
            id="session-123",
            user_id="user-456",
            team_id="team-789",
            provider=SSOProvider.GOOGLE,
            provider_user_id="google-user-123",
            expires_at=expires_at,
        )

        assert session.id == "session-123"
        assert session.user_id == "user-456"
        assert session.provider == SSOProvider.GOOGLE
        assert session.expires_at == expires_at

    def test_session_is_valid(self):
        """Test session validity check."""
        # Valid session (expires in future)
        valid_session = SSOSession(
            id="session-1",
            user_id="user-1",
            team_id="team-1",
            provider=SSOProvider.GOOGLE,
            provider_user_id="google-1",
            expires_at=datetime.now(UTC) + timedelta(hours=1),
        )
        assert valid_session.is_valid() is True

        # Expired session
        expired_session = SSOSession(
            id="session-2",
            user_id="user-2",
            team_id="team-2",
            provider=SSOProvider.GOOGLE,
            provider_user_id="google-2",
            expires_at=datetime.now(UTC) - timedelta(hours=1),
        )
        assert expired_session.is_valid() is False

    def test_session_revoked_is_invalid(self):
        """Test that revoked session is invalid."""
        session = SSOSession(
            id="session-1",
            user_id="user-1",
            team_id="team-1",
            provider=SSOProvider.GOOGLE,
            provider_user_id="google-1",
            expires_at=datetime.now(UTC) + timedelta(hours=1),
            revoked=True,
        )
        assert session.is_valid() is False

    def test_session_without_expiry_is_valid(self):
        """Test session without expiry is valid if not revoked."""
        session = SSOSession(
            id="session-1",
            user_id="user-1",
            team_id="team-1",
            provider=SSOProvider.GOOGLE,
            provider_user_id="google-1",
            expires_at=None,
        )
        assert session.is_valid() is True


class TestSSOAuthResult:
    """Tests for SSOAuthResult model."""

    def test_successful_result(self):
        """Test successful auth result."""
        user_info = SSOUserInfo(
            provider_user_id="user-123",
            email="user@example.com",
        )
        result = SSOAuthResult(
            success=True,
            user_info=user_info,
            access_token="access-token",
            refresh_token="refresh-token",
            expires_at=datetime.now(UTC) + timedelta(hours=1),
        )

        assert result.success is True
        assert result.user_info.email == "user@example.com"
        assert result.access_token == "access-token"
        assert result.error is None

    def test_failed_result(self):
        """Test failed auth result."""
        result = SSOAuthResult(
            success=False,
            error="Invalid credentials",
            error_code="invalid_credentials",
        )

        assert result.success is False
        assert result.error == "Invalid credentials"
        assert result.error_code == "invalid_credentials"
        assert result.user_info is None


class TestSSODomainVerification:
    """Tests for SSODomainVerification model."""

    def test_create_verification(self):
        """Test creating domain verification."""
        verification = SSODomainVerification(
            id="verify-123",
            team_id="team-456",
            domain="example.com",
            verification_method="dns_txt",
            verification_token="abc123",
        )

        assert verification.domain == "example.com"
        assert verification.verification_method == "dns_txt"
        assert verification.verified is False

    def test_verified_domain(self):
        """Test verified domain."""
        verification = SSODomainVerification(
            id="verify-123",
            team_id="team-456",
            domain="example.com",
            verification_method="dns_txt",
            verification_token="abc123",
            verified=True,
            verified_at=datetime.now(UTC),
        )

        assert verification.verified is True
        assert verification.verified_at is not None
