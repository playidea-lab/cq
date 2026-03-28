-- Migration: 00054_ai_sessions_heartbeat
-- Adds last_seen column for heartbeat tracking and timeout cron job.
-- Enables ChatGPT active session tracking via MCP tool call heartbeats.

-- 1. Add last_seen column for heartbeat tracking
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS last_seen timestamptz NOT NULL DEFAULT now();

-- 2. Unique partial index for upsert: one active session per owner+tool
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_sessions_active_upsert
    ON ai_sessions (owner_id, tool) WHERE status = 'active';

-- 3. Cron job: timeout active sessions with no heartbeat for 30 minutes
-- Runs every 5 minutes.
SELECT cron.schedule(
    'ai_sessions_timeout',
    '*/5 * * * *',
    $$UPDATE ai_sessions SET status = 'done', ended_at = now() WHERE status = 'active' AND last_seen < now() - interval '30 minutes'$$
);
