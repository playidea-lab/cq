"""C4 Session Management - Store and manage authentication sessions."""

import json
import os
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any


@dataclass
class Session:
    """Represents an authenticated user session."""

    access_token: str
    refresh_token: str
    token_type: str = "bearer"
    expires_at: datetime | None = None
    user_id: str | None = None
    email: str | None = None
    provider: str | None = None
    provider_token: str | None = None  # e.g., GitHub token
    metadata: dict[str, Any] = field(default_factory=dict)

    @property
    def is_expired(self) -> bool:
        """Check if the session has expired."""
        if self.expires_at is None:
            return False
        return datetime.now() >= self.expires_at

    @property
    def expires_in_seconds(self) -> int:
        """Get seconds until expiration."""
        if self.expires_at is None:
            return -1
        delta = self.expires_at - datetime.now()
        return max(0, int(delta.total_seconds()))

    def to_dict(self) -> dict[str, Any]:
        """Convert session to dictionary for storage."""
        return {
            "access_token": self.access_token,
            "refresh_token": self.refresh_token,
            "token_type": self.token_type,
            "expires_at": self.expires_at.isoformat() if self.expires_at else None,
            "user_id": self.user_id,
            "email": self.email,
            "provider": self.provider,
            "provider_token": self.provider_token,
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Session":
        """Create session from dictionary."""
        expires_at = None
        if data.get("expires_at"):
            expires_at = datetime.fromisoformat(data["expires_at"])

        return cls(
            access_token=data["access_token"],
            refresh_token=data["refresh_token"],
            token_type=data.get("token_type", "bearer"),
            expires_at=expires_at,
            user_id=data.get("user_id"),
            email=data.get("email"),
            provider=data.get("provider"),
            provider_token=data.get("provider_token"),
            metadata=data.get("metadata", {}),
        )

    @classmethod
    def from_supabase_response(cls, response: dict[str, Any]) -> "Session":
        """Create session from Supabase auth response.

        Args:
            response: Supabase auth response containing session and user info

        Returns:
            Session object
        """
        session_data = response.get("session", response)
        user_data = response.get("user", {})

        expires_at = None
        if session_data.get("expires_at"):
            # Supabase returns Unix timestamp
            expires_at = datetime.fromtimestamp(session_data["expires_at"])
        elif session_data.get("expires_in"):
            expires_at = datetime.now() + timedelta(seconds=session_data["expires_in"])

        # Extract provider token if available
        provider_token = None
        provider = None
        identities = user_data.get("identities", [])
        if identities:
            identity = identities[0]
            provider = identity.get("provider")
            # Provider token might be in identity_data
            identity_data = identity.get("identity_data", {})
            provider_token = identity_data.get("provider_token")

        return cls(
            access_token=session_data["access_token"],
            refresh_token=session_data.get("refresh_token", ""),
            token_type=session_data.get("token_type", "bearer"),
            expires_at=expires_at,
            user_id=user_data.get("id"),
            email=user_data.get("email"),
            provider=provider,
            provider_token=provider_token,
            metadata={
                "user_metadata": user_data.get("user_metadata", {}),
                "app_metadata": user_data.get("app_metadata", {}),
            },
        )


class SessionManager:
    """Manages session persistence and retrieval."""

    DEFAULT_SESSION_FILE = "session.json"

    def __init__(self, config_dir: Path | None = None):
        """Initialize SessionManager.

        Args:
            config_dir: Directory to store session file (default: ~/.c4/)
        """
        if config_dir is None:
            config_dir = Path.home() / ".c4"
        self.config_dir = config_dir
        self.session_file = config_dir / self.DEFAULT_SESSION_FILE

    def save(self, session: Session) -> bool:
        """Save session to file.

        Args:
            session: Session to save

        Returns:
            True if successful
        """
        try:
            self.config_dir.mkdir(parents=True, exist_ok=True)

            # Set restrictive permissions on session file
            self.session_file.write_text(json.dumps(session.to_dict(), indent=2))
            # Make file readable only by owner
            os.chmod(self.session_file, 0o600)

            return True
        except OSError:
            return False

    def load(self) -> Session | None:
        """Load session from file.

        Returns:
            Session if found and valid, None otherwise
        """
        if not self.session_file.exists():
            return None

        try:
            data = json.loads(self.session_file.read_text())
            return Session.from_dict(data)
        except (json.JSONDecodeError, KeyError):
            return None

    def clear(self) -> bool:
        """Clear stored session.

        Returns:
            True if successful or already cleared
        """
        if self.session_file.exists():
            try:
                self.session_file.unlink()
                return True
            except OSError:
                return False
        return True

    def get_valid_session(self) -> Session | None:
        """Get session only if it's not expired.

        Returns:
            Session if valid, None if expired or not found
        """
        session = self.load()
        if session is None:
            return None
        if session.is_expired:
            return None
        return session

    def is_logged_in(self) -> bool:
        """Check if user is logged in with valid session.

        Returns:
            True if logged in with valid session
        """
        return self.get_valid_session() is not None
