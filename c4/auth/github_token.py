"""GitHub Token Management - OAuth token handling with Supabase integration."""

from __future__ import annotations

import json
import logging
import os
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from enum import Enum
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

logger = logging.getLogger(__name__)


class GitHubTokenError(Exception):
    """Error related to GitHub token operations."""

    pass


class TokenStatus(str, Enum):
    """Token status states."""

    VALID = "valid"
    EXPIRED = "expired"
    EXPIRING_SOON = "expiring_soon"
    INVALID = "invalid"
    UNKNOWN = "unknown"


@dataclass
class GitHubOAuthToken:
    """GitHub OAuth token with metadata.

    Attributes:
        access_token: GitHub access token
        token_type: Token type (usually "bearer")
        scope: Granted scopes
        expires_at: Token expiration timestamp
        refresh_token: Refresh token for renewal (if available)
        user_id: Associated user ID
        provider_id: OAuth provider ID
    """

    access_token: str
    token_type: str = "bearer"
    scope: str = ""
    expires_at: datetime | None = None
    refresh_token: str | None = None
    user_id: str | None = None
    provider_id: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)

    @property
    def is_expired(self) -> bool:
        """Check if token is expired."""
        if self.expires_at is None:
            return False
        return datetime.now() >= self.expires_at

    @property
    def is_expiring_soon(self, threshold_minutes: int = 5) -> bool:
        """Check if token will expire soon."""
        if self.expires_at is None:
            return False
        threshold = datetime.now() + timedelta(minutes=threshold_minutes)
        return threshold >= self.expires_at

    @property
    def status(self) -> TokenStatus:
        """Get current token status."""
        if not self.access_token:
            return TokenStatus.INVALID
        if self.is_expired:
            return TokenStatus.EXPIRED
        if self.is_expiring_soon:
            return TokenStatus.EXPIRING_SOON
        return TokenStatus.VALID

    @property
    def remaining_seconds(self) -> int | None:
        """Get remaining seconds until expiration."""
        if self.expires_at is None:
            return None
        remaining = (self.expires_at - datetime.now()).total_seconds()
        return max(0, int(remaining))

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "access_token": self.access_token,
            "token_type": self.token_type,
            "scope": self.scope,
            "expires_at": self.expires_at.isoformat() if self.expires_at else None,
            "refresh_token": self.refresh_token,
            "user_id": self.user_id,
            "provider_id": self.provider_id,
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> GitHubOAuthToken:
        """Create from dictionary."""
        expires_at = None
        if data.get("expires_at"):
            expires_at = datetime.fromisoformat(data["expires_at"])

        return cls(
            access_token=data.get("access_token", ""),
            token_type=data.get("token_type", "bearer"),
            scope=data.get("scope", ""),
            expires_at=expires_at,
            refresh_token=data.get("refresh_token"),
            user_id=data.get("user_id"),
            provider_id=data.get("provider_id"),
            metadata=data.get("metadata", {}),
        )

    @classmethod
    def from_supabase_provider(cls, provider_data: dict[str, Any]) -> GitHubOAuthToken:
        """Create from Supabase provider_token data.

        Args:
            provider_data: Data from Supabase auth.users identities

        Returns:
            GitHubOAuthToken instance
        """
        # Supabase stores identity_data with provider tokens
        identity_data = provider_data.get("identity_data", {})
        provider_token = identity_data.get("provider_token", "")

        # Check for refresh token
        provider_refresh_token = identity_data.get("provider_refresh_token")

        # Calculate expiration if provided
        expires_at = None
        if "expires_at" in identity_data:
            try:
                expires_at = datetime.fromtimestamp(identity_data["expires_at"])
            except (ValueError, TypeError):
                pass
        elif "expires_in" in identity_data:
            try:
                expires_in = int(identity_data["expires_in"])
                expires_at = datetime.now() + timedelta(seconds=expires_in)
            except (ValueError, TypeError):
                pass

        return cls(
            access_token=provider_token,
            token_type="bearer",
            scope=identity_data.get("scope", ""),
            expires_at=expires_at,
            refresh_token=provider_refresh_token,
            user_id=provider_data.get("user_id"),
            provider_id=provider_data.get("id"),
            metadata={
                "provider": provider_data.get("provider", "github"),
                "created_at": provider_data.get("created_at"),
                "updated_at": provider_data.get("updated_at"),
            },
        )


@dataclass
class TokenRefreshResult:
    """Result of a token refresh operation."""

    success: bool
    token: GitHubOAuthToken | None
    message: str
    error: str | None = None


class GitHubTokenManager:
    """Manages GitHub OAuth tokens with Supabase integration.

    Features:
    - Token validation
    - Automatic refresh
    - Supabase provider_token integration
    - Token caching

    Environment Variables:
        SUPABASE_URL: Supabase project URL
        SUPABASE_KEY: Supabase service key
        GITHUB_CLIENT_ID: GitHub OAuth app client ID (for refresh)
        GITHUB_CLIENT_SECRET: GitHub OAuth app client secret (for refresh)

    Example:
        manager = GitHubTokenManager()
        token = manager.get_valid_token(user_id="123")

        if token:
            client = GitHubClient(token=token.access_token)
    """

    GITHUB_TOKEN_URL = "https://github.com/login/oauth/access_token"

    def __init__(
        self,
        supabase_url: str | None = None,
        supabase_key: str | None = None,
        github_client_id: str | None = None,
        github_client_secret: str | None = None,
        auto_refresh: bool = True,
        refresh_threshold_minutes: int = 5,
    ):
        """Initialize token manager.

        Args:
            supabase_url: Supabase project URL
            supabase_key: Supabase service key
            github_client_id: GitHub OAuth app client ID
            github_client_secret: GitHub OAuth app client secret
            auto_refresh: Automatically refresh expiring tokens
            refresh_threshold_minutes: Minutes before expiry to refresh
        """
        self._supabase_url = supabase_url or os.environ.get("SUPABASE_URL", "")
        self._supabase_key = supabase_key or os.environ.get("SUPABASE_KEY", "")
        self._github_client_id = github_client_id or os.environ.get(
            "GITHUB_CLIENT_ID", ""
        )
        self._github_client_secret = github_client_secret or os.environ.get(
            "GITHUB_CLIENT_SECRET", ""
        )
        self._auto_refresh = auto_refresh
        self._refresh_threshold = refresh_threshold_minutes
        self._token_cache: dict[str, GitHubOAuthToken] = {}

    @property
    def has_supabase_config(self) -> bool:
        """Check if Supabase is configured."""
        return bool(self._supabase_url and self._supabase_key)

    @property
    def has_github_oauth_config(self) -> bool:
        """Check if GitHub OAuth is configured for refresh."""
        return bool(self._github_client_id and self._github_client_secret)

    # =========================================================================
    # Token Retrieval
    # =========================================================================

    def get_token_from_env(self) -> GitHubOAuthToken | None:
        """Get token from environment variable.

        Returns:
            GitHubOAuthToken if GITHUB_TOKEN is set, None otherwise
        """
        token = os.environ.get("GITHUB_TOKEN")
        if token:
            return GitHubOAuthToken(
                access_token=token,
                token_type="bearer",
                metadata={"source": "environment"},
            )
        return None

    def get_token_from_supabase(self, user_id: str) -> GitHubOAuthToken | None:
        """Get GitHub token from Supabase provider_token.

        Args:
            user_id: Supabase user ID

        Returns:
            GitHubOAuthToken if found, None otherwise
        """
        if not self.has_supabase_config:
            logger.debug("Supabase not configured")
            return None

        try:
            # Get user's GitHub identity from Supabase
            url = f"{self._supabase_url}/rest/v1/rpc/get_user_provider_token"
            headers = {
                "apikey": self._supabase_key,
                "Authorization": f"Bearer {self._supabase_key}",
                "Content-Type": "application/json",
            }

            # Call RPC function to get provider token
            data = json.dumps({"user_id_param": user_id, "provider_param": "github"})
            req = Request(url, data=data.encode(), headers=headers, method="POST")

            with urlopen(req, timeout=10) as response:
                result = json.loads(response.read().decode())

            if result:
                return GitHubOAuthToken.from_supabase_provider(result)

        except HTTPError as e:
            logger.warning(f"Failed to get token from Supabase: {e.code}")
        except URLError as e:
            logger.warning(f"Network error getting token from Supabase: {e.reason}")
        except Exception as e:
            logger.warning(f"Error getting token from Supabase: {e}")

        return None

    def get_valid_token(
        self,
        user_id: str | None = None,
        force_refresh: bool = False,
    ) -> GitHubOAuthToken | None:
        """Get a valid GitHub token.

        Tries multiple sources in order:
        1. Cache (if not expired)
        2. Supabase provider_token
        3. Environment variable

        Args:
            user_id: User ID for Supabase lookup
            force_refresh: Force token refresh even if valid

        Returns:
            Valid GitHubOAuthToken or None
        """
        # Check cache first
        cache_key = user_id or "_env_"
        if not force_refresh and cache_key in self._token_cache:
            cached = self._token_cache[cache_key]
            if cached.status == TokenStatus.VALID:
                return cached
            elif cached.status == TokenStatus.EXPIRING_SOON and self._auto_refresh:
                # Try to refresh
                refreshed = self.refresh_token(cached)
                if refreshed.success and refreshed.token:
                    self._token_cache[cache_key] = refreshed.token
                    return refreshed.token

        # Try Supabase if user_id provided
        if user_id:
            token = self.get_token_from_supabase(user_id)
            if token and token.status == TokenStatus.VALID:
                self._token_cache[cache_key] = token
                return token
            elif token and token.status == TokenStatus.EXPIRING_SOON:
                if self._auto_refresh:
                    refreshed = self.refresh_token(token)
                    if refreshed.success and refreshed.token:
                        self._token_cache[cache_key] = refreshed.token
                        return refreshed.token
                else:
                    self._token_cache[cache_key] = token
                    return token

        # Fallback to environment
        env_token = self.get_token_from_env()
        if env_token:
            self._token_cache[cache_key] = env_token
            return env_token

        return None

    # =========================================================================
    # Token Refresh
    # =========================================================================

    def refresh_token(self, token: GitHubOAuthToken) -> TokenRefreshResult:
        """Refresh an OAuth token using the refresh token.

        Args:
            token: Token to refresh

        Returns:
            TokenRefreshResult with new token or error
        """
        if not token.refresh_token:
            return TokenRefreshResult(
                success=False,
                token=None,
                message="No refresh token available",
                error="missing_refresh_token",
            )

        if not self.has_github_oauth_config:
            return TokenRefreshResult(
                success=False,
                token=None,
                message="GitHub OAuth credentials not configured",
                error="missing_oauth_config",
            )

        try:
            # Call GitHub token endpoint
            data = {
                "client_id": self._github_client_id,
                "client_secret": self._github_client_secret,
                "grant_type": "refresh_token",
                "refresh_token": token.refresh_token,
            }

            headers = {
                "Accept": "application/json",
                "Content-Type": "application/x-www-form-urlencoded",
            }

            body = "&".join(f"{k}={v}" for k, v in data.items())
            req = Request(
                self.GITHUB_TOKEN_URL,
                data=body.encode(),
                headers=headers,
                method="POST",
            )

            with urlopen(req, timeout=10) as response:
                result = json.loads(response.read().decode())

            if "error" in result:
                error_desc = result.get("error_description", result["error"])
                return TokenRefreshResult(
                    success=False,
                    token=None,
                    message=f"Token refresh failed: {error_desc}",
                    error=result["error"],
                )

            # Create new token
            expires_at = None
            if "expires_in" in result:
                expires_at = datetime.now() + timedelta(seconds=result["expires_in"])

            new_token = GitHubOAuthToken(
                access_token=result["access_token"],
                token_type=result.get("token_type", "bearer"),
                scope=result.get("scope", token.scope),
                expires_at=expires_at,
                refresh_token=result.get("refresh_token", token.refresh_token),
                user_id=token.user_id,
                provider_id=token.provider_id,
                metadata={
                    **token.metadata,
                    "refreshed_at": datetime.now().isoformat(),
                },
            )

            logger.info(f"Token refreshed successfully for user {token.user_id}")

            # Update Supabase if configured
            if token.user_id and self.has_supabase_config:
                self._update_supabase_token(token.user_id, new_token)

            return TokenRefreshResult(
                success=True,
                token=new_token,
                message="Token refreshed successfully",
            )

        except HTTPError as e:
            error_body = ""
            try:
                error_body = e.read().decode()
            except Exception:
                pass
            return TokenRefreshResult(
                success=False,
                token=None,
                message=f"HTTP error during refresh: {e.code}",
                error=error_body or str(e),
            )
        except URLError as e:
            return TokenRefreshResult(
                success=False,
                token=None,
                message=f"Network error during refresh: {e.reason}",
                error=str(e),
            )
        except Exception as e:
            return TokenRefreshResult(
                success=False,
                token=None,
                message=f"Error during refresh: {e}",
                error=str(e),
            )

    def _update_supabase_token(
        self, user_id: str, token: GitHubOAuthToken
    ) -> bool:
        """Update token in Supabase after refresh.

        Args:
            user_id: User ID
            token: New token to store

        Returns:
            True if updated successfully
        """
        if not self.has_supabase_config:
            return False

        try:
            url = f"{self._supabase_url}/rest/v1/rpc/update_user_provider_token"
            headers = {
                "apikey": self._supabase_key,
                "Authorization": f"Bearer {self._supabase_key}",
                "Content-Type": "application/json",
            }

            data = json.dumps({
                "user_id_param": user_id,
                "provider_param": "github",
                "token_data": token.to_dict(),
            })
            req = Request(url, data=data.encode(), headers=headers, method="POST")

            with urlopen(req, timeout=10) as response:
                response.read()

            logger.debug(f"Updated Supabase token for user {user_id}")
            return True

        except Exception as e:
            logger.warning(f"Failed to update Supabase token: {e}")
            return False

    # =========================================================================
    # Token Validation
    # =========================================================================

    def validate_token(self, token: GitHubOAuthToken) -> TokenStatus:
        """Validate a GitHub token by making an API call.

        Args:
            token: Token to validate

        Returns:
            TokenStatus indicating validity
        """
        if not token.access_token:
            return TokenStatus.INVALID

        # Check expiration first
        if token.is_expired:
            return TokenStatus.EXPIRED

        try:
            # Call GitHub user endpoint to validate
            url = "https://api.github.com/user"
            headers = {
                "Authorization": f"Bearer {token.access_token}",
                "Accept": "application/vnd.github+json",
                "X-GitHub-Api-Version": "2022-11-28",
            }

            req = Request(url, headers=headers, method="GET")

            with urlopen(req, timeout=10) as response:
                if response.status == 200:
                    if token.is_expiring_soon:
                        return TokenStatus.EXPIRING_SOON
                    return TokenStatus.VALID

            return TokenStatus.UNKNOWN

        except HTTPError as e:
            if e.code == 401:
                return TokenStatus.INVALID
            return TokenStatus.UNKNOWN
        except Exception:
            return TokenStatus.UNKNOWN

    def get_token_info(self, token: GitHubOAuthToken) -> dict[str, Any]:
        """Get information about a token.

        Args:
            token: Token to get info for

        Returns:
            Dictionary with token information
        """
        return {
            "status": self.validate_token(token).value,
            "is_expired": token.is_expired,
            "is_expiring_soon": token.is_expiring_soon,
            "remaining_seconds": token.remaining_seconds,
            "has_refresh_token": bool(token.refresh_token),
            "scope": token.scope,
            "user_id": token.user_id,
        }

    # =========================================================================
    # Cache Management
    # =========================================================================

    def clear_cache(self, user_id: str | None = None) -> int:
        """Clear token cache.

        Args:
            user_id: Specific user to clear, or None for all

        Returns:
            Number of entries cleared
        """
        if user_id:
            if user_id in self._token_cache:
                del self._token_cache[user_id]
                return 1
            return 0

        count = len(self._token_cache)
        self._token_cache.clear()
        return count

    def get_cached_tokens(self) -> dict[str, dict[str, Any]]:
        """Get all cached tokens (without secrets).

        Returns:
            Dictionary of user_id -> token info
        """
        return {
            user_id: {
                "status": token.status.value,
                "expires_at": token.expires_at.isoformat() if token.expires_at else None,
                "scope": token.scope,
            }
            for user_id, token in self._token_cache.items()
        }
