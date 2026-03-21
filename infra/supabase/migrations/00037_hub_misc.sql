-- Migration: 00037_hub_misc
-- Hub tables: api_keys, metrics, job_logs, job_durations, device_sessions,
--             c9_research_state, edge_control_queue,
--             experiment_runs, experiment_checkpoints
-- Converted from c5/internal/store/sqlite.go SQLite schema.

-- ============================================================
-- api_keys
-- ============================================================
CREATE TABLE hub_api_keys (
    key_hash    TEXT        PRIMARY KEY,
    project_id  TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    scope       TEXT        NOT NULL DEFAULT 'full',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_api_keys_project ON hub_api_keys(project_id);

-- ============================================================
-- metrics
-- ============================================================
CREATE TABLE hub_metrics (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id     TEXT        NOT NULL,
    step       INTEGER     NOT NULL,
    metrics    TEXT        NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_metrics_job ON hub_metrics(job_id, step);

-- ============================================================
-- job_logs
-- ============================================================
CREATE TABLE hub_job_logs (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id     TEXT        NOT NULL,
    line       TEXT        NOT NULL,
    stream     TEXT        NOT NULL DEFAULT 'stdout',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_job_logs_job ON hub_job_logs(job_id);

-- ============================================================
-- job_durations
-- ============================================================
CREATE TABLE hub_job_durations (
    id           BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    command_hash TEXT        NOT NULL,
    duration_sec REAL        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_job_durations_hash ON hub_job_durations(command_hash);

-- ============================================================
-- device_sessions
-- ============================================================
CREATE TABLE hub_device_sessions (
    state          TEXT    PRIMARY KEY,
    user_code      TEXT    UNIQUE NOT NULL,
    csrf_token     TEXT    NOT NULL DEFAULT '',
    code_challenge TEXT    NOT NULL,
    supabase_url   TEXT    NOT NULL,
    auth_code      TEXT    NOT NULL DEFAULT '',
    status         TEXT    NOT NULL DEFAULT 'pending',
    poll_count     INTEGER NOT NULL DEFAULT 0,
    token_attempts INTEGER NOT NULL DEFAULT 0,
    expires_at     BIGINT  NOT NULL,
    created_at     BIGINT  NOT NULL
);

CREATE INDEX idx_hub_device_sessions_user_code  ON hub_device_sessions(user_code);
CREATE INDEX idx_hub_device_sessions_expires_at ON hub_device_sessions(expires_at);

-- ============================================================
-- c9_research_state
-- ============================================================
CREATE TABLE hub_c9_research_state (
    project_id      TEXT        NOT NULL DEFAULT '',
    round           INTEGER     NOT NULL DEFAULT 1,
    phase           TEXT        NOT NULL DEFAULT 'CONFERENCE',
    version         INTEGER     NOT NULL DEFAULT 0,
    lock_holder     TEXT        NOT NULL DEFAULT '',
    lock_expires_at TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id)
);

-- ============================================================
-- edge_control_queue
-- ============================================================
CREATE TABLE hub_edge_control_queue (
    id         TEXT        PRIMARY KEY,
    edge_id    TEXT        NOT NULL,
    action     TEXT        NOT NULL,
    params     TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_edge_control_queue_edge ON hub_edge_control_queue(edge_id);

-- ============================================================
-- experiment_runs
-- ============================================================
CREATE TABLE hub_experiment_runs (
    run_id       TEXT        PRIMARY KEY,
    name         TEXT        NOT NULL DEFAULT '',
    capability   TEXT        NOT NULL DEFAULT '',
    status       TEXT        NOT NULL DEFAULT 'running',
    best_metric  REAL,
    final_metric REAL,
    summary      TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_experiment_runs_capability ON hub_experiment_runs(capability);

-- ============================================================
-- experiment_checkpoints
-- ============================================================
CREATE TABLE hub_experiment_checkpoints (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id     TEXT        NOT NULL,
    metric     REAL        NOT NULL,
    path       TEXT        NOT NULL DEFAULT '',
    is_best    BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_experiment_checkpoints_run ON hub_experiment_checkpoints(run_id);
