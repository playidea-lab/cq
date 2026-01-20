"""Tests for C4 Authentication module."""

from __future__ import annotations

from datetime import datetime, timedelta
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

from c4.auth import Session, SessionManager
from c4.auth.oauth import OAuthConfig, OAuthResult

if TYPE_CHECKING:
    from c4.auth.token_manager import TokenManager


class TestSession:
    """Test Session dataclass."""

    def test_create_session(self) -> None:
        """Test creating a session."""
        session = Session(
            access_token="test_token",
            refresh_token="refresh_token",
        )
        assert session.access_token == "test_token"
        assert session.refresh_token == "refresh_token"
        assert session.token_type == "bearer"

    def test_session_not_expired(self) -> None:
        """Test session expiration check when not expired."""
        session = Session(
            access_token="test",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        assert session.is_expired is False

    def test_session_expired(self) -> None:
        """Test session expiration check when expired."""
        session = Session(
            access_token="test",
            refresh_token="refresh",
            expires_at=datetime.now() - timedelta(hours=1),
        )
        assert session.is_expired is True

    def test_session_no_expiry(self) -> None:
        """Test session with no expiry is never expired."""
        session = Session(
            access_token="test",
            refresh_token="refresh",
        )
        assert session.is_expired is False

    def test_expires_in_seconds(self) -> None:
        """Test calculating seconds until expiration."""
        session = Session(
            access_token="test",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(minutes=30),
        )
        # Should be around 1800 seconds (30 minutes)
        assert 1750 < session.expires_in_seconds < 1850

    def test_to_dict(self) -> None:
        """Test serializing session to dictionary."""
        session = Session(
            access_token="token",
            refresh_token="refresh",
            user_id="user123",
            email="test@example.com",
            provider="github",
        )
        data = session.to_dict()

        assert data["access_token"] == "token"
        assert data["refresh_token"] == "refresh"
        assert data["user_id"] == "user123"
        assert data["email"] == "test@example.com"
        assert data["provider"] == "github"

    def test_from_dict(self) -> None:
        """Test deserializing session from dictionary."""
        data = {
            "access_token": "token",
            "refresh_token": "refresh",
            "user_id": "user123",
            "email": "test@example.com",
            "provider": "google",
            "expires_at": (datetime.now() + timedelta(hours=1)).isoformat(),
        }
        session = Session.from_dict(data)

        assert session.access_token == "token"
        assert session.user_id == "user123"
        assert session.provider == "google"
        assert not session.is_expired

    def test_from_supabase_response(self) -> None:
        """Test creating session from Supabase response."""
        response = {
            "session": {
                "access_token": "sb_token",
                "refresh_token": "sb_refresh",
                "expires_in": 3600,
            },
            "user": {
                "id": "user-uuid",
                "email": "user@example.com",
                "identities": [{"provider": "github", "identity_data": {}}],
            },
        }
        session = Session.from_supabase_response(response)

        assert session.access_token == "sb_token"
        assert session.user_id == "user-uuid"
        assert session.email == "user@example.com"
        assert session.provider == "github"


class TestSessionManager:
    """Test SessionManager class."""

    @pytest.fixture
    def temp_config_dir(self, tmp_path: Path) -> Path:
        """Create temporary config directory."""
        config_dir = tmp_path / ".c4"
        config_dir.mkdir()
        return config_dir

    @pytest.fixture
    def manager(self, temp_config_dir: Path) -> SessionManager:
        """Create SessionManager with temp directory."""
        return SessionManager(temp_config_dir)

    def test_save_session(self, manager: SessionManager) -> None:
        """Test saving session to file."""
        session = Session(
            access_token="test_token",
            refresh_token="refresh_token",
            email="test@example.com",
        )
        result = manager.save(session)

        assert result is True
        assert manager.session_file.exists()

    def test_load_session(self, manager: SessionManager) -> None:
        """Test loading session from file."""
        # Save first
        session = Session(
            access_token="saved_token",
            refresh_token="saved_refresh",
            user_id="user123",
        )
        manager.save(session)

        # Load
        loaded = manager.load()

        assert loaded is not None
        assert loaded.access_token == "saved_token"
        assert loaded.user_id == "user123"

    def test_load_nonexistent(self, manager: SessionManager) -> None:
        """Test loading when no session exists."""
        loaded = manager.load()
        assert loaded is None

    def test_clear_session(self, manager: SessionManager) -> None:
        """Test clearing session."""
        # Save first
        session = Session(access_token="token", refresh_token="refresh")
        manager.save(session)
        assert manager.session_file.exists()

        # Clear
        result = manager.clear()

        assert result is True
        assert not manager.session_file.exists()

    def test_clear_nonexistent(self, manager: SessionManager) -> None:
        """Test clearing when no session exists."""
        result = manager.clear()
        assert result is True

    def test_is_logged_in(self, manager: SessionManager) -> None:
        """Test checking login status."""
        assert manager.is_logged_in() is False

        # Login
        session = Session(
            access_token="token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        manager.save(session)

        assert manager.is_logged_in() is True

    def test_is_logged_in_expired(self, manager: SessionManager) -> None:
        """Test that expired session is not logged in."""
        session = Session(
            access_token="token",
            refresh_token="refresh",
            expires_at=datetime.now() - timedelta(hours=1),
        )
        manager.save(session)

        assert manager.is_logged_in() is False

    def test_get_valid_session(self, manager: SessionManager) -> None:
        """Test getting valid session only."""
        # Valid session
        session = Session(
            access_token="token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        manager.save(session)

        valid = manager.get_valid_session()
        assert valid is not None
        assert valid.access_token == "token"

    def test_get_valid_session_expired(self, manager: SessionManager) -> None:
        """Test that expired session returns None."""
        session = Session(
            access_token="token",
            refresh_token="refresh",
            expires_at=datetime.now() - timedelta(hours=1),
        )
        manager.save(session)

        valid = manager.get_valid_session()
        assert valid is None

    def test_session_file_permissions(self, manager: SessionManager, temp_config_dir: Path) -> None:
        """Test that session file has restrictive permissions."""
        import os
        import stat

        session = Session(access_token="secret", refresh_token="refresh")
        manager.save(session)

        # Check file mode (owner read/write only)
        mode = os.stat(manager.session_file).st_mode
        # On Unix, should be 0o600 (rw-------)
        assert stat.S_IMODE(mode) == 0o600


class TestOAuthConfig:
    """Test OAuthConfig dataclass."""

    def test_default_config(self) -> None:
        """Test default OAuth configuration."""
        config = OAuthConfig(supabase_url="https://test.supabase.co")

        assert config.redirect_port == 8765
        assert config.redirect_path == "/auth/callback"
        assert config.redirect_uri == "http://localhost:8765/auth/callback"

    def test_custom_port(self) -> None:
        """Test custom redirect port."""
        config = OAuthConfig(
            supabase_url="https://test.supabase.co",
            redirect_port=9000,
        )
        assert config.redirect_uri == "http://localhost:9000/auth/callback"


class TestOAuthResult:
    """Test OAuthResult dataclass."""

    def test_success_result(self) -> None:
        """Test successful OAuth result."""
        result = OAuthResult(
            success=True,
            access_token="token123",
            refresh_token="refresh123",
        )
        assert result.success is True
        assert result.access_token == "token123"
        assert result.error is None

    def test_error_result(self) -> None:
        """Test failed OAuth result."""
        result = OAuthResult(
            success=False,
            error="access_denied",
        )
        assert result.success is False
        assert result.error == "access_denied"
        assert result.access_token is None


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
    def token_manager(self, session_manager: SessionManager) -> "TokenManager":
        """Create TokenManager with session manager."""
        from c4.auth.token_manager import TokenManager

        return TokenManager(session_manager=session_manager)

    def test_get_session_not_logged_in(self, token_manager: "TokenManager") -> None:
        """Test getting session when not logged in."""
        from c4.auth.token_manager import NotAuthenticatedError

        with pytest.raises(NotAuthenticatedError):
            token_manager.get_session()

    def test_get_session_valid(
        self, token_manager: "TokenManager", session_manager: SessionManager
    ) -> None:
        """Test getting valid session."""
        session = Session(
            access_token="valid_token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        session_manager.save(session)

        result = token_manager.get_session()
        assert result.access_token == "valid_token"

    def test_get_access_token(
        self, token_manager: "TokenManager", session_manager: SessionManager
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
        self, token_manager: "TokenManager", session_manager: SessionManager
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

    def test_needs_refresh_false(self, token_manager: "TokenManager") -> None:
        """Test that refresh is not needed for fresh session."""
        session = Session(
            access_token="token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        assert token_manager._needs_refresh(session) is False

    def test_needs_refresh_true(self, token_manager: "TokenManager") -> None:
        """Test that refresh is needed when near expiry."""
        session = Session(
            access_token="token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(seconds=60),  # 1 minute left
        )
        assert token_manager._needs_refresh(session) is True

    def test_relogin_callback(
        self, token_manager: "TokenManager", session_manager: SessionManager
    ) -> None:
        """Test re-login callback is called on expired token."""

        # Create expired session
        session = Session(
            access_token="expired",
            refresh_token="",  # No refresh token
            expires_at=datetime.now() - timedelta(hours=1),
        )
        session_manager.save(session)

        # Set up callback that creates new session
        callback_called = [False]

        def relogin_callback() -> bool:
            callback_called[0] = True
            new_session = Session(
                access_token="new_token",
                refresh_token="new_refresh",
                expires_at=datetime.now() + timedelta(hours=1),
            )
            session_manager.save(new_session)
            return True

        token_manager.set_relogin_callback(relogin_callback)

        result = token_manager.get_session()
        assert callback_called[0] is True
        assert result.access_token == "new_token"

    def test_expired_no_callback_raises(
        self, token_manager: "TokenManager", session_manager: SessionManager
    ) -> None:
        """Test that expired token without callback raises error."""
        from c4.auth.token_manager import TokenExpiredError

        session = Session(
            access_token="expired",
            refresh_token="",
            expires_at=datetime.now() - timedelta(hours=1),
        )
        session_manager.save(session)

        with pytest.raises(TokenExpiredError):
            token_manager.get_session()


class TestAuthenticatedClient:
    """Test AuthenticatedClient class."""

    @pytest.fixture
    def temp_config_dir(self, tmp_path: Path) -> Path:
        """Create temporary config directory."""
        config_dir = tmp_path / ".c4"
        config_dir.mkdir()
        return config_dir

    def test_get_headers(self, temp_config_dir: Path) -> None:
        """Test getting headers with auth token."""
        from c4.auth.token_manager import AuthenticatedClient, TokenManager

        session_manager = SessionManager(temp_config_dir)
        session = Session(
            access_token="client_token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        session_manager.save(session)

        token_manager = TokenManager(session_manager=session_manager)
        client = AuthenticatedClient(token_manager=token_manager)

        headers = client._get_headers()
        assert headers["Authorization"] == "Bearer client_token"

    def test_get_headers_with_extra(self, temp_config_dir: Path) -> None:
        """Test getting headers with extra headers."""
        from c4.auth.token_manager import AuthenticatedClient, TokenManager

        session_manager = SessionManager(temp_config_dir)
        session = Session(
            access_token="token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        session_manager.save(session)

        token_manager = TokenManager(session_manager=session_manager)
        client = AuthenticatedClient(token_manager=token_manager)

        headers = client._get_headers({"X-Custom": "value"})
        assert headers["Authorization"] == "Bearer token"
        assert headers["X-Custom"] == "value"

    def test_context_manager(self, temp_config_dir: Path) -> None:
        """Test client as context manager."""
        from c4.auth.token_manager import AuthenticatedClient, TokenManager

        session_manager = SessionManager(temp_config_dir)
        session = Session(
            access_token="token",
            refresh_token="refresh",
            expires_at=datetime.now() + timedelta(hours=1),
        )
        session_manager.save(session)

        token_manager = TokenManager(session_manager=session_manager)

        with AuthenticatedClient(token_manager=token_manager) as client:
            assert client is not None
            # Client should work within context
            assert client._client is None  # Lazy initialization
