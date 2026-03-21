-- Migration: 00035_hub_dags_edges
-- Hub tables: dags, dag_nodes, dag_dependencies, edges
-- Converted from c5/internal/store/sqlite.go SQLite schema.

-- ============================================================
-- dags
-- ============================================================
CREATE TABLE hub_dags (
    id          TEXT        PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    tags        TEXT        NOT NULL DEFAULT '[]',
    project_id  TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_hub_dags_project ON hub_dags(project_id);
CREATE INDEX idx_hub_dags_status  ON hub_dags(status);

-- ============================================================
-- dag_nodes
-- ============================================================
CREATE TABLE hub_dag_nodes (
    id          TEXT        PRIMARY KEY,
    dag_id      TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    command     TEXT        NOT NULL,
    working_dir TEXT        NOT NULL DEFAULT '.',
    env         TEXT        NOT NULL DEFAULT '{}',
    gpu_count   INTEGER     NOT NULL DEFAULT 0,
    max_retries INTEGER     NOT NULL DEFAULT 3,
    status      TEXT        NOT NULL DEFAULT 'pending',
    job_id      TEXT        NOT NULL DEFAULT '',
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    exit_code   INTEGER
);

CREATE INDEX idx_hub_dag_nodes_dag ON hub_dag_nodes(dag_id);

-- ============================================================
-- dag_dependencies
-- ============================================================
CREATE TABLE hub_dag_dependencies (
    id        BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dag_id    TEXT        NOT NULL,
    source_id TEXT        NOT NULL,
    target_id TEXT        NOT NULL,
    dep_type  TEXT        NOT NULL DEFAULT 'sequential'
);

CREATE INDEX idx_hub_dag_deps_dag ON hub_dag_dependencies(dag_id);

-- ============================================================
-- edges
-- ============================================================
CREATE TABLE hub_edges (
    id         TEXT        PRIMARY KEY,
    name       TEXT        NOT NULL,
    project_id TEXT        NOT NULL DEFAULT '',
    status     TEXT        NOT NULL DEFAULT 'online',
    tags       TEXT        NOT NULL DEFAULT '[]',
    arch       TEXT        NOT NULL DEFAULT '',
    runtime    TEXT        NOT NULL DEFAULT '',
    storage    REAL        NOT NULL DEFAULT 0,
    metadata   TEXT        NOT NULL DEFAULT '{}',
    last_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_edges_project ON hub_edges(project_id);
CREATE INDEX idx_hub_edges_status  ON hub_edges(status);
