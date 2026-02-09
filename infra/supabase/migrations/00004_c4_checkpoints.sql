-- Migration: 00004_c4_checkpoints
-- Checkpoints table. Stores supervisor checkpoint decisions.

CREATE TABLE c4_checkpoints (
    checkpoint_id    TEXT         NOT NULL,
    project_id       UUID         NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    decision         TEXT         NOT NULL CHECK (decision IN ('approve', 'request_changes', 'reject')),
    notes            TEXT         NOT NULL DEFAULT '',
    required_changes JSONB        NOT NULL DEFAULT '[]'::jsonb,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),

    PRIMARY KEY (project_id, checkpoint_id)
);

CREATE INDEX idx_c4_checkpoints_created ON c4_checkpoints(project_id, created_at DESC);

-- ============================================================
-- RLS
-- ============================================================
ALTER TABLE c4_checkpoints ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view checkpoints"
    ON c4_checkpoints FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can create checkpoints"
    ON c4_checkpoints FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update checkpoints"
    ON c4_checkpoints FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));
