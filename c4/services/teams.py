"""Team Management Service.

Provides CRUD operations for teams, members, and invitations
using Supabase as the backend.

Environment Variables:
    SUPABASE_URL: Supabase project URL
    SUPABASE_KEY: Supabase anon/service key
    SUPABASE_SERVICE_KEY: Supabase service role key (for admin operations)
"""

from __future__ import annotations

import logging
import os
import re
import secrets
from dataclasses import dataclass
from datetime import datetime, timedelta
from enum import Enum
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from supabase import Client

    from c4.services.audit import AuditLogger

logger = logging.getLogger(__name__)


# =============================================================================
# Domain Types
# =============================================================================


class TeamRole(str, Enum):
    """Team member roles for RBAC."""

    OWNER = "owner"
    ADMIN = "admin"
    MEMBER = "member"
    VIEWER = "viewer"


class TeamPlan(str, Enum):
    """Team subscription plans."""

    FREE = "free"
    PRO = "pro"
    TEAM = "team"
    AGENCY = "agency"
    ENTERPRISE = "enterprise"


class InviteStatus(str, Enum):
    """Team invitation status."""

    PENDING = "pending"
    ACCEPTED = "accepted"
    EXPIRED = "expired"


@dataclass
class Team:
    """Team entity."""

    id: str
    name: str
    slug: str
    owner_id: str
    plan: TeamPlan = TeamPlan.FREE
    settings: dict[str, Any] | None = None
    stripe_customer_id: str | None = None
    created_at: datetime | None = None
    updated_at: datetime | None = None


@dataclass
class TeamMember:
    """Team member entity."""

    id: str
    team_id: str
    user_id: str
    role: TeamRole
    email: str | None = None
    joined_at: datetime | None = None


@dataclass
class TeamInvite:
    """Team invitation entity."""

    id: str
    team_id: str
    email: str
    role: TeamRole
    status: InviteStatus
    token: str
    invited_by: str
    invited_at: datetime
    expires_at: datetime


# =============================================================================
# Exceptions
# =============================================================================


class TeamError(Exception):
    """Base exception for team operations."""

    pass


class TeamNotFoundError(TeamError):
    """Team not found."""

    def __init__(self, team_id: str):
        super().__init__(f"Team not found: {team_id}")
        self.team_id = team_id


class TeamPermissionError(TeamError):
    """Permission denied for team operation."""

    def __init__(self, action: str, role: str | None = None):
        msg = f"Permission denied: {action}"
        if role:
            msg += f" (current role: {role})"
        super().__init__(msg)
        self.action = action


class MemberNotFoundError(TeamError):
    """Team member not found."""

    def __init__(self, member_id: str):
        super().__init__(f"Member not found: {member_id}")
        self.member_id = member_id


class InviteNotFoundError(TeamError):
    """Team invite not found."""

    def __init__(self, identifier: str):
        super().__init__(f"Invite not found: {identifier}")
        self.identifier = identifier


class InviteExpiredError(TeamError):
    """Team invite has expired."""

    def __init__(self, token: str):
        super().__init__("Invite has expired")
        self.token = token


class DuplicateMemberError(TeamError):
    """User is already a team member."""

    def __init__(self, email: str):
        super().__init__(f"User is already a team member: {email}")
        self.email = email


class DuplicateSlugError(TeamError):
    """Team slug already exists."""

    def __init__(self, slug: str):
        super().__init__(f"Team slug already exists: {slug}")
        self.slug = slug


# =============================================================================
# Team Service
# =============================================================================


class TeamService:
    """Service for managing teams, members, and invitations.

    Uses Supabase for persistent storage with RLS support.

    Example:
        service = TeamService()
        team = await service.create_team(
            owner_id="user-123",
            name="My Team",
        )
    """

    TABLE_TEAMS = "teams"
    TABLE_MEMBERS = "team_members"
    TABLE_INVITES = "team_invites"

    INVITE_EXPIRY_DAYS = 7

    # Role hierarchy: owner > admin > member > viewer
    ROLE_HIERARCHY = {
        TeamRole.OWNER: 4,
        TeamRole.ADMIN: 3,
        TeamRole.MEMBER: 2,
        TeamRole.VIEWER: 1,
    }

    def __init__(
        self,
        url: str | None = None,
        key: str | None = None,
        service_key: str | None = None,
        audit_logger: AuditLogger | None = None,
    ) -> None:
        """Initialize team service.

        Args:
            url: Supabase project URL (or SUPABASE_URL env)
            key: Supabase anon key (or SUPABASE_KEY env)
            service_key: Supabase service key for admin ops (or SUPABASE_SERVICE_KEY env)
            audit_logger: Optional audit logger for compliance logging
        """
        self._url = url or os.environ.get("SUPABASE_URL", "")
        self._key = key or os.environ.get("SUPABASE_KEY", "")
        self._service_key = service_key or os.environ.get("SUPABASE_SERVICE_KEY", "")
        self._client: Client | None = None
        self._service_client: Client | None = None
        self._audit_logger = audit_logger

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

    @property
    def service_client(self) -> Client:
        """Lazy-initialize Supabase service client (bypasses RLS)."""
        if self._service_client is None:
            if not self._url or not self._service_key:
                # Fall back to regular client if no service key
                return self.client
            from supabase import create_client

            self._service_client = create_client(self._url, self._service_key)
        return self._service_client

    # =========================================================================
    # Team CRUD
    # =========================================================================

    async def create_team(
        self,
        owner_id: str,
        name: str,
        slug: str | None = None,
        settings: dict[str, Any] | None = None,
    ) -> Team:
        """Create a new team.

        Args:
            owner_id: User ID of team owner
            name: Team display name
            slug: URL-friendly identifier (auto-generated if not provided)
            settings: Optional team settings

        Returns:
            Created Team entity

        Raises:
            DuplicateSlugError: If slug already exists
        """
        # Generate slug if not provided
        if not slug:
            slug = self._generate_slug(name)

        # Check slug uniqueness
        existing = (
            self.client.table(self.TABLE_TEAMS)
            .select("id")
            .eq("slug", slug)
            .maybe_single()
            .execute()
        )
        if existing.data:
            raise DuplicateSlugError(slug)

        # Create team
        team_data = {
            "name": name,
            "slug": slug,
            "owner_id": owner_id,
            "settings": settings or {},
        }

        response = self.client.table(self.TABLE_TEAMS).insert(team_data).execute()

        if not response.data:
            raise TeamError("Failed to create team")

        team = self._row_to_team(response.data[0])

        # Add owner as team member
        await self._add_member_internal(
            team_id=team.id,
            user_id=owner_id,
            role=TeamRole.OWNER,
        )

        # Audit log
        if self._audit_logger:
            await self._audit_logger.log(
                team_id=team.id,
                action="team.created",
                resource_type="team",
                resource_id=team.id,
                actor_id=owner_id,
                new_value={"name": name, "slug": slug, "settings": settings},
            )

        logger.info(f"Created team: {team.name} ({team.slug}) by {owner_id}")
        return team

    async def get_team(self, team_id: str) -> Team:
        """Get team by ID.

        Args:
            team_id: Team identifier

        Returns:
            Team entity

        Raises:
            TeamNotFoundError: If team not found
        """
        response = (
            self.client.table(self.TABLE_TEAMS)
            .select("*")
            .eq("id", team_id)
            .maybe_single()
            .execute()
        )

        if not response.data:
            raise TeamNotFoundError(team_id)

        return self._row_to_team(response.data)

    async def get_team_by_slug(self, slug: str) -> Team:
        """Get team by slug.

        Args:
            slug: Team slug

        Returns:
            Team entity

        Raises:
            TeamNotFoundError: If team not found
        """
        response = (
            self.client.table(self.TABLE_TEAMS)
            .select("*")
            .eq("slug", slug)
            .maybe_single()
            .execute()
        )

        if not response.data:
            raise TeamNotFoundError(slug)

        return self._row_to_team(response.data)

    async def update_team(
        self,
        team_id: str,
        name: str | None = None,
        settings: dict[str, Any] | None = None,
        actor_id: str | None = None,
    ) -> Team:
        """Update team details.

        Args:
            team_id: Team identifier
            name: New team name
            settings: Settings to merge
            actor_id: User performing the update (for audit)

        Returns:
            Updated Team entity
        """
        # Get current state for audit
        current = await self.get_team(team_id)
        old_value = {"name": current.name, "settings": current.settings}

        update_data: dict[str, Any] = {"updated_at": datetime.now().isoformat()}

        if name is not None:
            update_data["name"] = name

        if settings is not None:
            # Merge settings
            merged = {**(current.settings or {}), **settings}
            update_data["settings"] = merged

        response = (
            self.client.table(self.TABLE_TEAMS)
            .update(update_data)
            .eq("id", team_id)
            .execute()
        )

        if not response.data:
            raise TeamNotFoundError(team_id)

        team = self._row_to_team(response.data[0])

        # Audit log
        if self._audit_logger and actor_id:
            await self._audit_logger.log(
                team_id=team_id,
                action="team.updated",
                resource_type="team",
                resource_id=team_id,
                actor_id=actor_id,
                old_value=old_value,
                new_value={"name": team.name, "settings": team.settings},
            )

        return team

    async def delete_team(self, team_id: str, actor_id: str | None = None) -> bool:
        """Delete a team.

        This will cascade delete all members and invites.

        Args:
            team_id: Team identifier
            actor_id: User performing the deletion (for audit)

        Returns:
            True if deleted
        """
        # Get current state for audit before deletion
        old_value = None
        try:
            current = await self.get_team(team_id)
            old_value = {"name": current.name, "slug": current.slug}
        except Exception:
            # Don't fail deletion if we can't get team info for audit
            pass

        response = (
            self.client.table(self.TABLE_TEAMS)
            .delete()
            .eq("id", team_id)
            .execute()
        )

        deleted = len(response.data) > 0
        if deleted:
            # Audit log
            if self._audit_logger and actor_id and old_value:
                await self._audit_logger.log(
                    team_id=team_id,
                    action="team.deleted",
                    resource_type="team",
                    resource_id=team_id,
                    actor_id=actor_id,
                    old_value=old_value,
                )

            logger.info(f"Deleted team: {team_id}")

        return deleted

    async def get_user_teams(self, user_id: str) -> list[Team]:
        """Get all teams a user belongs to.

        Args:
            user_id: User identifier

        Returns:
            List of Team entities
        """
        # First get member records
        members_response = (
            self.client.table(self.TABLE_MEMBERS)
            .select("team_id")
            .eq("user_id", user_id)
            .execute()
        )

        if not members_response.data:
            return []

        team_ids = [m["team_id"] for m in members_response.data]

        # Then get teams
        teams_response = (
            self.client.table(self.TABLE_TEAMS)
            .select("*")
            .in_("id", team_ids)
            .execute()
        )

        return [self._row_to_team(row) for row in teams_response.data]

    # =========================================================================
    # Member Management
    # =========================================================================

    async def get_team_members(self, team_id: str) -> list[TeamMember]:
        """Get all members of a team.

        Args:
            team_id: Team identifier

        Returns:
            List of TeamMember entities
        """
        response = (
            self.client.table(self.TABLE_MEMBERS)
            .select("*")
            .eq("team_id", team_id)
            .execute()
        )

        return [self._row_to_member(row) for row in response.data]

    async def get_member(self, team_id: str, user_id: str) -> TeamMember | None:
        """Get a specific team member.

        Args:
            team_id: Team identifier
            user_id: User identifier

        Returns:
            TeamMember or None if not found
        """
        response = (
            self.client.table(self.TABLE_MEMBERS)
            .select("*")
            .eq("team_id", team_id)
            .eq("user_id", user_id)
            .maybe_single()
            .execute()
        )

        if not response.data:
            return None

        return self._row_to_member(response.data)

    async def get_member_by_id(self, member_id: str) -> TeamMember | None:
        """Get team member by member record ID.

        Args:
            member_id: Member record identifier

        Returns:
            TeamMember or None if not found
        """
        response = (
            self.client.table(self.TABLE_MEMBERS)
            .select("*")
            .eq("id", member_id)
            .maybe_single()
            .execute()
        )

        if not response.data:
            return None

        return self._row_to_member(response.data)

    async def update_member_role(
        self,
        team_id: str,
        member_id: str,
        new_role: TeamRole,
        actor_id: str,
    ) -> TeamMember:
        """Update a member's role.

        Args:
            team_id: Team identifier
            member_id: Member record ID
            new_role: New role to assign
            actor_id: User performing the action

        Returns:
            Updated TeamMember

        Raises:
            TeamPermissionError: If actor lacks permission
            MemberNotFoundError: If member not found
        """
        # Check actor's permission
        actor_member = await self.get_member(team_id, actor_id)
        if not actor_member:
            raise TeamPermissionError("update member role", None)

        if not self._can_modify_role(actor_member.role, new_role):
            raise TeamPermissionError("assign this role", actor_member.role.value)

        # Get target member
        target_member = await self.get_member_by_id(member_id)
        if not target_member or target_member.team_id != team_id:
            raise MemberNotFoundError(member_id)

        old_role = target_member.role

        # Cannot demote owner (must transfer ownership first)
        if old_role == TeamRole.OWNER and new_role != TeamRole.OWNER:
            raise TeamPermissionError("demote team owner")

        # Update role
        response = (
            self.client.table(self.TABLE_MEMBERS)
            .update({"role": new_role.value})
            .eq("id", member_id)
            .execute()
        )

        if not response.data:
            raise MemberNotFoundError(member_id)

        member = self._row_to_member(response.data[0])

        # Audit log
        if self._audit_logger:
            await self._audit_logger.log(
                team_id=team_id,
                action="member.role_changed",
                resource_type="team_member",
                resource_id=member_id,
                actor_id=actor_id,
                old_value={"role": old_role.value, "user_id": target_member.user_id},
                new_value={"role": new_role.value, "user_id": target_member.user_id},
            )

        logger.info(
            f"Updated member role: {member_id} -> {new_role.value} "
            f"in team {team_id} by {actor_id}"
        )

        return member

    async def remove_member(
        self,
        team_id: str,
        member_id: str,
        actor_id: str,
    ) -> bool:
        """Remove a member from a team.

        Args:
            team_id: Team identifier
            member_id: Member record ID
            actor_id: User performing the action

        Returns:
            True if removed

        Raises:
            TeamPermissionError: If actor lacks permission
        """
        # Check actor's permission
        actor_member = await self.get_member(team_id, actor_id)
        if not actor_member:
            raise TeamPermissionError("remove member", None)

        if actor_member.role not in (TeamRole.OWNER, TeamRole.ADMIN):
            # Members can remove themselves
            target = await self.get_member_by_id(member_id)
            if not target or target.user_id != actor_id:
                raise TeamPermissionError("remove member", actor_member.role.value)

        # Get target member
        target_member = await self.get_member_by_id(member_id)
        if not target_member or target_member.team_id != team_id:
            raise MemberNotFoundError(member_id)

        # Cannot remove owner
        if target_member.role == TeamRole.OWNER:
            raise TeamPermissionError("remove team owner")

        # Store info for audit before removal
        old_value = {
            "user_id": target_member.user_id,
            "role": target_member.role.value,
            "email": target_member.email,
        }

        # Remove member
        response = (
            self.client.table(self.TABLE_MEMBERS)
            .delete()
            .eq("id", member_id)
            .execute()
        )

        removed = len(response.data) > 0
        if removed:
            # Audit log
            if self._audit_logger:
                await self._audit_logger.log(
                    team_id=team_id,
                    action="member.removed",
                    resource_type="team_member",
                    resource_id=member_id,
                    actor_id=actor_id,
                    old_value=old_value,
                )

            logger.info(
                f"Removed member: {member_id} from team {team_id} by {actor_id}"
            )

        return removed

    async def _add_member_internal(
        self,
        team_id: str,
        user_id: str,
        role: TeamRole,
        email: str | None = None,
    ) -> TeamMember:
        """Internal: Add a member to a team (no permission checks)."""
        member_data = {
            "team_id": team_id,
            "user_id": user_id,
            "role": role.value,
        }

        response = (
            self.client.table(self.TABLE_MEMBERS)
            .insert(member_data)
            .execute()
        )

        if not response.data:
            raise TeamError(f"Failed to add member {user_id} to team {team_id}")

        member = self._row_to_member(response.data[0])
        member.email = email
        return member

    # =========================================================================
    # Invitation Management
    # =========================================================================

    async def create_invite(
        self,
        team_id: str,
        email: str,
        role: TeamRole,
        invited_by: str,
    ) -> TeamInvite:
        """Create an invitation to join a team.

        Args:
            team_id: Team identifier
            email: Email to invite
            role: Role to assign when accepted
            invited_by: User ID sending the invite

        Returns:
            TeamInvite entity

        Raises:
            DuplicateMemberError: If user is already a member
            TeamPermissionError: If inviter lacks permission
        """
        # Check inviter's permission
        inviter = await self.get_member(team_id, invited_by)
        if not inviter:
            raise TeamPermissionError("invite members", None)

        if not self._can_modify_role(inviter.role, role):
            raise TeamPermissionError(
                f"invite with role {role.value}",
                inviter.role.value,
            )

        # Check if email is already a member (would need user lookup)
        # For now, rely on accept_invite to catch duplicates

        # Generate token
        token = secrets.token_urlsafe(32)
        expires_at = datetime.now() + timedelta(days=self.INVITE_EXPIRY_DAYS)

        invite_data = {
            "team_id": team_id,
            "email": email.lower(),
            "role": role.value,
            "status": InviteStatus.PENDING.value,
            "token": token,
            "invited_by": invited_by,
            "expires_at": expires_at.isoformat(),
        }

        response = (
            self.client.table(self.TABLE_INVITES)
            .insert(invite_data)
            .execute()
        )

        if not response.data:
            raise TeamError(f"Failed to create invite for {email}")

        invite = self._row_to_invite(response.data[0])

        # Audit log
        if self._audit_logger:
            await self._audit_logger.log(
                team_id=team_id,
                action="member.invited",
                resource_type="team_invite",
                resource_id=invite.id,
                actor_id=invited_by,
                new_value={"email": email.lower(), "role": role.value},
            )

        logger.info(
            f"Created invite: {email} -> team {team_id} "
            f"as {role.value} by {invited_by}"
        )

        return invite

    async def get_invite_by_token(self, token: str) -> TeamInvite:
        """Get invitation by token.

        Args:
            token: Invite token

        Returns:
            TeamInvite entity

        Raises:
            InviteNotFoundError: If invite not found
            InviteExpiredError: If invite has expired
        """
        response = (
            self.service_client.table(self.TABLE_INVITES)
            .select("*")
            .eq("token", token)
            .maybe_single()
            .execute()
        )

        if not response.data:
            raise InviteNotFoundError(token)

        invite = self._row_to_invite(response.data)

        if invite.status == InviteStatus.EXPIRED:
            raise InviteExpiredError(token)

        if invite.expires_at < datetime.now():
            # Mark as expired
            self.service_client.table(self.TABLE_INVITES).update(
                {"status": InviteStatus.EXPIRED.value}
            ).eq("id", invite.id).execute()
            raise InviteExpiredError(token)

        return invite

    async def accept_invite(
        self,
        token: str,
        user_id: str,
        user_email: str,
    ) -> TeamMember:
        """Accept an invitation.

        Args:
            token: Invite token
            user_id: User accepting the invite
            user_email: User's email (for verification)

        Returns:
            Created TeamMember

        Raises:
            InviteNotFoundError: If invite not found
            InviteExpiredError: If invite has expired
            DuplicateMemberError: If user is already a member
        """
        invite = await self.get_invite_by_token(token)

        # Verify email matches
        if invite.email.lower() != user_email.lower():
            raise TeamPermissionError(
                "accept invite (email mismatch)",
                None,
            )

        # Check if already a member
        existing = await self.get_member(invite.team_id, user_id)
        if existing:
            raise DuplicateMemberError(user_email)

        # Add member
        member = await self._add_member_internal(
            team_id=invite.team_id,
            user_id=user_id,
            role=invite.role,
            email=user_email,
        )

        # Mark invite as accepted
        self.client.table(self.TABLE_INVITES).update(
            {"status": InviteStatus.ACCEPTED.value}
        ).eq("id", invite.id).execute()

        # Audit log
        if self._audit_logger:
            await self._audit_logger.log(
                team_id=invite.team_id,
                action="member.joined",
                resource_type="team_member",
                resource_id=member.id,
                actor_id=user_id,
                actor_email=user_email,
                new_value={"email": user_email, "role": invite.role.value},
            )

        logger.info(
            f"Invite accepted: {user_email} joined team {invite.team_id} "
            f"as {invite.role.value}"
        )

        return member

    async def get_pending_invites(self, team_id: str) -> list[TeamInvite]:
        """Get pending invites for a team.

        Args:
            team_id: Team identifier

        Returns:
            List of pending TeamInvite entities
        """
        response = (
            self.client.table(self.TABLE_INVITES)
            .select("*")
            .eq("team_id", team_id)
            .eq("status", InviteStatus.PENDING.value)
            .execute()
        )

        return [self._row_to_invite(row) for row in response.data]

    async def cancel_invite(self, team_id: str, invite_id: str) -> bool:
        """Cancel a pending invite.

        Args:
            team_id: Team identifier
            invite_id: Invite identifier

        Returns:
            True if cancelled
        """
        response = (
            self.client.table(self.TABLE_INVITES)
            .delete()
            .eq("id", invite_id)
            .eq("team_id", team_id)
            .eq("status", InviteStatus.PENDING.value)
            .execute()
        )

        return len(response.data) > 0

    # =========================================================================
    # Permission Checks
    # =========================================================================

    async def check_permission(
        self,
        team_id: str,
        user_id: str,
        action: str,
    ) -> bool:
        """Check if user has permission for an action.

        Args:
            team_id: Team identifier
            user_id: User identifier
            action: Action to check

        Returns:
            True if permitted
        """
        member = await self.get_member(team_id, user_id)
        if not member:
            return False

        # Define action -> required roles
        action_roles = {
            "view": (TeamRole.OWNER, TeamRole.ADMIN, TeamRole.MEMBER, TeamRole.VIEWER),
            "edit": (TeamRole.OWNER, TeamRole.ADMIN, TeamRole.MEMBER),
            "manage_members": (TeamRole.OWNER, TeamRole.ADMIN),
            "manage_integrations": (TeamRole.OWNER, TeamRole.ADMIN),
            "manage_settings": (TeamRole.OWNER, TeamRole.ADMIN),
            "delete_team": (TeamRole.OWNER,),
            "transfer_ownership": (TeamRole.OWNER,),
        }

        required = action_roles.get(action, ())
        return member.role in required

    async def require_permission(
        self,
        team_id: str,
        user_id: str,
        action: str,
    ) -> TeamMember:
        """Check permission and return member if granted.

        Args:
            team_id: Team identifier
            user_id: User identifier
            action: Action to check

        Returns:
            TeamMember if permitted

        Raises:
            TeamPermissionError: If not permitted
        """
        member = await self.get_member(team_id, user_id)
        if not member:
            raise TeamPermissionError(action, None)

        if not await self.check_permission(team_id, user_id, action):
            raise TeamPermissionError(action, member.role.value)

        return member

    def _can_modify_role(self, actor_role: TeamRole, target_role: TeamRole) -> bool:
        """Check if actor can assign/modify a role.

        Role hierarchy: owner > admin > member > viewer
        An actor can only assign roles below their own level.
        """
        actor_level = self.ROLE_HIERARCHY.get(actor_role, 0)
        target_level = self.ROLE_HIERARCHY.get(target_role, 0)
        return actor_level > target_level

    # =========================================================================
    # Helpers
    # =========================================================================

    def _generate_slug(self, name: str) -> str:
        """Generate URL-friendly slug from name."""
        # Convert to lowercase
        slug = name.lower()
        # Replace spaces and special chars with hyphens
        slug = re.sub(r"[^a-z0-9]+", "-", slug)
        # Remove leading/trailing hyphens
        slug = slug.strip("-")
        # Add random suffix for uniqueness
        slug = f"{slug}-{secrets.token_hex(4)}"
        return slug[:50]  # Max 50 chars

    def _row_to_team(self, row: dict[str, Any]) -> Team:
        """Convert database row to Team entity."""
        return Team(
            id=row["id"],
            name=row["name"],
            slug=row["slug"],
            owner_id=row["owner_id"],
            plan=TeamPlan(row.get("plan", "free")),
            settings=row.get("settings"),
            stripe_customer_id=row.get("stripe_customer_id"),
            created_at=_parse_datetime(row.get("created_at")),
            updated_at=_parse_datetime(row.get("updated_at")),
        )

    def _row_to_member(self, row: dict[str, Any]) -> TeamMember:
        """Convert database row to TeamMember entity."""
        return TeamMember(
            id=row["id"],
            team_id=row["team_id"],
            user_id=row["user_id"],
            role=TeamRole(row["role"]),
            email=row.get("email"),
            joined_at=_parse_datetime(row.get("joined_at")),
        )

    def _row_to_invite(self, row: dict[str, Any]) -> TeamInvite:
        """Convert database row to TeamInvite entity."""
        return TeamInvite(
            id=row["id"],
            team_id=row["team_id"],
            email=row["email"],
            role=TeamRole(row["role"]),
            status=InviteStatus(row["status"]),
            token=row["token"],
            invited_by=row["invited_by"],
            invited_at=_parse_datetime(row["invited_at"]) or datetime.now(),
            expires_at=_parse_datetime(row["expires_at"]) or datetime.now(),
        )


def _parse_datetime(value: str | datetime | None) -> datetime | None:
    """Parse datetime from string or return as-is."""
    if value is None:
        return None
    if isinstance(value, datetime):
        return value
    try:
        # Handle ISO format with or without timezone
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except (ValueError, AttributeError):
        return None


# =============================================================================
# Factory Function
# =============================================================================


def create_team_service(
    url: str | None = None,
    key: str | None = None,
    service_key: str | None = None,
) -> TeamService:
    """Create TeamService instance.

    Args:
        url: Supabase project URL
        key: Supabase anon key
        service_key: Supabase service key for admin ops

    Returns:
        Configured TeamService instance
    """
    return TeamService(url=url, key=key, service_key=service_key)
