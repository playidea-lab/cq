"""Integration tests for authentication: login, token refresh, GitHub integration."""

from __future__ import annotations

import json
import os
from datetime import datetime, timedelta
from unittest.mock import MagicMock, patch

from c4.auth import GitHubOAuthToken, GitHubTokenManager
from c4.auth.github_token import TokenStatus
from c4.integrations.github import GitHubClient


class TestLoginFlow:
    """Integration tests for login flow."""

    def test_login_from_environment_variable(self):
        """Test login flow using environment variable token."""
        manager = GitHubTokenManager()

        with patch.dict(os.environ, {"GITHUB_TOKEN": "env_github_token"}):
            token = manager.get_valid_token()

        assert token is not None
        assert token.access_token == "env_github_token"
        assert token.metadata.get("source") == "environment"

    def test_login_from_env_no_expiry(self):
        """Test env token has no expiration."""
        manager = GitHubTokenManager()

        with patch.dict(os.environ, {"GITHUB_TOKEN": "test_token"}):
            token = manager.get_valid_token()

        assert token is not None
        assert token.expires_at is None
        assert token.is_expired is False
        assert token.status == TokenStatus.VALID

    @patch("c4.auth.github_token.urlopen")
    def test_login_from_supabase_provider_token(self, mock_urlopen):
        """Test login flow using Supabase provider token."""
        # Mock Supabase response
        mock_response = MagicMock()
        mock_response.__enter__ = MagicMock(return_value=mock_response)
        mock_response.__exit__ = MagicMock(return_value=False)
        mock_response.read.return_value = json.dumps(
            {
                "id": "identity_123",
                "user_id": "user_456",
                "provider": "github",
                "identity_data": {
                    "provider_token": "supabase_github_token",
                    "provider_refresh_token": "refresh_token_789",
                    "scope": "repo,user",
                    "expires_in": 7200,
                },
            }
        ).encode()
        mock_urlopen.return_value = mock_response

        manager = GitHubTokenManager(
            supabase_url="https://test.supabase.co",
            supabase_key="test_key",
        )
        token = manager.get_token_from_supabase(user_id="user_456")

        assert token is not None
        assert token.access_token == "supabase_github_token"
        assert token.refresh_token == "refresh_token_789"
        assert token.scope == "repo,user"
        assert token.user_id == "user_456"

    def test_login_fallback_chain(self):
        """Test login uses fallback chain (Supabase -> env)."""
        manager = GitHubTokenManager()  # No Supabase config

        with patch.dict(os.environ, {"GITHUB_TOKEN": "fallback_token"}):
            # No user_id, so Supabase won't be tried
            token = manager.get_valid_token()

        assert token is not None
        assert token.access_token == "fallback_token"

    @patch("c4.auth.github_token.urlopen")
    def test_login_caches_token(self, mock_urlopen):
        """Test that tokens are cached after login."""
        mock_response = MagicMock()
        mock_response.__enter__ = MagicMock(return_value=mock_response)
        mock_response.__exit__ = MagicMock(return_value=False)
        mock_response.read.return_value = json.dumps(
            {
                "identity_data": {
                    "provider_token": "cached_token",
                    "expires_in": 3600,
                },
            }
        ).encode()
        mock_urlopen.return_value = mock_response

        manager = GitHubTokenManager(
            supabase_url="https://test.supabase.co",
            supabase_key="test_key",
        )

        # First call - fetches from Supabase
        token1 = manager.get_valid_token(user_id="user_123")
        # Second call - should use cache
        token2 = manager.get_valid_token(user_id="user_123")

        # Only one Supabase call should be made
        assert mock_urlopen.call_count == 1
        assert token1 is token2  # Same object from cache


class TestTokenRefreshFlow:
    """Integration tests for token refresh flow."""

    @patch("c4.auth.github_token.urlopen")
    def test_refresh_token_success(self, mock_urlopen):
        """Test successful token refresh."""
        mock_response = MagicMock()
        mock_response.__enter__ = MagicMock(return_value=mock_response)
        mock_response.__exit__ = MagicMock(return_value=False)
        mock_response.read.return_value = json.dumps(
            {
                "access_token": "new_access_token",
                "token_type": "bearer",
                "expires_in": 7200,
                "refresh_token": "new_refresh_token",
                "scope": "repo,user",
            }
        ).encode()
        mock_urlopen.return_value = mock_response

        manager = GitHubTokenManager(
            github_client_id="test_client_id",
            github_client_secret="test_client_secret",
        )

        old_token = GitHubOAuthToken(
            access_token="old_token",
            refresh_token="old_refresh_token",
            scope="repo,user",
        )

        result = manager.refresh_token(old_token)

        assert result.success is True
        assert result.token is not None
        assert result.token.access_token == "new_access_token"
        assert result.token.refresh_token == "new_refresh_token"
        assert result.token.expires_at is not None
        # Should be about 2 hours from now
        assert result.token.remaining_seconds is not None
        assert 7100 <= result.token.remaining_seconds <= 7300

    @patch("c4.auth.github_token.urlopen")
    def test_refresh_token_preserves_metadata(self, mock_urlopen):
        """Test that refresh preserves user metadata."""
        mock_response = MagicMock()
        mock_response.__enter__ = MagicMock(return_value=mock_response)
        mock_response.__exit__ = MagicMock(return_value=False)
        mock_response.read.return_value = json.dumps(
            {
                "access_token": "refreshed_token",
                "expires_in": 3600,
            }
        ).encode()
        mock_urlopen.return_value = mock_response

        manager = GitHubTokenManager(
            github_client_id="client",
            github_client_secret="secret",
        )

        old_token = GitHubOAuthToken(
            access_token="old",
            refresh_token="refresh",
            user_id="user_123",
            provider_id="provider_456",
            metadata={"original_source": "supabase"},
        )

        result = manager.refresh_token(old_token)

        assert result.success is True
        assert result.token.user_id == "user_123"
        assert result.token.provider_id == "provider_456"
        assert "original_source" in result.token.metadata
        assert "refreshed_at" in result.token.metadata

    def test_refresh_token_missing_refresh_token(self):
        """Test refresh fails without refresh token."""
        manager = GitHubTokenManager(
            github_client_id="client",
            github_client_secret="secret",
        )

        token = GitHubOAuthToken(access_token="test")

        result = manager.refresh_token(token)

        assert result.success is False
        assert result.error == "missing_refresh_token"

    def test_refresh_token_missing_oauth_config(self):
        """Test refresh fails without OAuth credentials."""
        manager = GitHubTokenManager()  # No OAuth config

        token = GitHubOAuthToken(
            access_token="test",
            refresh_token="refresh_token",
        )

        result = manager.refresh_token(token)

        assert result.success is False
        assert result.error == "missing_oauth_config"

    @patch("c4.auth.github_token.urlopen")
    def test_refresh_token_invalid_grant(self, mock_urlopen):
        """Test refresh handles invalid_grant error."""
        mock_response = MagicMock()
        mock_response.__enter__ = MagicMock(return_value=mock_response)
        mock_response.__exit__ = MagicMock(return_value=False)
        mock_response.read.return_value = json.dumps(
            {
                "error": "invalid_grant",
                "error_description": "The refresh token has expired",
            }
        ).encode()
        mock_urlopen.return_value = mock_response

        manager = GitHubTokenManager(
            github_client_id="client",
            github_client_secret="secret",
        )

        token = GitHubOAuthToken(
            access_token="test",
            refresh_token="expired_refresh_token",
        )

        result = manager.refresh_token(token)

        assert result.success is False
        assert result.error == "invalid_grant"
        assert "expired" in result.message.lower()

    @patch("c4.auth.github_token.urlopen")
    def test_auto_refresh_expiring_token(self, mock_urlopen):
        """Test auto-refresh when token is expiring soon."""
        # First call returns expiring token from Supabase
        # Second call refreshes the token
        call_count = [0]

        def mock_urlopen_impl(req, **kwargs):
            call_count[0] += 1
            mock_response = MagicMock()
            mock_response.__enter__ = MagicMock(return_value=mock_response)
            mock_response.__exit__ = MagicMock(return_value=False)

            if call_count[0] == 1:
                # Supabase returns expiring token
                mock_response.read.return_value = json.dumps(
                    {
                        "identity_data": {
                            "provider_token": "expiring_token",
                            "provider_refresh_token": "refresh_token",
                            "expires_in": 60,  # Expiring in 1 minute
                        },
                    }
                ).encode()
            else:
                # GitHub returns refreshed token
                mock_response.read.return_value = json.dumps(
                    {
                        "access_token": "fresh_token",
                        "expires_in": 3600,
                    }
                ).encode()
            return mock_response

        mock_urlopen.side_effect = mock_urlopen_impl

        manager = GitHubTokenManager(
            supabase_url="https://test.supabase.co",
            supabase_key="test_key",
            github_client_id="client",
            github_client_secret="secret",
            auto_refresh=True,
        )

        token = manager.get_valid_token(user_id="user_123")

        assert token is not None
        assert token.access_token == "fresh_token"  # Got refreshed token
        assert call_count[0] == 2  # Supabase + refresh calls

    def test_no_auto_refresh_when_disabled(self):
        """Test auto-refresh is skipped when disabled."""
        manager = GitHubTokenManager(auto_refresh=False)

        # Put expiring token in cache
        expiring_token = GitHubOAuthToken(
            access_token="expiring",
            expires_at=datetime.now() + timedelta(minutes=2),
            refresh_token="refresh",
        )
        manager._token_cache["user_123"] = expiring_token

        with patch.dict(os.environ, {"GITHUB_TOKEN": "env_token"}):
            # Should return env token since cached one is expiring
            # and auto_refresh is disabled
            token = manager.get_valid_token(user_id="user_123")

        # Returns env token as fallback since expiring token not refreshed
        assert token.access_token == "env_token"


class TestGitHubIntegration:
    """Integration tests for GitHub API operations."""

    @patch("c4.integrations.github.subprocess.run")
    def test_github_client_with_gh_cli(self, mock_run):
        """Test GitHubClient uses gh CLI when available."""
        # Mock gh --version
        mock_run.return_value = MagicMock(returncode=0, stdout="", stderr="")

        client = GitHubClient(token="test_token")
        assert client.is_gh_available() is True

    @patch("c4.integrations.github.subprocess.run")
    def test_github_client_without_gh_cli(self, mock_run):
        """Test GitHubClient falls back to API without gh CLI."""
        mock_run.side_effect = FileNotFoundError("gh not found")

        client = GitHubClient(token="test_token")
        assert client.is_gh_available() is False

    def test_check_org_membership_api(self):
        """Test org membership check via API."""
        client = GitHubClient(token="test_token")
        client._gh_available = False  # Force API usage

        # Mock _api_request to return 204 (member)
        with patch.object(client, "_api_request", return_value=(204, None)):
            result = client.check_org_membership("test-org", "test-user")

        assert result.success is True
        assert result.data["is_member"] is True

    @patch("c4.integrations.github.urlopen")
    def test_check_org_membership_not_member(self, mock_urlopen):
        """Test org membership check when user is not a member."""
        from urllib.error import HTTPError

        mock_urlopen.side_effect = HTTPError(
            url="test", code=404, msg="Not Found", hdrs={}, fp=MagicMock()
        )
        mock_urlopen.side_effect.read = MagicMock(return_value=b'{"message": "Not Found"}')

        client = GitHubClient(token="test_token")
        client._gh_available = False

        result = client.check_org_membership("test-org", "nonmember")

        assert result.success is True
        assert result.data["is_member"] is False

    @patch("c4.integrations.github.urlopen")
    def test_check_org_membership_unauthorized(self, mock_urlopen):
        """Test org membership check with invalid token."""
        from urllib.error import HTTPError

        mock_urlopen.side_effect = HTTPError(
            url="test", code=401, msg="Unauthorized", hdrs={}, fp=MagicMock()
        )
        mock_urlopen.side_effect.read = MagicMock(return_value=b'{"message": "Bad credentials"}')

        client = GitHubClient(token="invalid_token")
        client._gh_available = False

        result = client.check_org_membership("test-org", "test-user")

        assert result.success is False
        assert "401" in str(result.data) or "Bad credentials" in str(result.data)

    def test_github_client_no_token(self):
        """Test GitHubClient handles missing token."""
        with patch.dict(os.environ, {}, clear=True):
            os.environ.pop("GITHUB_TOKEN", None)
            client = GitHubClient()

        assert client.token is None

        client._gh_available = False
        status, data = client._api_request("GET", "/user")

        assert status == 401
        assert "No GitHub token" in data.get("message", "")


class TestAuthWithGitHub:
    """Integration tests combining auth and GitHub operations."""

    @patch("c4.auth.github_token.urlopen")
    def test_full_auth_flow_with_github_api(self, mock_auth_urlopen):
        """Test complete flow: get token from Supabase, use with GitHub API."""
        # Setup auth mock
        mock_auth_response = MagicMock()
        mock_auth_response.__enter__ = MagicMock(return_value=mock_auth_response)
        mock_auth_response.__exit__ = MagicMock(return_value=False)
        mock_auth_response.read.return_value = json.dumps(
            {
                "identity_data": {
                    "provider_token": "github_token_from_supabase",
                    "scope": "repo,user",
                },
            }
        ).encode()
        mock_auth_urlopen.return_value = mock_auth_response

        # Get token from auth manager
        manager = GitHubTokenManager(
            supabase_url="https://test.supabase.co",
            supabase_key="test_key",
        )
        token = manager.get_token_from_supabase(user_id="user_123")

        assert token is not None
        assert token.access_token == "github_token_from_supabase"

        # Use token with GitHub client
        client = GitHubClient(token=token.access_token)
        client._gh_available = False

        # Mock _api_request to return 204 (member)
        with patch.object(client, "_api_request", return_value=(204, None)):
            result = client.check_org_membership("my-org", "my-user")

        assert result.success is True
        assert result.data["is_member"] is True

    @patch("c4.auth.github_token.urlopen")
    def test_token_validation_with_github(self, mock_urlopen):
        """Test token validation by calling GitHub API."""
        mock_response = MagicMock()
        mock_response.__enter__ = MagicMock(return_value=mock_response)
        mock_response.__exit__ = MagicMock(return_value=False)
        mock_response.status = 200
        mock_response.read.return_value = json.dumps(
            {
                "login": "test-user",
                "id": 123456,
            }
        ).encode()
        mock_urlopen.return_value = mock_response

        manager = GitHubTokenManager()
        token = GitHubOAuthToken(
            access_token="valid_token",
            expires_at=datetime.now() + timedelta(hours=1),
        )

        status = manager.validate_token(token)

        assert status == TokenStatus.VALID

    @patch("c4.auth.github_token.urlopen")
    def test_token_validation_invalid(self, mock_urlopen):
        """Test token validation detects invalid token."""
        from urllib.error import HTTPError

        mock_urlopen.side_effect = HTTPError(
            url="test", code=401, msg="Unauthorized", hdrs={}, fp=MagicMock()
        )

        manager = GitHubTokenManager()
        token = GitHubOAuthToken(access_token="invalid_token")

        status = manager.validate_token(token)

        assert status == TokenStatus.INVALID

    def test_get_token_info(self):
        """Test getting comprehensive token information."""
        manager = GitHubTokenManager()
        token = GitHubOAuthToken(
            access_token="test_token",
            scope="repo,user,gist",
            expires_at=datetime.now() + timedelta(hours=2),
            refresh_token="has_refresh",
            user_id="user_123",
        )

        with patch.object(manager, "validate_token", return_value=TokenStatus.VALID):
            info = manager.get_token_info(token)

        assert info["status"] == "valid"
        assert info["is_expired"] is False
        assert info["is_expiring_soon"] is False
        assert info["has_refresh_token"] is True
        assert info["scope"] == "repo,user,gist"
        assert info["user_id"] == "user_123"


class TestCacheManagement:
    """Integration tests for token cache management."""

    def test_cache_lifecycle(self):
        """Test complete cache lifecycle."""
        manager = GitHubTokenManager()

        # Initially empty
        assert manager.get_cached_tokens() == {}

        # Add tokens via get_valid_token
        with patch.dict(os.environ, {"GITHUB_TOKEN": "token1"}):
            manager.get_valid_token(user_id="user1")
        with patch.dict(os.environ, {"GITHUB_TOKEN": "token2"}):
            manager.get_valid_token(user_id="user2")

        cached = manager.get_cached_tokens()
        assert "user1" in cached
        assert "user2" in cached
        assert "access_token" not in cached["user1"]  # Secrets hidden

        # Clear specific user
        cleared = manager.clear_cache(user_id="user1")
        assert cleared == 1
        assert "user1" not in manager.get_cached_tokens()
        assert "user2" in manager.get_cached_tokens()

        # Clear all
        cleared = manager.clear_cache()
        assert cleared == 1
        assert manager.get_cached_tokens() == {}

    def test_cache_prevents_redundant_calls(self):
        """Test that cache prevents redundant token fetches."""
        manager = GitHubTokenManager()
        call_count = [0]

        original_get_env = manager.get_token_from_env

        def counting_get_env():
            call_count[0] += 1
            return original_get_env()

        manager.get_token_from_env = counting_get_env

        with patch.dict(os.environ, {"GITHUB_TOKEN": "test"}):
            # Multiple calls
            manager.get_valid_token()
            manager.get_valid_token()
            manager.get_valid_token()

        # Only first call should fetch
        assert call_count[0] == 1

    def test_force_refresh_bypasses_cache(self):
        """Test force_refresh bypasses cache."""
        manager = GitHubTokenManager()

        # Prime cache
        with patch.dict(os.environ, {"GITHUB_TOKEN": "cached_token"}):
            manager.get_valid_token()  # Prime the cache

        # Change env token
        with patch.dict(os.environ, {"GITHUB_TOKEN": "new_token"}):
            # Normal call uses cache
            token2 = manager.get_valid_token()
            assert token2.access_token == "cached_token"

            # Force refresh bypasses cache
            token3 = manager.get_valid_token(force_refresh=True)
            assert token3.access_token == "new_token"
