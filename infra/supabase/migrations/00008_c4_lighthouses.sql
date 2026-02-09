-- Migration: 00008_c4_lighthouses
-- Lighthouse registry. Spec-as-MCP stub tools that can be promoted to live.

CREATE TABLE c4_lighthouses (
    name         TEXT        NOT NULL,
    project_id   UUID        NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    description  TEXT        NOT NULL DEFAULT '',
    input_schema JSONB       NOT NULL DEFAULT '{}'::jsonb,
    spec         TEXT        NOT NULL DEFAULT '',
    status       TEXT        NOT NULL DEFAULT 'stub'
                             CHECK (status IN ('stub', 'implemented', 'deprecated')),
    version      INTEGER     NOT NULL DEFAULT 1,
    created_by   TEXT        NOT NULL DEFAULT '',
    promoted_by  TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (project_id, name)
);

CREATE INDEX idx_c4_lighthouses_status ON c4_lighthouses(project_id, status);

-- Auto-update updated_at
CREATE TRIGGER trg_c4_lighthouses_updated_at
    BEFORE UPDATE ON c4_lighthouses
    FOR EACH ROW
    EXECUTE FUNCTION c4_set_updated_at();

-- ============================================================
-- RLS
-- ============================================================
ALTER TABLE c4_lighthouses ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view lighthouses"
    ON c4_lighthouses FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can create lighthouses"
    ON c4_lighthouses FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update lighthouses"
    ON c4_lighthouses FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can delete lighthouses"
    ON c4_lighthouses FOR DELETE
    USING (c4_is_project_member(project_id));
