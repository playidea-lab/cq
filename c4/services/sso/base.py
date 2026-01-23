"""SSO Provider Base Class.

Abstract base class for all SSO providers (OIDC and SAML).
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import Any

from c4.services.sso.models import (
    SSOAuthResult,
    SSOConfig,
    SSOProvider,
    SSOUserInfo,
)


class SSOProviderBase(ABC):
    """Abstract base class for SSO providers.

    All SSO providers (Google, Microsoft, Okta, etc.) must implement
    this interface to be used with the SSOService.
    """

    @property
    @abstractmethod
    def provider_type(self) -> SSOProvider:
        """Return the provider type."""
        ...

    @property
    @abstractmethod
    def name(self) -> str:
        """Return the provider display name."""
        ...

    @abstractmethod
    def get_authorization_url(
        self,
        config: SSOConfig,
        redirect_uri: str,
        state: str,
        *,
        nonce: str | None = None,
        scopes: list[str] | None = None,
    ) -> str:
        """Generate the authorization URL for SSO login.

        Args:
            config: SSO configuration for the team
            redirect_uri: Callback URL after authentication
            state: State parameter for CSRF protection
            nonce: Nonce for replay protection (OIDC)
            scopes: Additional scopes to request

        Returns:
            Authorization URL to redirect user to
        """
        ...

    @abstractmethod
    async def exchange_code(
        self,
        config: SSOConfig,
        code: str,
        redirect_uri: str,
        *,
        nonce: str | None = None,
    ) -> SSOAuthResult:
        """Exchange authorization code for tokens.

        Args:
            config: SSO configuration for the team
            code: Authorization code from callback
            redirect_uri: Callback URL used in authorization
            nonce: Nonce used in authorization (for validation)

        Returns:
            Authentication result with tokens and user info
        """
        ...

    @abstractmethod
    async def get_user_info(
        self,
        config: SSOConfig,
        access_token: str,
    ) -> SSOUserInfo:
        """Get user information from provider.

        Args:
            config: SSO configuration for the team
            access_token: Access token from authentication

        Returns:
            User information from provider
        """
        ...

    @abstractmethod
    async def refresh_token(
        self,
        config: SSOConfig,
        refresh_token: str,
    ) -> SSOAuthResult:
        """Refresh access token.

        Args:
            config: SSO configuration for the team
            refresh_token: Refresh token from previous authentication

        Returns:
            New authentication result with refreshed tokens
        """
        ...

    @abstractmethod
    async def revoke_token(
        self,
        config: SSOConfig,
        token: str,
        *,
        token_type: str = "access_token",
    ) -> bool:
        """Revoke a token.

        Args:
            config: SSO configuration for the team
            token: Token to revoke
            token_type: Type of token (access_token or refresh_token)

        Returns:
            True if revocation succeeded
        """
        ...

    def validate_config(self, config: SSOConfig) -> list[str]:
        """Validate SSO configuration.

        Args:
            config: SSO configuration to validate

        Returns:
            List of validation errors (empty if valid)
        """
        errors = []

        if config.provider != self.provider_type:
            errors.append(
                f"Provider mismatch: expected {self.provider_type}, "
                f"got {config.provider}"
            )

        return errors

    def is_domain_allowed(self, config: SSOConfig, email: str) -> bool:
        """Check if email domain is allowed.

        Args:
            config: SSO configuration
            email: User email to check

        Returns:
            True if domain is allowed (or no restrictions)
        """
        if not config.allowed_domains:
            return True

        domain = email.split("@")[-1].lower()
        return domain in [d.lower() for d in config.allowed_domains]


class OIDCProviderBase(SSOProviderBase):
    """Base class for OIDC providers (Google, Microsoft).

    Provides common OIDC functionality.
    """

    @property
    @abstractmethod
    def discovery_url(self) -> str:
        """Return the OIDC discovery URL."""
        ...

    @property
    def default_scopes(self) -> list[str]:
        """Return default OIDC scopes."""
        return ["openid", "email", "profile"]

    async def discover_endpoints(self) -> dict[str, Any]:
        """Discover OIDC endpoints from provider.

        Returns:
            OIDC configuration from discovery endpoint
        """
        import httpx

        async with httpx.AsyncClient() as client:
            response = await client.get(self.discovery_url)
            response.raise_for_status()
            return response.json()

    def validate_config(self, config: SSOConfig) -> list[str]:
        """Validate OIDC configuration."""
        errors = super().validate_config(config)

        if not config.client_id:
            errors.append("client_id is required for OIDC")

        return errors


class SAMLProviderBase(SSOProviderBase):
    """Base class for SAML providers (Okta, custom).

    Provides common SAML functionality.
    """

    @abstractmethod
    async def parse_saml_response(
        self,
        config: SSOConfig,
        saml_response: str,
        *,
        relay_state: str | None = None,
    ) -> SSOAuthResult:
        """Parse SAML response from IdP.

        Args:
            config: SSO configuration
            saml_response: Base64-encoded SAML response
            relay_state: Relay state from request

        Returns:
            Authentication result
        """
        ...

    @abstractmethod
    def generate_saml_request(
        self,
        config: SSOConfig,
        redirect_uri: str,
        *,
        relay_state: str | None = None,
    ) -> str:
        """Generate SAML authentication request.

        Args:
            config: SSO configuration
            redirect_uri: Assertion consumer service URL
            relay_state: Relay state to include

        Returns:
            Encoded SAML request
        """
        ...

    def validate_config(self, config: SSOConfig) -> list[str]:
        """Validate SAML configuration."""
        errors = super().validate_config(config)

        if not config.entity_id:
            errors.append("entity_id is required for SAML")
        if not config.sso_url:
            errors.append("sso_url is required for SAML")
        if not config.certificate:
            errors.append("certificate is required for SAML")

        return errors
