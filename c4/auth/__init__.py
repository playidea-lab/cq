"""C4 Authentication module for Supabase OAuth."""

from .oauth import OAuthConfig, OAuthFlow, OAuthResult
from .session import Session, SessionManager
from .token_manager import (
    AuthenticatedClient,
    NotAuthenticatedError,
    TokenExpiredError,
    TokenManager,
)

__all__ = [
    "AuthenticatedClient",
    "NotAuthenticatedError",
    "OAuthConfig",
    "OAuthFlow",
    "OAuthResult",
    "Session",
    "SessionManager",
    "TokenExpiredError",
    "TokenManager",
]
