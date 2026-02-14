-- Migration: 00013_c1_context_hub
-- Creates C1 Context Hub tables: channels, messages, participants, channel_summaries.
-- Includes FTS support, RLS policies, and indexes.

-- ============================================================
-- c1_channels — team communication channels
-- ============================================================
CREATE TABLE c1_channels (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    UUID NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    channel_type  TEXT NOT NULL DEFAULT 'chat',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE INDEX idx_c1_channels_project ON c1_channels(project_id);

CREATE TRIGGER trg_c1_channels_updated_at
    BEFORE UPDATE ON c1_channels
    FOR EACH ROW
    EXECUTE FUNCTION c4_set_updated_at();

-- ============================================================
-- c1_messages — channel messages with FTS
-- ============================================================
CREATE TABLE c1_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID NOT NULL REFERENCES c1_channels(id) ON DELETE CASCADE,
    project_id      UUID NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    sender_name     TEXT NOT NULL DEFAULT '',
    sender_type     TEXT NOT NULL DEFAULT 'user',
    sender_id       TEXT NOT NULL DEFAULT '',
    participant_id  TEXT DEFAULT auth.uid()::text,
    content         TEXT NOT NULL DEFAULT '',
    thread_id       UUID,
    metadata        JSONB DEFAULT '{}'::jsonb,
    tsv             TSVECTOR,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_c1_messages_channel ON c1_messages(channel_id);
CREATE INDEX idx_c1_messages_project ON c1_messages(project_id);
CREATE INDEX idx_c1_messages_created ON c1_messages(channel_id, created_at);
CREATE INDEX idx_c1_messages_tsv ON c1_messages USING GIN(tsv);

-- Auto-generate tsvector from content
CREATE OR REPLACE FUNCTION c1_messages_tsv_trigger() RETURNS trigger AS $$
BEGIN
    NEW.tsv := to_tsvector('english', COALESCE(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_c1_messages_tsv
    BEFORE INSERT OR UPDATE OF content ON c1_messages
    FOR EACH ROW
    EXECUTE FUNCTION c1_messages_tsv_trigger();

-- ============================================================
-- c1_participants — channel membership and read tracking
-- ============================================================
CREATE TABLE c1_participants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID NOT NULL REFERENCES c1_channels(id) ON DELETE CASCADE,
    project_id      UUID NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    agent_name      TEXT NOT NULL DEFAULT '',
    participant_id  TEXT DEFAULT auth.uid()::text,
    last_read_at    TIMESTAMPTZ,
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (channel_id, participant_id)
);

CREATE INDEX idx_c1_participants_channel ON c1_participants(channel_id);
CREATE INDEX idx_c1_participants_project ON c1_participants(project_id);

-- ============================================================
-- c1_channel_summaries — LLM-generated channel summaries
-- ============================================================
CREATE TABLE c1_channel_summaries (
    channel_id      UUID NOT NULL REFERENCES c1_channels(id) ON DELETE CASCADE,
    summary         TEXT NOT NULL DEFAULT '',
    key_decisions   JSONB DEFAULT '[]'::jsonb,
    open_questions  JSONB DEFAULT '[]'::jsonb,
    active_tasks    JSONB DEFAULT '[]'::jsonb,
    last_message_id UUID,
    message_count   INTEGER NOT NULL DEFAULT 0,
    unread_count    INTEGER NOT NULL DEFAULT 0,
    last_message_at TIMESTAMPTZ,
    participant_count INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (channel_id)
);

CREATE TRIGGER trg_c1_channel_summaries_updated_at
    BEFORE UPDATE ON c1_channel_summaries
    FOR EACH ROW
    EXECUTE FUNCTION c4_set_updated_at();

-- ============================================================
-- RLS: c1_channels
-- ============================================================
ALTER TABLE c1_channels ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view channels"
    ON c1_channels FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can create channels"
    ON c1_channels FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update channels"
    ON c1_channels FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can delete channels"
    ON c1_channels FOR DELETE
    USING (c4_is_project_member(project_id));

-- ============================================================
-- RLS: c1_messages
-- ============================================================
ALTER TABLE c1_messages ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view messages"
    ON c1_messages FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can send messages"
    ON c1_messages FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can delete own messages"
    ON c1_messages FOR DELETE
    USING (c4_is_project_member(project_id)
           AND (participant_id = auth.uid()::text OR sender_type = 'system'));

-- ============================================================
-- RLS: c1_participants
-- ============================================================
ALTER TABLE c1_participants ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view participants"
    ON c1_participants FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can join channels"
    ON c1_participants FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Users can update own participant records"
    ON c1_participants FOR UPDATE
    USING (c4_is_project_member(project_id)
           AND participant_id = auth.uid()::text)
    WITH CHECK (participant_id = auth.uid()::text);

CREATE POLICY "Users can leave channels"
    ON c1_participants FOR DELETE
    USING (c4_is_project_member(project_id)
           AND participant_id = auth.uid()::text);

-- ============================================================
-- RLS: c1_channel_summaries
-- ============================================================
ALTER TABLE c1_channel_summaries ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view summaries"
    ON c1_channel_summaries FOR SELECT
    USING (EXISTS (
        SELECT 1 FROM c1_channels ch
        WHERE ch.id = channel_id AND c4_is_project_member(ch.project_id)
    ));

CREATE POLICY "Members can create summaries"
    ON c1_channel_summaries FOR INSERT
    WITH CHECK (EXISTS (
        SELECT 1 FROM c1_channels ch
        WHERE ch.id = channel_id AND c4_is_project_member(ch.project_id)
    ));

CREATE POLICY "Members can update summaries"
    ON c1_channel_summaries FOR UPDATE
    USING (EXISTS (
        SELECT 1 FROM c1_channels ch
        WHERE ch.id = channel_id AND c4_is_project_member(ch.project_id)
    ));
