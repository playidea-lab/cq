"""C4 API Authentication and Security.

Provides JWT (Supabase Auth) and API key authentication for the C4 API.

Environment Variables:
    SUPABASE_JWT_SECRET: JWT secret for verifying Supabase Auth tokens
    SUPABASE_JWT_ISSUER: Expected JWT issuer (optional, e.g., https://project.supabase.co/auth/v1)
    C4_API_KEYS: Comma-separated list of valid API keys for server-to-server auth

Usage:
    from c4.api.auth import get_current_user, get_optional_user, require_auth, User

    # Protected endpoint (requires JWT or API key)
    @router.get("/protected")
    async def protected_route(user: User = Depends(get_current_user)):
        return {"user_id": user.user_id}

    # Optional authentication
    @router.get("/optional")
    async def optional_route(user: User | None = Depends(get_optional_user)):
        if user:
            return {"authenticated": True}
        return {"authenticated": False}
"""

from __future__ import annotations

import logging
import os
from dataclasses import dataclass, field
from functools import lru_cache
from typing import Annotated

import jwt
from fastapi import Depends, HTTPException, Request, status
from fastapi.security import APIKeyHeader, HTTPAuthorizationCredentials, HTTPBearer
from pydantic import BaseModel

logger = logging.getLogger(__name__)


# -----------------------------------------------------------------------------
# Exceptions
# -----------------------------------------------------------------------------


class AuthenticationError(Exception):
    """Raised when authentication fails."""

    def __init__(self, detail: str):
        self.detail = detail
        super().__init__(detail)


class AuthorizationError(Exception):
    """Raised when authorization fails."""

    def __init__(self, detail: str):
        self.detail = detail
        super().__init__(detail)


# -----------------------------------------------------------------------------
# Configuration
# -----------------------------------------------------------------------------


@dataclass
class AuthConfig:
    """Authentication configuration from environment variables."""

    jwt_secret: str | None = None
    jwt_algorithm: str = "HS256"
    jwt_issuer: str | None = None
    api_keys: list[str] = field(default_factory=list)

    def __post_init__(self):
        """Load configuration from environment variables."""
        self.jwt_secret = os.environ.get("SUPABASE_JWT_SECRET")
        self.jwt_issuer = os.environ.get("SUPABASE_JWT_ISSUER")

        api_keys_str = os.environ.get("C4_API_KEYS", "")
        if api_keys_str:
            self.api_keys = [k.strip() for k in api_keys_str.split(",") if k.strip()]
        else:
            self.api_keys = []


@lru_cache(maxsize=1)
def get_auth_config() -> AuthConfig:
    """Get cached authentication configuration.

    Returns:
        AuthConfig instance
    """
    return AuthConfig()


# -----------------------------------------------------------------------------
# JWT Payload and User Models
# -----------------------------------------------------------------------------


class JWTPayload(BaseModel):
    """JWT payload for Supabase Auth tokens.

    Supabase JWT tokens contain these standard claims:
        - sub: User ID (UUID)
        - email: User email
        - aud: Audience (typically "authenticated")
        - role: User role
        - iat: Issued at timestamp
        - exp: Expiration timestamp
        - iss: Issuer URL
    """

    sub: str
    email: str | None = None
    aud: str | None = None
    role: str | None = None
    iat: int
    exp: int
    iss: str | None = None

    # Optional Supabase-specific claims
    phone: str | None = None
    app_metadata: dict | None = None
    user_metadata: dict | None = None
    session_id: str | None = None


@dataclass
class User:
    """Authenticated user information."""

    user_id: str
    email: str | None = None
    is_api_key_user: bool = False
    metadata: dict | None = None

    @classmethod
    def from_jwt_payload(cls, payload: JWTPayload) -> User:
        """Create User from JWT payload.

        Args:
            payload: Decoded JWT payload

        Returns:
            User instance
        """
        return cls(
            user_id=payload.sub,
            email=payload.email,
            is_api_key_user=False,
            metadata=payload.user_metadata,
        )

    @classmethod
    def api_key_user(cls) -> User:
        """Create an API key user.

        Returns:
            User instance for API key authentication
        """
        return cls(
            user_id="api-key",
            email=None,
            is_api_key_user=True,
        )


# -----------------------------------------------------------------------------
# Token Verification
# -----------------------------------------------------------------------------


def verify_jwt_token(token: str, config: AuthConfig) -> JWTPayload:
    """Verify and decode a JWT token.

    Args:
        token: JWT token string
        config: Authentication configuration

    Returns:
        Decoded JWT payload

    Raises:
        AuthenticationError: If token is invalid, expired, or missing required claims
    """
    if not config.jwt_secret:
        raise AuthenticationError("JWT authentication not configured")

    try:
        # Decode and verify the token
        options = {"require": ["sub", "iat", "exp"]}

        # Add issuer validation if configured
        if config.jwt_issuer:
            options["require"].append("iss")

        # For Supabase tokens, the audience is "authenticated"
        # We accept this audience by default for compatibility
        decoded = jwt.decode(
            token,
            config.jwt_secret,
            algorithms=[config.jwt_algorithm],
            options=options,
            audience=["authenticated"],  # Accept Supabase's default audience
        )

        # Validate issuer if configured
        if config.jwt_issuer:
            if decoded.get("iss") != config.jwt_issuer:
                raise AuthenticationError(
                    f"Invalid issuer. Expected: {config.jwt_issuer}, got: {decoded.get('iss')}"
                )

        return JWTPayload(**decoded)

    except jwt.ExpiredSignatureError:
        raise AuthenticationError("Token has expired")
    except jwt.InvalidSignatureError:
        raise AuthenticationError("Invalid token signature")
    except jwt.InvalidAudienceError:
        raise AuthenticationError("Invalid token audience")
    except jwt.DecodeError:
        raise AuthenticationError("Invalid token format")
    except jwt.MissingRequiredClaimError as e:
        raise AuthenticationError(f"Missing required claim: {e}")
    except jwt.InvalidTokenError as e:
        raise AuthenticationError(f"Invalid token: {e}")
    except Exception as e:
        logger.exception("Unexpected error during JWT verification")
        raise AuthenticationError(f"Token verification failed: {e}")


def verify_api_key(api_key: str, config: AuthConfig) -> User:
    """Verify an API key.

    Args:
        api_key: API key string
        config: Authentication configuration

    Returns:
        User instance for API key auth

    Raises:
        AuthenticationError: If API key is invalid
    """
    if not config.api_keys:
        raise AuthenticationError("API key authentication not configured")

    if api_key not in config.api_keys:
        raise AuthenticationError("Invalid API key")

    return User.api_key_user()


# -----------------------------------------------------------------------------
# FastAPI Dependencies
# -----------------------------------------------------------------------------

# Security schemes
bearer_scheme = HTTPBearer(auto_error=False)
api_key_header = APIKeyHeader(name="X-API-Key", auto_error=False)


async def get_current_user(
    request: Request,
    bearer: HTTPAuthorizationCredentials | None = Depends(bearer_scheme),
    api_key: str | None = Depends(api_key_header),
    config: AuthConfig = Depends(get_auth_config),
) -> User:
    """Get current authenticated user.

    Supports both JWT Bearer tokens and API keys.

    Args:
        request: FastAPI request
        bearer: Bearer token from Authorization header
        api_key: API key from X-API-Key header
        config: Authentication configuration

    Returns:
        Authenticated User

    Raises:
        HTTPException: 401 if authentication fails
    """
    # Try Bearer token first
    if bearer and bearer.credentials:
        try:
            payload = verify_jwt_token(bearer.credentials, config)
            return User.from_jwt_payload(payload)
        except AuthenticationError as e:
            logger.debug(f"JWT authentication failed: {e.detail}")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail=e.detail,
                headers={"WWW-Authenticate": "Bearer"},
            )

    # Try API key
    if api_key:
        try:
            return verify_api_key(api_key, config)
        except AuthenticationError as e:
            logger.debug(f"API key authentication failed: {e.detail}")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail=e.detail,
            )

    # No credentials provided
    raise HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Authentication required. Provide Bearer token or X-API-Key header.",
        headers={"WWW-Authenticate": "Bearer"},
    )


async def get_optional_user(
    request: Request,
    bearer: HTTPAuthorizationCredentials | None = Depends(bearer_scheme),
    api_key: str | None = Depends(api_key_header),
    config: AuthConfig = Depends(get_auth_config),
) -> User | None:
    """Get current user if authenticated, otherwise None.

    Does not raise errors for missing credentials, but still validates
    provided credentials.

    Args:
        request: FastAPI request
        bearer: Bearer token from Authorization header
        api_key: API key from X-API-Key header
        config: Authentication configuration

    Returns:
        Authenticated User or None if no valid credentials
    """
    # No credentials provided
    if not bearer and not api_key:
        return None

    # Try to authenticate
    try:
        return await get_current_user(request, bearer, api_key, config)
    except HTTPException:
        # Credentials were provided but invalid - still return None for optional auth
        return None


# Alias for require_auth (same as get_current_user)
require_auth = get_current_user


# Type alias for dependency injection
CurrentUser = Annotated[User, Depends(get_current_user)]
OptionalUser = Annotated[User | None, Depends(get_optional_user)]


# -----------------------------------------------------------------------------
# Utility Functions
# -----------------------------------------------------------------------------


def clear_auth_config_cache() -> None:
    """Clear the auth config cache.

    Useful for testing or when environment variables change.
    """
    get_auth_config.cache_clear()
    logger.debug("Auth config cache cleared")
