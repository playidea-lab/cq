"""C4 Token Manager - Automatic token refresh and HTTP client with auth."""

import os
from datetime import datetime, timedelta
from typing import Any, Callable

import httpx

from .session import Session, SessionManager


class TokenExpiredError(Exception):
    """Raised when token is expired and refresh failed."""

    pass


class NotAuthenticatedError(Exception):
    """Raised when no valid session exists."""

    pass


class TokenManager:
    """Manages token lifecycle including refresh and authorization headers."""

    # Refresh token when less than this many seconds remain
    REFRESH_THRESHOLD_SECONDS = 300  # 5 minutes

    def __init__(
        self,
        session_manager: SessionManager | None = None,
        supabase_url: str | None = None,
    ):
        """Initialize TokenManager.

        Args:
            session_manager: SessionManager instance (uses default if None)
            supabase_url: Supabase project URL (from env if None)
        """
        self.session_manager = session_manager or SessionManager()
        self._supabase_url = supabase_url
        self._on_relogin_required: Callable[[], bool] | None = None

    @property
    def supabase_url(self) -> str | None:
        """Get Supabase URL from config or environment."""
        if self._supabase_url:
            return self._supabase_url

        # Try environment
        url = os.environ.get("SUPABASE_URL")
        if url:
            return url

        # Try config file
        from pathlib import Path

        import yaml

        config_path = Path.home() / ".c4" / "cloud.yaml"
        if config_path.exists():
            try:
                config = yaml.safe_load(config_path.read_text())
                return config.get("supabase_url")
            except Exception:
                pass

        return None

    def set_relogin_callback(self, callback: Callable[[], bool]) -> None:
        """Set callback for when re-login is required.

        Args:
            callback: Function that handles re-login, returns True if successful
        """
        self._on_relogin_required = callback

    def get_session(self) -> Session:
        """Get current valid session, refreshing if needed.

        Returns:
            Valid Session object

        Raises:
            NotAuthenticatedError: If no session exists
            TokenExpiredError: If token expired and refresh failed
        """
        session = self.session_manager.load()

        if session is None:
            raise NotAuthenticatedError("Not logged in. Run 'c4 login' first.")

        # Check if refresh is needed
        if self._needs_refresh(session):
            session = self._refresh_token(session)

        return session

    def get_access_token(self) -> str:
        """Get valid access token.

        Returns:
            Access token string

        Raises:
            NotAuthenticatedError: If no session exists
            TokenExpiredError: If token expired and refresh failed
        """
        return self.get_session().access_token

    def get_auth_headers(self) -> dict[str, str]:
        """Get authorization headers for HTTP requests.

        Returns:
            Dict with Authorization header

        Raises:
            NotAuthenticatedError: If no session exists
            TokenExpiredError: If token expired and refresh failed
        """
        token = self.get_access_token()
        return {"Authorization": f"Bearer {token}"}

    def _needs_refresh(self, session: Session) -> bool:
        """Check if session token needs refresh.

        Args:
            session: Session to check

        Returns:
            True if refresh is needed
        """
        if session.expires_at is None:
            return False

        remaining = (session.expires_at - datetime.now()).total_seconds()
        return remaining < self.REFRESH_THRESHOLD_SECONDS

    def _refresh_token(self, session: Session) -> Session:
        """Refresh the session token.

        Args:
            session: Session with refresh token

        Returns:
            New Session with refreshed tokens

        Raises:
            TokenExpiredError: If refresh failed
        """
        if not session.refresh_token:
            return self._handle_refresh_failure(session, "No refresh token")

        supabase_url = self.supabase_url
        if not supabase_url:
            return self._handle_refresh_failure(session, "Supabase URL not configured")

        try:
            response = httpx.post(
                f"{supabase_url}/auth/v1/token",
                params={"grant_type": "refresh_token"},
                json={"refresh_token": session.refresh_token},
                headers={"apikey": os.environ.get("SUPABASE_ANON_KEY", "")},
                timeout=10.0,
            )

            if response.status_code != 200:
                return self._handle_refresh_failure(
                    session,
                    f"Refresh failed: {response.status_code}",
                )

            data = response.json()

            # Create new session with refreshed tokens
            new_session = Session(
                access_token=data["access_token"],
                refresh_token=data.get("refresh_token", session.refresh_token),
                token_type=data.get("token_type", "bearer"),
                expires_at=(datetime.now() + timedelta(seconds=data.get("expires_in", 3600))),
                user_id=session.user_id,
                email=session.email,
                provider=session.provider,
                provider_token=session.provider_token,
                metadata=session.metadata,
            )

            # Save refreshed session
            self.session_manager.save(new_session)

            return new_session

        except httpx.RequestError as e:
            return self._handle_refresh_failure(session, f"Network error: {e}")

    def _handle_refresh_failure(self, session: Session, reason: str) -> Session:
        """Handle token refresh failure.

        Args:
            session: Original session
            reason: Reason for failure

        Returns:
            Original session if still valid

        Raises:
            TokenExpiredError: If session is expired
        """
        # If session is expired, try re-login callback
        if session.is_expired:
            if self._on_relogin_required:
                if self._on_relogin_required():
                    # Re-login successful, load new session
                    new_session = self.session_manager.load()
                    if new_session and not new_session.is_expired:
                        return new_session

            raise TokenExpiredError(f"Token expired and refresh failed: {reason}")

        # Session not expired yet, continue with existing token
        return session

    def ensure_authenticated(self) -> bool:
        """Ensure user is authenticated, prompting if needed.

        Returns:
            True if authenticated

        Raises:
            NotAuthenticatedError: If authentication failed
        """
        try:
            self.get_session()
            return True
        except NotAuthenticatedError:
            if self._on_relogin_required:
                if self._on_relogin_required():
                    return True
            raise


class AuthenticatedClient:
    """HTTP client with automatic authentication."""

    def __init__(
        self,
        token_manager: TokenManager | None = None,
        base_url: str | None = None,
    ):
        """Initialize authenticated HTTP client.

        Args:
            token_manager: TokenManager instance
            base_url: Base URL for requests
        """
        self.token_manager = token_manager or TokenManager()
        self.base_url = base_url or ""
        self._client: httpx.Client | None = None

    @property
    def client(self) -> httpx.Client:
        """Get or create HTTP client."""
        if self._client is None:
            self._client = httpx.Client(
                base_url=self.base_url,
                timeout=30.0,
            )
        return self._client

    def _get_headers(self, extra_headers: dict[str, str] | None = None) -> dict[str, str]:
        """Get headers including authorization.

        Args:
            extra_headers: Additional headers to include

        Returns:
            Combined headers dict
        """
        headers = self.token_manager.get_auth_headers()
        if extra_headers:
            headers.update(extra_headers)
        return headers

    def get(
        self,
        url: str,
        params: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> httpx.Response:
        """Make authenticated GET request.

        Args:
            url: Request URL
            params: Query parameters
            headers: Additional headers

        Returns:
            Response object
        """
        return self.client.get(
            url,
            params=params,
            headers=self._get_headers(headers),
        )

    def post(
        self,
        url: str,
        json: dict[str, Any] | None = None,
        data: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> httpx.Response:
        """Make authenticated POST request.

        Args:
            url: Request URL
            json: JSON body
            data: Form data
            headers: Additional headers

        Returns:
            Response object
        """
        return self.client.post(
            url,
            json=json,
            data=data,
            headers=self._get_headers(headers),
        )

    def put(
        self,
        url: str,
        json: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> httpx.Response:
        """Make authenticated PUT request.

        Args:
            url: Request URL
            json: JSON body
            headers: Additional headers

        Returns:
            Response object
        """
        return self.client.put(
            url,
            json=json,
            headers=self._get_headers(headers),
        )

    def delete(
        self,
        url: str,
        headers: dict[str, str] | None = None,
    ) -> httpx.Response:
        """Make authenticated DELETE request.

        Args:
            url: Request URL
            headers: Additional headers

        Returns:
            Response object
        """
        return self.client.delete(
            url,
            headers=self._get_headers(headers),
        )

    def close(self) -> None:
        """Close the HTTP client."""
        if self._client:
            self._client.close()
            self._client = None

    def __enter__(self) -> "AuthenticatedClient":
        """Context manager entry."""
        return self

    def __exit__(self, *args: Any) -> None:
        """Context manager exit."""
        self.close()
