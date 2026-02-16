-- Migration: 00018_c1_members
-- Adds unified member model for C1 Messenger: users, agents, and system
-- participants are treated as equal "members" with presence tracking.

-- ============================================================
-- c1_members — unified member registry
-- ============================================================
CREATE TABLE c1_members (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    UUID NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    member_type   TEXT NOT NULL CHECK (member_type IN ('user', 'agent', 'system')),
    external_id   TEXT NOT NULL DEFAULT '',
    display_name  TEXT NOT NULL DEFAULT '',
    avatar        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'offline' CHECK (status IN ('online', 'working', 'idle', 'offline')),
    status_text   TEXT NOT NULL DEFAULT '',
    last_seen_at  TIMESTAMPTZ DEFAULT now(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, member_type, external_id)
);

CREATE INDEX idx_c1_members_project ON c1_members(project_id);
CREATE INDEX idx_c1_members_status ON c1_members(project_id, status);

CREATE TRIGGER trg_c1_members_updated_at
    BEFORE UPDATE ON c1_members
    FOR EACH ROW
    EXECUTE FUNCTION c4_set_updated_at();

-- ============================================================
-- Add member_id to existing tables (nullable for backward compat)
-- ============================================================
ALTER TABLE c1_messages ADD COLUMN member_id UUID REFERENCES c1_members(id);
ALTER TABLE c1_participants ADD COLUMN member_id UUID REFERENCES c1_members(id);

CREATE INDEX idx_c1_messages_member ON c1_messages(member_id);
CREATE INDEX idx_c1_members_type ON c1_members(project_id, member_type);

-- ============================================================
-- RLS: c1_members
-- ============================================================
ALTER TABLE c1_members ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view project members"
    ON c1_members FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can create members"
    ON c1_members FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update members"
    ON c1_members FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));
