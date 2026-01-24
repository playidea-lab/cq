"""Team Management API Routes.

Provides endpoints for managing teams and team members:
- POST /teams                             - Create new team
- GET  /teams                             - List user's teams
- GET  /teams/{team_id}                   - Get team details
- PATCH /teams/{team_id}                  - Update team
- DELETE /teams/{team_id}                 - Delete team
- GET  /teams/{team_id}/members           - List team members
- POST /teams/{team_id}/members           - Invite member
- PATCH /teams/{team_id}/members/{id}     - Update member role
- DELETE /teams/{team_id}/members/{id}    - Remove member
- GET  /invites/{token}                   - Get invite details
- POST /invites/{token}/accept            - Accept invite

Security:
- All endpoints require authentication (JWT or API key)
- Team operations require appropriate role permissions (RBAC)
"""

from __future__ import annotations

import logging
from typing import Annotated, Any

from fastapi import APIRouter, Depends, HTTPException, status

from c4.services.activity import ActivityCollector, create_activity_collector
from c4.services.audit import (
    ActorType,
    AuditAction,
    AuditLogger,
    create_audit_logger,
)
from c4.services.teams import (
    DuplicateMemberError,
    DuplicateSlugError,
    InviteExpiredError,
    InviteNotFoundError,
    MemberNotFoundError,
    TeamNotFoundError,
    TeamPermissionError,
    TeamRole,
    TeamService,
    create_team_service,
)

from ..auth import User, get_current_user
from ..models import (
    TeamCreateRequest,
    TeamInviteRequest,
    TeamMemberResponse,
    TeamMemberUpdateRequest,
    TeamResponse,
    TeamUpdateRequest,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/teams", tags=["Teams"])


# ============================================================================
# Dependencies
# ============================================================================


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
TeamSvc = Annotated[TeamService, Depends(get_team_service)]
AuditLog = Annotated[AuditLogger, Depends(get_audit_logger)]
Activity = Annotated[ActivityCollector, Depends(get_activity_collector)]


# ============================================================================
# Team CRUD Endpoints
# ============================================================================


@router.post(
    "",
    response_model=TeamResponse,
    status_code=status.HTTP_201_CREATED,
    summary="Create Team",
    description="Create a new team. The authenticated user becomes the owner.",
)
async def create_team(
    request: TeamCreateRequest,
    user: CurrentUser,
    service: TeamSvc,
    audit: AuditLog,
    activity: Activity,
) -> TeamResponse:
    """Create a new team.

    The authenticated user automatically becomes the team owner.

    Args:
        request: Team creation data
        user: Current authenticated user
        service: Team service
        audit: Audit logger

    Returns:
        Created team details

    Raises:
        HTTPException: 409 if slug already exists
    """
    try:
        team = await service.create_team(
            owner_id=user.user_id,
            name=request.name,
            slug=request.slug,
            settings=request.settings,
        )

        # Audit log: Team created
        await audit.log(
            team_id=team.id,
            action=AuditAction.TEAM_CREATED,
            resource_type="team",
            resource_id=team.id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            new_value={"name": team.name, "slug": team.slug},
        )

        # Activity tracking
        await activity.log_activity(
            team_id=team.id,
            activity_type="team_created",
            user_id=user.user_id,
            resource_type="team",
            resource_id=team.id,
            metadata={"name": team.name, "slug": team.slug},
        )

        return TeamResponse(
            id=team.id,
            name=team.name,
            slug=team.slug,
            owner_id=team.owner_id,
            plan=team.plan.value,
            settings=team.settings or {},
            created_at=team.created_at,
            updated_at=team.updated_at,
        )

    except DuplicateSlugError as e:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=str(e),
        ) from e


@router.get(
    "",
    response_model=list[TeamResponse],
    summary="List Teams",
    description="Get all teams the authenticated user belongs to.",
)
async def list_teams(
    user: CurrentUser,
    service: TeamSvc,
) -> list[TeamResponse]:
    """List all teams the user belongs to.

    Args:
        user: Current authenticated user
        service: Team service

    Returns:
        List of teams
    """
    teams = await service.get_user_teams(user.user_id)

    return [
        TeamResponse(
            id=team.id,
            name=team.name,
            slug=team.slug,
            owner_id=team.owner_id,
            plan=team.plan.value,
            settings=team.settings or {},
            created_at=team.created_at,
            updated_at=team.updated_at,
        )
        for team in teams
    ]


@router.get(
    "/{team_id}",
    response_model=TeamResponse,
    summary="Get Team",
    description="Get team details by ID.",
)
async def get_team(
    team_id: str,
    user: CurrentUser,
    service: TeamSvc,
) -> TeamResponse:
    """Get team details.

    Requires the user to be a member of the team.

    Args:
        team_id: Team identifier
        user: Current authenticated user
        service: Team service

    Returns:
        Team details

    Raises:
        HTTPException: 404 if not found, 403 if not a member
    """
    # Check membership
    member = await service.get_member(team_id, user.user_id)
    if not member:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Not a member of this team",
        )

    try:
        team = await service.get_team(team_id)
        return TeamResponse(
            id=team.id,
            name=team.name,
            slug=team.slug,
            owner_id=team.owner_id,
            plan=team.plan.value,
            settings=team.settings or {},
            created_at=team.created_at,
            updated_at=team.updated_at,
        )
    except TeamNotFoundError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e),
        ) from e


@router.patch(
    "/{team_id}",
    response_model=TeamResponse,
    summary="Update Team",
    description="Update team details. Requires admin or owner role.",
)
async def update_team(
    team_id: str,
    request: TeamUpdateRequest,
    user: CurrentUser,
    service: TeamSvc,
    audit: AuditLog,
    activity: Activity,
) -> TeamResponse:
    """Update team details.

    Requires admin or owner role.

    Args:
        team_id: Team identifier
        request: Update data
        user: Current authenticated user
        service: Team service
        audit: Audit logger

    Returns:
        Updated team details

    Raises:
        HTTPException: 404 if not found, 403 if not authorized
    """
    try:
        await service.require_permission(team_id, user.user_id, "manage_settings")

        # Get old values for audit
        old_team = await service.get_team(team_id)
        old_value = {"name": old_team.name, "settings": old_team.settings}

        team = await service.update_team(
            team_id=team_id,
            name=request.name,
            settings=request.settings,
        )

        # Audit log: Team updated
        await audit.log(
            team_id=team_id,
            action=AuditAction.TEAM_UPDATED,
            resource_type="team",
            resource_id=team_id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            old_value=old_value,
            new_value={"name": team.name, "settings": team.settings},
        )

        # Activity tracking
        await activity.log_activity(
            team_id=team_id,
            activity_type="team_updated",
            user_id=user.user_id,
            resource_type="team",
            resource_id=team_id,
            metadata={"name": team.name},
        )

        return TeamResponse(
            id=team.id,
            name=team.name,
            slug=team.slug,
            owner_id=team.owner_id,
            plan=team.plan.value,
            settings=team.settings or {},
            created_at=team.created_at,
            updated_at=team.updated_at,
        )

    except TeamPermissionError as e:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=str(e),
        ) from e
    except TeamNotFoundError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e),
        ) from e


@router.delete(
    "/{team_id}",
    status_code=status.HTTP_204_NO_CONTENT,
    summary="Delete Team",
    description="Delete a team. Only the owner can delete a team.",
)
async def delete_team(
    team_id: str,
    user: CurrentUser,
    service: TeamSvc,
    audit: AuditLog,
    activity: Activity,
) -> None:
    """Delete a team.

    Only the team owner can delete a team.
    This cascades to delete all members and invites.

    Args:
        team_id: Team identifier
        user: Current authenticated user
        service: Team service
        audit: Audit logger

    Raises:
        HTTPException: 404 if not found, 403 if not owner
    """
    try:
        await service.require_permission(team_id, user.user_id, "delete_team")

        # Get team info for audit before deletion
        team = await service.get_team(team_id)
        old_value = {"name": team.name, "slug": team.slug}

        await service.delete_team(team_id)

        # Audit log: Team deleted
        await audit.log(
            team_id=team_id,
            action=AuditAction.TEAM_DELETED,
            resource_type="team",
            resource_id=team_id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            old_value=old_value,
        )

        # Activity tracking
        await activity.log_activity(
            team_id=team_id,
            activity_type="team_deleted",
            user_id=user.user_id,
            resource_type="team",
            resource_id=team_id,
            metadata=old_value,
        )

    except TeamPermissionError as e:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=str(e),
        ) from e
    except TeamNotFoundError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e),
        ) from e


# ============================================================================
# Member Management Endpoints
# ============================================================================


@router.get(
    "/{team_id}/members",
    response_model=list[TeamMemberResponse],
    summary="List Team Members",
    description="Get all members of a team.",
)
async def list_members(
    team_id: str,
    user: CurrentUser,
    service: TeamSvc,
) -> list[TeamMemberResponse]:
    """List all team members.

    Requires membership in the team.

    Args:
        team_id: Team identifier
        user: Current authenticated user
        service: Team service

    Returns:
        List of team members

    Raises:
        HTTPException: 403 if not a member
    """
    # Check membership
    member = await service.get_member(team_id, user.user_id)
    if not member:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Not a member of this team",
        )

    members = await service.get_team_members(team_id)

    return [
        TeamMemberResponse(
            id=m.id,
            team_id=m.team_id,
            user_id=m.user_id,
            role=m.role.value,
            email=m.email,
            joined_at=m.joined_at,
        )
        for m in members
    ]


@router.post(
    "/{team_id}/members",
    response_model=dict[str, Any],
    status_code=status.HTTP_201_CREATED,
    summary="Invite Member",
    description="Invite a user to join the team by email.",
)
async def invite_member(
    team_id: str,
    request: TeamInviteRequest,
    user: CurrentUser,
    service: TeamSvc,
    audit: AuditLog,
    activity: Activity,
) -> dict[str, Any]:
    """Invite a user to join the team.

    Sends an invitation that the user can accept.
    Requires admin or owner role.

    Args:
        team_id: Team identifier
        request: Invite data (email, role)
        user: Current authenticated user
        service: Team service
        audit: Audit logger

    Returns:
        Invite details including token

    Raises:
        HTTPException: 403 if not authorized, 409 if already member
    """
    try:
        role = TeamRole(request.role) if request.role else TeamRole.MEMBER

        invite = await service.create_invite(
            team_id=team_id,
            email=request.email,
            role=role,
            invited_by=user.user_id,
        )

        # Audit log: Member invited
        await audit.log(
            team_id=team_id,
            action=AuditAction.MEMBER_INVITED,
            resource_type="invite",
            resource_id=invite.id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            new_value={"email": invite.email, "role": role.value},
        )

        # Activity tracking
        await activity.log_activity(
            team_id=team_id,
            activity_type="member_invited",
            user_id=user.user_id,
            resource_type="invite",
            resource_id=invite.id,
            metadata={"email": invite.email, "role": role.value},
        )

        return {
            "id": invite.id,
            "team_id": invite.team_id,
            "email": invite.email,
            "role": invite.role.value,
            "status": invite.status.value,
            "token": invite.token,
            "expires_at": invite.expires_at.isoformat() if invite.expires_at else None,
        }

    except TeamNotFoundError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e),
        ) from e
    except TeamPermissionError as e:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=str(e),
        ) from e
    except DuplicateMemberError as e:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=str(e),
        ) from e


@router.get(
    "/{team_id}/members/{member_id}",
    response_model=TeamMemberResponse,
    summary="Get Member",
    description="Get details of a specific team member.",
)
async def get_member(
    team_id: str,
    member_id: str,
    user: CurrentUser,
    service: TeamSvc,
) -> TeamMemberResponse:
    """Get team member details.

    Args:
        team_id: Team identifier
        member_id: Member record identifier
        user: Current authenticated user
        service: Team service

    Returns:
        Member details

    Raises:
        HTTPException: 403 if not authorized, 404 if not found
    """
    # Check membership
    actor = await service.get_member(team_id, user.user_id)
    if not actor:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Not a member of this team",
        )

    member = await service.get_member_by_id(member_id)
    if not member or member.team_id != team_id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=f"Member not found: {member_id}",
        )

    return TeamMemberResponse(
        id=member.id,
        team_id=member.team_id,
        user_id=member.user_id,
        role=member.role.value,
        email=member.email,
        joined_at=member.joined_at,
    )


@router.patch(
    "/{team_id}/members/{member_id}",
    response_model=TeamMemberResponse,
    summary="Update Member Role",
    description="Update a team member's role. Requires admin or owner role.",
)
async def update_member_role(
    team_id: str,
    member_id: str,
    request: TeamMemberUpdateRequest,
    user: CurrentUser,
    service: TeamSvc,
    audit: AuditLog,
    activity: Activity,
) -> TeamMemberResponse:
    """Update a member's role.

    Requires admin or owner role.
    Cannot demote the team owner.

    Args:
        team_id: Team identifier
        member_id: Member record identifier
        request: New role
        user: Current authenticated user
        service: Team service
        audit: Audit logger

    Returns:
        Updated member details

    Raises:
        HTTPException: 403 if not authorized, 404 if not found
    """
    try:
        new_role = TeamRole(request.role)

        # Get old role for audit
        old_member = await service.get_member_by_id(member_id)
        old_role = old_member.role.value if old_member else None

        member = await service.update_member_role(
            team_id=team_id,
            member_id=member_id,
            new_role=new_role,
            actor_id=user.user_id,
        )

        # Audit log: Member role changed
        await audit.log(
            team_id=team_id,
            action=AuditAction.MEMBER_ROLE_CHANGED,
            resource_type="member",
            resource_id=member_id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            old_value={"role": old_role, "user_id": member.user_id},
            new_value={"role": new_role.value, "user_id": member.user_id},
        )

        # Activity tracking
        await activity.log_activity(
            team_id=team_id,
            activity_type="member_role_changed",
            user_id=user.user_id,
            resource_type="member",
            resource_id=member_id,
            metadata={"old_role": old_role, "new_role": new_role.value},
        )

        return TeamMemberResponse(
            id=member.id,
            team_id=member.team_id,
            user_id=member.user_id,
            role=member.role.value,
            email=member.email,
            joined_at=member.joined_at,
        )

    except TeamPermissionError as e:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=str(e),
        ) from e
    except MemberNotFoundError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e),
        ) from e


@router.delete(
    "/{team_id}/members/{member_id}",
    status_code=status.HTTP_204_NO_CONTENT,
    summary="Remove Member",
    description="Remove a member from the team. Requires admin or owner role.",
)
async def remove_member(
    team_id: str,
    member_id: str,
    user: CurrentUser,
    service: TeamSvc,
    audit: AuditLog,
    activity: Activity,
) -> None:
    """Remove a member from the team.

    Requires admin or owner role, or the member themselves.
    Cannot remove the team owner.

    Args:
        team_id: Team identifier
        member_id: Member record identifier
        user: Current authenticated user
        service: Team service
        audit: Audit logger

    Raises:
        HTTPException: 403 if not authorized, 404 if not found
    """
    try:
        # Get member info for audit before removal
        member = await service.get_member_by_id(member_id)
        old_value = {
            "user_id": member.user_id if member else None,
            "email": member.email if member else None,
            "role": member.role.value if member else None,
        }

        await service.remove_member(
            team_id=team_id,
            member_id=member_id,
            actor_id=user.user_id,
        )

        # Audit log: Member removed
        await audit.log(
            team_id=team_id,
            action=AuditAction.MEMBER_REMOVED,
            resource_type="member",
            resource_id=member_id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            old_value=old_value,
        )

        # Activity tracking
        await activity.log_activity(
            team_id=team_id,
            activity_type="member_removed",
            user_id=user.user_id,
            resource_type="member",
            resource_id=member_id,
            metadata=old_value,
        )

    except TeamPermissionError as e:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=str(e),
        ) from e
    except MemberNotFoundError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e),
        ) from e


# ============================================================================
# Invite Endpoints
# ============================================================================


@router.get(
    "/{team_id}/invites",
    response_model=list[dict[str, Any]],
    summary="List Pending Invites",
    description="Get all pending invitations for a team.",
)
async def list_invites(
    team_id: str,
    user: CurrentUser,
    service: TeamSvc,
) -> list[dict[str, Any]]:
    """List pending invitations.

    Requires admin or owner role.

    Args:
        team_id: Team identifier
        user: Current authenticated user
        service: Team service

    Returns:
        List of pending invitations

    Raises:
        HTTPException: 403 if not authorized
    """
    try:
        await service.require_permission(team_id, user.user_id, "manage_members")
        invites = await service.get_pending_invites(team_id)

        return [
            {
                "id": inv.id,
                "team_id": inv.team_id,
                "email": inv.email,
                "role": inv.role.value,
                "status": inv.status.value,
                "invited_by": inv.invited_by,
                "invited_at": inv.invited_at.isoformat() if inv.invited_at else None,
                "expires_at": inv.expires_at.isoformat() if inv.expires_at else None,
            }
            for inv in invites
        ]

    except TeamPermissionError as e:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=str(e),
        ) from e


@router.delete(
    "/{team_id}/invites/{invite_id}",
    status_code=status.HTTP_204_NO_CONTENT,
    summary="Cancel Invite",
    description="Cancel a pending invitation.",
)
async def cancel_invite(
    team_id: str,
    invite_id: str,
    user: CurrentUser,
    service: TeamSvc,
) -> None:
    """Cancel a pending invitation.

    Requires admin or owner role.

    Args:
        team_id: Team identifier
        invite_id: Invite identifier
        user: Current authenticated user
        service: Team service

    Raises:
        HTTPException: 403 if not authorized, 404 if not found
    """
    try:
        await service.require_permission(team_id, user.user_id, "manage_members")
        cancelled = await service.cancel_invite(team_id, invite_id)

        if not cancelled:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Invite not found: {invite_id}",
            )

    except TeamPermissionError as e:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=str(e),
        ) from e


# ============================================================================
# Invite Acceptance (Public with token)
# ============================================================================


# Separate router for invite acceptance (no team_id prefix)
invite_router = APIRouter(prefix="/invites", tags=["Invites"])


@invite_router.get(
    "/{token}",
    response_model=dict[str, Any],
    summary="Get Invite Details",
    description="Get invitation details by token. Used to show invite info before accepting.",
)
async def get_invite(
    token: str,
    service: TeamSvc,
) -> dict[str, Any]:
    """Get invitation details by token.

    This endpoint does not require authentication.
    Used to display invite info before the user logs in/signs up.

    Args:
        token: Invite token
        service: Team service

    Returns:
        Invite details (without sensitive data)

    Raises:
        HTTPException: 404 if not found, 410 if expired
    """
    try:
        invite = await service.get_invite_by_token(token)

        # Get team name
        team = await service.get_team(invite.team_id)

        return {
            "email": invite.email,
            "role": invite.role.value,
            "team_name": team.name,
            "team_slug": team.slug,
            "expires_at": invite.expires_at.isoformat() if invite.expires_at else None,
        }

    except InviteNotFoundError:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Invite not found or invalid",
        )
    except InviteExpiredError:
        raise HTTPException(
            status_code=status.HTTP_410_GONE,
            detail="Invite has expired",
        )


@invite_router.post(
    "/{token}/accept",
    response_model=TeamMemberResponse,
    summary="Accept Invite",
    description="Accept an invitation to join a team.",
)
async def accept_invite(
    token: str,
    user: CurrentUser,
    service: TeamSvc,
    audit: AuditLog,
    activity: Activity,
) -> TeamMemberResponse:
    """Accept an invitation to join a team.

    Requires authentication. The user's email must match the invite email.

    Args:
        token: Invite token
        user: Current authenticated user
        service: Team service
        audit: Audit logger

    Returns:
        New team membership details

    Raises:
        HTTPException: 403 if email mismatch, 404 if not found, 409 if already member
    """
    try:
        member = await service.accept_invite(
            token=token,
            user_id=user.user_id,
            user_email=user.email,
        )

        # Audit log: Member joined
        await audit.log(
            team_id=member.team_id,
            action=AuditAction.MEMBER_JOINED,
            resource_type="member",
            resource_id=member.id,
            actor_type=ActorType.USER,
            actor_id=user.user_id,
            actor_email=user.email,
            new_value={"role": member.role.value, "email": member.email},
        )

        # Activity tracking
        await activity.log_activity(
            team_id=member.team_id,
            activity_type="member_joined",
            user_id=user.user_id,
            resource_type="member",
            resource_id=member.id,
            metadata={"role": member.role.value, "email": member.email},
        )

        return TeamMemberResponse(
            id=member.id,
            team_id=member.team_id,
            user_id=member.user_id,
            role=member.role.value,
            email=member.email,
            joined_at=member.joined_at,
        )

    except InviteNotFoundError:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Invite not found or invalid",
        )
    except InviteExpiredError:
        raise HTTPException(
            status_code=status.HTTP_410_GONE,
            detail="Invite has expired",
        )
    except TeamPermissionError as e:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=str(e),
        )
    except DuplicateMemberError as e:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=str(e),
        )
