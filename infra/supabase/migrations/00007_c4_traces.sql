-- Migration: 00007_c4_traces
-- Agent trace logs. Records agent lifecycle events for observability.

CREATE TABLE c4_agent_traces (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id UUID        NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    event_type TEXT        NOT NULL,
    agent_id   TEXT        NOT NULL DEFAULT '',
    task_id    TEXT        NOT NULL DEFAULT '',
    detail     TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_c4_traces_project    ON c4_agent_traces(project_id, created_at DESC);
CREATE INDEX idx_c4_traces_event_type ON c4_agent_traces(project_id, event_type);
CREATE INDEX idx_c4_traces_agent      ON c4_agent_traces(project_id, agent_id) WHERE agent_id != '';
CREATE INDEX idx_c4_traces_task       ON c4_agent_traces(project_id, task_id)  WHERE task_id  != '';

-- ============================================================
-- RLS
-- ============================================================
ALTER TABLE c4_agent_traces ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view traces"
    ON c4_agent_traces FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can create traces"
    ON c4_agent_traces FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));
