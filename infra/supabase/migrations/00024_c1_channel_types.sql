-- Migration: 00024_c1_channel_types
-- Adds channel_type CHECK constraint, c1_channel_pins table, and agent_work_id column.
-- Note: existing 'chat' values are migrated to 'general' (intentional mapping).

-- ============================================================
-- 1. Normalize legacy channel_type values
-- ============================================================
UPDATE c1_channels
SET channel_type = 'general'
WHERE channel_type NOT IN ('general', 'project', 'knowledge', 'session', 'dm');

-- ============================================================
-- 2. Add CHECK constraint for channel_type
-- ============================================================
ALTER TABLE c1_channels
    ADD CONSTRAINT chk_channel_type
    CHECK (channel_type IN ('general', 'project', 'knowledge', 'session', 'dm'));

-- ============================================================
-- 3. Unique index for session channels (project_id + name)
-- ============================================================
CREATE UNIQUE INDEX IF NOT EXISTS uniq_c1_channels_session
    ON c1_channels(project_id, name)
    WHERE channel_type = 'session';

-- ============================================================
-- 4. Create c1_channel_pins table
-- ============================================================
CREATE TABLE c1_channel_pins (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID        NOT NULL REFERENCES c1_channels(id) ON DELETE CASCADE,
    content    TEXT        NOT NULL DEFAULT '',
    pin_type   TEXT        NOT NULL DEFAULT 'artifact',
    version    INT         NOT NULL DEFAULT 1,
    created_by TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 5. Add agent_work_id column to c1_messages
-- ============================================================
ALTER TABLE c1_messages
    ADD COLUMN IF NOT EXISTS agent_work_id TEXT;

-- ============================================================
-- 6. Index for agent_work_id lookups
-- ============================================================
CREATE INDEX idx_c1_messages_agent_work
    ON c1_messages(channel_id, agent_work_id);

-- ============================================================
-- 7. RLS: c1_channel_pins (same pattern as c1_channel_summaries)
-- ============================================================
ALTER TABLE c1_channel_pins ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view pins"
    ON c1_channel_pins FOR SELECT
    USING (EXISTS (
        SELECT 1 FROM c1_channels ch
        WHERE ch.id = channel_id AND c4_is_project_member(ch.project_id)
    ));

CREATE POLICY "Members can create pins"
    ON c1_channel_pins FOR INSERT
    WITH CHECK (EXISTS (
        SELECT 1 FROM c1_channels ch
        WHERE ch.id = channel_id AND c4_is_project_member(ch.project_id)
    ));

CREATE POLICY "Members can update pins"
    ON c1_channel_pins FOR UPDATE
    USING (EXISTS (
        SELECT 1 FROM c1_channels ch
        WHERE ch.id = channel_id AND c4_is_project_member(ch.project_id)
    ));
