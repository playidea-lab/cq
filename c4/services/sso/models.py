"""SSO Models.

Data models for SSO/SAML authentication.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any


class SSOProvider(str, Enum):
    """Supported SSO providers."""

    GOOGLE = "google"
    MICROSOFT = "microsoft"
    OKTA = "okta"
    SAML = "saml"  # Generic SAML provider


class SSOStatus(str, Enum):
    """SSO configuration status."""

    ACTIVE = "active"
    PENDING = "pending"
    DISABLED = "disabled"
    ERROR = "error"


@dataclass
class SSOConfig:
    """SSO configuration for a team.

    Attributes:
        id: Unique identifier
        team_id: Team this config belongs to
        provider: SSO provider type
        client_id: OAuth2/OIDC client ID (for Google, Microsoft)
        issuer_url: OIDC issuer URL
        entity_id: SAML entity ID
        sso_url: SAML SSO URL
        slo_url: SAML Single Logout URL
        certificate: SAML X.509 certificate
        auto_provision: Enable JIT user provisioning
        default_role: Default role for new users
        allowed_domains: Allowed email domains
        enabled: Whether SSO is enabled
        verified: Whether configuration is verified
        created_at: Creation timestamp
        updated_at: Last update timestamp
    """

    id: str
    team_id: str
    provider: SSOProvider

    # OIDC settings (Google, Microsoft)
    client_id: str | None = None
    issuer_url: str | None = None

    # SAML settings (Okta, custom)
    entity_id: str | None = None
    sso_url: str | None = None
    slo_url: str | None = None
    certificate: str | None = None

    # Common settings
    auto_provision: bool = True
    default_role: str = "member"
    allowed_domains: list[str] = field(default_factory=list)

    # Status
    enabled: bool = False
    verified: bool = False

    # Timestamps
    created_at: datetime | None = None
    updated_at: datetime | None = None
    created_by: str | None = None

    def is_oidc(self) -> bool:
        """Check if this is an OIDC provider."""
        return self.provider in (SSOProvider.GOOGLE, SSOProvider.MICROSOFT)

    def is_saml(self) -> bool:
        """Check if this is a SAML provider."""
        return self.provider in (SSOProvider.OKTA, SSOProvider.SAML)


@dataclass
class SSOSession:
    """SSO session for tracking and audit.

    Attributes:
        id: Unique identifier
        user_id: Associated user ID
        team_id: Team ID
        provider: SSO provider used
        provider_user_id: User ID from provider
        provider_email: Email from provider
        session_token_hash: Hash of session token
        assertion_id: SAML assertion ID (if SAML)
        assertion_hash: Hash of SAML assertion
        authenticated_at: Authentication timestamp
        expires_at: Session expiry timestamp
        last_activity_at: Last activity timestamp
        revoked: Whether session is revoked
        revoked_at: Revocation timestamp
        revoked_reason: Reason for revocation
    """

    id: str
    user_id: str
    team_id: str
    provider: SSOProvider

    provider_user_id: str | None = None
    provider_email: str | None = None

    session_token_hash: str | None = None
    assertion_id: str | None = None
    assertion_hash: str | None = None

    authenticated_at: datetime | None = None
    expires_at: datetime | None = None
    last_activity_at: datetime | None = None

    revoked: bool = False
    revoked_at: datetime | None = None
    revoked_reason: str | None = None

    created_at: datetime | None = None


@dataclass
class SSOUserInfo:
    """User information from SSO provider.

    Attributes:
        provider_user_id: User ID from provider
        email: User email
        name: User display name
        given_name: First name
        family_name: Last name
        picture: Profile picture URL
        email_verified: Whether email is verified
        locale: User locale
        raw_claims: Raw claims from provider
    """

    provider_user_id: str
    email: str
    name: str | None = None
    given_name: str | None = None
    family_name: str | None = None
    picture: str | None = None
    email_verified: bool = False
    locale: str | None = None
    raw_claims: dict[str, Any] = field(default_factory=dict)


@dataclass
class SSOAuthResult:
    """Result of SSO authentication.

    Attributes:
        success: Whether authentication succeeded
        user_info: User information from provider
        session_id: Created session ID
        access_token: Access token for API calls
        refresh_token: Refresh token (if available)
        expires_at: Token expiry timestamp
        error: Error message if failed
        error_code: Error code if failed
    """

    success: bool
    user_info: SSOUserInfo | None = None
    session_id: str | None = None
    access_token: str | None = None
    refresh_token: str | None = None
    expires_at: datetime | None = None
    error: str | None = None
    error_code: str | None = None


@dataclass
class SSODomainVerification:
    """Domain verification for SSO.

    Attributes:
        id: Unique identifier
        team_id: Team ID
        domain: Domain to verify
        verification_method: Method (dns_txt, dns_cname, meta_tag)
        verification_token: Token to use for verification
        verified: Whether domain is verified
        verified_at: Verification timestamp
        created_at: Creation timestamp
        expires_at: Token expiry timestamp
    """

    id: str
    team_id: str
    domain: str
    verification_method: str
    verification_token: str

    verified: bool = False
    verified_at: datetime | None = None
    created_at: datetime | None = None
    expires_at: datetime | None = None
