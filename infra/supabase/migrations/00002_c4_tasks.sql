-- Migration: 00002_c4_tasks
-- Tasks table matching SQLite c4_tasks schema with project_id FK
-- and row_version for optimistic locking.

CREATE TABLE c4_tasks (
    task_id      TEXT         NOT NULL,
    project_id   UUID         NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    title        TEXT         NOT NULL,
    scope        TEXT         NOT NULL DEFAULT '',
    dod          TEXT         NOT NULL DEFAULT '',
    status       TEXT         NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'in_progress', 'done', 'blocked', 'review')),
    dependencies JSONB        NOT NULL DEFAULT '[]'::jsonb,
    domain       TEXT         NOT NULL DEFAULT '',
    priority     INTEGER      NOT NULL DEFAULT 0,
    model        TEXT         NOT NULL DEFAULT '',
    worker_id    TEXT         NOT NULL DEFAULT '',
    branch       TEXT         NOT NULL DEFAULT '',
    commit_sha   TEXT         NOT NULL DEFAULT '',
    handoff      TEXT         NOT NULL DEFAULT '',
    row_version  INTEGER      NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),

    PRIMARY KEY (project_id, task_id)
);

-- Query patterns: list by project+status, lookup by task_id within project
CREATE INDEX idx_c4_tasks_status    ON c4_tasks(project_id, status);
CREATE INDEX idx_c4_tasks_domain    ON c4_tasks(project_id, domain) WHERE domain != '';
CREATE INDEX idx_c4_tasks_worker    ON c4_tasks(project_id, worker_id) WHERE worker_id != '';
CREATE INDEX idx_c4_tasks_priority  ON c4_tasks(project_id, priority DESC);

-- Auto-update updated_at
CREATE TRIGGER trg_c4_tasks_updated_at
    BEFORE UPDATE ON c4_tasks
    FOR EACH ROW
    EXECUTE FUNCTION c4_set_updated_at();

-- ============================================================
-- RLS
-- ============================================================
ALTER TABLE c4_tasks ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view project tasks"
    ON c4_tasks FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can create tasks"
    ON c4_tasks FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update tasks"
    ON c4_tasks FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can delete tasks"
    ON c4_tasks FOR DELETE
    USING (c4_is_project_member(project_id));
