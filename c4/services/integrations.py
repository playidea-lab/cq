"""Integration Management Service.

Provides CRUD operations for workspace integrations using Supabase as the backend.

Environment Variables:
    SUPABASE_URL: Supabase project URL
    SUPABASE_KEY: Supabase anon/service key
"""

from __future__ import annotations

import logging
import os
from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from supabase import Client

logger = logging.getLogger(__name__)


# =============================================================================
# Domain Types
# =============================================================================


class IntegrationStatus(str, Enum):
    """Integration connection status."""

    ACTIVE = "active"
    SUSPENDED = "suspended"
    REVOKED = "revoked"


@dataclass
class Integration:
    """Integration entity."""

    id: str
    team_id: str
    provider_id: str
    external_id: str
    external_name: str | None = None
    credentials: dict[str, Any] | None = None
    settings: dict[str, Any] | None = None
    status: IntegrationStatus = IntegrationStatus.ACTIVE
    connected_by: str | None = None
    connected_at: datetime | None = None
    last_used_at: datetime | None = None


# =============================================================================
# Exceptions
# =============================================================================


class IntegrationError(Exception):
    """Base exception for integration operations."""

    pass


class IntegrationNotFoundError(IntegrationError):
    """Integration not found."""

    def __init__(self, integration_id: str):
        super().__init__(f"Integration not found: {integration_id}")
        self.integration_id = integration_id


# =============================================================================
# Integration Service
# =============================================================================


class IntegrationService:
    """Service for managing workspace integrations.

    Uses Supabase for persistent storage with RLS support.
    """

    TABLE_INTEGRATIONS = "workspace_integrations"

    def __init__(
        self,
        url: str | None = None,
        key: str | None = None,
    ) -> None:
        """Initialize integration service.

        Args:
            url: Supabase project URL (or SUPABASE_URL env)
            key: Supabase anon key (or SUPABASE_KEY env)
        """
        self._url = url or os.environ.get("SUPABASE_URL", "")
        self._key = key or os.environ.get("SUPABASE_KEY", "")
        self._client: Client | None = None

    @property
    def client(self) -> Client:
        """Lazy-initialize Supabase client."""
        if self._client is None:
            if not self._url or not self._key:
                raise ValueError(
                    "Supabase URL and key required. "
                    "Set SUPABASE_URL and SUPABASE_KEY environment variables."
                )
            from supabase import create_client

            self._client = create_client(self._url, self._key)
        return self._client

    # =========================================================================
    # CRUD Operations
    # =========================================================================

    async def get_team_integrations(
        self,
        team_id: str,
        status_filter: str | None = None,
        provider_filter: str | None = None,
    ) -> list[Integration]:
        """Get all integrations for a team.

        Args:
            team_id: Team identifier
            status_filter: Optional status filter
            provider_filter: Optional provider filter

        Returns:
            List of Integration entities
        """
        query = (
            self.client.table(self.TABLE_INTEGRATIONS)
            .select("*")
            .eq("team_id", team_id)
        )

        if status_filter:
            query = query.eq("status", status_filter)

        if provider_filter:
            query = query.eq("provider_id", provider_filter)

        response = query.execute()

        return [self._row_to_integration(row) for row in response.data]

    async def get_integration(
        self,
        team_id: str,
        integration_id: str,
    ) -> Integration | None:
        """Get a specific integration.

        Args:
            team_id: Team identifier
            integration_id: Integration identifier

        Returns:
            Integration entity or None if not found
        """
        response = (
            self.client.table(self.TABLE_INTEGRATIONS)
            .select("*")
            .eq("id", integration_id)
            .eq("team_id", team_id)
            .maybe_single()
            .execute()
        )

        if not response.data:
            return None

        return self._row_to_integration(response.data)

    async def get_integration_by_external_id(
        self,
        provider_id: str,
        external_id: str,
    ) -> Integration | None:
        """Get integration by provider and external ID.

        Used for webhook routing.

        Args:
            provider_id: Provider identifier
            external_id: External service ID (e.g., GitHub installation_id)

        Returns:
            Integration entity or None if not found
        """
        response = (
            self.client.table(self.TABLE_INTEGRATIONS)
            .select("*")
            .eq("provider_id", provider_id)
            .eq("external_id", external_id)
            .eq("status", IntegrationStatus.ACTIVE.value)
            .maybe_single()
            .execute()
        )

        if not response.data:
            return None

        return self._row_to_integration(response.data)

    async def save_integration(
        self,
        team_id: str,
        provider_id: str,
        external_id: str,
        external_name: str | None = None,
        credentials: dict[str, Any] | None = None,
        connected_by: str | None = None,
    ) -> Integration:
        """Save a new integration.

        Args:
            team_id: Team identifier
            provider_id: Provider identifier
            external_id: External service ID
            external_name: External service name
            credentials: OAuth credentials (encrypted in production)
            connected_by: User who connected

        Returns:
            Created Integration entity
        """
        integration_data = {
            "team_id": team_id,
            "provider_id": provider_id,
            "external_id": external_id,
            "external_name": external_name,
            "credentials": credentials or {},
            "settings": {},
            "status": IntegrationStatus.ACTIVE.value,
            "connected_by": connected_by,
        }

        response = (
            self.client.table(self.TABLE_INTEGRATIONS)
            .insert(integration_data)
            .execute()
        )

        if not response.data:
            raise IntegrationError(f"Failed to save integration for {provider_id}")

        integration = self._row_to_integration(response.data[0])

        logger.info(
            f"Saved integration: {provider_id} ({external_name}) for team {team_id}"
        )

        return integration

    async def update_integration_settings(
        self,
        team_id: str,
        integration_id: str,
        settings: dict[str, Any],
    ) -> Integration | None:
        """Update integration settings.

        Args:
            team_id: Team identifier
            integration_id: Integration identifier
            settings: Settings to merge

        Returns:
            Updated Integration or None if not found
        """
        # Get current settings
        current = await self.get_integration(team_id, integration_id)
        if not current:
            return None

        # Merge settings
        merged = {**(current.settings or {}), **settings}

        response = (
            self.client.table(self.TABLE_INTEGRATIONS)
            .update({"settings": merged})
            .eq("id", integration_id)
            .eq("team_id", team_id)
            .execute()
        )

        if not response.data:
            return None

        return self._row_to_integration(response.data[0])

    async def update_last_used(self, integration_id: str) -> None:
        """Update the last_used_at timestamp.

        Args:
            integration_id: Integration identifier
        """
        self.client.table(self.TABLE_INTEGRATIONS).update(
            {"last_used_at": datetime.now().isoformat()}
        ).eq("id", integration_id).execute()

    async def delete_integration(
        self,
        team_id: str,
        integration_id: str,
    ) -> bool:
        """Delete an integration.

        Args:
            team_id: Team identifier
            integration_id: Integration identifier

        Returns:
            True if deleted
        """
        response = (
            self.client.table(self.TABLE_INTEGRATIONS)
            .delete()
            .eq("id", integration_id)
            .eq("team_id", team_id)
            .execute()
        )

        deleted = len(response.data) > 0
        if deleted:
            logger.info(f"Deleted integration: {integration_id} from team {team_id}")

        return deleted

    # =========================================================================
    # Helpers
    # =========================================================================

    def _row_to_integration(self, row: dict[str, Any]) -> Integration:
        """Convert database row to Integration entity."""
        return Integration(
            id=row["id"],
            team_id=row["team_id"],
            provider_id=row["provider_id"],
            external_id=row["external_id"],
            external_name=row.get("external_name"),
            credentials=row.get("credentials"),
            settings=row.get("settings"),
            status=IntegrationStatus(row.get("status", "active")),
            connected_by=row.get("connected_by"),
            connected_at=_parse_datetime(row.get("connected_at")),
            last_used_at=_parse_datetime(row.get("last_used_at")),
        )


def _parse_datetime(value: str | datetime | None) -> datetime | None:
    """Parse datetime from string or return as-is."""
    if value is None:
        return None
    if isinstance(value, datetime):
        return value
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except (ValueError, AttributeError):
        return None


# =============================================================================
# Factory Function
# =============================================================================


def create_integration_service(
    url: str | None = None,
    key: str | None = None,
) -> IntegrationService:
    """Create IntegrationService instance.

    Args:
        url: Supabase project URL
        key: Supabase anon key

    Returns:
        Configured IntegrationService instance
    """
    return IntegrationService(url=url, key=key)
