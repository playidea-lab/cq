-- Migration: 00003_c4_state
-- State machine table. Stores the current project state as JSON.
-- One row per project (1:1 relationship).

CREATE TABLE c4_state (
    project_id UUID        PRIMARY KEY REFERENCES c4_projects(id) ON DELETE CASCADE,
    state_json JSONB       NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Auto-update updated_at
CREATE TRIGGER trg_c4_state_updated_at
    BEFORE UPDATE ON c4_state
    FOR EACH ROW
    EXECUTE FUNCTION c4_set_updated_at();

-- ============================================================
-- RLS
-- ============================================================
ALTER TABLE c4_state ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view project state"
    ON c4_state FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can create project state"
    ON c4_state FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update project state"
    ON c4_state FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));
