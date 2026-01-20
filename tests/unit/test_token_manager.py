"""Tests for C4 Token Manager module."""

from datetime import datetime, timedelta
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.auth import Session, SessionManager
from c4.auth.token_manager import (
    AuthenticatedClient,
    NotAuthenticatedError,
    TokenExpiredError,
    TokenManager,
)


class TestTokenManager:
    """Test TokenManager class."""

    @pytest.fixture
    def temp_config_dir(self, tmp_path: Path) -> Path:
        """Create temporary config directory."""
        config_dir = tmp_path / ".c4"
        config_dir.mkdir()
        return config_dir

    @pytest.fixture
    def session_manager(self, temp_config_dir: Path) -> SessionManager:
        """Create SessionManager with temp directory."""
        return SessionManager(temp_config_dir)

    @pytest.fixture
    def token_manager(self, session_manager: SessionManager) -> TokenManager:
        """Create TokenManager with test session manager."""
        return TokenManager(
            session_manager=session_manager,
            supabase_url="https://test.supabase.co",
        )

    def test_get_session_no_session(self, token_manager: TokenManager) -> None:
        """Test getting session when not logged in."""
        with pytest.raises(NotAuthenticatedError, match="Not logged in"):
            token_manager.get_session()

    def test_get_session_valid(
        self, token_manager: TokenManager, session_manager: SessionManager
    ) -> None:
        """Test getting valid session."""
        # Create valid session
        session = Session(
            access_token="valid_token",
            refresh_token="refresh_token",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        session_manager.save(session)

        result = token_manager.get_session()
        assert result.access_token == "valid_token"

    def test_get_access_token(
        self, token_manager: TokenManager, session_manager: SessionManager
    ) -> None:
        """Test getting access token."""
        session = Session(
            access_token="my_token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        session_manager.save(session)

        token = token_manager.get_access_token()
        assert token == "my_token"

    def test_get_auth_headers(
        self, token_manager: TokenManager, session_manager: SessionManager
    ) -> None:
        """Test getting authorization headers."""
        session = Session(
            access_token="bearer_token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        session_manager.save(session)

        headers = token_manager.get_auth_headers()
        assert headers == {"Authorization": "Bearer bearer_token"}

    def test_needs_refresh_no_expiry(self, token_manager: TokenManager) -> None:
        """Test needs_refresh when no expiry set."""
        session = Session(
            access_token="token",
            refresh_token="refresh",
        )
        assert token_manager._needs_refresh(session) is False

    def test_needs_refresh_not_needed(self, token_manager: TokenManager) -> None:
        """Test needs_refresh when plenty of time remaining."""
        session = Session(
            access_token="token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        assert token_manager._needs_refresh(session) is False

    def test_needs_refresh_needed(self, token_manager: TokenManager) -> None:
        """Test needs_refresh when threshold reached."""
        session = Session(
            access_token="token",
            refresh_token="refresh",
            # Less than 5 minutes remaining
            expires_at=datetime.now() + timedelta(minutes=3),
        )
        assert token_manager._needs_refresh(session) is True

    def test_relogin_callback(
        self, token_manager: TokenManager, session_manager: SessionManager
    ) -> None:
        """Test re-login callback is called on expired token."""
        # Create expired session
        session = Session(
            access_token="expired",
            refresh_token="",  # No refresh token
            expires_at=datetime.now() - timedelta(hours=1),
        )
        session_manager.save(session)

        # Setup callback
        callback_called = False

        def relogin_callback() -> bool:
            nonlocal callback_called
            callback_called = True
            # Simulate successful re-login
            new_session = Session(
                access_token="new_token",
                refresh_token="new_refresh",
                expires_at=datetime.now() + timedelta(hours=1),
            )
            session_manager.save(new_session)
            return True

        token_manager.set_relogin_callback(relogin_callback)

        # Should trigger callback and get new session
        result = token_manager.get_session()
        assert callback_called is True
        assert result.access_token == "new_token"

    def test_expired_token_no_callback(
        self, token_manager: TokenManager, session_manager: SessionManager
    ) -> None:
        """Test expired token with no callback raises error."""
        session = Session(
            access_token="expired",
            refresh_token="",
            expires_at=datetime.now() - timedelta(hours=1),
        )
        session_manager.save(session)

        with pytest.raises(TokenExpiredError, match="Token expired"):
            token_manager.get_session()

    def test_ensure_authenticated_valid(
        self, token_manager: TokenManager, session_manager: SessionManager
    ) -> None:
        """Test ensure_authenticated with valid session."""
        session = Session(
            access_token="token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        session_manager.save(session)

        assert token_manager.ensure_authenticated() is True

    def test_ensure_authenticated_no_session(self, token_manager: TokenManager) -> None:
        """Test ensure_authenticated with no session."""
        with pytest.raises(NotAuthenticatedError):
            token_manager.ensure_authenticated()

    @patch("c4.auth.token_manager.httpx.post")
    def test_refresh_token_success(
        self,
        mock_post: MagicMock,
        token_manager: TokenManager,
        session_manager: SessionManager,
    ) -> None:
        """Test successful token refresh."""
        # Create session that needs refresh
        session = Session(
            access_token="old_token",
            refresh_token="valid_refresh",
            expires_at=datetime.now() + timedelta(minutes=2),  # Needs refresh
            email="test@example.com",
        )
        session_manager.save(session)

        # Mock successful refresh response
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "access_token": "new_token",
            "refresh_token": "new_refresh",
            "expires_in": 3600,
        }
        mock_post.return_value = mock_response

        result = token_manager.get_session()
        assert result.access_token == "new_token"
        # Verify session was saved
        loaded = session_manager.load()
        assert loaded is not None
        assert loaded.access_token == "new_token"

    @patch("c4.auth.token_manager.httpx.post")
    def test_refresh_token_failure_not_expired(
        self,
        mock_post: MagicMock,
        token_manager: TokenManager,
        session_manager: SessionManager,
    ) -> None:
        """Test refresh failure with non-expired token continues."""
        # Create session that needs refresh but isn't expired
        session = Session(
            access_token="old_token",
            refresh_token="valid_refresh",
            expires_at=datetime.now() + timedelta(minutes=2),
        )
        session_manager.save(session)

        # Mock failed refresh
        mock_response = MagicMock()
        mock_response.status_code = 401
        mock_post.return_value = mock_response

        # Should return original session since not yet expired
        result = token_manager.get_session()
        assert result.access_token == "old_token"


class TestAuthenticatedClient:
    """Test AuthenticatedClient class."""

    @pytest.fixture
    def temp_config_dir(self, tmp_path: Path) -> Path:
        """Create temporary config directory."""
        config_dir = tmp_path / ".c4"
        config_dir.mkdir()
        return config_dir

    @pytest.fixture
    def session_manager(self, temp_config_dir: Path) -> SessionManager:
        """Create SessionManager with temp directory."""
        return SessionManager(temp_config_dir)

    @pytest.fixture
    def token_manager(self, session_manager: SessionManager) -> TokenManager:
        """Create TokenManager with test session manager."""
        manager = SessionManager(session_manager.config_dir)
        return TokenManager(
            session_manager=manager,
            supabase_url="https://test.supabase.co",
        )

    @pytest.fixture
    def auth_client(
        self, token_manager: TokenManager, session_manager: SessionManager
    ) -> AuthenticatedClient:
        """Create AuthenticatedClient with valid session."""
        # Create valid session
        session = Session(
            access_token="client_token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        session_manager.save(session)
        # Use the same session manager for token manager
        token_manager.session_manager = session_manager

        return AuthenticatedClient(
            token_manager=token_manager,
            base_url="https://api.example.com",
        )

    def test_get_headers(self, auth_client: AuthenticatedClient) -> None:
        """Test getting headers with authorization."""
        headers = auth_client._get_headers()
        assert "Authorization" in headers
        assert headers["Authorization"] == "Bearer client_token"

    def test_get_headers_with_extra(self, auth_client: AuthenticatedClient) -> None:
        """Test getting headers with extra headers."""
        headers = auth_client._get_headers({"X-Custom": "value"})
        assert headers["Authorization"] == "Bearer client_token"
        assert headers["X-Custom"] == "value"

    def test_context_manager(self, auth_client: AuthenticatedClient) -> None:
        """Test context manager usage."""
        with auth_client as client:
            assert client is auth_client
        # Client should be closed
        assert auth_client._client is None

    @patch.object(AuthenticatedClient, "client", new_callable=lambda: MagicMock())
    def test_get_request(self, mock_client: MagicMock, auth_client: AuthenticatedClient) -> None:
        """Test GET request includes auth headers."""
        # Setup mock
        mock_response = MagicMock()
        mock_client.get.return_value = mock_response

        auth_client._client = mock_client
        auth_client.get("/test")

        mock_client.get.assert_called_once()
        call_kwargs = mock_client.get.call_args[1]
        assert "Authorization" in call_kwargs["headers"]

    @patch.object(AuthenticatedClient, "client", new_callable=lambda: MagicMock())
    def test_post_request(self, mock_client: MagicMock, auth_client: AuthenticatedClient) -> None:
        """Test POST request includes auth headers."""
        mock_response = MagicMock()
        mock_client.post.return_value = mock_response

        auth_client._client = mock_client
        auth_client.post("/test", json={"key": "value"})

        mock_client.post.assert_called_once()
        call_kwargs = mock_client.post.call_args[1]
        assert "Authorization" in call_kwargs["headers"]
        assert call_kwargs["json"] == {"key": "value"}

    @patch.object(AuthenticatedClient, "client", new_callable=lambda: MagicMock())
    def test_put_request(self, mock_client: MagicMock, auth_client: AuthenticatedClient) -> None:
        """Test PUT request includes auth headers."""
        mock_response = MagicMock()
        mock_client.put.return_value = mock_response

        auth_client._client = mock_client
        auth_client.put("/test", json={"data": "update"})

        mock_client.put.assert_called_once()
        call_kwargs = mock_client.put.call_args[1]
        assert "Authorization" in call_kwargs["headers"]

    @patch.object(AuthenticatedClient, "client", new_callable=lambda: MagicMock())
    def test_delete_request(self, mock_client: MagicMock, auth_client: AuthenticatedClient) -> None:
        """Test DELETE request includes auth headers."""
        mock_response = MagicMock()
        mock_client.delete.return_value = mock_response

        auth_client._client = mock_client
        auth_client.delete("/test")

        mock_client.delete.assert_called_once()
        call_kwargs = mock_client.delete.call_args[1]
        assert "Authorization" in call_kwargs["headers"]


class TestTokenManagerIntegration:
    """Integration tests for token management flow."""

    @pytest.fixture
    def temp_config_dir(self, tmp_path: Path) -> Path:
        """Create temporary config directory."""
        config_dir = tmp_path / ".c4"
        config_dir.mkdir()
        return config_dir

    def test_full_auth_flow(self, temp_config_dir: Path) -> None:
        """Test complete authentication flow."""
        session_manager = SessionManager(temp_config_dir)
        token_manager = TokenManager(
            session_manager=session_manager,
            supabase_url="https://test.supabase.co",
        )

        # Initially not authenticated
        with pytest.raises(NotAuthenticatedError):
            token_manager.get_session()

        # Simulate login
        session = Session(
            access_token="user_token",
            refresh_token="user_refresh",
            expires_at=datetime.now() + timedelta(hours=1),
            email="user@example.com",
            provider="github",
        )
        session_manager.save(session)

        # Now authenticated
        result = token_manager.get_session()
        assert result.access_token == "user_token"
        assert result.email == "user@example.com"

        # Get auth headers
        headers = token_manager.get_auth_headers()
        assert headers["Authorization"] == "Bearer user_token"

        # Simulate logout
        session_manager.clear()

        # Not authenticated again
        with pytest.raises(NotAuthenticatedError):
            token_manager.get_session()
