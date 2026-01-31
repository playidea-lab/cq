-- Migration: 00008_telemetry_events
-- Description: Telemetry events table for anonymous usage tracking
-- Date: 2026-01-31

-- ==============================================================================
-- TELEMETRY EVENTS TABLE
-- ==============================================================================
-- Stores anonymous usage telemetry for product analytics.
-- No PII is collected - only tool usage patterns and performance metrics.

CREATE TABLE IF NOT EXISTS c4_telemetry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Event identification
    event_type TEXT NOT NULL,           -- 'tool_call', 'state_change', 'task_complete', 'error'
    anonymous_id TEXT NOT NULL,         -- SHA-256 hash of device identifier (no PII)

    -- Timestamps
    event_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),  -- When the event occurred
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),       -- When the record was inserted

    -- Event details
    tool_name TEXT,                     -- MCP tool name (e.g., 'c4_quick', 'c4_submit')
    metadata JSONB DEFAULT '{}'::jsonb, -- Additional event data (no code content)

    -- Optional context
    session_id TEXT,                    -- Optional session identifier
    c4_version TEXT,                    -- C4 version
    platform TEXT                       -- 'darwin', 'linux', 'win32'
);

-- ==============================================================================
-- INDEXES
-- ==============================================================================

-- Primary lookup patterns
CREATE INDEX IF NOT EXISTS idx_telemetry_anonymous_id
    ON c4_telemetry(anonymous_id);

CREATE INDEX IF NOT EXISTS idx_telemetry_event_type
    ON c4_telemetry(event_type);

CREATE INDEX IF NOT EXISTS idx_telemetry_timestamp
    ON c4_telemetry(event_timestamp DESC);

-- Composite index for analytics queries
CREATE INDEX IF NOT EXISTS idx_telemetry_analytics
    ON c4_telemetry(event_type, event_timestamp DESC);

-- Tool usage analysis
CREATE INDEX IF NOT EXISTS idx_telemetry_tool_name
    ON c4_telemetry(tool_name)
    WHERE tool_name IS NOT NULL;

-- ==============================================================================
-- ROW LEVEL SECURITY
-- ==============================================================================
-- Telemetry is write-only for anonymous users.
-- Read access is restricted to service role only.

ALTER TABLE c4_telemetry ENABLE ROW LEVEL SECURITY;

-- Allow anonymous inserts (using anon key)
CREATE POLICY "Allow anonymous insert"
    ON c4_telemetry
    FOR INSERT
    WITH CHECK (true);

-- Only service role can read (for analytics)
CREATE POLICY "Service role read access"
    ON c4_telemetry
    FOR SELECT
    USING (auth.role() = 'service_role');

-- ==============================================================================
-- RETENTION POLICY (Optional - for cost management)
-- ==============================================================================
-- Uncomment to enable automatic data retention

-- CREATE OR REPLACE FUNCTION delete_old_telemetry()
-- RETURNS void AS $$
-- BEGIN
--     DELETE FROM c4_telemetry
--     WHERE created_at < NOW() - INTERVAL '90 days';
-- END;
-- $$ LANGUAGE plpgsql;

-- ==============================================================================
-- ANALYTICS VIEWS
-- ==============================================================================

-- Daily tool usage summary
CREATE OR REPLACE VIEW telemetry_daily_summary AS
SELECT
    date_trunc('day', event_timestamp) AS day,
    event_type,
    tool_name,
    COUNT(*) AS event_count,
    COUNT(DISTINCT anonymous_id) AS unique_users
FROM c4_telemetry
GROUP BY date_trunc('day', event_timestamp), event_type, tool_name
ORDER BY day DESC, event_count DESC;

-- Tool popularity ranking
CREATE OR REPLACE VIEW telemetry_tool_ranking AS
SELECT
    tool_name,
    COUNT(*) AS total_calls,
    COUNT(DISTINCT anonymous_id) AS unique_users,
    AVG((metadata->>'duration_ms')::numeric) AS avg_duration_ms
FROM c4_telemetry
WHERE tool_name IS NOT NULL
  AND event_type = 'tool_call'
GROUP BY tool_name
ORDER BY total_calls DESC;

-- Grant view access to service role
GRANT SELECT ON telemetry_daily_summary TO service_role;
GRANT SELECT ON telemetry_tool_ranking TO service_role;

-- ==============================================================================
-- COMMENTS
-- ==============================================================================

COMMENT ON TABLE c4_telemetry IS 'Anonymous usage telemetry for C4 product analytics';
COMMENT ON COLUMN c4_telemetry.anonymous_id IS 'SHA-256 hash of device identifier - no PII';
COMMENT ON COLUMN c4_telemetry.metadata IS 'Additional event data - never contains code content';
