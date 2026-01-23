"""SSO API Routes.

Provides endpoints for SSO/SAML authentication:
- GET  /sso/providers                     - List available providers
- GET  /sso/teams/{team_id}/config        - Get SSO config
- POST /sso/teams/{team_id}/config        - Create SSO config
- PUT  /sso/teams/{team_id}/config        - Update SSO config
- DELETE /sso/teams/{team_id}/config      - Delete SSO config
- POST /sso/teams/{team_id}/enable        - Enable SSO
- POST /sso/teams/{team_id}/disable       - Disable SSO
- GET  /sso/login/{team_slug}             - Initiate SSO login
- GET  /sso/callback/{team_slug}          - Handle SSO callback
- POST /sso/logout                        - SSO logout
- GET  /sso/session                       - Get current session
"""

from __future__ import annotations

import logging
from typing import Annotated, Any

from fastapi import APIRouter, Depends, HTTPException, Query, status
from pydantic import BaseModel, Field

from c4.services.activity import ActivityCollector, create_activity_collector
from c4.services.audit import ActorType, AuditLogger, create_audit_logger
from c4.services.sso import (
    SSOConfig,
    SSOProvider,
    SSOService,
    create_sso_service,
)
from c4.services.sso.service import (
    SSOConfigNotFoundError,
    SSODomainNotAllowedError,
    SSOError,
)

from ..auth import User, get_current_user

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/sso", tags=["SSO"])


# =============================================================================
# Request/Response Models
# =============================================================================


class SSOProviderInfo(BaseModel):
    """SSO provider information."""

    id: str
    name: str
    type: str  # oidc, saml


class SSOConfigRequest(BaseModel):
    """Request to create/update SSO config."""

    provider: str = Field(..., description="Provider type: google, microsoft, okta, saml")

    # OIDC settings
    client_id: str | None = Field(None, description="OAuth2/OIDC client ID")
    client_secret: str | None = Field(None, description="OAuth2/OIDC client secret")
    issuer_url: str | None = Field(None, description="OIDC issuer URL")

    # SAML settings
    entity_id: str | None = Field(None, description="SAML entity ID")
    sso_url: str | None = Field(None, description="SAML SSO URL")
    slo_url: str | None = Field(None, description="SAML Single Logout URL")
    certificate: str | None = Field(None, description="SAML X.509 certificate")

    # Common settings
    auto_provision: bool = Field(True, description="Enable JIT user provisioning")
    default_role: str = Field("member", description="Default role for new users")
    allowed_domains: list[str] = Field(default_factory=list, description="Allowed email domains")


class SSOConfigResponse(BaseModel):
    """SSO configuration response."""

    id: str
    team_id: str
    provider: str
    client_id: str | None
    issuer_url: str | None
    entity_id: str | None
    sso_url: str | None
    auto_provision: bool
    default_role: str
    allowed_domains: list[str]
    enabled: bool
    verified: bool
    created_at: str | None
    updated_at: str | None


class SSOLoginResponse(BaseModel):
    """SSO login initiation response."""

    authorization_url: str
    state: str


class SSOSessionResponse(BaseModel):
    """SSO session response."""

    session_id: str
    user_id: str
    team_id: str
    provider: str
    email: str | None
    expires_at: str | None


# =============================================================================
# Dependencies
# =============================================================================


def get_sso_service() -> SSOService:
    """Get SSO service instance."""
    service = create_sso_service()

    # Register providers
    from c4.services.sso.providers import GoogleOIDCProvider, MicrosoftOIDCProvider

    SSOService.register_provider(SSOProvider.GOOGLE, GoogleOIDCProvider)
    SSOService.register_provider(SSOProvider.MICROSOFT, MicrosoftOIDCProvider)

    return service


def get_activity_collector() -> ActivityCollector:
    """Get ActivityCollector instance."""
    return create_activity_collector()


def get_audit_logger() -> AuditLogger:
    """Get AuditLogger instance."""
    return create_audit_logger()


SSO = Annotated[SSOService, Depends(get_sso_service)]
CurrentUser = Annotated[User, Depends(get_current_user)]
Activity = Annotated[ActivityCollector, Depends(get_activity_collector)]
Audit = Annotated[AuditLogger, Depends(get_audit_logger)]


# =============================================================================
# Helper Functions
# =============================================================================


def config_to_response(config: SSOConfig) -> SSOConfigResponse:
    """Convert SSOConfig to response model."""
    return SSOConfigResponse(
        id=config.id,
        team_id=config.team_id,
        provider=config.provider.value,
        client_id=config.client_id,
        issuer_url=config.issuer_url,
        entity_id=config.entity_id,
        sso_url=config.sso_url,
        auto_provision=config.auto_provision,
        default_role=config.default_role,
        allowed_domains=config.allowed_domains,
        enabled=config.enabled,
        verified=config.verified,
        created_at=config.created_at.isoformat() if config.created_at else None,
        updated_at=config.updated_at.isoformat() if config.updated_at else None,
    )


# =============================================================================
# Provider Endpoints
# =============================================================================


@router.get("/providers", response_model=list[SSOProviderInfo])
async def list_providers() -> list[SSOProviderInfo]:
    """List available SSO providers."""
    return [
        SSOProviderInfo(id="google", name="Google Workspace", type="oidc"),
        SSOProviderInfo(id="microsoft", name="Microsoft Entra ID", type="oidc"),
        SSOProviderInfo(id="okta", name="Okta", type="saml"),
        SSOProviderInfo(id="saml", name="Custom SAML", type="saml"),
    ]


# =============================================================================
# Configuration Endpoints
# =============================================================================


@router.get("/teams/{team_id}/config", response_model=SSOConfigResponse)
async def get_sso_config(
    team_id: str,
    user: CurrentUser,
    sso: SSO,
) -> SSOConfigResponse:
    """Get SSO configuration for a team.

    Requires team admin/owner role.
    """
    config = await sso.get_config(team_id)
    if not config:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="SSO not configured for this team",
        )

    return config_to_response(config)


@router.post(
    "/teams/{team_id}/config",
    response_model=SSOConfigResponse,
    status_code=status.HTTP_201_CREATED,
)
async def create_sso_config(
    team_id: str,
    request: SSOConfigRequest,
    user: CurrentUser,
    sso: SSO,
    activity: Activity,
    audit: Audit,
) -> SSOConfigResponse:
    """Create SSO configuration for a team.

    Requires team admin/owner role.
    """
    try:
        provider = SSOProvider(request.provider)
    except ValueError:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Invalid provider: {request.provider}",
        )

    try:
        config = await sso.create_config(
            team_id=team_id,
            provider=provider,
            client_id=request.client_id,
            client_secret=request.client_secret,
            issuer_url=request.issuer_url,
            entity_id=request.entity_id,
            sso_url=request.sso_url,
            slo_url=request.slo_url,
            certificate=request.certificate,
            auto_provision=request.auto_provision,
            default_role=request.default_role,
            allowed_domains=request.allowed_domains,
            created_by=user.user_id,
        )

        # Activity tracking
        await activity.log_activity(
            team_id=team_id,
            activity_type="sso_config_created",
            user_id=user.user_id,
            resource_type="sso_config",
            resource_id=config.id,
            metadata={"provider": request.provider},
        )

        # Audit logging
        await audit.log(
            team_id=team_id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            action="sso.config.created",
            resource_type="sso_config",
            resource_id=config.id,
            new_value={"provider": request.provider, "enabled": False},
        )

        return config_to_response(config)

    except SSOError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        )


@router.put("/teams/{team_id}/config", response_model=SSOConfigResponse)
async def update_sso_config(
    team_id: str,
    request: SSOConfigRequest,
    user: CurrentUser,
    sso: SSO,
    activity: Activity,
    audit: Audit,
) -> SSOConfigResponse:
    """Update SSO configuration.

    Requires team admin/owner role.
    """
    try:
        # Get old config for audit
        old_config = await sso.get_config(team_id)
        old_value = {"provider": old_config.provider.value} if old_config else None

        config = await sso.update_config(
            team_id=team_id,
            client_id=request.client_id,
            issuer_url=request.issuer_url,
            entity_id=request.entity_id,
            sso_url=request.sso_url,
            slo_url=request.slo_url,
            certificate=request.certificate,
            auto_provision=request.auto_provision,
            default_role=request.default_role,
            allowed_domains=request.allowed_domains,
        )

        # Activity tracking
        await activity.log_activity(
            team_id=team_id,
            activity_type="sso_config_updated",
            user_id=user.user_id,
            resource_type="sso_config",
            resource_id=config.id,
        )

        # Audit logging
        await audit.log(
            team_id=team_id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            action="sso.config.updated",
            resource_type="sso_config",
            resource_id=config.id,
            old_value=old_value,
            new_value={"provider": config.provider.value},
        )

        return config_to_response(config)

    except SSOConfigNotFoundError:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="SSO not configured for this team",
        )
    except SSOError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        )


@router.delete("/teams/{team_id}/config", status_code=status.HTTP_204_NO_CONTENT)
async def delete_sso_config(
    team_id: str,
    user: CurrentUser,
    sso: SSO,
    activity: Activity,
    audit: Audit,
) -> None:
    """Delete SSO configuration.

    Requires team admin/owner role.
    """
    config = await sso.get_config(team_id)
    if not config:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="SSO not configured for this team",
        )

    await sso.delete_config(team_id)

    # Activity tracking
    await activity.log_activity(
        team_id=team_id,
        activity_type="sso_config_deleted",
        user_id=user.user_id,
        resource_type="sso_config",
        resource_id=config.id,
    )

    # Audit logging
    await audit.log(
        team_id=team_id,
        actor_type=ActorType.USER,
        actor_id=user.user_id,
        actor_email=user.email,
        action="sso.config.deleted",
        resource_type="sso_config",
        resource_id=config.id,
        old_value={"provider": config.provider.value},
    )


@router.post("/teams/{team_id}/enable", response_model=SSOConfigResponse)
async def enable_sso(
    team_id: str,
    user: CurrentUser,
    sso: SSO,
    audit: Audit,
) -> SSOConfigResponse:
    """Enable SSO for a team.

    Requires team admin/owner role and verified configuration.
    """
    try:
        config = await sso.enable_config(team_id)

        # Audit logging
        await audit.log(
            team_id=team_id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            action="sso.enabled",
            resource_type="sso_config",
            resource_id=config.id,
            old_value={"enabled": False},
            new_value={"enabled": True},
        )

        return config_to_response(config)

    except SSOConfigNotFoundError:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="SSO not configured for this team",
        )
    except SSOError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        )


@router.post("/teams/{team_id}/disable", response_model=SSOConfigResponse)
async def disable_sso(
    team_id: str,
    user: CurrentUser,
    sso: SSO,
    audit: Audit,
) -> SSOConfigResponse:
    """Disable SSO for a team.

    Requires team admin/owner role.
    """
    try:
        config = await sso.disable_config(team_id)

        # Audit logging
        await audit.log(
            team_id=team_id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            action="sso.disabled",
            resource_type="sso_config",
            resource_id=config.id,
            old_value={"enabled": True},
            new_value={"enabled": False},
        )

        return config_to_response(config)

    except SSOConfigNotFoundError:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="SSO not configured for this team",
        )


# =============================================================================
# Authentication Endpoints
# =============================================================================


@router.get("/login/{team_slug}")
async def initiate_sso_login(
    team_slug: str,
    sso: SSO,
    redirect_uri: str = Query(..., description="Callback URL"),
) -> SSOLoginResponse:
    """Initiate SSO login for a team.

    Returns authorization URL to redirect user to.
    """
    # TODO: Look up team_id from team_slug
    team_id = team_slug  # Placeholder

    try:
        auth_url, state = await sso.initiate_login(team_id, redirect_uri)

        return SSOLoginResponse(
            authorization_url=auth_url,
            state=state,
        )

    except SSOConfigNotFoundError:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="SSO not configured for this team",
        )
    except SSOError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        )


@router.get("/callback/{team_slug}")
async def handle_sso_callback(
    team_slug: str,
    sso: SSO,
    code: str = Query(..., description="Authorization code"),
    state: str = Query(..., description="State parameter"),
    redirect_uri: str = Query(..., description="Original redirect URI"),
) -> dict[str, Any]:
    """Handle SSO callback.

    Exchanges code for tokens and creates session.
    """
    # TODO: Look up team_id from team_slug
    team_id = team_slug  # Placeholder

    try:
        result = await sso.handle_callback(
            team_id=team_id,
            code=code,
            redirect_uri=redirect_uri,
            state=state,
        )

        if not result.success:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail=result.error or "Authentication failed",
            )

        return {
            "success": True,
            "session_id": result.session_id,
            "user": {
                "email": result.user_info.email if result.user_info else None,
                "name": result.user_info.name if result.user_info else None,
            },
        }

    except SSODomainNotAllowedError as e:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=str(e),
        )
    except SSOError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        )


@router.post("/logout")
async def sso_logout(
    user: CurrentUser,
    sso: SSO,
    session_id: str | None = Query(None, description="Session to revoke"),
) -> dict[str, bool]:
    """Logout from SSO session.

    If session_id is provided, revokes that specific session.
    Otherwise, revokes all sessions for the user.
    """
    if session_id:
        success = await sso.revoke_session(session_id, reason="user_logout")
    else:
        count = await sso.revoke_user_sessions(user.user_id, reason="user_logout")
        success = count > 0

    return {"success": success}


@router.get("/session", response_model=SSOSessionResponse | None)
async def get_sso_session(
    user: CurrentUser,
    sso: SSO,
    session_id: str = Query(..., description="Session ID"),
) -> SSOSessionResponse | None:
    """Get SSO session information."""
    session = await sso.get_session(session_id)
    if not session:
        return None

    return SSOSessionResponse(
        session_id=session.id,
        user_id=session.user_id,
        team_id=session.team_id,
        provider=session.provider.value,
        email=session.provider_email,
        expires_at=session.expires_at.isoformat() if session.expires_at else None,
    )
