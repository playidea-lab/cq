-- =============================================================================
-- C4 Cloud Integration Providers Schema
-- =============================================================================
-- Adds support for external service integrations (GitHub, Discord, Dooray, etc.)
-- Run with: supabase db push

-- =============================================================================
-- Integration Providers (System Table)
-- =============================================================================
-- Defines available integration providers. Managed by system, not users.

CREATE TABLE IF NOT EXISTS integration_providers (
    id TEXT PRIMARY KEY,  -- 'github', 'discord', 'dooray', 'slack'
    name TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('source_control', 'messaging', 'collaboration', 'ci_cd')),
    description TEXT,
    oauth_url TEXT,
    webhook_path TEXT,
    icon_url TEXT,
    docs_url TEXT,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Seed initial providers
INSERT INTO integration_providers (id, name, category, description, webhook_path, icon_url, docs_url) VALUES
    ('github', 'GitHub', 'source_control', 'GitHub integration for PR reviews, code analysis, and notifications', '/webhooks/github', 'https://github.githubassets.com/images/modules/logos_page/GitHub-Mark.png', 'https://docs.github.com/en/apps'),
    ('discord', 'Discord', 'messaging', 'Discord bot for notifications and slash commands', '/webhooks/discord', 'https://assets-global.website-files.com/6257adef93867e50d84d30e2/636e0a6a49cf127bf92de1e2_icon_clyde_blurple_RGB.png', 'https://discord.com/developers/docs'),
    ('dooray', 'Dooray', 'collaboration', 'Dooray integration for project management and notifications', '/webhooks/dooray', NULL, 'https://helpdesk.dooray.com/'),
    ('slack', 'Slack', 'messaging', 'Slack bot for notifications and slash commands', '/webhooks/slack', 'https://a.slack-edge.com/80588/marketing/img/icons/icon_slack_hash_colored.png', 'https://api.slack.com/docs')
ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- Team Integrations (Per-Team Connections)
-- =============================================================================
-- Stores integration connections for each team. Credentials are encrypted.

CREATE TABLE IF NOT EXISTS team_integrations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL REFERENCES integration_providers(id),

    -- Provider-specific identifier
    external_id TEXT NOT NULL,  -- GitHub: installation_id, Discord: guild_id, Dooray: project_id
    external_name TEXT,         -- repo name, server name, project name

    -- Authentication (encrypted at application level)
    credentials JSONB,  -- access_token, refresh_token, webhook_secret, etc.

    -- Settings
    settings JSONB DEFAULT '{}',  -- notification channel, auto-review on/off, etc.

    -- Status
    status TEXT DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'revoked', 'error')),
    connected_by UUID REFERENCES auth.users(id),
    connected_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    error_message TEXT,  -- Last error if status = 'error'

    -- Timestamps
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(team_id, provider_id, external_id)
);

-- =============================================================================
-- Indexes for Efficient Queries
-- =============================================================================

-- Webhook routing: Find integration by provider + external_id
CREATE INDEX IF NOT EXISTS idx_integrations_provider_external
ON team_integrations(provider_id, external_id)
WHERE status = 'active';

-- Team integrations list
CREATE INDEX IF NOT EXISTS idx_integrations_team
ON team_integrations(team_id, status);

-- Provider stats
CREATE INDEX IF NOT EXISTS idx_integrations_provider_status
ON team_integrations(provider_id, status);

-- =============================================================================
-- Row Level Security (RLS)
-- =============================================================================

-- Integration providers: Public read (system table)
ALTER TABLE integration_providers ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Anyone can view enabled providers"
ON integration_providers FOR SELECT
USING (enabled = true);

-- Team integrations: RLS based on team membership
ALTER TABLE team_integrations ENABLE ROW LEVEL SECURITY;

-- View: Team members can view their team's integrations
CREATE POLICY "Team members can view integrations"
ON team_integrations FOR SELECT
USING (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

-- Insert: Admins and owners can add integrations
CREATE POLICY "Team admins can add integrations"
ON team_integrations FOR INSERT
WITH CHECK (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

-- Update: Admins and owners can update integrations
CREATE POLICY "Team admins can update integrations"
ON team_integrations FOR UPDATE
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

-- Delete: Admins and owners can remove integrations
CREATE POLICY "Team admins can delete integrations"
ON team_integrations FOR DELETE
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

-- =============================================================================
-- Triggers
-- =============================================================================

-- Update updated_at on changes
CREATE TRIGGER update_team_integrations_updated_at
BEFORE UPDATE ON team_integrations
FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Notify on integration changes (for real-time updates)
CREATE OR REPLACE FUNCTION notify_integration_change()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify(
        'integration_change',
        json_build_object(
            'team_id', COALESCE(NEW.team_id, OLD.team_id),
            'provider_id', COALESCE(NEW.provider_id, OLD.provider_id),
            'external_id', COALESCE(NEW.external_id, OLD.external_id),
            'status', COALESCE(NEW.status, 'deleted'),
            'operation', TG_OP
        )::text
    );
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER on_integration_change
AFTER INSERT OR UPDATE OR DELETE ON team_integrations
FOR EACH ROW
EXECUTE FUNCTION notify_integration_change();

-- =============================================================================
-- Integration Events (Audit Log)
-- =============================================================================

CREATE TABLE IF NOT EXISTS integration_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    integration_id UUID REFERENCES team_integrations(id) ON DELETE SET NULL,
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    event_type TEXT NOT NULL,  -- 'webhook_received', 'oauth_completed', 'notification_sent', 'error'
    actor TEXT,  -- user_id or 'system' or external identifier
    payload JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Index for efficient event queries
CREATE INDEX IF NOT EXISTS idx_integration_events_team_created
ON integration_events(team_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_integration_events_integration
ON integration_events(integration_id, created_at DESC);

-- RLS for integration events
ALTER TABLE integration_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Team members can view integration events"
ON integration_events FOR SELECT
USING (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

CREATE POLICY "System can insert integration events"
ON integration_events FOR INSERT
WITH CHECK (true);  -- Allow service role to insert events

-- =============================================================================
-- Helper Functions
-- =============================================================================

-- Find integration by webhook routing info
CREATE OR REPLACE FUNCTION find_integration_by_external(
    p_provider_id TEXT,
    p_external_id TEXT
) RETURNS TABLE (
    integration_id UUID,
    team_id UUID,
    credentials JSONB,
    settings JSONB
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        ti.id as integration_id,
        ti.team_id,
        ti.credentials,
        ti.settings
    FROM team_integrations ti
    WHERE ti.provider_id = p_provider_id
      AND ti.external_id = p_external_id
      AND ti.status = 'active';
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Update last_used_at timestamp
CREATE OR REPLACE FUNCTION touch_integration(
    p_integration_id UUID
) RETURNS VOID AS $$
BEGIN
    UPDATE team_integrations
    SET last_used_at = NOW()
    WHERE id = p_integration_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
