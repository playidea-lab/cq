"""Branding API routes for team white-label customization."""

from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status

from c4.api.auth import User, get_current_user
from c4.api.models import (
    BrandingResponse,
    BrandingUpdateRequest,
    DomainVerificationRequest,
    DomainVerificationResponse,
    DomainVerifyResponse,
    PublicBrandingResponse,
)
from c4.services.branding import (
    BrandingError,
    BrandingNotFoundError,
    BrandingService,
    DomainVerificationError,
    TeamBranding,
    create_branding_service,
)
from c4.services.teams import TeamService, create_team_service

router = APIRouter(prefix="/teams/{team_id}/branding", tags=["Branding"])

# Public router for domain lookups (no auth required)
public_router = APIRouter(prefix="/branding", tags=["Branding"])


# Dependency injection
def get_branding_service() -> BrandingService:
    """Get branding service instance."""
    return create_branding_service()


def get_team_service() -> TeamService:
    """Get team service instance."""
    return create_team_service()


# Type aliases for cleaner signatures
CurrentUser = Annotated[User, Depends(get_current_user)]
BrandingSvc = Annotated[BrandingService, Depends(get_branding_service)]
TeamSvc = Annotated[TeamService, Depends(get_team_service)]


def _branding_to_response(branding: TeamBranding) -> BrandingResponse:
    """Convert service TeamBranding to API BrandingResponse."""
    return BrandingResponse(
        id=branding.id,
        team_id=branding.team_id,
        logo_url=branding.logo_url,
        logo_dark_url=branding.logo_dark_url,
        favicon_url=branding.favicon_url,
        brand_name=branding.brand_name,
        primary_color=branding.primary_color,
        secondary_color=branding.secondary_color,
        accent_color=branding.accent_color,
        background_color=branding.background_color,
        text_color=branding.text_color,
        heading_font=branding.heading_font,
        body_font=branding.body_font,
        font_scale=branding.font_scale,
        custom_domain=branding.custom_domain,
        custom_domain_verified=branding.custom_domain_verified,
        email_from_name=branding.email_from_name,
        email_footer_text=branding.email_footer_text,
        meta_description=branding.meta_description,
        social_preview_image_url=branding.social_preview_image_url,
        custom_login_background_url=branding.custom_login_background_url,
        hide_powered_by=branding.hide_powered_by,
        created_at=branding.created_at,
        updated_at=branding.updated_at,
    )


@router.get("", response_model=BrandingResponse)
async def get_branding(
    team_id: str,
    user: CurrentUser,
    branding_service: BrandingSvc,
    team_service: TeamSvc,
) -> BrandingResponse:
    """Get team branding configuration.

    Returns the branding settings for the specified team.
    Creates default branding if none exists.

    Requires: Team member (any role)
    """
    try:
        # Verify user is a team member
        await team_service.require_permission(team_id, user.user_id, "view_team")

        branding = await branding_service.get_or_create_branding(
            team_id=team_id,
            user_id=user.user_id,
        )

        return _branding_to_response(branding)

    except Exception as e:
        if "permission" in str(e).lower() or "not authorized" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Not authorized to view this team's branding",
            ) from e
        if "not found" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Team not found",
            ) from e
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.patch("", response_model=BrandingResponse)
async def update_branding(
    team_id: str,
    request: BrandingUpdateRequest,
    user: CurrentUser,
    branding_service: BrandingSvc,
    team_service: TeamSvc,
) -> BrandingResponse:
    """Update team branding configuration.

    Partially updates branding settings. Only provided fields are updated.

    Requires: Team admin or owner
    """
    try:
        # Verify user has admin permissions
        await team_service.require_permission(team_id, user.user_id, "manage_settings")

        # Convert Pydantic model to dict, excluding None values
        updates = request.model_dump(exclude_none=True)

        branding = await branding_service.update_branding(
            team_id=team_id,
            updates=updates,
            user_id=user.user_id,
        )

        return _branding_to_response(branding)

    except Exception as e:
        if "permission" in str(e).lower() or "not authorized" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Not authorized to update branding",
            ) from e
        if "not found" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Team not found",
            ) from e
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post("/domain", response_model=DomainVerificationResponse)
async def initiate_domain_verification(
    team_id: str,
    request: DomainVerificationRequest,
    user: CurrentUser,
    branding_service: BrandingSvc,
    team_service: TeamSvc,
) -> DomainVerificationResponse:
    """Initiate custom domain verification.

    Generates a DNS TXT record token for domain ownership verification.
    The token must be added as a TXT record at _c4-verification.{domain}.

    Requires: Team admin or owner
    """
    try:
        # Verify user has admin permissions
        await team_service.require_permission(team_id, user.user_id, "manage_settings")

        result = await branding_service.initiate_domain_verification(
            team_id=team_id,
            domain=request.domain,
            user_id=user.user_id,
        )

        return DomainVerificationResponse(
            success=result.success,
            verification_token=result.verification_token,
            instructions=result.instructions,
            error=result.error,
        )

    except Exception as e:
        if "permission" in str(e).lower() or "not authorized" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Not authorized to configure custom domain",
            ) from e
        if "not found" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Team not found",
            ) from e
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post("/domain/verify", response_model=DomainVerifyResponse)
async def verify_domain(
    team_id: str,
    user: CurrentUser,
    branding_service: BrandingSvc,
    team_service: TeamSvc,
) -> DomainVerifyResponse:
    """Verify custom domain ownership.

    Call this after adding the DNS TXT record.
    Checks DNS and marks the domain as verified if successful.

    Requires: Team admin or owner
    """
    try:
        # Verify user has admin permissions
        await team_service.require_permission(team_id, user.user_id, "manage_settings")

        await branding_service.verify_domain(
            team_id=team_id,
            user_id=user.user_id,
        )

        # Get updated branding to return details
        branding = await branding_service.get_branding(team_id)

        return DomainVerifyResponse(
            success=True,
            domain=branding.custom_domain if branding else None,
            verified_at=branding.custom_domain_verified_at if branding else None,
            error=None,
        )

    except BrandingNotFoundError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e),
        ) from e
    except DomainVerificationError as e:
        return DomainVerifyResponse(
            success=False,
            domain=None,
            verified_at=None,
            error=str(e),
        )
    except Exception as e:
        if "permission" in str(e).lower() or "not authorized" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Not authorized to verify domain",
            ) from e
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.delete("/domain")
async def remove_custom_domain(
    team_id: str,
    user: CurrentUser,
    branding_service: BrandingSvc,
    team_service: TeamSvc,
) -> dict:
    """Remove custom domain from team branding.

    Removes the custom domain configuration and verification status.

    Requires: Team admin or owner
    """
    try:
        # Verify user has admin permissions
        await team_service.require_permission(team_id, user.user_id, "manage_settings")

        await branding_service.remove_custom_domain(
            team_id=team_id,
            user_id=user.user_id,
        )

        return {"success": True, "message": "Custom domain removed"}

    except BrandingError as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e
    except Exception as e:
        if "permission" in str(e).lower() or "not authorized" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Not authorized to remove custom domain",
            ) from e
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


# ============================================================================
# PUBLIC ROUTES (No authentication required)
# ============================================================================


@public_router.get("/by-domain/{domain}", response_model=PublicBrandingResponse)
async def get_branding_by_domain(
    domain: str,
    branding_service: BrandingSvc,
) -> PublicBrandingResponse:
    """Get public branding by custom domain.

    This endpoint is public and used for login pages.
    Only returns branding for verified custom domains.

    No authentication required.
    """
    try:
        branding = await branding_service.get_branding_by_domain(domain)

        if not branding:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="No branding found for this domain",
            )

        return PublicBrandingResponse(
            brand_name=branding.brand_name,
            logo_url=branding.logo_url,
            logo_dark_url=branding.logo_dark_url,
            favicon_url=branding.favicon_url,
            primary_color=branding.primary_color,
            background_color=branding.background_color,
            text_color=branding.text_color,
            heading_font=branding.heading_font,
            body_font=branding.body_font,
            custom_login_background_url=branding.custom_login_background_url,
        )

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e
