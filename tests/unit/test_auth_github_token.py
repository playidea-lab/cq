"""Unit tests for GitHub token management."""

from __future__ import annotations

import json
import os
from datetime import datetime, timedelta
from unittest.mock import MagicMock, patch

from c4.auth import (
    GitHubOAuthToken,
    GitHubTokenManager,
    TokenRefreshResult,
)
from c4.auth.github_token import TokenStatus


class TestTokenStatus:
    """Tests for TokenStatus enum."""

    def test_enum_values(self):
        """Test all enum values exist."""
        assert TokenStatus.VALID == "valid"
        assert TokenStatus.EXPIRED == "expired"
        assert TokenStatus.EXPIRING_SOON == "expiring_soon"
        assert TokenStatus.INVALID == "invalid"
        assert TokenStatus.UNKNOWN == "unknown"


class TestGitHubOAuthToken:
    """Tests for GitHubOAuthToken dataclass."""

    def test_create_basic_token(self):
        """Test creating a basic token."""
        token = GitHubOAuthToken(access_token="test_token_123")

        assert token.access_token == "test_token_123"
        assert token.token_type == "bearer"
        assert token.scope == ""
        assert token.expires_at is None
        assert token.refresh_token is None
        assert token.user_id is None
        assert token.provider_id is None
        assert token.metadata == {}

    def test_create_full_token(self):
        """Test creating a token with all fields."""
        expires = datetime.now() + timedelta(hours=1)
        token = GitHubOAuthToken(
            access_token="test_token",
            token_type="bearer",
            scope="repo,user",
            expires_at=expires,
            refresh_token="refresh_token_123",
            user_id="user_123",
            provider_id="provider_456",
            metadata={"source": "test"},
        )

        assert token.access_token == "test_token"
        assert token.scope == "repo,user"
        assert token.expires_at == expires
        assert token.refresh_token == "refresh_token_123"
        assert token.user_id == "user_123"
        assert token.metadata == {"source": "test"}

    def test_is_expired_no_expiry(self):
        """Test is_expired when no expiration is set."""
        token = GitHubOAuthToken(access_token="test")
        assert token.is_expired is False

    def test_is_expired_future(self):
        """Test is_expired when expiration is in the future."""
        token = GitHubOAuthToken(
            access_token="test",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        assert token.is_expired is False

    def test_is_expired_past(self):
        """Test is_expired when expiration is in the past."""
        token = GitHubOAuthToken(
            access_token="test",
            expires_at=datetime.now() - timedelta(hours=1),
        )
        assert token.is_expired is True

    def test_is_expiring_soon_no_expiry(self):
        """Test is_expiring_soon when no expiration is set."""
        token = GitHubOAuthToken(access_token="test")
        assert token.is_expiring_soon is False

    def test_is_expiring_soon_far_future(self):
        """Test is_expiring_soon when expiration is far in the future."""
        token = GitHubOAuthToken(
            access_token="test",
            expires_at=datetime.now() + timedelta(hours=2),
        )
        assert token.is_expiring_soon is False

    def test_is_expiring_soon_near_future(self):
        """Test is_expiring_soon when expiration is within threshold."""
        token = GitHubOAuthToken(
            access_token="test",
            expires_at=datetime.now() + timedelta(minutes=3),
        )
        assert token.is_expiring_soon is True

    def test_status_valid(self):
        """Test status returns VALID for good token."""
        token = GitHubOAuthToken(
            access_token="test",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        assert token.status == TokenStatus.VALID

    def test_status_expired(self):
        """Test status returns EXPIRED for expired token."""
        token = GitHubOAuthToken(
            access_token="test",
            expires_at=datetime.now() - timedelta(hours=1),
        )
        assert token.status == TokenStatus.EXPIRED

    def test_status_expiring_soon(self):
        """Test status returns EXPIRING_SOON for nearly expired token."""
        token = GitHubOAuthToken(
            access_token="test",
            expires_at=datetime.now() + timedelta(minutes=2),
        )
        assert token.status == TokenStatus.EXPIRING_SOON

    def test_status_invalid_no_token(self):
        """Test status returns INVALID for empty access token."""
        token = GitHubOAuthToken(access_token="")
        assert token.status == TokenStatus.INVALID

    def test_remaining_seconds_no_expiry(self):
        """Test remaining_seconds returns None when no expiration."""
        token = GitHubOAuthToken(access_token="test")
        assert token.remaining_seconds is None

    def test_remaining_seconds_future(self):
        """Test remaining_seconds returns positive value for future expiry."""
        token = GitHubOAuthToken(
            access_token="test",
            expires_at=datetime.now() + timedelta(minutes=10),
        )
        remaining = token.remaining_seconds
        assert remaining is not None
        assert 550 <= remaining <= 610  # Allow some tolerance

    def test_remaining_seconds_past(self):
        """Test remaining_seconds returns 0 for past expiry."""
        token = GitHubOAuthToken(
            access_token="test",
            expires_at=datetime.now() - timedelta(hours=1),
        )
        assert token.remaining_seconds == 0

    def test_to_dict(self):
        """Test to_dict serialization."""
        expires = datetime(2025, 6, 15, 12, 0, 0)
        token = GitHubOAuthToken(
            access_token="test_token",
            token_type="bearer",
            scope="repo",
            expires_at=expires,
            refresh_token="refresh_123",
            user_id="user_123",
            provider_id="prov_123",
            metadata={"key": "value"},
        )

        data = token.to_dict()

        assert data["access_token"] == "test_token"
        assert data["token_type"] == "bearer"
        assert data["scope"] == "repo"
        assert data["expires_at"] == "2025-06-15T12:00:00"
        assert data["refresh_token"] == "refresh_123"
        assert data["user_id"] == "user_123"
        assert data["provider_id"] == "prov_123"
        assert data["metadata"] == {"key": "value"}

    def test_to_dict_no_expiry(self):
        """Test to_dict serialization with no expiry."""
        token = GitHubOAuthToken(access_token="test")
        data = token.to_dict()
        assert data["expires_at"] is None

    def test_from_dict(self):
        """Test from_dict deserialization."""
        data = {
            "access_token": "test_token",
            "token_type": "bearer",
            "scope": "repo",
            "expires_at": "2025-06-15T12:00:00",
            "refresh_token": "refresh_123",
            "user_id": "user_123",
            "provider_id": "prov_123",
            "metadata": {"key": "value"},
        }

        token = GitHubOAuthToken.from_dict(data)

        assert token.access_token == "test_token"
        assert token.token_type == "bearer"
        assert token.scope == "repo"
        assert token.expires_at == datetime(2025, 6, 15, 12, 0, 0)
        assert token.refresh_token == "refresh_123"
        assert token.user_id == "user_123"

    def test_from_dict_minimal(self):
        """Test from_dict with minimal data."""
        data = {"access_token": "test"}
        token = GitHubOAuthToken.from_dict(data)

        assert token.access_token == "test"
        assert token.token_type == "bearer"
        assert token.expires_at is None

    def test_roundtrip_serialization(self):
        """Test to_dict -> from_dict roundtrip."""
        original = GitHubOAuthToken(
            access_token="test_token",
            scope="repo,user",
            expires_at=datetime(2025, 6, 15, 12, 0, 0),
            refresh_token="refresh",
            user_id="user",
        )

        data = original.to_dict()
        restored = GitHubOAuthToken.from_dict(data)

        assert restored.access_token == original.access_token
        assert restored.scope == original.scope
        assert restored.expires_at == original.expires_at
        assert restored.refresh_token == original.refresh_token
        assert restored.user_id == original.user_id

    def test_from_supabase_provider_basic(self):
        """Test from_supabase_provider with basic data."""
        provider_data = {
            "id": "prov_123",
            "user_id": "user_123",
            "provider": "github",
            "identity_data": {
                "provider_token": "github_token_123",
                "scope": "repo",
            },
        }

        token = GitHubOAuthToken.from_supabase_provider(provider_data)

        assert token.access_token == "github_token_123"
        assert token.scope == "repo"
        assert token.user_id == "user_123"
        assert token.provider_id == "prov_123"
        assert token.metadata["provider"] == "github"

    def test_from_supabase_provider_with_expires_at(self):
        """Test from_supabase_provider with expires_at timestamp."""
        future_ts = (datetime.now() + timedelta(hours=1)).timestamp()
        provider_data = {
            "identity_data": {
                "provider_token": "token",
                "expires_at": future_ts,
            },
        }

        token = GitHubOAuthToken.from_supabase_provider(provider_data)

        assert token.expires_at is not None
        assert not token.is_expired

    def test_from_supabase_provider_with_expires_in(self):
        """Test from_supabase_provider with expires_in seconds."""
        provider_data = {
            "identity_data": {
                "provider_token": "token",
                "expires_in": 3600,  # 1 hour
            },
        }

        token = GitHubOAuthToken.from_supabase_provider(provider_data)

        assert token.expires_at is not None
        remaining = token.remaining_seconds
        assert remaining is not None
        assert 3500 <= remaining <= 3700

    def test_from_supabase_provider_with_refresh_token(self):
        """Test from_supabase_provider with refresh token."""
        provider_data = {
            "identity_data": {
                "provider_token": "token",
                "provider_refresh_token": "refresh_token_123",
            },
        }

        token = GitHubOAuthToken.from_supabase_provider(provider_data)

        assert token.refresh_token == "refresh_token_123"


class TestTokenRefreshResult:
    """Tests for TokenRefreshResult dataclass."""

    def test_success_result(self):
        """Test successful refresh result."""
        token = GitHubOAuthToken(access_token="new_token")
        result = TokenRefreshResult(
            success=True,
            token=token,
            message="Token refreshed successfully",
        )

        assert result.success is True
        assert result.token is not None
        assert result.token.access_token == "new_token"
        assert result.error is None

    def test_failure_result(self):
        """Test failed refresh result."""
        result = TokenRefreshResult(
            success=False,
            token=None,
            message="Refresh failed",
            error="invalid_grant",
        )

        assert result.success is False
        assert result.token is None
        assert result.error == "invalid_grant"


class TestGitHubTokenManager:
    """Tests for GitHubTokenManager class."""

    def test_init_default(self):
        """Test default initialization."""
        manager = GitHubTokenManager()

        assert manager._auto_refresh is True
        assert manager._refresh_threshold == 5
        assert manager._token_cache == {}

    def test_init_with_config(self):
        """Test initialization with config."""
        manager = GitHubTokenManager(
            supabase_url="https://test.supabase.co",
            supabase_key="test_key",
            github_client_id="client_id",
            github_client_secret="client_secret",
            auto_refresh=False,
            refresh_threshold_minutes=10,
        )

        assert manager._supabase_url == "https://test.supabase.co"
        assert manager._supabase_key == "test_key"
        assert manager._github_client_id == "client_id"
        assert manager._github_client_secret == "client_secret"
        assert manager._auto_refresh is False
        assert manager._refresh_threshold == 10

    def test_has_supabase_config_true(self):
        """Test has_supabase_config when configured."""
        manager = GitHubTokenManager(
            supabase_url="https://test.supabase.co",
            supabase_key="test_key",
        )
        assert manager.has_supabase_config is True

    def test_has_supabase_config_false(self):
        """Test has_supabase_config when not configured."""
        manager = GitHubTokenManager()
        assert manager.has_supabase_config is False

    def test_has_supabase_config_partial(self):
        """Test has_supabase_config with partial config."""
        manager = GitHubTokenManager(supabase_url="https://test.supabase.co")
        assert manager.has_supabase_config is False

    def test_has_github_oauth_config_true(self):
        """Test has_github_oauth_config when configured."""
        manager = GitHubTokenManager(
            github_client_id="client",
            github_client_secret="secret",
        )
        assert manager.has_github_oauth_config is True

    def test_has_github_oauth_config_false(self):
        """Test has_github_oauth_config when not configured."""
        manager = GitHubTokenManager()
        assert manager.has_github_oauth_config is False

    def test_get_token_from_env_exists(self):
        """Test get_token_from_env when GITHUB_TOKEN is set."""
        manager = GitHubTokenManager()

        with patch.dict(os.environ, {"GITHUB_TOKEN": "env_token_123"}):
            token = manager.get_token_from_env()

        assert token is not None
        assert token.access_token == "env_token_123"
        assert token.metadata.get("source") == "environment"

    def test_get_token_from_env_not_exists(self):
        """Test get_token_from_env when GITHUB_TOKEN is not set."""
        manager = GitHubTokenManager()

        with patch.dict(os.environ, {}, clear=True):
            # Also clear GITHUB_TOKEN if it exists
            os.environ.pop("GITHUB_TOKEN", None)
            token = manager.get_token_from_env()

        assert token is None

    def test_get_token_from_supabase_not_configured(self):
        """Test get_token_from_supabase when Supabase is not configured."""
        manager = GitHubTokenManager()
        token = manager.get_token_from_supabase("user_123")
        assert token is None

    @patch("c4.auth.github_token.urlopen")
    def test_get_token_from_supabase_success(self, mock_urlopen):
        """Test get_token_from_supabase success."""
        # Mock response
        mock_response = MagicMock()
        mock_response.__enter__ = MagicMock(return_value=mock_response)
        mock_response.__exit__ = MagicMock(return_value=False)
        mock_response.read.return_value = json.dumps(
            {
                "id": "prov_123",
                "user_id": "user_123",
                "provider": "github",
                "identity_data": {
                    "provider_token": "supabase_token",
                    "scope": "repo",
                },
            }
        ).encode()
        mock_urlopen.return_value = mock_response

        manager = GitHubTokenManager(
            supabase_url="https://test.supabase.co",
            supabase_key="test_key",
        )
        token = manager.get_token_from_supabase("user_123")

        assert token is not None
        assert token.access_token == "supabase_token"
        mock_urlopen.assert_called_once()

    @patch("c4.auth.github_token.urlopen")
    def test_get_token_from_supabase_http_error(self, mock_urlopen):
        """Test get_token_from_supabase handles HTTP errors."""
        from urllib.error import HTTPError

        mock_urlopen.side_effect = HTTPError(
            url="test", code=404, msg="Not Found", hdrs={}, fp=None
        )

        manager = GitHubTokenManager(
            supabase_url="https://test.supabase.co",
            supabase_key="test_key",
        )
        token = manager.get_token_from_supabase("user_123")

        assert token is None

    def test_get_valid_token_from_cache(self):
        """Test get_valid_token returns cached token."""
        manager = GitHubTokenManager()
        cached_token = GitHubOAuthToken(
            access_token="cached_token",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        manager._token_cache["user_123"] = cached_token

        token = manager.get_valid_token(user_id="user_123")

        assert token is not None
        assert token.access_token == "cached_token"

    def test_get_valid_token_from_env_fallback(self):
        """Test get_valid_token falls back to env."""
        manager = GitHubTokenManager()

        with patch.dict(os.environ, {"GITHUB_TOKEN": "env_token"}):
            token = manager.get_valid_token()

        assert token is not None
        assert token.access_token == "env_token"

    def test_get_valid_token_force_refresh(self):
        """Test get_valid_token with force_refresh ignores cache."""
        manager = GitHubTokenManager()
        cached_token = GitHubOAuthToken(
            access_token="cached_token",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        manager._token_cache["_env_"] = cached_token

        with patch.dict(os.environ, {"GITHUB_TOKEN": "new_env_token"}):
            token = manager.get_valid_token(force_refresh=True)

        assert token is not None
        assert token.access_token == "new_env_token"

    def test_refresh_token_no_refresh_token(self):
        """Test refresh_token fails without refresh token."""
        manager = GitHubTokenManager(
            github_client_id="client",
            github_client_secret="secret",
        )
        token = GitHubOAuthToken(access_token="test")

        result = manager.refresh_token(token)

        assert result.success is False
        assert result.error == "missing_refresh_token"

    def test_refresh_token_no_oauth_config(self):
        """Test refresh_token fails without OAuth config."""
        manager = GitHubTokenManager()
        token = GitHubOAuthToken(
            access_token="test",
            refresh_token="refresh_123",
        )

        result = manager.refresh_token(token)

        assert result.success is False
        assert result.error == "missing_oauth_config"

    @patch("c4.auth.github_token.urlopen")
    def test_refresh_token_success(self, mock_urlopen):
        """Test refresh_token success."""
        mock_response = MagicMock()
        mock_response.__enter__ = MagicMock(return_value=mock_response)
        mock_response.__exit__ = MagicMock(return_value=False)
        mock_response.read.return_value = json.dumps(
            {
                "access_token": "new_access_token",
                "token_type": "bearer",
                "expires_in": 3600,
                "refresh_token": "new_refresh_token",
                "scope": "repo",
            }
        ).encode()
        mock_urlopen.return_value = mock_response

        manager = GitHubTokenManager(
            github_client_id="client",
            github_client_secret="secret",
        )
        token = GitHubOAuthToken(
            access_token="old_token",
            refresh_token="old_refresh",
            scope="repo",
        )

        result = manager.refresh_token(token)

        assert result.success is True
        assert result.token is not None
        assert result.token.access_token == "new_access_token"
        assert result.token.refresh_token == "new_refresh_token"
        assert result.token.expires_at is not None

    @patch("c4.auth.github_token.urlopen")
    def test_refresh_token_api_error(self, mock_urlopen):
        """Test refresh_token handles API error response."""
        mock_response = MagicMock()
        mock_response.__enter__ = MagicMock(return_value=mock_response)
        mock_response.__exit__ = MagicMock(return_value=False)
        mock_response.read.return_value = json.dumps(
            {
                "error": "invalid_grant",
                "error_description": "The refresh token is invalid",
            }
        ).encode()
        mock_urlopen.return_value = mock_response

        manager = GitHubTokenManager(
            github_client_id="client",
            github_client_secret="secret",
        )
        token = GitHubOAuthToken(
            access_token="test",
            refresh_token="invalid_refresh",
        )

        result = manager.refresh_token(token)

        assert result.success is False
        assert result.error == "invalid_grant"

    @patch("c4.auth.github_token.urlopen")
    def test_validate_token_valid(self, mock_urlopen):
        """Test validate_token returns VALID for good token."""
        mock_response = MagicMock()
        mock_response.__enter__ = MagicMock(return_value=mock_response)
        mock_response.__exit__ = MagicMock(return_value=False)
        mock_response.status = 200
        mock_urlopen.return_value = mock_response

        manager = GitHubTokenManager()
        token = GitHubOAuthToken(
            access_token="valid_token",
            expires_at=datetime.now() + timedelta(hours=1),
        )

        status = manager.validate_token(token)

        assert status == TokenStatus.VALID

    @patch("c4.auth.github_token.urlopen")
    def test_validate_token_invalid_401(self, mock_urlopen):
        """Test validate_token returns INVALID for 401."""
        from urllib.error import HTTPError

        mock_urlopen.side_effect = HTTPError(
            url="test", code=401, msg="Unauthorized", hdrs={}, fp=None
        )

        manager = GitHubTokenManager()
        token = GitHubOAuthToken(access_token="invalid_token")

        status = manager.validate_token(token)

        assert status == TokenStatus.INVALID

    def test_validate_token_empty(self):
        """Test validate_token returns INVALID for empty token."""
        manager = GitHubTokenManager()
        token = GitHubOAuthToken(access_token="")

        status = manager.validate_token(token)

        assert status == TokenStatus.INVALID

    def test_validate_token_expired(self):
        """Test validate_token returns EXPIRED for expired token."""
        manager = GitHubTokenManager()
        token = GitHubOAuthToken(
            access_token="test",
            expires_at=datetime.now() - timedelta(hours=1),
        )

        status = manager.validate_token(token)

        assert status == TokenStatus.EXPIRED

    def test_get_token_info(self):
        """Test get_token_info returns token information."""
        manager = GitHubTokenManager()
        token = GitHubOAuthToken(
            access_token="test",
            scope="repo,user",
            expires_at=datetime.now() + timedelta(hours=1),
            refresh_token="refresh",
            user_id="user_123",
        )

        with patch.object(manager, "validate_token", return_value=TokenStatus.VALID):
            info = manager.get_token_info(token)

        assert info["status"] == "valid"
        assert info["is_expired"] is False
        assert info["is_expiring_soon"] is False
        assert info["has_refresh_token"] is True
        assert info["scope"] == "repo,user"
        assert info["user_id"] == "user_123"

    def test_clear_cache_all(self):
        """Test clear_cache removes all entries."""
        manager = GitHubTokenManager()
        manager._token_cache["user1"] = GitHubOAuthToken(access_token="t1")
        manager._token_cache["user2"] = GitHubOAuthToken(access_token="t2")

        count = manager.clear_cache()

        assert count == 2
        assert len(manager._token_cache) == 0

    def test_clear_cache_specific_user(self):
        """Test clear_cache removes specific user."""
        manager = GitHubTokenManager()
        manager._token_cache["user1"] = GitHubOAuthToken(access_token="t1")
        manager._token_cache["user2"] = GitHubOAuthToken(access_token="t2")

        count = manager.clear_cache(user_id="user1")

        assert count == 1
        assert "user1" not in manager._token_cache
        assert "user2" in manager._token_cache

    def test_clear_cache_nonexistent_user(self):
        """Test clear_cache returns 0 for nonexistent user."""
        manager = GitHubTokenManager()

        count = manager.clear_cache(user_id="nonexistent")

        assert count == 0

    def test_get_cached_tokens(self):
        """Test get_cached_tokens returns sanitized cache info."""
        manager = GitHubTokenManager()
        manager._token_cache["user1"] = GitHubOAuthToken(
            access_token="secret_token",
            scope="repo",
            expires_at=datetime(2025, 6, 15, 12, 0, 0),
        )

        cached = manager.get_cached_tokens()

        assert "user1" in cached
        assert "access_token" not in cached["user1"]  # Should not expose token
        assert cached["user1"]["scope"] == "repo"
        assert cached["user1"]["expires_at"] == "2025-06-15T12:00:00"

    def test_get_cached_tokens_empty(self):
        """Test get_cached_tokens returns empty dict when no cache."""
        manager = GitHubTokenManager()

        cached = manager.get_cached_tokens()

        assert cached == {}


class TestGitHubTokenManagerIntegration:
    """Integration-style tests for GitHubTokenManager."""

    def test_auto_refresh_expiring_token_from_cache(self):
        """Test auto-refresh of expiring token from cache."""
        manager = GitHubTokenManager(
            github_client_id="client",
            github_client_secret="secret",
            auto_refresh=True,
        )

        # Put expiring token in cache
        expiring_token = GitHubOAuthToken(
            access_token="expiring",
            expires_at=datetime.now() + timedelta(minutes=2),
            refresh_token="refresh_token",
        )
        manager._token_cache["user_123"] = expiring_token

        # Mock refresh
        with patch.object(manager, "refresh_token") as mock_refresh:
            new_token = GitHubOAuthToken(
                access_token="refreshed_token",
                expires_at=datetime.now() + timedelta(hours=1),
            )
            mock_refresh.return_value = TokenRefreshResult(
                success=True,
                token=new_token,
                message="Refreshed",
            )

            token = manager.get_valid_token(user_id="user_123")

            assert token is not None
            assert token.access_token == "refreshed_token"
            mock_refresh.assert_called_once()

    def test_fallback_chain_no_user_id(self):
        """Test fallback to env when no user_id provided."""
        manager = GitHubTokenManager()

        with patch.dict(os.environ, {"GITHUB_TOKEN": "env_fallback"}):
            token = manager.get_valid_token()

        assert token is not None
        assert token.access_token == "env_fallback"

    def test_caches_token_after_retrieval(self):
        """Test that tokens are cached after retrieval."""
        manager = GitHubTokenManager()

        with patch.dict(os.environ, {"GITHUB_TOKEN": "env_token"}):
            # First call
            token1 = manager.get_valid_token()
            # Second call should use cache
            token2 = manager.get_valid_token()

        assert token1 is token2  # Same object from cache
        assert "_env_" in manager._token_cache
