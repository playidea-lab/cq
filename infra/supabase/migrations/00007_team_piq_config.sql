-- Team PIQ Configuration Schema
-- Migration: 00007_team_piq_config.sql
-- Description: Adds PIQ (Paper Intelligence Query) configuration per team

-- ============================================================================
-- TEAM PIQ CONFIG TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS team_piq_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- PIQ Connection Settings
    piq_enabled BOOLEAN DEFAULT FALSE,           -- Whether PIQ is enabled for this team
    piq_endpoint_url TEXT,                       -- Custom PIQ server URL (if self-hosted)
    piq_api_key_encrypted TEXT,                  -- Encrypted API key for PIQ access

    -- Knowledge Base Configuration
    knowledge_domains TEXT[] DEFAULT '{}',       -- Allowed knowledge domains (e.g., 'ml', 'cv', 'nlp')
    max_queries_per_day INTEGER DEFAULT 1000,    -- Rate limit for PIQ queries
    cache_ttl_seconds INTEGER DEFAULT 3600,      -- How long to cache PIQ responses

    -- Experiment Sync Settings
    experiment_sync_enabled BOOLEAN DEFAULT FALSE, -- Auto-sync experiments to PIQ
    experiment_visibility TEXT DEFAULT 'private',  -- 'private', 'team', 'public'
    sync_metrics_only BOOLEAN DEFAULT FALSE,       -- Only sync metrics, not code

    -- Paper Recommendations
    paper_recommendations_enabled BOOLEAN DEFAULT TRUE,
    paper_recommendation_frequency TEXT DEFAULT 'weekly', -- 'daily', 'weekly', 'monthly'
    paper_topics TEXT[] DEFAULT '{}',              -- Topics to track

    -- Usage Tracking
    queries_used_today INTEGER DEFAULT 0,
    queries_reset_at TIMESTAMPTZ DEFAULT NOW(),
    total_queries_all_time INTEGER DEFAULT 0,

    -- Metadata
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    configured_by UUID REFERENCES auth.users(id),

    -- One config per team
    UNIQUE(team_id)
);

-- Index for team lookups
CREATE INDEX IF NOT EXISTS idx_team_piq_config_team_id ON team_piq_config(team_id);

-- ============================================================================
-- ROW LEVEL SECURITY
-- ============================================================================

ALTER TABLE team_piq_config ENABLE ROW LEVEL SECURITY;

-- Team members can view their team's PIQ config
CREATE POLICY "Team members can view PIQ config"
ON team_piq_config FOR SELECT
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid()
    )
);

-- Team admins/owners can manage PIQ config
CREATE POLICY "Team admins can manage PIQ config"
ON team_piq_config FOR ALL
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid()
        AND role IN ('owner', 'admin')
    )
);

-- ============================================================================
-- HELPER FUNCTIONS
-- ============================================================================

-- Function to get or create default PIQ config for a team
CREATE OR REPLACE FUNCTION get_or_create_team_piq_config(p_team_id UUID)
RETURNS team_piq_config
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_config team_piq_config;
BEGIN
    -- Try to get existing config
    SELECT * INTO v_config
    FROM team_piq_config
    WHERE team_id = p_team_id;

    -- If not found, create default config
    IF v_config.id IS NULL THEN
        INSERT INTO team_piq_config (team_id, configured_by)
        VALUES (p_team_id, auth.uid())
        RETURNING * INTO v_config;
    END IF;

    RETURN v_config;
END;
$$;

-- Function to check and increment query usage
CREATE OR REPLACE FUNCTION check_and_increment_piq_usage(p_team_id UUID)
RETURNS JSON
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_config team_piq_config;
    v_now TIMESTAMPTZ := NOW();
BEGIN
    -- Get current config
    SELECT * INTO v_config
    FROM team_piq_config
    WHERE team_id = p_team_id;

    IF v_config.id IS NULL THEN
        RETURN json_build_object(
            'success', false,
            'error', 'PIQ not configured for this team'
        );
    END IF;

    IF NOT v_config.piq_enabled THEN
        RETURN json_build_object(
            'success', false,
            'error', 'PIQ is disabled for this team'
        );
    END IF;

    -- Check if we need to reset daily counter
    IF v_config.queries_reset_at < date_trunc('day', v_now) THEN
        UPDATE team_piq_config
        SET queries_used_today = 1,
            queries_reset_at = v_now,
            total_queries_all_time = total_queries_all_time + 1,
            updated_at = v_now
        WHERE team_id = p_team_id
        RETURNING * INTO v_config;

        RETURN json_build_object(
            'success', true,
            'queries_used', 1,
            'queries_remaining', v_config.max_queries_per_day - 1
        );
    END IF;

    -- Check rate limit
    IF v_config.queries_used_today >= v_config.max_queries_per_day THEN
        RETURN json_build_object(
            'success', false,
            'error', 'Daily query limit reached',
            'queries_used', v_config.queries_used_today,
            'max_queries', v_config.max_queries_per_day,
            'resets_at', date_trunc('day', v_now) + interval '1 day'
        );
    END IF;

    -- Increment usage
    UPDATE team_piq_config
    SET queries_used_today = queries_used_today + 1,
        total_queries_all_time = total_queries_all_time + 1,
        updated_at = v_now
    WHERE team_id = p_team_id
    RETURNING * INTO v_config;

    RETURN json_build_object(
        'success', true,
        'queries_used', v_config.queries_used_today,
        'queries_remaining', v_config.max_queries_per_day - v_config.queries_used_today
    );
END;
$$;

-- Function to update PIQ config
CREATE OR REPLACE FUNCTION update_team_piq_config(
    p_team_id UUID,
    p_updates JSONB
)
RETURNS team_piq_config
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_config team_piq_config;
BEGIN
    -- Check user has admin access
    IF NOT EXISTS (
        SELECT 1 FROM team_members
        WHERE team_id = p_team_id
        AND user_id = auth.uid()
        AND role IN ('owner', 'admin')
    ) THEN
        RAISE EXCEPTION 'Not authorized';
    END IF;

    -- Ensure config exists
    INSERT INTO team_piq_config (team_id, configured_by)
    VALUES (p_team_id, auth.uid())
    ON CONFLICT (team_id) DO NOTHING;

    -- Update allowed fields only
    UPDATE team_piq_config
    SET
        piq_enabled = COALESCE((p_updates->>'piq_enabled')::boolean, piq_enabled),
        piq_endpoint_url = COALESCE(p_updates->>'piq_endpoint_url', piq_endpoint_url),
        knowledge_domains = COALESCE(
            (SELECT array_agg(x::text) FROM jsonb_array_elements_text(p_updates->'knowledge_domains') AS x),
            knowledge_domains
        ),
        max_queries_per_day = COALESCE((p_updates->>'max_queries_per_day')::integer, max_queries_per_day),
        cache_ttl_seconds = COALESCE((p_updates->>'cache_ttl_seconds')::integer, cache_ttl_seconds),
        experiment_sync_enabled = COALESCE((p_updates->>'experiment_sync_enabled')::boolean, experiment_sync_enabled),
        experiment_visibility = COALESCE(p_updates->>'experiment_visibility', experiment_visibility),
        sync_metrics_only = COALESCE((p_updates->>'sync_metrics_only')::boolean, sync_metrics_only),
        paper_recommendations_enabled = COALESCE((p_updates->>'paper_recommendations_enabled')::boolean, paper_recommendations_enabled),
        paper_recommendation_frequency = COALESCE(p_updates->>'paper_recommendation_frequency', paper_recommendation_frequency),
        paper_topics = COALESCE(
            (SELECT array_agg(x::text) FROM jsonb_array_elements_text(p_updates->'paper_topics') AS x),
            paper_topics
        ),
        updated_at = NOW()
    WHERE team_id = p_team_id
    RETURNING * INTO v_config;

    RETURN v_config;
END;
$$;

-- ============================================================================
-- TRIGGER FOR UPDATED_AT
-- ============================================================================

CREATE OR REPLACE FUNCTION update_team_piq_config_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER team_piq_config_updated_at
    BEFORE UPDATE ON team_piq_config
    FOR EACH ROW
    EXECUTE FUNCTION update_team_piq_config_updated_at();

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE team_piq_config IS 'PIQ (Paper Intelligence Query) configuration per team';
COMMENT ON COLUMN team_piq_config.piq_api_key_encrypted IS 'Encrypted API key for PIQ - decrypt with vault';
COMMENT ON COLUMN team_piq_config.knowledge_domains IS 'Allowed knowledge domains: ml, cv, nlp, rl, dl, etc.';
COMMENT ON COLUMN team_piq_config.experiment_visibility IS 'Experiment sharing: private (team only), team, public';
