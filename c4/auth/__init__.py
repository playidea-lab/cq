"""C4 Authentication module for Supabase OAuth."""

from .oauth import OAuthConfig, OAuthFlow, OAuthResult
from .session import Session, SessionManager

__all__ = [
    "OAuthConfig",
    "OAuthFlow",
    "OAuthResult",
    "Session",
    "SessionManager",
]
