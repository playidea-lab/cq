/**
 * C4 Reports API Client
 *
 * Provides type-safe API calls for usage reports, activity logs,
 * and audit logs with export capabilities.
 */

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:4000';

// =============================================================================
// Types
// =============================================================================

export interface UsageReport {
  team_id: string;
  start_date: string;
  end_date: string;
  total_activities: number;
  total_duration_seconds: number;
  by_type: Record<string, number>;
  by_date: DailySummary[];
}

export interface DailySummary {
  date: string;
  activity_type: string;
  activity_count: number;
  total_seconds: number;
  unique_users: number;
}

export interface ActivityLog {
  id: string;
  team_id: string;
  activity_type: string;
  user_id: string | null;
  workspace_id: string | null;
  resource_type: string | null;
  resource_id: string | null;
  metadata: Record<string, unknown>;
  started_at: string | null;
  ended_at: string | null;
  duration_seconds: number | null;
  created_at: string | null;
}

export interface AuditLog {
  id: string;
  team_id: string;
  actor_type: string;
  actor_id: string;
  actor_email: string | null;
  action: string;
  resource_type: string;
  resource_id: string;
  old_value: Record<string, unknown> | null;
  new_value: Record<string, unknown> | null;
  ip_address: string | null;
  user_agent: string | null;
  request_id: string | null;
  created_at: string | null;
  hash: string | null;
}

export interface AuditLogsResponse {
  team_id: string;
  logs: AuditLog[];
  total: number;
  limit: number;
  offset: number;
}

// =============================================================================
// Filter Types
// =============================================================================

export interface ActivityFilter {
  start_date?: string;
  end_date?: string;
  activity_type?: string;
  user_id?: string;
  limit?: number;
}

export interface AuditFilter {
  actor_id?: string;
  action?: string;
  resource_type?: string;
  resource_id?: string;
  start_date?: string;
  end_date?: string;
  limit?: number;
  offset?: number;
}

export type ExportFormat = 'csv' | 'json';

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

  return response.json();
}

function buildQueryString(params: Record<string, string | number | undefined>): string {
  const searchParams = new URLSearchParams();

  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) {
      searchParams.append(key, String(value));
    }
  }

  const queryString = searchParams.toString();
  return queryString ? `?${queryString}` : '';
}

// =============================================================================
// Usage Report API
// =============================================================================

/**
 * Get usage report for a team
 */
export async function getUsageReport(
  teamId: string,
  startDate: string,
  endDate: string,
  options?: ApiOptions
): Promise<UsageReport> {
  const query = buildQueryString({ start_date: startDate, end_date: endDate });
  const response = await fetch(
    `${API_BASE}/api/reports/teams/${teamId}/usage${query}`,
    {
      method: 'GET',
      headers: getHeaders(options),
    }
  );

  return handleResponse<UsageReport>(response);
}

// =============================================================================
// Activity Log API
// =============================================================================

/**
 * Get activity logs for a team
 */
export async function getTeamActivities(
  teamId: string,
  filter?: ActivityFilter,
  options?: ApiOptions
): Promise<ActivityLog[]> {
  const query = buildQueryString({
    start_date: filter?.start_date,
    end_date: filter?.end_date,
    activity_type: filter?.activity_type,
    user_id: filter?.user_id,
    limit: filter?.limit,
  });

  const response = await fetch(
    `${API_BASE}/api/reports/teams/${teamId}/activities${query}`,
    {
      method: 'GET',
      headers: getHeaders(options),
    }
  );

  return handleResponse<ActivityLog[]>(response);
}

// =============================================================================
// Audit Log API
// =============================================================================

/**
 * Get audit logs for a team
 */
export async function getAuditLogs(
  teamId: string,
  filter?: AuditFilter,
  options?: ApiOptions
): Promise<AuditLogsResponse> {
  const query = buildQueryString({
    actor_id: filter?.actor_id,
    action: filter?.action,
    resource_type: filter?.resource_type,
    resource_id: filter?.resource_id,
    start_date: filter?.start_date,
    end_date: filter?.end_date,
    limit: filter?.limit,
    offset: filter?.offset,
  });

  const response = await fetch(
    `${API_BASE}/api/reports/teams/${teamId}/audit${query}`,
    {
      method: 'GET',
      headers: getHeaders(options),
    }
  );

  return handleResponse<AuditLogsResponse>(response);
}

/**
 * Export audit logs in CSV or JSON format
 */
export async function exportAuditLogs(
  teamId: string,
  format: ExportFormat = 'csv',
  filter?: Omit<AuditFilter, 'limit' | 'offset'>,
  options?: ApiOptions
): Promise<Blob> {
  const query = buildQueryString({
    format,
    actor_id: filter?.actor_id,
    action: filter?.action,
    resource_type: filter?.resource_type,
    start_date: filter?.start_date,
    end_date: filter?.end_date,
  });

  const response = await fetch(
    `${API_BASE}/api/reports/teams/${teamId}/audit/export${query}`,
    {
      method: 'GET',
      headers: getHeaders(options),
    }
  );

  if (!response.ok) {
    const error = await response.text();
    throw new Error(`API error ${response.status}: ${error}`);
  }

  return response.blob();
}

/**
 * Download audit logs as a file
 */
export async function downloadAuditLogs(
  teamId: string,
  format: ExportFormat = 'csv',
  filter?: Omit<AuditFilter, 'limit' | 'offset'>,
  options?: ApiOptions
): Promise<void> {
  const blob = await exportAuditLogs(teamId, format, filter, options);

  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `audit_logs_${teamId}.${format}`;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Format duration in seconds to human readable string
 */
export function formatDuration(seconds: number): string {
  if (seconds < 60) {
    return `${seconds}s`;
  }

  if (seconds < 3600) {
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = seconds % 60;
    return remainingSeconds > 0
      ? `${minutes}m ${remainingSeconds}s`
      : `${minutes}m`;
  }

  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
}

/**
 * Get activity type display name
 */
export function getActivityTypeDisplayName(activityType: string): string {
  const displayNames: Record<string, string> = {
    task_started: 'Task Started',
    task_completed: 'Task Completed',
    pr_created: 'PR Created',
    review_submitted: 'Review Submitted',
    command_executed: 'Command Executed',
    checkpoint_approved: 'Checkpoint Approved',
    checkpoint_rejected: 'Checkpoint Rejected',
    workspace_created: 'Workspace Created',
    workspace_deleted: 'Workspace Deleted',
  };

  return displayNames[activityType] || activityType;
}

/**
 * Get action display name for audit logs
 */
export function getActionDisplayName(action: string): string {
  const displayNames: Record<string, string> = {
    'team.created': 'Team Created',
    'team.updated': 'Team Updated',
    'team.deleted': 'Team Deleted',
    'member.invited': 'Member Invited',
    'member.joined': 'Member Joined',
    'member.role_changed': 'Member Role Changed',
    'member.removed': 'Member Removed',
    'workspace.created': 'Workspace Created',
    'workspace.updated': 'Workspace Updated',
    'workspace.deleted': 'Workspace Deleted',
    'integration.connected': 'Integration Connected',
    'integration.disconnected': 'Integration Disconnected',
    'settings.updated': 'Settings Updated',
  };

  return displayNames[action] || action;
}

/**
 * Get actor type display name
 */
export function getActorTypeDisplayName(actorType: string): string {
  const displayNames: Record<string, string> = {
    user: 'User',
    api_key: 'API Key',
    system: 'System',
    worker: 'Worker',
  };

  return displayNames[actorType] || actorType;
}

// =============================================================================
// Reports API Class (Alternative API)
// =============================================================================

/**
 * Reports API client class for object-oriented usage
 */
export class ReportsAPI {
  private apiKey?: string;

  constructor(apiKey?: string) {
    this.apiKey = apiKey;
  }

  private get options(): ApiOptions {
    return { apiKey: this.apiKey };
  }

  // Usage reports
  getUsageReport(
    teamId: string,
    startDate: string,
    endDate: string
  ): Promise<UsageReport> {
    return getUsageReport(teamId, startDate, endDate, this.options);
  }

  // Activity logs
  getTeamActivities(
    teamId: string,
    filter?: ActivityFilter
  ): Promise<ActivityLog[]> {
    return getTeamActivities(teamId, filter, this.options);
  }

  // Audit logs
  getAuditLogs(teamId: string, filter?: AuditFilter): Promise<AuditLogsResponse> {
    return getAuditLogs(teamId, filter, this.options);
  }

  exportAuditLogs(
    teamId: string,
    format?: ExportFormat,
    filter?: Omit<AuditFilter, 'limit' | 'offset'>
  ): Promise<Blob> {
    return exportAuditLogs(teamId, format, filter, this.options);
  }

  downloadAuditLogs(
    teamId: string,
    format?: ExportFormat,
    filter?: Omit<AuditFilter, 'limit' | 'offset'>
  ): Promise<void> {
    return downloadAuditLogs(teamId, format, filter, this.options);
  }
}

// Export default instance
export const reportsApi = new ReportsAPI();
