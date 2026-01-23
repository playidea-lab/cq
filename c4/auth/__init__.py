"""C4 Authentication module for Supabase OAuth."""

from .github_token import GitHubOAuthToken, GitHubTokenManager
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
    "GitHubOAuthToken",
    "GitHubTokenManager",
    "NotAuthenticatedError",
    "OAuthConfig",
    "OAuthFlow",
    "OAuthResult",
    "Session",
    "SessionManager",
    "TokenExpiredError",
    "TokenManager",
]
