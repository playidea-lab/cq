"""Tests for C4 API Authentication and Security.

TDD RED Phase: Define tests before implementation.
"""

from datetime import datetime, timedelta, timezone
from unittest.mock import patch

import pytest
from fastapi import Depends, FastAPI
from fastapi.testclient import TestClient

# Will be imported after implementation
# from c4.api.auth import (
#     AuthConfig,
#     JWTPayload,
#     User,
#     verify_jwt_token,
#     verify_api_key,
#     get_current_user,
#     get_optional_user,
#     require_auth,
#     AuthenticationError,
#     AuthorizationError,
# )


class TestAuthConfig:
    """Tests for AuthConfig."""

    def test_config_from_env_defaults(self):
        """Test AuthConfig uses defaults when env vars not set."""
        from c4.api.auth import AuthConfig

        with patch.dict("os.environ", {}, clear=True):
            config = AuthConfig()
            assert config.jwt_secret is None
            assert config.api_keys == []
            assert config.jwt_algorithm == "HS256"
            assert config.jwt_issuer is None

    def test_config_from_env_with_values(self):
        """Test AuthConfig reads from environment variables."""
        from c4.api.auth import AuthConfig

        env = {
            "SUPABASE_JWT_SECRET": "test-secret-key-12345",
            "C4_API_KEYS": "key1,key2,key3",
            "SUPABASE_JWT_ISSUER": "https://test.supabase.co/auth/v1",
        }
        with patch.dict("os.environ", env, clear=True):
            config = AuthConfig()
            assert config.jwt_secret == "test-secret-key-12345"
            assert config.api_keys == ["key1", "key2", "key3"]
            assert config.jwt_issuer == "https://test.supabase.co/auth/v1"

    def test_config_api_keys_strips_whitespace(self):
        """Test AuthConfig strips whitespace from API keys."""
        from c4.api.auth import AuthConfig

        env = {"C4_API_KEYS": " key1 , key2 , key3 "}
        with patch.dict("os.environ", env, clear=True):
            config = AuthConfig()
            assert config.api_keys == ["key1", "key2", "key3"]

    def test_config_empty_api_keys(self):
        """Test AuthConfig handles empty API keys string."""
        from c4.api.auth import AuthConfig

        env = {"C4_API_KEYS": ""}
        with patch.dict("os.environ", env, clear=True):
            config = AuthConfig()
            assert config.api_keys == []


class TestJWTVerification:
    """Tests for JWT token verification."""

    @pytest.fixture
    def jwt_secret(self):
        """Test JWT secret."""
        return "test-jwt-secret-key-for-testing-purposes"

    @pytest.fixture
    def valid_jwt_payload(self):
        """Create a valid JWT payload."""
        now = datetime.now(timezone.utc)
        return {
            "sub": "user-123-uuid",
            "email": "test@example.com",
            "aud": "authenticated",
            "role": "authenticated",
            "iat": int(now.timestamp()),
            "exp": int((now + timedelta(hours=1)).timestamp()),
            "iss": "https://test.supabase.co/auth/v1",
        }

    @pytest.fixture
    def create_jwt_token(self, jwt_secret):
        """Helper to create JWT tokens."""
        import jwt

        def _create(payload: dict, secret: str = None) -> str:
            return jwt.encode(payload, secret or jwt_secret, algorithm="HS256")

        return _create

    def test_verify_valid_jwt(self, jwt_secret, valid_jwt_payload, create_jwt_token):
        """Test verification of valid JWT token."""
        from c4.api.auth import AuthConfig, verify_jwt_token

        token = create_jwt_token(valid_jwt_payload)
        config = AuthConfig()
        config.jwt_secret = jwt_secret

        payload = verify_jwt_token(token, config)
        assert payload.sub == "user-123-uuid"
        assert payload.email == "test@example.com"
        assert payload.aud == "authenticated"

    def test_verify_expired_jwt(self, jwt_secret, valid_jwt_payload, create_jwt_token):
        """Test verification rejects expired JWT."""
        from c4.api.auth import AuthConfig, AuthenticationError, verify_jwt_token

        # Set expiration to past
        expired_time = datetime.now(timezone.utc) - timedelta(hours=1)
        valid_jwt_payload["exp"] = int(expired_time.timestamp())
        token = create_jwt_token(valid_jwt_payload)
        config = AuthConfig()
        config.jwt_secret = jwt_secret

        with pytest.raises(AuthenticationError) as exc_info:
            verify_jwt_token(token, config)
        assert "expired" in str(exc_info.value).lower()

    def test_verify_jwt_invalid_signature(self, jwt_secret, valid_jwt_payload, create_jwt_token):
        """Test verification rejects JWT with invalid signature."""
        from c4.api.auth import AuthConfig, AuthenticationError, verify_jwt_token

        token = create_jwt_token(valid_jwt_payload, secret="wrong-secret")
        config = AuthConfig()
        config.jwt_secret = jwt_secret

        with pytest.raises(AuthenticationError) as exc_info:
            verify_jwt_token(token, config)
        error_msg = str(exc_info.value).lower()
        assert "invalid" in error_msg or "signature" in error_msg

    def test_verify_jwt_malformed_token(self, jwt_secret):
        """Test verification rejects malformed JWT."""
        from c4.api.auth import AuthConfig, AuthenticationError, verify_jwt_token

        config = AuthConfig()
        config.jwt_secret = jwt_secret

        with pytest.raises(AuthenticationError):
            verify_jwt_token("not-a-valid-jwt", config)

    def test_verify_jwt_missing_required_claims(self, jwt_secret, create_jwt_token):
        """Test verification rejects JWT missing required claims."""
        from c4.api.auth import AuthConfig, AuthenticationError, verify_jwt_token

        payload = {
            "iat": int(datetime.now(timezone.utc).timestamp()),
            "exp": int((datetime.now(timezone.utc) + timedelta(hours=1)).timestamp()),
            # Missing 'sub' claim
        }
        token = create_jwt_token(payload)
        config = AuthConfig()
        config.jwt_secret = jwt_secret

        with pytest.raises(AuthenticationError) as exc_info:
            verify_jwt_token(token, config)
        assert "sub" in str(exc_info.value).lower() or "claim" in str(exc_info.value).lower()

    def test_verify_jwt_validates_issuer(self, jwt_secret, valid_jwt_payload, create_jwt_token):
        """Test verification validates issuer when configured."""
        from c4.api.auth import AuthConfig, AuthenticationError, verify_jwt_token

        token = create_jwt_token(valid_jwt_payload)
        config = AuthConfig()
        config.jwt_secret = jwt_secret
        config.jwt_issuer = "https://other.supabase.co/auth/v1"

        with pytest.raises(AuthenticationError) as exc_info:
            verify_jwt_token(token, config)
        assert "issuer" in str(exc_info.value).lower()


class TestAPIKeyVerification:
    """Tests for API key verification."""

    def test_verify_valid_api_key(self):
        """Test verification of valid API key."""
        from c4.api.auth import AuthConfig, verify_api_key

        config = AuthConfig()
        config.api_keys = ["valid-api-key-123", "another-key"]

        user = verify_api_key("valid-api-key-123", config)
        assert user.user_id == "api-key"
        assert user.email is None
        assert user.is_api_key_user is True

    def test_verify_invalid_api_key(self):
        """Test verification rejects invalid API key."""
        from c4.api.auth import AuthConfig, AuthenticationError, verify_api_key

        config = AuthConfig()
        config.api_keys = ["valid-api-key-123"]

        with pytest.raises(AuthenticationError):
            verify_api_key("invalid-key", config)

    def test_verify_api_key_empty_config(self):
        """Test verification rejects when no API keys configured."""
        from c4.api.auth import AuthConfig, AuthenticationError, verify_api_key

        config = AuthConfig()
        config.api_keys = []

        with pytest.raises(AuthenticationError):
            verify_api_key("any-key", config)


class TestUserModel:
    """Tests for User model."""

    def test_user_from_jwt_payload(self):
        """Test creating User from JWT payload."""
        from c4.api.auth import JWTPayload, User

        payload = JWTPayload(
            sub="user-123",
            email="test@example.com",
            aud="authenticated",
            role="authenticated",
            iat=int(datetime.now(timezone.utc).timestamp()),
            exp=int((datetime.now(timezone.utc) + timedelta(hours=1)).timestamp()),
        )

        user = User.from_jwt_payload(payload)
        assert user.user_id == "user-123"
        assert user.email == "test@example.com"
        assert user.is_api_key_user is False

    def test_user_api_key_user(self):
        """Test creating API key user."""
        from c4.api.auth import User

        user = User.api_key_user()
        assert user.user_id == "api-key"
        assert user.email is None
        assert user.is_api_key_user is True


class TestAuthDependencies:
    """Integration tests for FastAPI auth dependencies."""

    @pytest.fixture
    def auth_config(self):
        """Test auth configuration."""
        from c4.api.auth import AuthConfig

        config = AuthConfig()
        config.jwt_secret = "test-jwt-secret-key"
        config.api_keys = ["test-api-key-123"]
        return config

    @pytest.fixture
    def app_with_auth(self, auth_config):
        """Create FastAPI app with auth dependencies."""
        from c4.api.auth import (
            User,
            get_auth_config,
            get_current_user,
            get_optional_user,
            require_auth,
        )

        app = FastAPI()

        # Override the dependency at the app level
        app.dependency_overrides[get_auth_config] = lambda: auth_config

        @app.get("/protected")
        async def protected_route(user: User = Depends(get_current_user)):
            return {"user_id": user.user_id, "email": user.email}

        @app.get("/optional")
        async def optional_auth_route(user: User | None = Depends(get_optional_user)):
            if user:
                return {"authenticated": True, "user_id": user.user_id}
            return {"authenticated": False}

        @app.get("/api-key-only")
        async def api_key_route(user: User = Depends(require_auth)):
            return {"user_id": user.user_id}

        yield app

        # Cleanup
        app.dependency_overrides.clear()

    @pytest.fixture
    def client(self, app_with_auth):
        """Create test client."""
        return TestClient(app_with_auth)

    @pytest.fixture
    def valid_token(self, auth_config):
        """Create a valid JWT token."""
        import jwt

        now = datetime.now(timezone.utc)
        payload = {
            "sub": "user-456",
            "email": "auth@example.com",
            "aud": "authenticated",
            "role": "authenticated",
            "iat": int(now.timestamp()),
            "exp": int((now + timedelta(hours=1)).timestamp()),
        }
        return jwt.encode(payload, auth_config.jwt_secret, algorithm="HS256")

    def test_protected_route_with_bearer_token(self, client, valid_token):
        """Test protected route accepts valid Bearer token."""
        response = client.get(
            "/protected",
            headers={"Authorization": f"Bearer {valid_token}"},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["user_id"] == "user-456"
        assert data["email"] == "auth@example.com"

    def test_protected_route_with_api_key(self, client):
        """Test protected route accepts valid API key."""
        response = client.get(
            "/protected",
            headers={"X-API-Key": "test-api-key-123"},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["user_id"] == "api-key"

    def test_protected_route_no_auth(self, client):
        """Test protected route rejects requests without auth."""
        response = client.get("/protected")
        assert response.status_code == 401
        data = response.json()
        assert "detail" in data

    def test_protected_route_invalid_token(self, client):
        """Test protected route rejects invalid token."""
        response = client.get(
            "/protected",
            headers={"Authorization": "Bearer invalid-token"},
        )
        assert response.status_code == 401

    def test_protected_route_invalid_api_key(self, client):
        """Test protected route rejects invalid API key."""
        response = client.get(
            "/protected",
            headers={"X-API-Key": "invalid-key"},
        )
        assert response.status_code == 401

    def test_optional_auth_with_token(self, client, valid_token):
        """Test optional auth route with valid token."""
        response = client.get(
            "/optional",
            headers={"Authorization": f"Bearer {valid_token}"},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["authenticated"] is True
        assert data["user_id"] == "user-456"

    def test_optional_auth_without_token(self, client):
        """Test optional auth route without token."""
        response = client.get("/optional")
        assert response.status_code == 200
        data = response.json()
        assert data["authenticated"] is False

    def test_bearer_token_wrong_format(self, client):
        """Test handling of malformed Authorization header."""
        response = client.get(
            "/protected",
            headers={"Authorization": "InvalidFormat token123"},
        )
        assert response.status_code == 401


class TestAuthErrors:
    """Tests for authentication error handling."""

    def test_authentication_error_message(self):
        """Test AuthenticationError has proper message."""
        from c4.api.auth import AuthenticationError

        error = AuthenticationError("Token expired")
        assert str(error) == "Token expired"
        assert error.detail == "Token expired"

    def test_authorization_error_message(self):
        """Test AuthorizationError has proper message."""
        from c4.api.auth import AuthorizationError

        error = AuthorizationError("Insufficient permissions")
        assert str(error) == "Insufficient permissions"
        assert error.detail == "Insufficient permissions"


class TestSupabaseJWTFormat:
    """Tests specific to Supabase JWT format."""

    @pytest.fixture
    def supabase_jwt_payload(self):
        """Create a Supabase-format JWT payload."""
        now = datetime.now(timezone.utc)
        return {
            "aud": "authenticated",
            "exp": int((now + timedelta(hours=1)).timestamp()),
            "iat": int(now.timestamp()),
            "iss": "https://myproject.supabase.co/auth/v1",
            "sub": "12345678-1234-1234-1234-123456789012",
            "email": "user@example.com",
            "phone": "",
            "app_metadata": {"provider": "email", "providers": ["email"]},
            "user_metadata": {"name": "Test User"},
            "role": "authenticated",
            "aal": "aal1",
            "amr": [{"method": "password", "timestamp": int(now.timestamp())}],
            "session_id": "session-uuid",
        }

    def test_parse_full_supabase_jwt(self, supabase_jwt_payload):
        """Test parsing of full Supabase JWT format."""
        import jwt

        from c4.api.auth import AuthConfig, verify_jwt_token

        secret = "supabase-test-secret"
        token = jwt.encode(supabase_jwt_payload, secret, algorithm="HS256")

        config = AuthConfig()
        config.jwt_secret = secret

        payload = verify_jwt_token(token, config)
        assert payload.sub == "12345678-1234-1234-1234-123456789012"
        assert payload.email == "user@example.com"
        assert payload.aud == "authenticated"
        assert payload.role == "authenticated"

    def test_supabase_jwt_with_custom_claims(self, supabase_jwt_payload):
        """Test Supabase JWT with custom user metadata."""
        import jwt

        from c4.api.auth import AuthConfig, verify_jwt_token

        supabase_jwt_payload["user_metadata"] = {
            "name": "Test User",
            "avatar_url": "https://example.com/avatar.png",
        }

        secret = "supabase-test-secret"
        token = jwt.encode(supabase_jwt_payload, secret, algorithm="HS256")

        config = AuthConfig()
        config.jwt_secret = secret

        payload = verify_jwt_token(token, config)
        assert payload.sub == supabase_jwt_payload["sub"]
