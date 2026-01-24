/**
 * C4 Teams API Client
 *
 * Provides type-safe API calls for team management, member management,
 * and invitation flows.
 */

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:4000';

// =============================================================================
// Enums
// =============================================================================

export enum TeamRole {
  OWNER = 'owner',
  ADMIN = 'admin',
  MEMBER = 'member',
  VIEWER = 'viewer',
}

export enum TeamPlan {
  FREE = 'free',
  PRO = 'pro',
  TEAM = 'team',
  AGENCY = 'agency',
  ENTERPRISE = 'enterprise',
}

export enum InviteStatus {
  PENDING = 'pending',
  ACCEPTED = 'accepted',
  EXPIRED = 'expired',
}

// =============================================================================
// Types
// =============================================================================

export interface Team {
  id: string;
  name: string;
  slug: string;
  owner_id: string;
  plan: TeamPlan;
  settings: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface TeamMember {
  id: string;
  team_id: string;
  user_id: string;
  email: string | null;
  role: TeamRole;
  joined_at: string;
}

export interface TeamInvite {
  id: string;
  team_id: string;
  email: string;
  role: TeamRole;
  status: InviteStatus;
  token: string;
  invited_by: string;
  invited_at: string;
  expires_at: string | null;
}

export interface TeamInviteDetails {
  id: string;
  team_id: string;
  team_name: string;
  email: string;
  role: TeamRole;
  status: InviteStatus;
  expires_at: string | null;
  inviter_email: string | null;
}

// =============================================================================
// Request Types
// =============================================================================

export interface CreateTeamRequest {
  name: string;
  slug?: string;
  settings?: Record<string, unknown>;
}

export interface UpdateTeamRequest {
  name?: string;
  settings?: Record<string, unknown>;
}

export interface InviteMemberRequest {
  email: string;
  role?: TeamRole;
}

export interface UpdateMemberRoleRequest {
  role: TeamRole;
}

// =============================================================================
// API Client Options
// =============================================================================

interface ApiOptions {
  apiKey?: string;
}

// =============================================================================
// Helper Functions
// =============================================================================

function getHeaders(options?: ApiOptions): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  if (options?.apiKey) {
    headers['X-API-Key'] = options.apiKey;
  }

  return headers;
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error = await response.text();
    throw new Error(`API error ${response.status}: ${error}`);
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as T;
  }

  return response.json();
}

// =============================================================================
// Team CRUD Operations
// =============================================================================

/**
 * Create a new team
 */
export async function createTeam(
  data: CreateTeamRequest,
  options?: ApiOptions
): Promise<Team> {
  const response = await fetch(`${API_BASE}/api/teams`, {
    method: 'POST',
    headers: getHeaders(options),
    body: JSON.stringify(data),
  });

  return handleResponse<Team>(response);
}

/**
 * List all teams the current user belongs to
 */
export async function listTeams(options?: ApiOptions): Promise<Team[]> {
  const response = await fetch(`${API_BASE}/api/teams`, {
    method: 'GET',
    headers: getHeaders(options),
  });

  return handleResponse<Team[]>(response);
}

/**
 * Get team details by ID
 */
export async function getTeam(
  teamId: string,
  options?: ApiOptions
): Promise<Team> {
  const response = await fetch(`${API_BASE}/api/teams/${teamId}`, {
    method: 'GET',
    headers: getHeaders(options),
  });

  return handleResponse<Team>(response);
}

/**
 * Update team settings
 */
export async function updateTeam(
  teamId: string,
  data: UpdateTeamRequest,
  options?: ApiOptions
): Promise<Team> {
  const response = await fetch(`${API_BASE}/api/teams/${teamId}`, {
    method: 'PATCH',
    headers: getHeaders(options),
    body: JSON.stringify(data),
  });

  return handleResponse<Team>(response);
}

/**
 * Delete a team (owner only)
 */
export async function deleteTeam(
  teamId: string,
  options?: ApiOptions
): Promise<void> {
  const response = await fetch(`${API_BASE}/api/teams/${teamId}`, {
    method: 'DELETE',
    headers: getHeaders(options),
  });

  return handleResponse<void>(response);
}

// =============================================================================
// Member Management
// =============================================================================

/**
 * List all members of a team
 */
export async function listMembers(
  teamId: string,
  options?: ApiOptions
): Promise<TeamMember[]> {
  const response = await fetch(`${API_BASE}/api/teams/${teamId}/members`, {
    method: 'GET',
    headers: getHeaders(options),
  });

  return handleResponse<TeamMember[]>(response);
}

/**
 * Get a specific member's details
 */
export async function getMember(
  teamId: string,
  memberId: string,
  options?: ApiOptions
): Promise<TeamMember> {
  const response = await fetch(
    `${API_BASE}/api/teams/${teamId}/members/${memberId}`,
    {
      method: 'GET',
      headers: getHeaders(options),
    }
  );

  return handleResponse<TeamMember>(response);
}

/**
 * Update a member's role (admin/owner only)
 */
export async function updateMemberRole(
  teamId: string,
  memberId: string,
  data: UpdateMemberRoleRequest,
  options?: ApiOptions
): Promise<TeamMember> {
  const response = await fetch(
    `${API_BASE}/api/teams/${teamId}/members/${memberId}`,
    {
      method: 'PATCH',
      headers: getHeaders(options),
      body: JSON.stringify(data),
    }
  );

  return handleResponse<TeamMember>(response);
}

/**
 * Remove a member from the team (admin/owner only)
 */
export async function removeMember(
  teamId: string,
  memberId: string,
  options?: ApiOptions
): Promise<void> {
  const response = await fetch(
    `${API_BASE}/api/teams/${teamId}/members/${memberId}`,
    {
      method: 'DELETE',
      headers: getHeaders(options),
    }
  );

  return handleResponse<void>(response);
}

// =============================================================================
// Invitation Management
// =============================================================================

/**
 * Invite a new member to the team (admin/owner only)
 */
export async function inviteMember(
  teamId: string,
  data: InviteMemberRequest,
  options?: ApiOptions
): Promise<TeamInvite> {
  const response = await fetch(`${API_BASE}/api/teams/${teamId}/members`, {
    method: 'POST',
    headers: getHeaders(options),
    body: JSON.stringify(data),
  });

  return handleResponse<TeamInvite>(response);
}

/**
 * List pending invitations for a team
 */
export async function listInvites(
  teamId: string,
  options?: ApiOptions
): Promise<TeamInvite[]> {
  const response = await fetch(`${API_BASE}/api/teams/${teamId}/invites`, {
    method: 'GET',
    headers: getHeaders(options),
  });

  return handleResponse<TeamInvite[]>(response);
}

/**
 * Cancel a pending invitation
 */
export async function cancelInvite(
  teamId: string,
  inviteId: string,
  options?: ApiOptions
): Promise<void> {
  const response = await fetch(
    `${API_BASE}/api/teams/${teamId}/invites/${inviteId}`,
    {
      method: 'DELETE',
      headers: getHeaders(options),
    }
  );

  return handleResponse<void>(response);
}

/**
 * Get invitation details by token (public endpoint)
 */
export async function getInviteByToken(
  token: string
): Promise<TeamInviteDetails> {
  const response = await fetch(`${API_BASE}/api/invites/${token}`, {
    method: 'GET',
    headers: { 'Content-Type': 'application/json' },
  });

  return handleResponse<TeamInviteDetails>(response);
}

/**
 * Accept an invitation
 */
export async function acceptInvite(
  token: string,
  options?: ApiOptions
): Promise<TeamMember> {
  const response = await fetch(`${API_BASE}/api/invites/${token}/accept`, {
    method: 'POST',
    headers: getHeaders(options),
  });

  return handleResponse<TeamMember>(response);
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Check if a user has a specific role or higher
 */
export function hasRole(
  member: TeamMember,
  requiredRole: TeamRole
): boolean {
  const roleHierarchy: Record<TeamRole, number> = {
    [TeamRole.OWNER]: 4,
    [TeamRole.ADMIN]: 3,
    [TeamRole.MEMBER]: 2,
    [TeamRole.VIEWER]: 1,
  };

  return roleHierarchy[member.role] >= roleHierarchy[requiredRole];
}

/**
 * Check if a role can manage another role
 */
export function canManageRole(
  actorRole: TeamRole,
  targetRole: TeamRole
): boolean {
  // Owner can manage anyone
  if (actorRole === TeamRole.OWNER) return true;

  // Admin can manage member and viewer
  if (actorRole === TeamRole.ADMIN) {
    return targetRole === TeamRole.MEMBER || targetRole === TeamRole.VIEWER;
  }

  return false;
}

/**
 * Get role display name
 */
export function getRoleDisplayName(role: TeamRole): string {
  const displayNames: Record<TeamRole, string> = {
    [TeamRole.OWNER]: 'Owner',
    [TeamRole.ADMIN]: 'Admin',
    [TeamRole.MEMBER]: 'Member',
    [TeamRole.VIEWER]: 'Viewer',
  };

  return displayNames[role];
}

/**
 * Get plan display name
 */
export function getPlanDisplayName(plan: TeamPlan): string {
  const displayNames: Record<TeamPlan, string> = {
    [TeamPlan.FREE]: 'Free',
    [TeamPlan.PRO]: 'Pro',
    [TeamPlan.TEAM]: 'Team',
    [TeamPlan.AGENCY]: 'Agency',
    [TeamPlan.ENTERPRISE]: 'Enterprise',
  };

  return displayNames[plan];
}

// =============================================================================
// Teams API Class (Alternative API)
// =============================================================================

/**
 * Teams API client class for object-oriented usage
 */
export class TeamsAPI {
  private apiKey?: string;

  constructor(apiKey?: string) {
    this.apiKey = apiKey;
  }

  private get options(): ApiOptions {
    return { apiKey: this.apiKey };
  }

  // Team operations
  createTeam(data: CreateTeamRequest): Promise<Team> {
    return createTeam(data, this.options);
  }

  listTeams(): Promise<Team[]> {
    return listTeams(this.options);
  }

  getTeam(teamId: string): Promise<Team> {
    return getTeam(teamId, this.options);
  }

  updateTeam(teamId: string, data: UpdateTeamRequest): Promise<Team> {
    return updateTeam(teamId, data, this.options);
  }

  deleteTeam(teamId: string): Promise<void> {
    return deleteTeam(teamId, this.options);
  }

  // Member operations
  listMembers(teamId: string): Promise<TeamMember[]> {
    return listMembers(teamId, this.options);
  }

  getMember(teamId: string, memberId: string): Promise<TeamMember> {
    return getMember(teamId, memberId, this.options);
  }

  updateMemberRole(
    teamId: string,
    memberId: string,
    data: UpdateMemberRoleRequest
  ): Promise<TeamMember> {
    return updateMemberRole(teamId, memberId, data, this.options);
  }

  removeMember(teamId: string, memberId: string): Promise<void> {
    return removeMember(teamId, memberId, this.options);
  }

  // Invite operations
  inviteMember(teamId: string, data: InviteMemberRequest): Promise<TeamInvite> {
    return inviteMember(teamId, data, this.options);
  }

  listInvites(teamId: string): Promise<TeamInvite[]> {
    return listInvites(teamId, this.options);
  }

  cancelInvite(teamId: string, inviteId: string): Promise<void> {
    return cancelInvite(teamId, inviteId, this.options);
  }

  acceptInvite(token: string): Promise<TeamMember> {
    return acceptInvite(token, this.options);
  }
}

// Export default instance
export const teamsApi = new TeamsAPI();
