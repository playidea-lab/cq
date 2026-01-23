"""Google OIDC Provider.

Implements SSO authentication via Google Workspace / Google Identity.
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


class GoogleOIDCProvider(OIDCProviderBase):
    """Google OIDC provider implementation.

    Supports Google Workspace and personal Google accounts.
    Uses Google's OIDC endpoints for authentication.
    """

    # Google OIDC endpoints
    DISCOVERY_URL = "https://accounts.google.com/.well-known/openid-configuration"
    AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth"
    TOKEN_URL = "https://oauth2.googleapis.com/token"
    USERINFO_URL = "https://openidconnect.googleapis.com/v1/userinfo"
    REVOKE_URL = "https://oauth2.googleapis.com/revoke"

    @property
    def provider_type(self) -> SSOProvider:
        """Return the provider type."""
        return SSOProvider.GOOGLE

    @property
    def name(self) -> str:
        """Return the provider display name."""
        return "Google"

    @property
    def discovery_url(self) -> str:
        """Return the OIDC discovery URL."""
        return self.DISCOVERY_URL

    @property
    def default_scopes(self) -> list[str]:
        """Return default OIDC scopes."""
        return ["openid", "email", "profile"]

    def get_authorization_url(
        self,
        config: SSOConfig,
        redirect_uri: str,
        state: str,
        *,
        nonce: str | None = None,
        scopes: list[str] | None = None,
    ) -> str:
        """Generate the authorization URL for Google login.

        Args:
            config: SSO configuration
            redirect_uri: Callback URL
            state: State parameter for CSRF protection
            nonce: Nonce for replay protection
            scopes: Additional scopes to request

        Returns:
            Google authorization URL
        """
        all_scopes = list(set(self.default_scopes + (scopes or [])))

        params = {
            "client_id": config.client_id,
            "redirect_uri": redirect_uri,
            "response_type": "code",
            "scope": " ".join(all_scopes),
            "state": state,
            "access_type": "offline",  # Get refresh token
            "prompt": "select_account",  # Always show account selector
        }

        if nonce:
            params["nonce"] = nonce

        # Restrict to specific hosted domain (Google Workspace)
        if config.allowed_domains and len(config.allowed_domains) == 1:
            params["hd"] = config.allowed_domains[0]

        return f"{self.AUTH_URL}?{urlencode(params)}"

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
            async with httpx.AsyncClient() as client:
                # Exchange code for tokens
                token_response = await client.post(
                    self.TOKEN_URL,
                    data={
                        "client_id": config.client_id,
                        "code": code,
                        "grant_type": "authorization_code",
                        "redirect_uri": redirect_uri,
                    },
                    headers={"Content-Type": "application/x-www-form-urlencoded"},
                )

                if token_response.status_code != 200:
                    logger.error(f"Google token exchange failed: {token_response.text}")
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

                # Get user info
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
            logger.error(f"Google OIDC error: {e}")
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
        """Get user information from Google.

        Args:
            config: SSO configuration
            access_token: Access token

        Returns:
            User information
        """
        async with httpx.AsyncClient() as client:
            response = await client.get(
                self.USERINFO_URL,
                headers={"Authorization": f"Bearer {access_token}"},
            )
            response.raise_for_status()
            data = response.json()

        return SSOUserInfo(
            provider_user_id=data.get("sub", ""),
            email=data.get("email", ""),
            name=data.get("name"),
            given_name=data.get("given_name"),
            family_name=data.get("family_name"),
            picture=data.get("picture"),
            email_verified=data.get("email_verified", False),
            locale=data.get("locale"),
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
            async with httpx.AsyncClient() as client:
                response = await client.post(
                    self.TOKEN_URL,
                    data={
                        "client_id": config.client_id,
                        "grant_type": "refresh_token",
                        "refresh_token": refresh_token,
                    },
                    headers={"Content-Type": "application/x-www-form-urlencoded"},
                )

                if response.status_code != 200:
                    logger.error(f"Google token refresh failed: {response.text}")
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
            logger.error(f"Google token refresh error: {e}")
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

        Args:
            config: SSO configuration
            token: Token to revoke
            token_type: Type of token

        Returns:
            True if revocation succeeded
        """
        try:
            async with httpx.AsyncClient() as client:
                response = await client.post(
                    self.REVOKE_URL,
                    data={"token": token},
                    headers={"Content-Type": "application/x-www-form-urlencoded"},
                )
                return response.status_code == 200

        except httpx.HTTPError as e:
            logger.error(f"Google token revocation error: {e}")
            return False

    def validate_config(self, config: SSOConfig) -> list[str]:
        """Validate Google OIDC configuration."""
        errors = super().validate_config(config)

        if not config.client_id:
            errors.append("Google OAuth client_id is required")

        # client_secret is needed for token exchange but stored encrypted
        # We don't validate it here since it's not in the config model

        return errors
