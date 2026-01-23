"""Audit Logging Service.

Provides immutable audit trail for security and compliance.

Environment Variables:
    SUPABASE_URL: Supabase project URL
    SUPABASE_KEY: Supabase anon/service key
"""

from __future__ import annotations

import csv
import hashlib
import io
import json
import logging
import os
from dataclasses import dataclass
from datetime import date, datetime
from enum import Enum
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from fastapi import Request
    from supabase import Client

logger = logging.getLogger(__name__)


# =============================================================================
# Domain Types
# =============================================================================


class ActorType(str, Enum):
    """Type of actor performing the action."""

    USER = "user"
    API_KEY = "api_key"
    SYSTEM = "system"
    WORKER = "worker"


class AuditAction(str, Enum):
    """Standard audit actions."""

    # Team actions
    TEAM_CREATED = "team.created"
    TEAM_UPDATED = "team.updated"
    TEAM_DELETED = "team.deleted"

    # Member actions
    MEMBER_INVITED = "member.invited"
    MEMBER_JOINED = "member.joined"
    MEMBER_ROLE_CHANGED = "member.role_changed"
    MEMBER_REMOVED = "member.removed"

    # Workspace actions
    WORKSPACE_CREATED = "workspace.created"
    WORKSPACE_UPDATED = "workspace.updated"
    WORKSPACE_DELETED = "workspace.deleted"

    # Integration actions
    INTEGRATION_CONNECTED = "integration.connected"
    INTEGRATION_UPDATED = "integration.updated"
    INTEGRATION_DISCONNECTED = "integration.disconnected"

    # Settings actions
    SETTINGS_UPDATED = "settings.updated"

    # Authentication actions
    LOGIN_SUCCESS = "auth.login_success"
    LOGIN_FAILED = "auth.login_failed"
    LOGOUT = "auth.logout"

    # API key actions
    API_KEY_CREATED = "api_key.created"
    API_KEY_REVOKED = "api_key.revoked"


@dataclass
class AuditLog:
    """Audit log entry."""

    id: str
    team_id: str
    actor_type: ActorType
    actor_id: str
    action: str
    resource_type: str
    resource_id: str
    actor_email: str | None = None
    old_value: dict[str, Any] | None = None
    new_value: dict[str, Any] | None = None
    ip_address: str | None = None
    user_agent: str | None = None
    request_id: str | None = None
    created_at: datetime | None = None
    hash: str | None = None


@dataclass
class AuditFilter:
    """Filter for audit log queries."""

    actor_id: str | None = None
    action: str | None = None
    resource_type: str | None = None
    resource_id: str | None = None
    start_date: date | None = None
    end_date: date | None = None
    limit: int = 100
    offset: int = 0


# =============================================================================
# Exceptions
# =============================================================================


class AuditError(Exception):
    """Base exception for audit operations."""

    pass


# =============================================================================
# Audit Logger Service
# =============================================================================


class AuditLogger:
    """Service for security audit logging.

    Uses Supabase for persistent storage with RLS support.
    Audit logs are immutable - no update or delete operations.
    """

    TABLE_AUDIT_LOGS = "audit_logs"

    def __init__(
        self,
        url: str | None = None,
        key: str | None = None,
    ) -> None:
        """Initialize audit logger.

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
    # Logging Operations
    # =========================================================================

    async def log(
        self,
        team_id: str,
        action: str | AuditAction,
        resource_type: str,
        resource_id: str,
        *,
        actor_type: ActorType = ActorType.USER,
        actor_id: str | None = None,
        actor_email: str | None = None,
        old_value: dict[str, Any] | None = None,
        new_value: dict[str, Any] | None = None,
        request: Request | None = None,
    ) -> AuditLog:
        """Log an audit event.

        Args:
            team_id: Team identifier
            action: Action performed (use AuditAction enum)
            resource_type: Type of resource affected
            resource_id: Resource identifier
            actor_type: Type of actor
            actor_id: Actor identifier (user_id, api_key_id, etc.)
            actor_email: Actor's email for display
            old_value: Previous state (for changes)
            new_value: New state (for changes)
            request: FastAPI request for context (IP, User-Agent)

        Returns:
            Created AuditLog entry
        """
        # Extract request context
        ip_address = None
        user_agent = None
        request_id = None

        if request:
            ip_address = request.client.host if request.client else None
            user_agent = request.headers.get("user-agent")
            request_id = request.headers.get("x-request-id")

        action_str = action.value if isinstance(action, AuditAction) else action

        audit_data = {
            "team_id": team_id,
            "actor_type": actor_type.value,
            "actor_id": actor_id or "unknown",
            "actor_email": actor_email,
            "action": action_str,
            "resource_type": resource_type,
            "resource_id": resource_id,
            "old_value": old_value,
            "new_value": new_value,
            "ip_address": ip_address,
            "user_agent": user_agent,
            "request_id": request_id,
        }

        response = (
            self.client.table(self.TABLE_AUDIT_LOGS)
            .insert(audit_data)
            .execute()
        )

        if not response.data:
            raise AuditError(f"Failed to log audit event: {action_str}")

        audit_log = self._row_to_audit_log(response.data[0])

        logger.info(
            f"Audit: {action_str} on {resource_type}/{resource_id} "
            f"by {actor_type.value}/{actor_id}"
        )

        return audit_log

    # =========================================================================
    # Query Operations
    # =========================================================================

    async def get_logs(
        self,
        team_id: str,
        filter: AuditFilter | None = None,
    ) -> list[AuditLog]:
        """Get audit logs for a team.

        Args:
            team_id: Team identifier
            filter: Query filters

        Returns:
            List of AuditLog entries
        """
        if filter is None:
            filter = AuditFilter()

        query = (
            self.client.table(self.TABLE_AUDIT_LOGS)
            .select("*")
            .eq("team_id", team_id)
            .order("created_at", desc=True)
            .limit(filter.limit)
            .offset(filter.offset)
        )

        if filter.actor_id:
            query = query.eq("actor_id", filter.actor_id)

        if filter.action:
            query = query.eq("action", filter.action)

        if filter.resource_type:
            query = query.eq("resource_type", filter.resource_type)

        if filter.resource_id:
            query = query.eq("resource_id", filter.resource_id)

        if filter.start_date:
            query = query.gte("created_at", filter.start_date.isoformat())

        if filter.end_date:
            query = query.lte("created_at", filter.end_date.isoformat())

        response = query.execute()

        return [self._row_to_audit_log(row) for row in response.data]

    async def get_log_count(
        self,
        team_id: str,
        filter: AuditFilter | None = None,
    ) -> int:
        """Get count of audit logs matching filter.

        Args:
            team_id: Team identifier
            filter: Query filters

        Returns:
            Count of matching logs
        """
        if filter is None:
            filter = AuditFilter()

        query = (
            self.client.table(self.TABLE_AUDIT_LOGS)
            .select("id", count="exact")
            .eq("team_id", team_id)
        )

        if filter.actor_id:
            query = query.eq("actor_id", filter.actor_id)

        if filter.action:
            query = query.eq("action", filter.action)

        if filter.resource_type:
            query = query.eq("resource_type", filter.resource_type)

        if filter.start_date:
            query = query.gte("created_at", filter.start_date.isoformat())

        if filter.end_date:
            query = query.lte("created_at", filter.end_date.isoformat())

        response = query.execute()

        return response.count or 0

    # =========================================================================
    # Export Operations
    # =========================================================================

    async def export_logs(
        self,
        team_id: str,
        format: str = "csv",
        filter: AuditFilter | None = None,
    ) -> bytes:
        """Export audit logs in specified format.

        Args:
            team_id: Team identifier
            format: Export format ('csv' or 'json')
            filter: Query filters

        Returns:
            Exported data as bytes
        """
        # Remove limit for export (get all matching)
        if filter is None:
            filter = AuditFilter()
        filter.limit = 10000  # Safety limit

        logs = await self.get_logs(team_id, filter)

        if format == "json":
            return self._export_json(logs)
        else:  # default to CSV
            return self._export_csv(logs)

    def _export_csv(self, logs: list[AuditLog]) -> bytes:
        """Export logs to CSV format."""
        output = io.StringIO()
        writer = csv.writer(output)

        # Header
        writer.writerow([
            "id",
            "created_at",
            "actor_type",
            "actor_id",
            "actor_email",
            "action",
            "resource_type",
            "resource_id",
            "ip_address",
            "user_agent",
            "old_value",
            "new_value",
            "hash",
        ])

        # Data
        for log in logs:
            writer.writerow([
                log.id,
                log.created_at.isoformat() if log.created_at else "",
                log.actor_type.value if isinstance(log.actor_type, ActorType) else log.actor_type,
                log.actor_id,
                log.actor_email or "",
                log.action,
                log.resource_type,
                log.resource_id,
                log.ip_address or "",
                log.user_agent or "",
                json.dumps(log.old_value) if log.old_value else "",
                json.dumps(log.new_value) if log.new_value else "",
                log.hash or "",
            ])

        return output.getvalue().encode("utf-8")

    def _export_json(self, logs: list[AuditLog]) -> bytes:
        """Export logs to JSON format."""
        data = []
        for log in logs:
            data.append({
                "id": log.id,
                "created_at": log.created_at.isoformat() if log.created_at else None,
                "actor_type": log.actor_type.value if isinstance(log.actor_type, ActorType) else log.actor_type,
                "actor_id": log.actor_id,
                "actor_email": log.actor_email,
                "action": log.action,
                "resource_type": log.resource_type,
                "resource_id": log.resource_id,
                "ip_address": log.ip_address,
                "user_agent": log.user_agent,
                "old_value": log.old_value,
                "new_value": log.new_value,
                "hash": log.hash,
            })

        return json.dumps(data, indent=2).encode("utf-8")

    # =========================================================================
    # Verification
    # =========================================================================

    def verify_hash(self, log: AuditLog) -> bool:
        """Verify audit log hash for integrity check.

        Args:
            log: AuditLog to verify

        Returns:
            True if hash matches, False otherwise
        """
        if not log.hash or not log.created_at:
            return False

        computed = self._compute_hash(
            log.id,
            log.actor_id,
            log.action,
            log.resource_id,
            log.created_at.isoformat(),
        )

        return computed == log.hash

    def _compute_hash(
        self,
        id: str,
        actor_id: str,
        action: str,
        resource_id: str,
        created_at: str,
    ) -> str:
        """Compute SHA256 hash for audit log."""
        data = f"{id}{actor_id}{action}{resource_id}{created_at}"
        return hashlib.sha256(data.encode()).hexdigest()

    # =========================================================================
    # Helpers
    # =========================================================================

    def _row_to_audit_log(self, row: dict[str, Any]) -> AuditLog:
        """Convert database row to AuditLog entity."""
        actor_type_str = row.get("actor_type", "user")
        try:
            actor_type = ActorType(actor_type_str)
        except ValueError:
            actor_type = ActorType.USER

        return AuditLog(
            id=row["id"],
            team_id=row["team_id"],
            actor_type=actor_type,
            actor_id=row["actor_id"],
            actor_email=row.get("actor_email"),
            action=row["action"],
            resource_type=row["resource_type"],
            resource_id=row["resource_id"],
            old_value=row.get("old_value"),
            new_value=row.get("new_value"),
            ip_address=row.get("ip_address"),
            user_agent=row.get("user_agent"),
            request_id=row.get("request_id"),
            created_at=_parse_datetime(row.get("created_at")),
            hash=row.get("hash"),
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


def create_audit_logger(
    url: str | None = None,
    key: str | None = None,
) -> AuditLogger:
    """Create AuditLogger instance.

    Args:
        url: Supabase project URL
        key: Supabase anon key

    Returns:
        Configured AuditLogger instance
    """
    return AuditLogger(url=url, key=key)
