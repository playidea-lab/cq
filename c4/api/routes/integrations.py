"""Integration Management API Routes.

Provides endpoints for managing external service integrations:
- GET /integrations/providers - List available integration providers
- GET /teams/{team_id}/integrations - List team's connected integrations
- GET /integrations/{provider_id}/oauth/url - Get OAuth authorization URL
- GET /integrations/{provider_id}/oauth/callback - Handle OAuth callback
- PATCH /teams/{team_id}/integrations/{integration_id} - Update integration settings
- DELETE /teams/{team_id}/integrations/{integration_id} - Disconnect integration

Security:
- All endpoints require authentication (JWT or API key)
- Team-level operations require admin/owner role
"""

from __future__ import annotations

import base64
import json
import logging
import os
import secrets
from typing import Annotated, Any

from fastapi import APIRouter, Depends, HTTPException, Query, status
from fastapi.responses import RedirectResponse

from c4.integrations.registry import IntegrationRegistry, auto_discover_providers
from c4.services.activity import ActivityCollector, create_activity_collector
from c4.services.audit import (
    ActorType,
    AuditAction,
    AuditLogger,
    create_audit_logger,
)
from c4.services.integrations import (
    Integration,
    IntegrationService,
    create_integration_service,
)
from c4.services.teams import TeamService, create_team_service

from ..auth import User, get_current_user
from ..models import (
    IntegrationProviderResponse,
    IntegrationResponse,
    IntegrationSettingsUpdate,
    IntegrationsListResponse,
    OAuthUrlResponse,
    ProvidersListResponse,
)

logger = logging.getLogger(__name__)

router = APIRouter(tags=["Integrations"])

# Initialize providers on module load
auto_discover_providers()


# ============================================================================
# Dependencies
# ============================================================================


def get_integration_service() -> IntegrationService:
    """Get IntegrationService instance."""
    return create_integration_service()


def get_team_service() -> TeamService:
    """Get TeamService instance."""
    return create_team_service()


def get_audit_logger() -> AuditLogger:
    """Get AuditLogger instance."""
    return create_audit_logger()


def get_activity_collector() -> ActivityCollector:
    """Get ActivityCollector instance."""
    return create_activity_collector()


CurrentUser = Annotated[User, Depends(get_current_user)]
IntegrationSvc = Annotated[IntegrationService, Depends(get_integration_service)]
TeamSvc = Annotated[TeamService, Depends(get_team_service)]
AuditLog = Annotated[AuditLogger, Depends(get_audit_logger)]
Activity = Annotated[ActivityCollector, Depends(get_activity_collector)]


# ============================================================================
# State Encoding/Decoding
# ============================================================================


def encode_state(team_id: str, user_id: str, nonce: str | None = None) -> str:
    """Encode OAuth state parameter.

    Args:
        team_id: Team ID to connect
        user_id: User initiating connection
        nonce: Random nonce for CSRF protection

    Returns:
        Base64-encoded JSON string
    """
    if nonce is None:
        nonce = secrets.token_urlsafe(16)

    state_data = {
        "team_id": team_id,
        "user_id": user_id,
        "nonce": nonce,
    }
    return base64.urlsafe_b64encode(json.dumps(state_data).encode()).decode()


def decode_state(state: str) -> dict[str, str]:
    """Decode OAuth state parameter.

    Args:
        state: Base64-encoded state string

    Returns:
        Decoded state data

    Raises:
        ValueError: If state is invalid
    """
    try:
        decoded = base64.urlsafe_b64decode(state.encode())
        return json.loads(decoded)
    except Exception as e:
        raise ValueError(f"Invalid state parameter: {e}") from e


# ============================================================================
# Helper Functions
# ============================================================================


def integration_to_response(integration: Integration) -> IntegrationResponse:
    """Convert Integration entity to IntegrationResponse.

    Args:
        integration: Integration entity

    Returns:
        IntegrationResponse model
    """
    return IntegrationResponse(
        id=integration.id,
        team_id=integration.team_id,
        provider_id=integration.provider_id,
        external_id=integration.external_id,
        external_name=integration.external_name,
        status=integration.status.value,
        settings=integration.settings or {},
        connected_by=integration.connected_by,
        connected_at=integration.connected_at,
        last_used_at=integration.last_used_at,
    )


# ============================================================================
# Provider Endpoints
# ============================================================================


@router.get(
    "/integrations/providers",
    response_model=ProvidersListResponse,
    summary="List Integration Providers",
    description="Get all available integration providers.",
)
async def list_providers() -> ProvidersListResponse:
    """List all available integration providers.

    Returns provider information including capabilities and OAuth URLs.
    This endpoint is public for provider discovery.

    Returns:
        List of available providers
    """
    providers_info = IntegrationRegistry.list_all()

    providers = [
        IntegrationProviderResponse(
            id=info.id,
            name=info.name,
            category=info.category.value,
            capabilities=[cap.value for cap in info.capabilities],
            description=info.description,
            icon_url=info.icon_url,
            docs_url=info.docs_url,
        )
        for info in providers_info
    ]

    return ProvidersListResponse(
        providers=providers,
        total=len(providers),
    )


@router.get(
    "/integrations/providers/{provider_id}",
    response_model=IntegrationProviderResponse,
    summary="Get Provider Details",
    description="Get details of a specific integration provider.",
)
async def get_provider(provider_id: str) -> IntegrationProviderResponse:
    """Get details of a specific provider.

    Args:
        provider_id: Provider identifier

    Returns:
        Provider details

    Raises:
        HTTPException: 404 if provider not found
    """
    provider = IntegrationRegistry.get(provider_id)
    if not provider:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=f"Provider not found: {provider_id}",
        )

    info = provider.get_info()
    return IntegrationProviderResponse(
        id=info.id,
        name=info.name,
        category=info.category.value,
        capabilities=[cap.value for cap in info.capabilities],
        description=info.description,
        icon_url=info.icon_url,
        docs_url=info.docs_url,
    )


# ============================================================================
# OAuth Endpoints
# ============================================================================


@router.get(
    "/integrations/{provider_id}/oauth/url",
    response_model=OAuthUrlResponse,
    summary="Get OAuth URL",
    description="Generate OAuth authorization URL for connecting an integration.",
)
async def get_oauth_url(
    provider_id: str,
    team_id: str = Query(..., description="Team ID to connect"),
    user: CurrentUser = None,
    team_service: TeamSvc = None,
) -> OAuthUrlResponse:
    """Generate OAuth authorization URL.

    Creates a URL for the user to authorize the integration.
    The state parameter encodes team and user info for the callback.

    Args:
        provider_id: Provider to connect
        team_id: Team to connect the integration to
        user: Current authenticated user
        team_service: Team service for permission check

    Returns:
        OAuth URL and state

    Raises:
        HTTPException: 404 if provider not found, 403 if not authorized
    """
    # Check user is admin of team
    has_permission = await team_service.check_permission(
        team_id, user.user_id, "manage_integrations"
    )
    if not has_permission:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Only team admins can connect integrations",
        )

    # Get provider
    provider = IntegrationRegistry.get(provider_id)
    if not provider:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=f"Provider not found: {provider_id}",
        )

    # Generate state
    state = encode_state(team_id=team_id, user_id=user.user_id)

    # Get OAuth URL
    oauth_url = provider.get_oauth_url(state)

    return OAuthUrlResponse(url=oauth_url, state=state)


@router.get(
    "/integrations/{provider_id}/oauth/callback",
    summary="OAuth Callback",
    description="Handle OAuth callback from provider.",
)
async def oauth_callback(
    provider_id: str,
    integration_service: IntegrationSvc,
    audit: AuditLog,
    activity: Activity,
    code: str = Query(..., description="Authorization code"),
    state: str = Query(..., description="State parameter"),
    installation_id: str | None = Query(None, description="GitHub installation ID"),
) -> RedirectResponse:
    """Handle OAuth callback from provider.

    Exchanges the authorization code for tokens and saves the integration.
    Redirects to dashboard on success or error page on failure.

    Args:
        provider_id: Provider ID
        integration_service: Integration service for saving
        code: Authorization code (or installation_id for GitHub App)
        state: State parameter with encoded team/user info
        installation_id: GitHub App installation ID (for GitHub)

    Returns:
        Redirect to dashboard

    Raises:
        HTTPException: On various errors
    """
    # Get base URL for redirects
    base_url = os.environ.get("C4_DASHBOARD_URL", "http://localhost:3000")

    # Decode state
    try:
        state_data = decode_state(state)
    except ValueError as e:
        logger.warning(f"Invalid OAuth state: {e}")
        return RedirectResponse(url=f"{base_url}/integrations?error=invalid_state")

    team_id = state_data.get("team_id")
    user_id = state_data.get("user_id")

    if not team_id or not user_id:
        return RedirectResponse(
            url=f"{base_url}/integrations?error=missing_state_data"
        )

    # Get provider
    provider = IntegrationRegistry.get(provider_id)
    if not provider:
        return RedirectResponse(
            url=f"{base_url}/integrations?error=provider_not_found"
        )

    # For GitHub App, the "code" might be installation_id
    auth_code = installation_id or code

    # Exchange code for connection
    try:
        result = await provider.exchange_code(auth_code, state)
    except Exception as e:
        logger.error(f"OAuth exchange failed for {provider_id}: {e}")
        return RedirectResponse(
            url=f"{base_url}/integrations?error=exchange_failed&message={str(e)}"
        )

    if not result.success:
        error_msg = result.error_code or "connection_failed"
        return RedirectResponse(
            url=f"{base_url}/integrations?error={error_msg}&message={result.message}"
        )

    # Save integration using service
    try:
        integration = await integration_service.save_integration(
            team_id=team_id,
            provider_id=provider_id,
            external_id=result.external_id or "",
            external_name=result.external_name,
            credentials=result.credentials,
            connected_by=user_id,
        )

        # Audit log: Integration connected
        await audit.log(
            team_id=team_id,
            action=AuditAction.INTEGRATION_CONNECTED,
            resource_type="integration",
            resource_id=integration.id,
            actor_type=ActorType.USER,
            actor_id=user_id,
            new_value={
                "provider_id": provider_id,
                "external_id": integration.external_id,
                "external_name": integration.external_name,
            },
        )

        # Activity tracking
        await activity.log_activity(
            team_id=team_id,
            activity_type="integration_connected",
            user_id=user_id,
            resource_type="integration",
            resource_id=integration.id,
            metadata={
                "provider_id": provider_id,
                "external_name": integration.external_name,
            },
        )

        logger.info(
            f"Connected {provider_id} integration for team {team_id}: "
            f"{integration.external_name}"
        )

        return RedirectResponse(url=f"{base_url}/integrations?success={provider_id}")

    except Exception as e:
        logger.error(f"Failed to save integration: {e}")
        return RedirectResponse(url=f"{base_url}/integrations?error=save_failed")


# ============================================================================
# Team Integration Endpoints
# ============================================================================


@router.get(
    "/teams/{team_id}/integrations",
    response_model=IntegrationsListResponse,
    summary="List Team Integrations",
    description="Get all integrations connected to a team.",
)
async def list_team_integrations(
    team_id: str,
    user: CurrentUser,
    integration_service: IntegrationSvc,
    status_filter: str | None = Query(None, description="Filter by status"),
    provider_filter: str | None = Query(None, description="Filter by provider"),
) -> IntegrationsListResponse:
    """List all integrations for a team.

    Args:
        team_id: Team identifier
        user: Current authenticated user
        integration_service: Integration service for querying
        status_filter: Optional status filter
        provider_filter: Optional provider filter

    Returns:
        List of connected integrations

    Raises:
        HTTPException: 403 if not a team member
    """
    # Get integrations using service (RLS handles team membership)
    integrations = await integration_service.get_team_integrations(
        team_id=team_id,
        status_filter=status_filter,
        provider_filter=provider_filter,
    )

    response_integrations = [
        integration_to_response(integration) for integration in integrations
    ]

    return IntegrationsListResponse(
        integrations=response_integrations,
        total=len(response_integrations),
    )


@router.get(
    "/teams/{team_id}/integrations/{integration_id}",
    response_model=IntegrationResponse,
    summary="Get Integration",
    description="Get details of a specific integration.",
)
async def get_team_integration(
    team_id: str,
    integration_id: str,
    user: CurrentUser,
    integration_service: IntegrationSvc,
) -> IntegrationResponse:
    """Get details of a specific integration.

    Args:
        team_id: Team identifier
        integration_id: Integration identifier
        user: Current authenticated user
        integration_service: Integration service for querying

    Returns:
        Integration details

    Raises:
        HTTPException: 404 if not found, 403 if not authorized
    """
    integration = await integration_service.get_integration(team_id, integration_id)

    if not integration:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=f"Integration not found: {integration_id}",
        )

    return integration_to_response(integration)


@router.patch(
    "/teams/{team_id}/integrations/{integration_id}",
    response_model=IntegrationResponse,
    summary="Update Integration Settings",
    description="Update settings for an integration.",
)
async def update_integration(
    team_id: str,
    integration_id: str,
    request: IntegrationSettingsUpdate,
    user: CurrentUser,
    integration_service: IntegrationSvc,
    team_service: TeamSvc,
    audit: AuditLog,
    activity: Activity,
) -> IntegrationResponse:
    """Update integration settings.

    Args:
        team_id: Team identifier
        integration_id: Integration identifier
        request: New settings
        user: Current authenticated user
        integration_service: Integration service for updating
        team_service: Team service for permission check
        audit: Audit logger

    Returns:
        Updated integration

    Raises:
        HTTPException: 404 if not found, 403 if not authorized
    """
    # Check admin permission
    has_permission = await team_service.check_permission(
        team_id, user.user_id, "manage_integrations"
    )
    if not has_permission:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Only team admins can update integrations",
        )

    # Get old settings for audit
    old_integration = await integration_service.get_integration(team_id, integration_id)
    old_settings = old_integration.settings if old_integration else {}

    integration = await integration_service.update_integration_settings(
        team_id, integration_id, request.settings
    )

    if not integration:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=f"Integration not found: {integration_id}",
        )

    # Audit log: Integration updated
    await audit.log(
        team_id=team_id,
        action=AuditAction.INTEGRATION_UPDATED,
        resource_type="integration",
        resource_id=integration_id,
        actor_type=ActorType.USER,
        actor_id=user.user_id,
        actor_email=user.email,
        old_value={"settings": old_settings},
        new_value={"settings": integration.settings},
    )

    # Activity tracking
    await activity.log_activity(
        team_id=team_id,
        activity_type="integration_updated",
        user_id=user.user_id,
        resource_type="integration",
        resource_id=integration_id,
        metadata={
            "provider_id": integration.provider_id,
            "settings_changed": list(request.settings.keys()),
        },
    )

    return integration_to_response(integration)


@router.delete(
    "/teams/{team_id}/integrations/{integration_id}",
    summary="Disconnect Integration",
    description="Remove an integration from a team.",
)
async def disconnect_integration(
    team_id: str,
    integration_id: str,
    user: CurrentUser,
    integration_service: IntegrationSvc,
    team_service: TeamSvc,
    audit: AuditLog,
    activity: Activity,
) -> dict[str, Any]:
    """Disconnect an integration from a team.

    This removes the integration from C4 but does not uninstall
    the app from the external service.

    Args:
        team_id: Team identifier
        integration_id: Integration identifier
        user: Current authenticated user
        integration_service: Integration service for deletion
        team_service: Team service for permission check
        audit: Audit logger

    Returns:
        Success message

    Raises:
        HTTPException: 404 if not found, 403 if not authorized
    """
    # Check admin permission
    has_permission = await team_service.check_permission(
        team_id, user.user_id, "manage_integrations"
    )
    if not has_permission:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Only team admins can disconnect integrations",
        )

    # Get integration to find provider
    integration = await integration_service.get_integration(team_id, integration_id)
    if not integration:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=f"Integration not found: {integration_id}",
        )

    # Store info for audit before deletion
    old_value = {
        "provider_id": integration.provider_id,
        "external_id": integration.external_id,
        "external_name": integration.external_name,
    }

    # Call provider disconnect if available
    provider = IntegrationRegistry.get(integration.provider_id)
    if provider:
        try:
            await provider.disconnect(team_id, integration.external_id)
        except Exception as e:
            logger.warning(f"Provider disconnect hook failed: {e}")
            # Continue with deletion anyway

    # Delete from database
    success = await integration_service.delete_integration(team_id, integration_id)
    if not success:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to disconnect integration",
        )

    # Audit log: Integration disconnected
    await audit.log(
        team_id=team_id,
        action=AuditAction.INTEGRATION_DISCONNECTED,
        resource_type="integration",
        resource_id=integration_id,
        actor_type=ActorType.USER,
        actor_id=user.user_id,
        actor_email=user.email,
        old_value=old_value,
    )

    # Activity tracking
    await activity.log_activity(
        team_id=team_id,
        activity_type="integration_disconnected",
        user_id=user.user_id,
        resource_type="integration",
        resource_id=integration_id,
        metadata=old_value,
    )

    logger.info(
        f"Disconnected {integration.provider_id} integration "
        f"{integration_id} from team {team_id}"
    )

    return {
        "success": True,
        "message": f"Integration {integration_id} disconnected",
    }
