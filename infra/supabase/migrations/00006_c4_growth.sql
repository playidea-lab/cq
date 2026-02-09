-- Migration: 00006_c4_growth
-- Twin growth metrics. Weekly snapshots of per-user performance.

CREATE TABLE c4_twin_growth (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id  UUID             NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    username    TEXT             NOT NULL,
    metric      TEXT             NOT NULL,
    value       DOUBLE PRECISION NOT NULL,
    period      TEXT             NOT NULL,
    recorded_at TIMESTAMPTZ      NOT NULL DEFAULT now(),

    UNIQUE (project_id, username, metric, period)
);

CREATE INDEX idx_c4_twin_growth_user   ON c4_twin_growth(project_id, username);
CREATE INDEX idx_c4_twin_growth_period ON c4_twin_growth(project_id, period);

-- ============================================================
-- RLS
-- ============================================================
ALTER TABLE c4_twin_growth ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view growth metrics"
    ON c4_twin_growth FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can create growth metrics"
    ON c4_twin_growth FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update growth metrics"
    ON c4_twin_growth FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));
