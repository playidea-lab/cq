"""Team branding service for white-label functionality."""

from __future__ import annotations

import hashlib
import secrets
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any

from c4.services.audit import AuditLogger, create_audit_logger


# =============================================================================
# Exceptions
# =============================================================================


class BrandingError(Exception):
    """Base exception for branding operations."""

    pass


class BrandingNotFoundError(BrandingError):
    """Branding configuration not found."""

    pass


class BrandingPermissionError(BrandingError):
    """Permission denied for branding operation."""

    pass


class DomainVerificationError(BrandingError):
    """Domain verification failed."""

    pass


class DomainAlreadyInUseError(BrandingError):
    """Domain is already in use by another team."""

    pass


class InvalidDomainError(BrandingError):
    """Invalid domain format."""

    pass


# =============================================================================
# Data Classes
# =============================================================================


@dataclass
class TeamBranding:
    """Team branding configuration."""

    id: str
    team_id: str

    # Basic Branding
    logo_url: str | None = None
    logo_dark_url: str | None = None
    favicon_url: str | None = None
    brand_name: str | None = None

    # Color Scheme
    primary_color: str = "#2563EB"
    secondary_color: str = "#64748B"
    accent_color: str = "#F59E0B"
    background_color: str = "#FFFFFF"
    text_color: str = "#1F2937"

    # Typography
    heading_font: str | None = None
    body_font: str | None = None
    font_scale: float = 1.0

    # Custom Domain
    custom_domain: str | None = None
    custom_domain_verified: bool = False
    custom_domain_verification_token: str | None = None
    custom_domain_verified_at: datetime | None = None

    # Email Branding
    email_from_name: str | None = None
    email_footer_text: str | None = None
    email_header_html: str | None = None

    # Advanced Customization
    custom_css: str | None = None
    meta_description: str | None = None
    social_preview_image_url: str | None = None

    # Feature Flags
    hide_powered_by: bool = False
    custom_login_background_url: str | None = None

    # Metadata
    created_at: datetime | None = None
    updated_at: datetime | None = None
    created_by: str | None = None


@dataclass
class DomainVerificationResult:
    """Result of domain verification initiation."""

    success: bool
    verification_token: str | None = None
    instructions: dict[str, str] | None = None
    error: str | None = None


@dataclass
class BrandingUpdateRequest:
    """Request to update team branding."""

    logo_url: str | None = None
    logo_dark_url: str | None = None
    favicon_url: str | None = None
    brand_name: str | None = None
    primary_color: str | None = None
    secondary_color: str | None = None
    accent_color: str | None = None
    background_color: str | None = None
    text_color: str | None = None
    heading_font: str | None = None
    body_font: str | None = None
    font_scale: float | None = None
    email_from_name: str | None = None
    email_footer_text: str | None = None
    meta_description: str | None = None
    social_preview_image_url: str | None = None
    custom_login_background_url: str | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary, excluding None values."""
        result = {}
        for key, value in self.__dict__.items():
            if value is not None:
                result[key] = value
        return result


# =============================================================================
# Branding Service
# =============================================================================


class BrandingService:
    """Service for managing team branding configurations."""

    def __init__(
        self,
        supabase_url: str | None = None,
        supabase_key: str | None = None,
        audit_logger: AuditLogger | None = None,
    ) -> None:
        """Initialize branding service.

        Args:
            supabase_url: Supabase project URL
            supabase_key: Supabase service role key (for RLS bypass)
            audit_logger: Optional audit logger for compliance
        """
        self._supabase_url = supabase_url
        self._supabase_key = supabase_key
        self._client = None
        self._service_client = None
        self._audit_logger = audit_logger

    @property
    def client(self):
        """Lazy-load Supabase client."""
        if self._client is None:
            from supabase import create_client

            url = self._supabase_url or self._get_env("SUPABASE_URL")
            key = self._supabase_key or self._get_env("SUPABASE_ANON_KEY")
            self._client = create_client(url, key)
        return self._client

    @property
    def service_client(self):
        """Lazy-load Supabase service client (bypasses RLS)."""
        if self._service_client is None:
            from supabase import create_client

            url = self._supabase_url or self._get_env("SUPABASE_URL")
            key = self._get_env("SUPABASE_SERVICE_ROLE_KEY")
            self._service_client = create_client(url, key)
        return self._service_client

    def _get_env(self, key: str) -> str:
        """Get environment variable."""
        import os

        value = os.environ.get(key)
        if not value:
            raise BrandingError(f"Missing environment variable: {key}")
        return value

    def _row_to_branding(self, row: dict[str, Any]) -> TeamBranding:
        """Convert database row to TeamBranding object."""
        return TeamBranding(
            id=row["id"],
            team_id=row["team_id"],
            logo_url=row.get("logo_url"),
            logo_dark_url=row.get("logo_dark_url"),
            favicon_url=row.get("favicon_url"),
            brand_name=row.get("brand_name"),
            primary_color=row.get("primary_color", "#2563EB"),
            secondary_color=row.get("secondary_color", "#64748B"),
            accent_color=row.get("accent_color", "#F59E0B"),
            background_color=row.get("background_color", "#FFFFFF"),
            text_color=row.get("text_color", "#1F2937"),
            heading_font=row.get("heading_font"),
            body_font=row.get("body_font"),
            font_scale=float(row.get("font_scale", 1.0)),
            custom_domain=row.get("custom_domain"),
            custom_domain_verified=row.get("custom_domain_verified", False),
            custom_domain_verification_token=row.get("custom_domain_verification_token"),
            custom_domain_verified_at=(
                datetime.fromisoformat(row["custom_domain_verified_at"])
                if row.get("custom_domain_verified_at")
                else None
            ),
            email_from_name=row.get("email_from_name"),
            email_footer_text=row.get("email_footer_text"),
            email_header_html=row.get("email_header_html"),
            custom_css=row.get("custom_css"),
            meta_description=row.get("meta_description"),
            social_preview_image_url=row.get("social_preview_image_url"),
            hide_powered_by=row.get("hide_powered_by", False),
            custom_login_background_url=row.get("custom_login_background_url"),
            created_at=(
                datetime.fromisoformat(row["created_at"])
                if row.get("created_at")
                else None
            ),
            updated_at=(
                datetime.fromisoformat(row["updated_at"])
                if row.get("updated_at")
                else None
            ),
            created_by=row.get("created_by"),
        )

    async def get_branding(self, team_id: str) -> TeamBranding | None:
        """Get branding configuration for a team.

        Args:
            team_id: Team ID

        Returns:
            TeamBranding if found, None otherwise
        """
        response = (
            self.client.table("team_branding")
            .select("*")
            .eq("team_id", team_id)
            .execute()
        )

        if not response.data:
            return None

        return self._row_to_branding(response.data[0])

    async def get_or_create_branding(
        self, team_id: str, user_id: str | None = None
    ) -> TeamBranding:
        """Get or create default branding for a team.

        Args:
            team_id: Team ID
            user_id: Optional user ID for audit

        Returns:
            TeamBranding configuration
        """
        branding = await self.get_branding(team_id)
        if branding:
            return branding

        # Create default branding
        insert_data = {
            "team_id": team_id,
            "created_by": user_id,
        }

        response = (
            self.service_client.table("team_branding")
            .insert(insert_data)
            .execute()
        )

        if not response.data:
            raise BrandingError("Failed to create default branding")

        branding = self._row_to_branding(response.data[0])

        # Audit log
        if self._audit_logger:
            await self._audit_logger.log(
                team_id=team_id,
                action="branding.created",
                resource_type="team_branding",
                resource_id=branding.id,
                actor_id=user_id,
                new_value={"team_id": team_id},
            )

        return branding

    async def update_branding(
        self,
        team_id: str,
        updates: BrandingUpdateRequest | dict[str, Any],
        user_id: str | None = None,
    ) -> TeamBranding:
        """Update team branding configuration.

        Args:
            team_id: Team ID
            updates: Branding updates
            user_id: Optional user ID for audit

        Returns:
            Updated TeamBranding
        """
        # Get current branding for audit
        old_branding = await self.get_branding(team_id)

        # Convert to dict if needed
        if isinstance(updates, BrandingUpdateRequest):
            update_data = updates.to_dict()
        else:
            update_data = {k: v for k, v in updates.items() if v is not None}

        if not update_data:
            if old_branding:
                return old_branding
            return await self.get_or_create_branding(team_id, user_id)

        # Ensure branding exists
        await self.get_or_create_branding(team_id, user_id)

        # Update
        response = (
            self.service_client.table("team_branding")
            .update(update_data)
            .eq("team_id", team_id)
            .execute()
        )

        if not response.data:
            raise BrandingError("Failed to update branding")

        new_branding = self._row_to_branding(response.data[0])

        # Audit log
        if self._audit_logger:
            await self._audit_logger.log(
                team_id=team_id,
                action="branding.updated",
                resource_type="team_branding",
                resource_id=new_branding.id,
                actor_id=user_id,
                old_value=(
                    {
                        "logo_url": old_branding.logo_url,
                        "brand_name": old_branding.brand_name,
                        "primary_color": old_branding.primary_color,
                    }
                    if old_branding
                    else None
                ),
                new_value=update_data,
            )

        return new_branding

    async def get_branding_by_domain(self, domain: str) -> TeamBranding | None:
        """Get branding configuration by custom domain.

        Args:
            domain: Custom domain (e.g., 'projects.agency.com')

        Returns:
            TeamBranding if found and verified, None otherwise
        """
        response = (
            self.client.table("team_branding")
            .select("*")
            .eq("custom_domain", domain)
            .eq("custom_domain_verified", True)
            .execute()
        )

        if not response.data:
            return None

        return self._row_to_branding(response.data[0])

    async def initiate_domain_verification(
        self,
        team_id: str,
        domain: str,
        user_id: str | None = None,
    ) -> DomainVerificationResult:
        """Initiate custom domain verification.

        Args:
            team_id: Team ID
            domain: Custom domain to verify
            user_id: User initiating the verification

        Returns:
            DomainVerificationResult with verification instructions
        """
        # Validate domain format
        import re

        domain_pattern = r"^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$"
        if not re.match(domain_pattern, domain.lower()):
            return DomainVerificationResult(
                success=False,
                error="Invalid domain format",
            )

        domain = domain.lower()

        # Check if domain is already in use
        existing = (
            self.service_client.table("team_branding")
            .select("team_id")
            .eq("custom_domain", domain)
            .neq("team_id", team_id)
            .execute()
        )

        if existing.data:
            return DomainVerificationResult(
                success=False,
                error="Domain already in use by another team",
            )

        # Generate verification token
        token = f"c4-verify-{secrets.token_hex(16)}"

        # Ensure branding exists and update
        await self.get_or_create_branding(team_id, user_id)

        update_data = {
            "custom_domain": domain,
            "custom_domain_verification_token": token,
            "custom_domain_verified": False,
            "custom_domain_verified_at": None,
        }

        response = (
            self.service_client.table("team_branding")
            .update(update_data)
            .eq("team_id", team_id)
            .execute()
        )

        if not response.data:
            return DomainVerificationResult(
                success=False,
                error="Failed to save verification token",
            )

        # Audit log
        if self._audit_logger:
            await self._audit_logger.log(
                team_id=team_id,
                action="branding.domain_verification_initiated",
                resource_type="team_branding",
                resource_id=response.data[0]["id"],
                actor_id=user_id,
                new_value={"domain": domain},
            )

        return DomainVerificationResult(
            success=True,
            verification_token=token,
            instructions={
                "type": "TXT",
                "name": f"_c4-verification.{domain}",
                "value": token,
            },
        )

    async def verify_domain(
        self,
        team_id: str,
        user_id: str | None = None,
    ) -> bool:
        """Verify custom domain (called after DNS is set up).

        In production, this should verify the DNS TXT record.
        For now, it marks the domain as verified.

        Args:
            team_id: Team ID
            user_id: User performing the verification

        Returns:
            True if verification successful
        """
        branding = await self.get_branding(team_id)
        if not branding:
            raise BrandingNotFoundError(f"No branding found for team {team_id}")

        if not branding.custom_domain:
            raise DomainVerificationError("No custom domain configured")

        # TODO: In production, verify DNS TXT record
        # dns_verified = await verify_dns_txt_record(
        #     f"_c4-verification.{branding.custom_domain}",
        #     branding.custom_domain_verification_token,
        # )
        # if not dns_verified:
        #     raise DomainVerificationError("DNS verification failed")

        # Mark as verified
        from datetime import timezone

        now = datetime.now(timezone.utc).isoformat()

        response = (
            self.service_client.table("team_branding")
            .update(
                {
                    "custom_domain_verified": True,
                    "custom_domain_verified_at": now,
                }
            )
            .eq("team_id", team_id)
            .execute()
        )

        if not response.data:
            raise DomainVerificationError("Failed to update verification status")

        # Audit log
        if self._audit_logger:
            await self._audit_logger.log(
                team_id=team_id,
                action="branding.domain_verified",
                resource_type="team_branding",
                resource_id=branding.id,
                actor_id=user_id,
                new_value={
                    "domain": branding.custom_domain,
                    "verified_at": now,
                },
            )

        return True

    async def remove_custom_domain(
        self,
        team_id: str,
        user_id: str | None = None,
    ) -> bool:
        """Remove custom domain from team branding.

        Args:
            team_id: Team ID
            user_id: User removing the domain

        Returns:
            True if successful
        """
        branding = await self.get_branding(team_id)
        if not branding:
            return True

        old_domain = branding.custom_domain

        response = (
            self.service_client.table("team_branding")
            .update(
                {
                    "custom_domain": None,
                    "custom_domain_verified": False,
                    "custom_domain_verification_token": None,
                    "custom_domain_verified_at": None,
                }
            )
            .eq("team_id", team_id)
            .execute()
        )

        if not response.data:
            raise BrandingError("Failed to remove custom domain")

        # Audit log
        if self._audit_logger and old_domain:
            await self._audit_logger.log(
                team_id=team_id,
                action="branding.domain_removed",
                resource_type="team_branding",
                resource_id=branding.id,
                actor_id=user_id,
                old_value={"domain": old_domain},
            )

        return True

    async def get_public_branding(self, team_id: str) -> dict[str, Any] | None:
        """Get public-safe branding information.

        Returns branding without sensitive fields like verification tokens.

        Args:
            team_id: Team ID

        Returns:
            Public branding dict or None
        """
        branding = await self.get_branding(team_id)
        if not branding:
            return None

        return {
            "brand_name": branding.brand_name,
            "logo_url": branding.logo_url,
            "logo_dark_url": branding.logo_dark_url,
            "favicon_url": branding.favicon_url,
            "primary_color": branding.primary_color,
            "secondary_color": branding.secondary_color,
            "accent_color": branding.accent_color,
            "background_color": branding.background_color,
            "text_color": branding.text_color,
            "heading_font": branding.heading_font,
            "body_font": branding.body_font,
            "font_scale": branding.font_scale,
            "meta_description": branding.meta_description,
            "social_preview_image_url": branding.social_preview_image_url,
            "custom_login_background_url": branding.custom_login_background_url,
            "hide_powered_by": branding.hide_powered_by,
            "custom_domain": branding.custom_domain if branding.custom_domain_verified else None,
        }


# =============================================================================
# Factory Function
# =============================================================================


def create_branding_service(
    supabase_url: str | None = None,
    supabase_key: str | None = None,
    with_audit: bool = True,
) -> BrandingService:
    """Create a branding service instance.

    Args:
        supabase_url: Optional Supabase URL
        supabase_key: Optional Supabase key
        with_audit: Whether to enable audit logging

    Returns:
        BrandingService instance
    """
    audit_logger = create_audit_logger() if with_audit else None

    return BrandingService(
        supabase_url=supabase_url,
        supabase_key=supabase_key,
        audit_logger=audit_logger,
    )
