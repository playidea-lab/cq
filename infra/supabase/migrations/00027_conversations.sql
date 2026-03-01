-- Migration: 00027_conversations
-- Creates the conversations table for Dooray bot (and future messenger) multi-turn history.
-- Each row is a single chat message (user or assistant) keyed by channel_id.

CREATE TABLE IF NOT EXISTS conversations (
    id         uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    channel_id text NOT NULL,
    platform   text NOT NULL DEFAULT '',
    project_id text NOT NULL DEFAULT '',
    role       text NOT NULL,      -- 'user' | 'assistant'
    content    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Index for efficient per-channel lookups ordered by time.
CREATE INDEX IF NOT EXISTS idx_conversations_channel
    ON conversations (channel_id, created_at DESC);

-- Optional RLS: allow service role full access.
ALTER TABLE conversations ENABLE ROW LEVEL SECURITY;

CREATE POLICY "service_role_all" ON conversations
    FOR ALL TO service_role USING (true) WITH CHECK (true);
