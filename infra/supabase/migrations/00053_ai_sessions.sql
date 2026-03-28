-- Migration: 00053_ai_sessions
-- Tracks AI tool sessions across providers (Claude, Gemini, ChatGPT, Codex, etc.)
-- Fed by: CF Worker (c4_session_summary), cq session index, future hooks.

CREATE TABLE IF NOT EXISTS ai_sessions (
    id          uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    owner_id    text NOT NULL,          -- GitHub user ID or Supabase user ID
    project_id  text,                   -- optional project reference
    tool        text NOT NULL,          -- 'claude', 'gemini', 'chatgpt', 'codex', 'cursor'
    session_id  text,                   -- tool-specific session ID (UUID for claude, etc.)
    title       text,
    summary     text,
    status      text NOT NULL DEFAULT 'done',  -- 'active', 'done'
    turn_count  integer,
    started_at  timestamptz NOT NULL DEFAULT now(),
    ended_at    timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_sessions_owner
    ON ai_sessions (owner_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ai_sessions_tool
    ON ai_sessions (tool, created_at DESC);

ALTER TABLE ai_sessions ENABLE ROW LEVEL SECURITY;

CREATE POLICY "service_role_all" ON ai_sessions
    FOR ALL TO service_role USING (true) WITH CHECK (true);

CREATE POLICY "owner_read" ON ai_sessions
    FOR SELECT TO authenticated
    USING (owner_id = (auth.jwt() ->> 'sub'));
