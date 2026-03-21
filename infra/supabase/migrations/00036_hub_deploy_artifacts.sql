-- Migration: 00036_hub_deploy_artifacts
-- Hub tables: deploy_rules, deployments, deploy_targets, artifacts
-- Converted from c5/internal/store/sqlite.go SQLite schema.

-- ============================================================
-- deploy_rules
-- ============================================================
CREATE TABLE hub_deploy_rules (
    id                    TEXT        PRIMARY KEY,
    name                  TEXT        NOT NULL DEFAULT '',
    project_id            TEXT        NOT NULL DEFAULT '',
    trigger_expr          TEXT        NOT NULL,
    edge_filter           TEXT        NOT NULL,
    artifact_pattern      TEXT        NOT NULL,
    post_command          TEXT        NOT NULL DEFAULT '',
    enabled               BOOLEAN     NOT NULL DEFAULT TRUE,
    health_check          TEXT        NOT NULL DEFAULT '',
    health_check_timeout  INTEGER     NOT NULL DEFAULT 30,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_deploy_rules_project ON hub_deploy_rules(project_id);

-- ============================================================
-- deployments
-- ============================================================
CREATE TABLE hub_deployments (
    id          TEXT        PRIMARY KEY,
    rule_id     TEXT        NOT NULL DEFAULT '',
    job_id      TEXT        NOT NULL DEFAULT '',
    project_id  TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_hub_deployments_project ON hub_deployments(project_id);
CREATE INDEX idx_hub_deployments_rule    ON hub_deployments(rule_id);

-- ============================================================
-- deploy_targets
-- ============================================================
CREATE TABLE hub_deploy_targets (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    deploy_id  TEXT        NOT NULL,
    edge_id    TEXT        NOT NULL,
    edge_name  TEXT        NOT NULL DEFAULT '',
    status     TEXT        NOT NULL DEFAULT 'pending',
    error      TEXT        NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    done_at    TIMESTAMPTZ
);

CREATE INDEX idx_hub_deploy_targets_deploy ON hub_deploy_targets(deploy_id);

-- ============================================================
-- artifacts
-- ============================================================
CREATE TABLE hub_artifacts (
    id           TEXT        PRIMARY KEY,
    job_id       TEXT        NOT NULL,
    path         TEXT        NOT NULL,
    content_hash TEXT        NOT NULL DEFAULT '',
    size_bytes   BIGINT      NOT NULL DEFAULT 0,
    confirmed    BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hub_artifacts_job ON hub_artifacts(job_id);
