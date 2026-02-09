-- Migration: 00005_c4_personas
-- Persona stats table. Tracks per-task outcome and review scores.

CREATE TABLE c4_persona_stats (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id   UUID   NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    persona_id   TEXT   NOT NULL,
    task_id      TEXT   NOT NULL,
    outcome      TEXT   NOT NULL,
    review_score DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    feedback     TEXT   NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (project_id, persona_id, task_id)
);

CREATE INDEX idx_c4_persona_stats_persona ON c4_persona_stats(project_id, persona_id);
CREATE INDEX idx_c4_persona_stats_task    ON c4_persona_stats(project_id, task_id);

-- ============================================================
-- RLS
-- ============================================================
ALTER TABLE c4_persona_stats ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view persona stats"
    ON c4_persona_stats FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can create persona stats"
    ON c4_persona_stats FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update persona stats"
    ON c4_persona_stats FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));
