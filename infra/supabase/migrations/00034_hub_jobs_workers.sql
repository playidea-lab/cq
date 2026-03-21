-- Migration: 00034_hub_jobs_workers
-- Hub tables: jobs, workers, worker_history, leases, capabilities
-- Converted from c5/internal/store/sqlite.go SQLite schema.
-- project_id is TEXT (not UUID FK) — Hub uses API-key-based multi-tenancy,
-- not Supabase auth project membership.

-- ============================================================
-- jobs
-- ============================================================
CREATE TABLE hub_jobs (
    id                    TEXT        PRIMARY KEY,
    name                  TEXT        NOT NULL,
    status                TEXT        NOT NULL DEFAULT 'QUEUED',
    priority              INTEGER     NOT NULL DEFAULT 0,
    workdir               TEXT        NOT NULL DEFAULT '.',
    command               TEXT        NOT NULL,
    requires_gpu          BOOLEAN     NOT NULL DEFAULT FALSE,
    env                   TEXT        NOT NULL DEFAULT '{}',
    tags                  TEXT        NOT NULL DEFAULT '[]',
    exp_id                TEXT        NOT NULL DEFAULT '',
    memo                  TEXT        NOT NULL DEFAULT '',
    timeout_sec           INTEGER     NOT NULL DEFAULT 0,
    worker_id             TEXT        NOT NULL DEFAULT '',
    project_id            TEXT        NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at            TIMESTAMPTZ,
    finished_at           TIMESTAMPTZ,
    exit_code             INTEGER,
    input_artifacts       TEXT        NOT NULL DEFAULT '[]',
    output_artifacts      TEXT        NOT NULL DEFAULT '[]',
    vram_required_gb      REAL        NOT NULL DEFAULT 0,
    capability            TEXT        NOT NULL DEFAULT '',
    params                TEXT        NOT NULL DEFAULT '{}',
    result                TEXT        NOT NULL DEFAULT '{}',
    submitted_by          TEXT,
    snapshot_version_hash TEXT        NOT NULL DEFAULT '',
    git_hash              TEXT        NOT NULL DEFAULT '',
    required_tags         TEXT        NOT NULL DEFAULT '[]',
    runtime               TEXT        NOT NULL DEFAULT '',
    exp_run_id            TEXT        NOT NULL DEFAULT '',
    best_metric           REAL,
    target_worker         TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX idx_hub_jobs_status     ON hub_jobs(status);
CREATE INDEX idx_hub_jobs_priority   ON hub_jobs(priority DESC, created_at ASC);
CREATE INDEX idx_hub_jobs_project    ON hub_jobs(project_id);
CREATE INDEX idx_hub_jobs_worker     ON hub_jobs(worker_id);
CREATE INDEX idx_hub_jobs_capability ON hub_jobs(capability);

-- NOTIFY trigger: wake up polling workers on new job insert
CREATE OR REPLACE FUNCTION hub_notify_new_job()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('new_job', NEW.id);
    RETURN NEW;
END;
$$;

CREATE TRIGGER hub_jobs_insert_notify
    AFTER INSERT ON hub_jobs
    FOR EACH ROW EXECUTE FUNCTION hub_notify_new_job();

-- ============================================================
-- workers
-- ============================================================
CREATE TABLE hub_workers (
    id             TEXT        PRIMARY KEY,
    hostname       TEXT        NOT NULL DEFAULT '',
    name           TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL DEFAULT 'online',
    gpu_count      INTEGER     NOT NULL DEFAULT 0,
    gpu_model      TEXT        NOT NULL DEFAULT '',
    total_vram     REAL        NOT NULL DEFAULT 0,
    free_vram      REAL        NOT NULL DEFAULT 0,
    tags           TEXT        NOT NULL DEFAULT '[]',
    project_id     TEXT        NOT NULL DEFAULT '',
    version        TEXT        NOT NULL DEFAULT '',
    uptime_sec     INTEGER     NOT NULL DEFAULT 0,
    last_job_at    TIMESTAMPTZ,
    mcp_url        TEXT        NOT NULL DEFAULT '',
    last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT now(),
    registered_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_workers_project ON hub_workers(project_id);
CREATE INDEX idx_hub_workers_status  ON hub_workers(status);

-- ============================================================
-- worker_history
-- ============================================================
CREATE TABLE hub_worker_history (
    id              TEXT        NOT NULL,
    hostname        TEXT        NOT NULL DEFAULT '',
    gpu_model       TEXT        NOT NULL DEFAULT '',
    project_id      TEXT        NOT NULL DEFAULT '',
    registered_at   TIMESTAMPTZ NOT NULL,
    deregistered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_worker_history_id ON hub_worker_history(id);

-- ============================================================
-- leases
-- ============================================================
CREATE TABLE hub_leases (
    id         TEXT        PRIMARY KEY,
    job_id     TEXT        NOT NULL,
    worker_id  TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_hub_leases_worker  ON hub_leases(worker_id);
CREATE INDEX idx_hub_leases_expires ON hub_leases(expires_at);
CREATE INDEX idx_hub_leases_job     ON hub_leases(job_id);

-- ============================================================
-- capabilities
-- ============================================================
CREATE TABLE hub_capabilities (
    id           TEXT        PRIMARY KEY,
    worker_id    TEXT        NOT NULL,
    name         TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    input_schema TEXT        NOT NULL DEFAULT '{}',
    tags         TEXT        NOT NULL DEFAULT '[]',
    version      TEXT        NOT NULL DEFAULT '',
    command      TEXT        NOT NULL DEFAULT '',
    project_id   TEXT        NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(worker_id, name)
);

CREATE INDEX idx_hub_capabilities_name    ON hub_capabilities(name);
CREATE INDEX idx_hub_capabilities_project ON hub_capabilities(project_id);
CREATE INDEX idx_hub_capabilities_worker  ON hub_capabilities(worker_id);
