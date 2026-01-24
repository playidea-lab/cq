-- =============================================================================
-- C4 Cloud Activity & Audit Logs Schema
-- =============================================================================
-- Phase 4: Time Tracking + Audit Logs for Agency Billing & Enterprise Compliance
-- Run with: supabase db push

-- =============================================================================
-- Activity Logs (Time Tracking)
-- =============================================================================
-- Purpose: Track user/worker activities for usage reporting and billing

CREATE TABLE IF NOT EXISTS activity_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID REFERENCES auth.users(id),  -- NULL for system/worker activities
    workspace_id UUID REFERENCES projects(id) ON DELETE SET NULL,

    -- Activity Information
    activity_type TEXT NOT NULL,
    -- Examples: task_started, task_completed, pr_created, review_submitted,
    -- command_executed, checkpoint_approved, file_edited, etc.

    -- Resource Details
    resource_type TEXT,  -- task, pr, workspace, file, etc.
    resource_id TEXT,
    metadata JSONB DEFAULT '{}',  -- Additional context

    -- Timing
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ,
    -- Note: duration_seconds calculated on-the-fly in queries for PostgreSQL compatibility
    -- Use: EXTRACT(EPOCH FROM (ended_at - started_at))::INTEGER

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_activity_logs_team_created
ON activity_logs(team_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_activity_logs_user
ON activity_logs(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_activity_logs_resource
ON activity_logs(resource_type, resource_id);

CREATE INDEX IF NOT EXISTS idx_activity_logs_type
ON activity_logs(activity_type);

-- View for aggregated usage (billing reports)
-- Note: Calculate duration directly instead of using GENERATED column for compatibility
CREATE OR REPLACE VIEW team_usage_summary AS
SELECT
    team_id,
    DATE_TRUNC('day', started_at) as date,
    activity_type,
    COUNT(*) as activity_count,
    SUM(
        CASE
            WHEN ended_at IS NOT NULL
            THEN EXTRACT(EPOCH FROM (ended_at - started_at))::INTEGER
            ELSE 0
        END
    ) as total_seconds,
    COUNT(DISTINCT user_id) as unique_users
FROM activity_logs
GROUP BY team_id, DATE_TRUNC('day', started_at), activity_type;


-- =============================================================================
-- Audit Logs (Security & Compliance)
-- =============================================================================
-- Purpose: Track security-relevant actions for compliance and forensics

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- Actor Information
    actor_type TEXT NOT NULL,  -- user, api_key, system, worker
    actor_id TEXT NOT NULL,
    actor_email TEXT,

    -- Action
    action TEXT NOT NULL,
    -- Examples: team.created, member.invited, member.role_changed,
    -- workspace.created, workspace.deleted, integration.connected,
    -- integration.disconnected, settings.updated, etc.

    -- Target Resource
    resource_type TEXT NOT NULL,  -- team, member, workspace, integration, settings
    resource_id TEXT NOT NULL,

    -- Change Details
    old_value JSONB,
    new_value JSONB,

    -- Request Context
    ip_address INET,
    user_agent TEXT,
    request_id TEXT,

    -- Timestamp
    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- Immutability Hash (computed on insert)
    -- SHA256 of: id || actor_id || action || resource_id || created_at
    hash TEXT
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_audit_logs_team_created
ON audit_logs(team_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor
ON audit_logs(actor_id);

CREATE INDEX IF NOT EXISTS idx_audit_logs_resource
ON audit_logs(resource_type, resource_id);

CREATE INDEX IF NOT EXISTS idx_audit_logs_action
ON audit_logs(action);

-- Function to compute audit log hash
CREATE OR REPLACE FUNCTION compute_audit_hash()
RETURNS TRIGGER AS $$
BEGIN
    NEW.hash = encode(
        sha256(
            (NEW.id::text || NEW.actor_id || NEW.action || NEW.resource_id || NEW.created_at::text)::bytea
        ),
        'hex'
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-compute hash on insert
DROP TRIGGER IF EXISTS compute_audit_hash_trigger ON audit_logs;
CREATE TRIGGER compute_audit_hash_trigger
BEFORE INSERT ON audit_logs
FOR EACH ROW
EXECUTE FUNCTION compute_audit_hash();


-- =============================================================================
-- Row Level Security (RLS)
-- =============================================================================

ALTER TABLE activity_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;

-- Activity Logs: Team members can view their team's activities
CREATE POLICY "Team members can view activity logs"
ON activity_logs FOR SELECT
USING (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

-- Activity Logs: System can insert (service role bypasses RLS)
CREATE POLICY "Service can insert activity logs"
ON activity_logs FOR INSERT
WITH CHECK (true);

-- Audit Logs: Team admins/owners can view
CREATE POLICY "Team admins can view audit logs"
ON audit_logs FOR SELECT
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid()
        AND role IN ('owner', 'admin')
    )
);

-- Audit Logs: System can insert (service role bypasses RLS)
CREATE POLICY "Service can insert audit logs"
ON audit_logs FOR INSERT
WITH CHECK (true);

-- Audit Logs: Nobody can update or delete (immutable)
-- No UPDATE or DELETE policies = cannot modify


-- =============================================================================
-- Comments for documentation
-- =============================================================================

COMMENT ON TABLE activity_logs IS 'Tracks user/worker activities for usage reporting and billing';
COMMENT ON TABLE audit_logs IS 'Immutable security audit trail for compliance';
COMMENT ON VIEW team_usage_summary IS 'Aggregated activity statistics per team/day/type';
