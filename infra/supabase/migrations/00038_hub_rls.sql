-- Migration: 00038_hub_rls
-- Row Level Security policies for Hub tables.
-- Pattern: project_id TEXT-based isolation via hub_api_keys lookup.
-- Hub uses API-key auth (not Supabase auth), so RLS uses service_role bypass
-- for server-side operations, while anon/authenticated roles are restricted.
--
-- Design:
--   - service_role: full access (Hub server uses service role key)
--   - authenticated: members can view their own project's data (JWT project_id claim)
--   - anon: no access

-- Helper function: extract project_id from JWT claims (set by Hub on user JWTs)
CREATE OR REPLACE FUNCTION hub_jwt_project_id()
RETURNS TEXT LANGUAGE sql STABLE AS $$
    SELECT COALESCE(
        current_setting('request.jwt.claims', TRUE)::jsonb ->> 'project_id',
        ''
    );
$$;

-- ============================================================
-- RLS: hub_jobs
-- ============================================================
ALTER TABLE hub_jobs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_jobs: service_role full access"
    ON hub_jobs FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

CREATE POLICY "hub_jobs: members can view own project"
    ON hub_jobs FOR SELECT
    USING (project_id = hub_jwt_project_id() AND hub_jwt_project_id() != '');

-- ============================================================
-- RLS: hub_workers
-- ============================================================
ALTER TABLE hub_workers ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_workers: service_role full access"
    ON hub_workers FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

CREATE POLICY "hub_workers: members can view own project"
    ON hub_workers FOR SELECT
    USING (project_id = hub_jwt_project_id() AND hub_jwt_project_id() != '');

-- ============================================================
-- RLS: hub_worker_history
-- ============================================================
ALTER TABLE hub_worker_history ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_worker_history: service_role full access"
    ON hub_worker_history FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

CREATE POLICY "hub_worker_history: members can view own project"
    ON hub_worker_history FOR SELECT
    USING (project_id = hub_jwt_project_id() AND hub_jwt_project_id() != '');

-- ============================================================
-- RLS: hub_leases
-- ============================================================
ALTER TABLE hub_leases ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_leases: service_role full access"
    ON hub_leases FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_capabilities
-- ============================================================
ALTER TABLE hub_capabilities ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_capabilities: service_role full access"
    ON hub_capabilities FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

CREATE POLICY "hub_capabilities: members can view own project"
    ON hub_capabilities FOR SELECT
    USING (project_id = hub_jwt_project_id() AND hub_jwt_project_id() != '');

-- ============================================================
-- RLS: hub_dags
-- ============================================================
ALTER TABLE hub_dags ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_dags: service_role full access"
    ON hub_dags FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

CREATE POLICY "hub_dags: members can view own project"
    ON hub_dags FOR SELECT
    USING (project_id = hub_jwt_project_id() AND hub_jwt_project_id() != '');

-- ============================================================
-- RLS: hub_dag_nodes
-- ============================================================
ALTER TABLE hub_dag_nodes ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_dag_nodes: service_role full access"
    ON hub_dag_nodes FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_dag_dependencies
-- ============================================================
ALTER TABLE hub_dag_dependencies ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_dag_dependencies: service_role full access"
    ON hub_dag_dependencies FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_edges
-- ============================================================
ALTER TABLE hub_edges ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_edges: service_role full access"
    ON hub_edges FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

CREATE POLICY "hub_edges: members can view own project"
    ON hub_edges FOR SELECT
    USING (project_id = hub_jwt_project_id() AND hub_jwt_project_id() != '');

-- ============================================================
-- RLS: hub_deploy_rules
-- ============================================================
ALTER TABLE hub_deploy_rules ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_deploy_rules: service_role full access"
    ON hub_deploy_rules FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

CREATE POLICY "hub_deploy_rules: members can view own project"
    ON hub_deploy_rules FOR SELECT
    USING (project_id = hub_jwt_project_id() AND hub_jwt_project_id() != '');

-- ============================================================
-- RLS: hub_deployments
-- ============================================================
ALTER TABLE hub_deployments ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_deployments: service_role full access"
    ON hub_deployments FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

CREATE POLICY "hub_deployments: members can view own project"
    ON hub_deployments FOR SELECT
    USING (project_id = hub_jwt_project_id() AND hub_jwt_project_id() != '');

-- ============================================================
-- RLS: hub_deploy_targets
-- ============================================================
ALTER TABLE hub_deploy_targets ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_deploy_targets: service_role full access"
    ON hub_deploy_targets FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_artifacts
-- ============================================================
ALTER TABLE hub_artifacts ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_artifacts: service_role full access"
    ON hub_artifacts FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_api_keys
-- ============================================================
ALTER TABLE hub_api_keys ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_api_keys: service_role full access"
    ON hub_api_keys FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_metrics
-- ============================================================
ALTER TABLE hub_metrics ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_metrics: service_role full access"
    ON hub_metrics FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_job_logs
-- ============================================================
ALTER TABLE hub_job_logs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_job_logs: service_role full access"
    ON hub_job_logs FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_job_durations
-- ============================================================
ALTER TABLE hub_job_durations ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_job_durations: service_role full access"
    ON hub_job_durations FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_device_sessions
-- ============================================================
ALTER TABLE hub_device_sessions ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_device_sessions: service_role full access"
    ON hub_device_sessions FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_c9_research_state
-- ============================================================
ALTER TABLE hub_c9_research_state ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_c9_research_state: service_role full access"
    ON hub_c9_research_state FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

CREATE POLICY "hub_c9_research_state: members can view own project"
    ON hub_c9_research_state FOR SELECT
    USING (project_id = hub_jwt_project_id() AND hub_jwt_project_id() != '');

-- ============================================================
-- RLS: hub_edge_control_queue
-- ============================================================
ALTER TABLE hub_edge_control_queue ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_edge_control_queue: service_role full access"
    ON hub_edge_control_queue FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_experiment_runs
-- ============================================================
ALTER TABLE hub_experiment_runs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_experiment_runs: service_role full access"
    ON hub_experiment_runs FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

-- ============================================================
-- RLS: hub_experiment_checkpoints
-- ============================================================
ALTER TABLE hub_experiment_checkpoints ENABLE ROW LEVEL SECURITY;

CREATE POLICY "hub_experiment_checkpoints: service_role full access"
    ON hub_experiment_checkpoints FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');
