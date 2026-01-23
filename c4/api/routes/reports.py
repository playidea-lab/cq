"""Reports API Routes.

Provides endpoints for usage reports and audit logs.

Endpoints:
    GET  /reports/teams/{team_id}/usage           - Get usage report
    GET  /reports/teams/{team_id}/audit           - Get audit logs
    GET  /reports/teams/{team_id}/audit/export    - Export audit logs
"""

from __future__ import annotations

import logging
from datetime import date
from typing import Any

from fastapi import APIRouter, Depends, HTTPException, Query, Response
from pydantic import BaseModel

from c4.services.activity import (
    ActivityCollector,
    create_activity_collector,
)
from c4.services.audit import (
    AuditFilter,
    AuditLogger,
    create_audit_logger,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/reports", tags=["Reports"])


# =============================================================================
# Request/Response Models
# =============================================================================


class UsageReportRequest(BaseModel):
    """Request for usage report."""

    start_date: date
    end_date: date


class UsageReportResponse(BaseModel):
    """Usage report response."""

    team_id: str
    start_date: date
    end_date: date
    total_activities: int
    total_duration_seconds: int
    by_type: dict[str, int]
    by_date: list[dict[str, Any]]


class ActivityLogResponse(BaseModel):
    """Activity log response."""

    id: str
    team_id: str
    activity_type: str
    user_id: str | None
    workspace_id: str | None
    resource_type: str | None
    resource_id: str | None
    metadata: dict[str, Any]
    started_at: str | None
    ended_at: str | None
    duration_seconds: int | None
    created_at: str | None


class AuditLogResponse(BaseModel):
    """Audit log response."""

    id: str
    team_id: str
    actor_type: str
    actor_id: str
    actor_email: str | None
    action: str
    resource_type: str
    resource_id: str
    old_value: dict[str, Any] | None
    new_value: dict[str, Any] | None
    ip_address: str | None
    user_agent: str | None
    request_id: str | None
    created_at: str | None
    hash: str | None


class AuditLogsResponse(BaseModel):
    """Paginated audit logs response."""

    team_id: str
    logs: list[AuditLogResponse]
    total: int
    limit: int
    offset: int


# =============================================================================
# Dependencies
# =============================================================================


def get_activity_collector() -> ActivityCollector:
    """Get ActivityCollector instance."""
    return create_activity_collector()


def get_audit_logger() -> AuditLogger:
    """Get AuditLogger instance."""
    return create_audit_logger()


# =============================================================================
# Usage Report Endpoints
# =============================================================================


@router.get("/teams/{team_id}/usage", response_model=UsageReportResponse)
async def get_usage_report(
    team_id: str,
    start_date: date = Query(..., description="Report start date"),
    end_date: date = Query(..., description="Report end date"),
    collector: ActivityCollector = Depends(get_activity_collector),
) -> UsageReportResponse:
    """Get usage report for a team.

    Returns aggregated activity statistics for the specified date range.

    Args:
        team_id: Team identifier
        start_date: Report start date
        end_date: Report end date

    Returns:
        Usage report with activity counts and durations
    """
    try:
        report = await collector.get_team_usage(team_id, start_date, end_date)

        return UsageReportResponse(
            team_id=report.team_id,
            start_date=report.start_date,
            end_date=report.end_date,
            total_activities=report.total_activities,
            total_duration_seconds=report.total_duration_seconds,
            by_type=report.by_type,
            by_date=[
                {
                    "date": summary.date.isoformat(),
                    "activity_type": summary.activity_type,
                    "activity_count": summary.activity_count,
                    "total_seconds": summary.total_seconds,
                    "unique_users": summary.unique_users,
                }
                for summary in report.by_date
            ],
        )
    except Exception as e:
        logger.error(f"Failed to get usage report: {e}")
        raise HTTPException(status_code=500, detail="Failed to get usage report")


@router.get("/teams/{team_id}/activities", response_model=list[ActivityLogResponse])
async def get_team_activities(
    team_id: str,
    start_date: date | None = Query(None, description="Filter from date"),
    end_date: date | None = Query(None, description="Filter to date"),
    activity_type: str | None = Query(None, description="Filter by activity type"),
    user_id: str | None = Query(None, description="Filter by user"),
    limit: int = Query(100, ge=1, le=1000, description="Maximum results"),
    collector: ActivityCollector = Depends(get_activity_collector),
) -> list[ActivityLogResponse]:
    """Get activity logs for a team.

    Args:
        team_id: Team identifier
        start_date: Filter from date
        end_date: Filter to date
        activity_type: Filter by activity type
        user_id: Filter by user
        limit: Maximum results

    Returns:
        List of activity logs
    """
    try:
        activities = await collector.get_team_activities(
            team_id,
            start_date=start_date,
            end_date=end_date,
            activity_type=activity_type,
            user_id=user_id,
            limit=limit,
        )

        return [
            ActivityLogResponse(
                id=a.id,
                team_id=a.team_id,
                activity_type=a.activity_type,
                user_id=a.user_id,
                workspace_id=a.workspace_id,
                resource_type=a.resource_type,
                resource_id=a.resource_id,
                metadata=a.metadata,
                started_at=a.started_at.isoformat() if a.started_at else None,
                ended_at=a.ended_at.isoformat() if a.ended_at else None,
                duration_seconds=a.duration_seconds,
                created_at=a.created_at.isoformat() if a.created_at else None,
            )
            for a in activities
        ]
    except Exception as e:
        logger.error(f"Failed to get activities: {e}")
        raise HTTPException(status_code=500, detail="Failed to get activities")


# =============================================================================
# Audit Log Endpoints
# =============================================================================


@router.get("/teams/{team_id}/audit", response_model=AuditLogsResponse)
async def get_audit_logs(
    team_id: str,
    actor_id: str | None = Query(None, description="Filter by actor"),
    action: str | None = Query(None, description="Filter by action"),
    resource_type: str | None = Query(None, description="Filter by resource type"),
    resource_id: str | None = Query(None, description="Filter by resource ID"),
    start_date: date | None = Query(None, description="Filter from date"),
    end_date: date | None = Query(None, description="Filter to date"),
    limit: int = Query(100, ge=1, le=1000, description="Maximum results"),
    offset: int = Query(0, ge=0, description="Offset for pagination"),
    audit_logger: AuditLogger = Depends(get_audit_logger),
) -> AuditLogsResponse:
    """Get audit logs for a team.

    Args:
        team_id: Team identifier
        actor_id: Filter by actor
        action: Filter by action
        resource_type: Filter by resource type
        resource_id: Filter by resource ID
        start_date: Filter from date
        end_date: Filter to date
        limit: Maximum results
        offset: Offset for pagination

    Returns:
        Paginated audit logs
    """
    try:
        filter = AuditFilter(
            actor_id=actor_id,
            action=action,
            resource_type=resource_type,
            resource_id=resource_id,
            start_date=start_date,
            end_date=end_date,
            limit=limit,
            offset=offset,
        )

        logs = await audit_logger.get_logs(team_id, filter)
        total = await audit_logger.get_log_count(team_id, filter)

        return AuditLogsResponse(
            team_id=team_id,
            logs=[
                AuditLogResponse(
                    id=log.id,
                    team_id=log.team_id,
                    actor_type=log.actor_type.value if hasattr(log.actor_type, 'value') else str(log.actor_type),
                    actor_id=log.actor_id,
                    actor_email=log.actor_email,
                    action=log.action,
                    resource_type=log.resource_type,
                    resource_id=log.resource_id,
                    old_value=log.old_value,
                    new_value=log.new_value,
                    ip_address=log.ip_address,
                    user_agent=log.user_agent,
                    request_id=log.request_id,
                    created_at=log.created_at.isoformat() if log.created_at else None,
                    hash=log.hash,
                )
                for log in logs
            ],
            total=total,
            limit=limit,
            offset=offset,
        )
    except Exception as e:
        logger.error(f"Failed to get audit logs: {e}")
        raise HTTPException(status_code=500, detail="Failed to get audit logs")


@router.get("/teams/{team_id}/audit/export")
async def export_audit_logs(
    team_id: str,
    format: str = Query("csv", description="Export format: csv or json"),
    actor_id: str | None = Query(None, description="Filter by actor"),
    action: str | None = Query(None, description="Filter by action"),
    resource_type: str | None = Query(None, description="Filter by resource type"),
    start_date: date | None = Query(None, description="Filter from date"),
    end_date: date | None = Query(None, description="Filter to date"),
    audit_logger: AuditLogger = Depends(get_audit_logger),
) -> Response:
    """Export audit logs in CSV or JSON format.

    Args:
        team_id: Team identifier
        format: Export format (csv or json)
        actor_id: Filter by actor
        action: Filter by action
        resource_type: Filter by resource type
        start_date: Filter from date
        end_date: Filter to date

    Returns:
        File download response
    """
    if format not in ("csv", "json"):
        raise HTTPException(status_code=400, detail="Format must be 'csv' or 'json'")

    try:
        filter = AuditFilter(
            actor_id=actor_id,
            action=action,
            resource_type=resource_type,
            start_date=start_date,
            end_date=end_date,
        )

        data = await audit_logger.export_logs(team_id, format, filter)

        if format == "csv":
            media_type = "text/csv"
            filename = f"audit_logs_{team_id}.csv"
        else:
            media_type = "application/json"
            filename = f"audit_logs_{team_id}.json"

        return Response(
            content=data,
            media_type=media_type,
            headers={
                "Content-Disposition": f'attachment; filename="{filename}"',
            },
        )
    except Exception as e:
        logger.error(f"Failed to export audit logs: {e}")
        raise HTTPException(status_code=500, detail="Failed to export audit logs")
