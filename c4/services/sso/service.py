"""SSO Service.

High-level service for managing SSO authentication.
"""

from __future__ import annotations

import hashlib
import logging
import secrets
from datetime import UTC, datetime, timedelta
from typing import TYPE_CHECKING, Any

from c4.services.sso.models import (
    SSOAuthResult,
    SSOConfig,
    SSODomainVerification,
    SSOProvider,
    SSOSession,
    SSOUserInfo,
)

if TYPE_CHECKING:
    from c4.services.sso.base import SSOProviderBase

logger = logging.getLogger(__name__)


class SSOError(Exception):
    """Base exception for SSO errors."""

    def __init__(self, message: str, code: str = "sso_error") -> None:
        super().__init__(message)
        self.code = code


class SSOConfigNotFoundError(SSOError):
    """SSO configuration not found."""

    def __init__(self, team_id: str) -> None:
        super().__init__(f"SSO not configured for team {team_id}", "config_not_found")


class SSOProviderNotSupportedError(SSOError):
    """SSO provider not supported."""

    def __init__(self, provider: str) -> None:
        super().__init__(f"SSO provider not supported: {provider}", "provider_not_supported")


class SSODomainNotAllowedError(SSOError):
    """Email domain not allowed."""

    def __init__(self, email: str) -> None:
        domain = email.split("@")[-1]
        super().__init__(f"Email domain not allowed: {domain}", "domain_not_allowed")


class SSOService:
    """Service for managing SSO authentication.

    Handles:
    - SSO configuration management
    - Authentication flow orchestration
    - Session management
    - Domain verification
    """

    # Provider registry
    _providers: dict[SSOProvider, type[SSOProviderBase]] = {}

    def __init__(
        self,
        *,
        session_duration_hours: int = 24,
        max_sessions_per_user: int = 10,
    ) -> None:
        """Initialize SSO service.

        Args:
            session_duration_hours: SSO session duration in hours
            max_sessions_per_user: Maximum concurrent sessions per user
        """
        self._session_duration = timedelta(hours=session_duration_hours)
        self._max_sessions = max_sessions_per_user

        # In-memory storage (replace with database in production)
        self._configs: dict[str, SSOConfig] = {}  # team_id -> config
        self._sessions: dict[str, SSOSession] = {}  # session_id -> session
        self._domain_verifications: dict[str, SSODomainVerification] = {}

    @classmethod
    def register_provider(
        cls,
        provider_type: SSOProvider,
        provider_class: type[SSOProviderBase],
    ) -> None:
        """Register an SSO provider.

        Args:
            provider_type: Provider type to register
            provider_class: Provider implementation class
        """
        cls._providers[provider_type] = provider_class
        logger.info(f"Registered SSO provider: {provider_type}")

    def get_provider(self, provider_type: SSOProvider) -> SSOProviderBase:
        """Get SSO provider instance.

        Args:
            provider_type: Provider type

        Returns:
            Provider instance

        Raises:
            SSOProviderNotSupportedError: If provider not registered
        """
        if provider_type not in self._providers:
            raise SSOProviderNotSupportedError(provider_type.value)

        return self._providers[provider_type]()

    # =========================================================================
    # Configuration Management
    # =========================================================================

    async def get_config(self, team_id: str) -> SSOConfig | None:
        """Get SSO configuration for a team.

        Args:
            team_id: Team identifier

        Returns:
            SSO configuration or None if not configured
        """
        return self._configs.get(team_id)

    async def create_config(
        self,
        team_id: str,
        provider: SSOProvider,
        *,
        client_id: str | None = None,
        client_secret: str | None = None,
        issuer_url: str | None = None,
        entity_id: str | None = None,
        sso_url: str | None = None,
        slo_url: str | None = None,
        certificate: str | None = None,
        auto_provision: bool = True,
        default_role: str = "member",
        allowed_domains: list[str] | None = None,
        created_by: str | None = None,
    ) -> SSOConfig:
        """Create SSO configuration for a team.

        Args:
            team_id: Team identifier
            provider: SSO provider type
            client_id: OIDC client ID
            client_secret: OIDC client secret (will be encrypted)
            issuer_url: OIDC issuer URL
            entity_id: SAML entity ID
            sso_url: SAML SSO URL
            slo_url: SAML SLO URL
            certificate: SAML certificate
            auto_provision: Enable JIT provisioning
            default_role: Default role for new users
            allowed_domains: Allowed email domains
            created_by: User ID who created config

        Returns:
            Created SSO configuration
        """
        config_id = secrets.token_urlsafe(16)
        now = datetime.now(UTC)

        config = SSOConfig(
            id=config_id,
            team_id=team_id,
            provider=provider,
            client_id=client_id,
            issuer_url=issuer_url,
            entity_id=entity_id,
            sso_url=sso_url,
            slo_url=slo_url,
            certificate=certificate,
            auto_provision=auto_provision,
            default_role=default_role,
            allowed_domains=allowed_domains or [],
            enabled=False,  # Disabled until verified
            verified=False,
            created_at=now,
            updated_at=now,
            created_by=created_by,
        )

        # Validate configuration
        provider_instance = self.get_provider(provider)
        errors = provider_instance.validate_config(config)
        if errors:
            raise SSOError(f"Invalid configuration: {', '.join(errors)}", "invalid_config")

        self._configs[team_id] = config
        logger.info(f"Created SSO config for team {team_id}: provider={provider}")

        return config

    async def update_config(
        self,
        team_id: str,
        **updates: Any,
    ) -> SSOConfig:
        """Update SSO configuration.

        Args:
            team_id: Team identifier
            **updates: Fields to update

        Returns:
            Updated configuration

        Raises:
            SSOConfigNotFoundError: If config not found
        """
        config = await self.get_config(team_id)
        if not config:
            raise SSOConfigNotFoundError(team_id)

        # Apply updates
        for key, value in updates.items():
            if hasattr(config, key):
                setattr(config, key, value)

        config.updated_at = datetime.now(UTC)

        # Re-validate if provider-specific fields changed
        provider_instance = self.get_provider(config.provider)
        errors = provider_instance.validate_config(config)
        if errors:
            raise SSOError(f"Invalid configuration: {', '.join(errors)}", "invalid_config")

        self._configs[team_id] = config
        logger.info(f"Updated SSO config for team {team_id}")

        return config

    async def delete_config(self, team_id: str) -> bool:
        """Delete SSO configuration.

        Args:
            team_id: Team identifier

        Returns:
            True if deleted
        """
        if team_id in self._configs:
            del self._configs[team_id]
            logger.info(f"Deleted SSO config for team {team_id}")
            return True
        return False

    async def enable_config(self, team_id: str) -> SSOConfig:
        """Enable SSO for a team.

        Args:
            team_id: Team identifier

        Returns:
            Updated configuration

        Raises:
            SSOConfigNotFoundError: If config not found
            SSOError: If config not verified
        """
        config = await self.get_config(team_id)
        if not config:
            raise SSOConfigNotFoundError(team_id)

        if not config.verified:
            raise SSOError("SSO configuration must be verified before enabling", "not_verified")

        config.enabled = True
        config.updated_at = datetime.now(UTC)
        self._configs[team_id] = config

        logger.info(f"Enabled SSO for team {team_id}")
        return config

    async def disable_config(self, team_id: str) -> SSOConfig:
        """Disable SSO for a team.

        Args:
            team_id: Team identifier

        Returns:
            Updated configuration
        """
        config = await self.get_config(team_id)
        if not config:
            raise SSOConfigNotFoundError(team_id)

        config.enabled = False
        config.updated_at = datetime.now(UTC)
        self._configs[team_id] = config

        logger.info(f"Disabled SSO for team {team_id}")
        return config

    # =========================================================================
    # Authentication Flow
    # =========================================================================

    async def initiate_login(
        self,
        team_id: str,
        redirect_uri: str,
    ) -> tuple[str, str]:
        """Initiate SSO login flow.

        Args:
            team_id: Team identifier
            redirect_uri: Callback URL

        Returns:
            Tuple of (authorization_url, state)

        Raises:
            SSOConfigNotFoundError: If SSO not configured
            SSOError: If SSO disabled
        """
        config = await self.get_config(team_id)
        if not config:
            raise SSOConfigNotFoundError(team_id)

        if not config.enabled:
            raise SSOError("SSO is not enabled for this team", "sso_disabled")

        # Generate state and nonce
        state = secrets.token_urlsafe(32)
        nonce = secrets.token_urlsafe(32)

        # Get provider and generate auth URL
        provider = self.get_provider(config.provider)
        auth_url = provider.get_authorization_url(
            config=config,
            redirect_uri=redirect_uri,
            state=state,
            nonce=nonce,
        )

        logger.info(f"Initiated SSO login for team {team_id}")
        return auth_url, state

    async def handle_callback(
        self,
        team_id: str,
        code: str,
        redirect_uri: str,
        *,
        state: str | None = None,
        nonce: str | None = None,
    ) -> SSOAuthResult:
        """Handle SSO callback.

        Args:
            team_id: Team identifier
            code: Authorization code
            redirect_uri: Callback URL
            state: State parameter (for validation)
            nonce: Nonce parameter (for validation)

        Returns:
            Authentication result
        """
        config = await self.get_config(team_id)
        if not config:
            raise SSOConfigNotFoundError(team_id)

        # Exchange code for tokens
        provider = self.get_provider(config.provider)
        result = await provider.exchange_code(
            config=config,
            code=code,
            redirect_uri=redirect_uri,
            nonce=nonce,
        )

        if not result.success or not result.user_info:
            return result

        # Check domain restrictions
        if not provider.is_domain_allowed(config, result.user_info.email):
            raise SSODomainNotAllowedError(result.user_info.email)

        # Create session
        session = await self._create_session(
            team_id=team_id,
            provider=config.provider,
            user_info=result.user_info,
        )

        result.session_id = session.id
        logger.info(
            f"SSO callback successful for team {team_id}: "
            f"user={result.user_info.email}"
        )

        return result

    async def handle_saml_response(
        self,
        team_id: str,
        saml_response: str,
        *,
        relay_state: str | None = None,
    ) -> SSOAuthResult:
        """Handle SAML response.

        Args:
            team_id: Team identifier
            saml_response: Base64-encoded SAML response
            relay_state: Relay state

        Returns:
            Authentication result
        """
        from c4.services.sso.base import SAMLProviderBase

        config = await self.get_config(team_id)
        if not config:
            raise SSOConfigNotFoundError(team_id)

        provider = self.get_provider(config.provider)
        if not isinstance(provider, SAMLProviderBase):
            raise SSOError("Provider does not support SAML", "not_saml")

        result = await provider.parse_saml_response(
            config=config,
            saml_response=saml_response,
            relay_state=relay_state,
        )

        if not result.success or not result.user_info:
            return result

        # Check domain restrictions
        if not provider.is_domain_allowed(config, result.user_info.email):
            raise SSODomainNotAllowedError(result.user_info.email)

        # Create session
        session = await self._create_session(
            team_id=team_id,
            provider=config.provider,
            user_info=result.user_info,
        )

        result.session_id = session.id
        return result

    # =========================================================================
    # Session Management
    # =========================================================================

    async def _create_session(
        self,
        team_id: str,
        provider: SSOProvider,
        user_info: SSOUserInfo,
    ) -> SSOSession:
        """Create SSO session.

        Args:
            team_id: Team identifier
            provider: SSO provider
            user_info: User information

        Returns:
            Created session
        """
        session_id = secrets.token_urlsafe(32)
        now = datetime.now(UTC)

        session = SSOSession(
            id=session_id,
            user_id=user_info.provider_user_id,  # Will be mapped to actual user
            team_id=team_id,
            provider=provider,
            provider_user_id=user_info.provider_user_id,
            provider_email=user_info.email,
            authenticated_at=now,
            expires_at=now + self._session_duration,
            last_activity_at=now,
            created_at=now,
        )

        self._sessions[session_id] = session
        return session

    async def get_session(self, session_id: str) -> SSOSession | None:
        """Get SSO session.

        Args:
            session_id: Session identifier

        Returns:
            Session or None if not found/expired
        """
        session = self._sessions.get(session_id)
        if not session:
            return None

        # Check expiry
        if session.expires_at and session.expires_at < datetime.now(UTC):
            await self.revoke_session(session_id, reason="expired")
            return None

        if session.revoked:
            return None

        return session

    async def revoke_session(
        self,
        session_id: str,
        *,
        reason: str | None = None,
    ) -> bool:
        """Revoke SSO session.

        Args:
            session_id: Session identifier
            reason: Revocation reason

        Returns:
            True if revoked
        """
        session = self._sessions.get(session_id)
        if not session:
            return False

        session.revoked = True
        session.revoked_at = datetime.now(UTC)
        session.revoked_reason = reason

        logger.info(f"Revoked SSO session {session_id}: reason={reason}")
        return True

    async def revoke_user_sessions(
        self,
        user_id: str,
        *,
        team_id: str | None = None,
        reason: str | None = None,
    ) -> int:
        """Revoke all sessions for a user.

        Args:
            user_id: User identifier
            team_id: Optionally limit to specific team
            reason: Revocation reason

        Returns:
            Number of sessions revoked
        """
        count = 0
        for session in self._sessions.values():
            if session.user_id == user_id and not session.revoked:
                if team_id and session.team_id != team_id:
                    continue
                session.revoked = True
                session.revoked_at = datetime.now(UTC)
                session.revoked_reason = reason
                count += 1

        logger.info(f"Revoked {count} sessions for user {user_id}")
        return count

    # =========================================================================
    # Domain Verification
    # =========================================================================

    async def create_domain_verification(
        self,
        team_id: str,
        domain: str,
        method: str = "dns_txt",
    ) -> SSODomainVerification:
        """Create domain verification.

        Args:
            team_id: Team identifier
            domain: Domain to verify
            method: Verification method (dns_txt, dns_cname, meta_tag)

        Returns:
            Domain verification record
        """
        verification_id = secrets.token_urlsafe(16)
        token = f"c4-verify={secrets.token_urlsafe(32)}"
        now = datetime.now(UTC)

        verification = SSODomainVerification(
            id=verification_id,
            team_id=team_id,
            domain=domain.lower(),
            verification_method=method,
            verification_token=token,
            created_at=now,
            expires_at=now + timedelta(days=7),
        )

        key = f"{team_id}:{domain}"
        self._domain_verifications[key] = verification

        logger.info(f"Created domain verification for {domain} (team {team_id})")
        return verification

    async def verify_domain(
        self,
        team_id: str,
        domain: str,
    ) -> bool:
        """Verify domain ownership via DNS TXT record.

        Checks for a DNS TXT record at _c4-verify.{domain} containing
        the verification token.

        Args:
            team_id: Team identifier
            domain: Domain to verify

        Returns:
            True if verified
        """
        from c4.services.dns import verify_dns_txt_record

        key = f"{team_id}:{domain}"
        verification = self._domain_verifications.get(key)
        if not verification:
            return False

        # Verify DNS TXT record
        dns_verified = await verify_dns_txt_record(
            domain=domain,
            expected_token=verification.token,
        )

        if not dns_verified:
            logger.warning(f"DNS verification failed for domain {domain}")
            return False

        # Mark as verified
        verification.verified = True
        verification.verified_at = datetime.now(UTC)

        logger.info(f"Verified domain {domain} for team {team_id}")
        return True

    # =========================================================================
    # Utility Methods
    # =========================================================================

    @staticmethod
    def hash_token(token: str) -> str:
        """Hash a token for storage.

        Args:
            token: Token to hash

        Returns:
            SHA256 hash of token
        """
        return hashlib.sha256(token.encode()).hexdigest()


def create_sso_service(
    *,
    session_duration_hours: int = 24,
    max_sessions_per_user: int = 10,
) -> SSOService:
    """Create SSO service instance.

    Args:
        session_duration_hours: SSO session duration
        max_sessions_per_user: Max sessions per user

    Returns:
        SSO service instance
    """
    return SSOService(
        session_duration_hours=session_duration_hours,
        max_sessions_per_user=max_sessions_per_user,
    )
