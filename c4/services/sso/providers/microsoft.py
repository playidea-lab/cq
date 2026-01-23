"""Microsoft OIDC Provider.

Implements SSO authentication via Microsoft Entra ID (Azure AD).
"""

from __future__ import annotations

import logging
from urllib.parse import urlencode

import httpx

from c4.services.sso.base import OIDCProviderBase
from c4.services.sso.models import (
    SSOAuthResult,
    SSOConfig,
    SSOProvider,
    SSOUserInfo,
)

logger = logging.getLogger(__name__)


class MicrosoftOIDCProvider(OIDCProviderBase):
    """Microsoft Entra ID (Azure AD) OIDC provider.

    Supports Microsoft work/school accounts and personal Microsoft accounts.
    Uses Microsoft identity platform v2.0 endpoints.
    """

    # Microsoft identity platform endpoints
    # Using 'common' tenant for multi-tenant apps
    DISCOVERY_URL = "https://login.microsoftonline.com/common/v2.0/.well-known/openid-configuration"
    AUTH_URL = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
    TOKEN_URL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
    GRAPH_URL = "https://graph.microsoft.com/v1.0/me"

    @property
    def provider_type(self) -> SSOProvider:
        """Return the provider type."""
        return SSOProvider.MICROSOFT

    @property
    def name(self) -> str:
        """Return the provider display name."""
        return "Microsoft"

    @property
    def discovery_url(self) -> str:
        """Return the OIDC discovery URL."""
        return self.DISCOVERY_URL

    @property
    def default_scopes(self) -> list[str]:
        """Return default OIDC scopes."""
        return ["openid", "email", "profile", "User.Read", "offline_access"]

    def _get_tenant_url(self, config: SSOConfig, endpoint: str) -> str:
        """Get tenant-specific URL if issuer_url is configured.

        Args:
            config: SSO configuration
            endpoint: Endpoint path (authorize, token)

        Returns:
            Full URL for the endpoint
        """
        if config.issuer_url:
            # Custom tenant URL
            base = config.issuer_url.rstrip("/")
            return f"{base}/oauth2/v2.0/{endpoint}"

        # Default to common tenant
        return getattr(self, f"{endpoint.upper()}_URL")

    def get_authorization_url(
        self,
        config: SSOConfig,
        redirect_uri: str,
        state: str,
        *,
        nonce: str | None = None,
        scopes: list[str] | None = None,
    ) -> str:
        """Generate the authorization URL for Microsoft login.

        Args:
            config: SSO configuration
            redirect_uri: Callback URL
            state: State parameter for CSRF protection
            nonce: Nonce for replay protection
            scopes: Additional scopes to request

        Returns:
            Microsoft authorization URL
        """
        all_scopes = list(set(self.default_scopes + (scopes or [])))

        params = {
            "client_id": config.client_id,
            "redirect_uri": redirect_uri,
            "response_type": "code",
            "scope": " ".join(all_scopes),
            "state": state,
            "response_mode": "query",
            "prompt": "select_account",
        }

        if nonce:
            params["nonce"] = nonce

        # Domain hint for faster login
        if config.allowed_domains and len(config.allowed_domains) == 1:
            params["domain_hint"] = config.allowed_domains[0]

        auth_url = self._get_tenant_url(config, "authorize")
        return f"{auth_url}?{urlencode(params)}"

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
            config: SSO configuration
            code: Authorization code from callback
            redirect_uri: Callback URL
            nonce: Nonce for validation

        Returns:
            Authentication result with tokens and user info
        """
        try:
            token_url = self._get_tenant_url(config, "token")

            async with httpx.AsyncClient() as client:
                # Exchange code for tokens
                token_response = await client.post(
                    token_url,
                    data={
                        "client_id": config.client_id,
                        "code": code,
                        "grant_type": "authorization_code",
                        "redirect_uri": redirect_uri,
                        "scope": " ".join(self.default_scopes),
                    },
                    headers={"Content-Type": "application/x-www-form-urlencoded"},
                )

                if token_response.status_code != 200:
                    logger.error(f"Microsoft token exchange failed: {token_response.text}")
                    return SSOAuthResult(
                        success=False,
                        error="Token exchange failed",
                        error_code="token_exchange_failed",
                    )

                token_data = token_response.json()
                access_token = token_data.get("access_token")
                refresh_token = token_data.get("refresh_token")
                _id_token = token_data.get("id_token")  # TODO: Validate ID token
                expires_in = token_data.get("expires_in", 3600)

                if not access_token:
                    return SSOAuthResult(
                        success=False,
                        error="No access token in response",
                        error_code="no_access_token",
                    )

                # Get user info from Microsoft Graph
                user_info = await self.get_user_info(config, access_token)

                # TODO: Validate ID token and nonce

                from datetime import UTC, datetime, timedelta

                return SSOAuthResult(
                    success=True,
                    user_info=user_info,
                    access_token=access_token,
                    refresh_token=refresh_token,
                    expires_at=datetime.now(UTC) + timedelta(seconds=expires_in),
                )

        except httpx.HTTPError as e:
            logger.error(f"Microsoft OIDC error: {e}")
            return SSOAuthResult(
                success=False,
                error=str(e),
                error_code="http_error",
            )

    async def get_user_info(
        self,
        config: SSOConfig,
        access_token: str,
    ) -> SSOUserInfo:
        """Get user information from Microsoft Graph.

        Args:
            config: SSO configuration
            access_token: Access token

        Returns:
            User information
        """
        async with httpx.AsyncClient() as client:
            response = await client.get(
                self.GRAPH_URL,
                headers={"Authorization": f"Bearer {access_token}"},
            )
            response.raise_for_status()
            data = response.json()

        # Microsoft Graph uses different field names
        return SSOUserInfo(
            provider_user_id=data.get("id", ""),
            email=data.get("mail") or data.get("userPrincipalName", ""),
            name=data.get("displayName"),
            given_name=data.get("givenName"),
            family_name=data.get("surname"),
            picture=None,  # Graph API requires separate call for photo
            email_verified=True,  # Microsoft verifies emails
            locale=data.get("preferredLanguage"),
            raw_claims=data,
        )

    async def refresh_token(
        self,
        config: SSOConfig,
        refresh_token: str,
    ) -> SSOAuthResult:
        """Refresh access token.

        Args:
            config: SSO configuration
            refresh_token: Refresh token

        Returns:
            New authentication result
        """
        try:
            token_url = self._get_tenant_url(config, "token")

            async with httpx.AsyncClient() as client:
                response = await client.post(
                    token_url,
                    data={
                        "client_id": config.client_id,
                        "grant_type": "refresh_token",
                        "refresh_token": refresh_token,
                        "scope": " ".join(self.default_scopes),
                    },
                    headers={"Content-Type": "application/x-www-form-urlencoded"},
                )

                if response.status_code != 200:
                    logger.error(f"Microsoft token refresh failed: {response.text}")
                    return SSOAuthResult(
                        success=False,
                        error="Token refresh failed",
                        error_code="token_refresh_failed",
                    )

                token_data = response.json()
                access_token = token_data.get("access_token")
                new_refresh_token = token_data.get("refresh_token", refresh_token)
                expires_in = token_data.get("expires_in", 3600)

                from datetime import UTC, datetime, timedelta

                return SSOAuthResult(
                    success=True,
                    access_token=access_token,
                    refresh_token=new_refresh_token,
                    expires_at=datetime.now(UTC) + timedelta(seconds=expires_in),
                )

        except httpx.HTTPError as e:
            logger.error(f"Microsoft token refresh error: {e}")
            return SSOAuthResult(
                success=False,
                error=str(e),
                error_code="http_error",
            )

    async def revoke_token(
        self,
        config: SSOConfig,
        token: str,
        *,
        token_type: str = "access_token",
    ) -> bool:
        """Revoke a token.

        Note: Microsoft Entra ID doesn't support token revocation via API.
        Tokens expire naturally or can be revoked via Azure portal.

        Args:
            config: SSO configuration
            token: Token to revoke
            token_type: Type of token

        Returns:
            True (always returns true as we can't actually revoke)
        """
        logger.warning(
            "Microsoft OIDC does not support token revocation via API. "
            "Token will expire naturally."
        )
        return True

    def validate_config(self, config: SSOConfig) -> list[str]:
        """Validate Microsoft OIDC configuration."""
        errors = super().validate_config(config)

        if not config.client_id:
            errors.append("Microsoft Azure AD client_id is required")

        return errors
