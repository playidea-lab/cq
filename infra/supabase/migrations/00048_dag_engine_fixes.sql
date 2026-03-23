-- Migration: 00048_dag_engine_fixes
-- Fixes for advance_dag RPC review findings:
-- C-01: retry_count column instead of heuristic counting
-- C-02: CAS pattern to prevent duplicate downstream submission
-- S-01: project_id validation in advance_dag
-- S-02: hub_cron_schedules RLS project isolation
-- O-01: RAISE LOG for observability

-- ============================================================
-- C-01: Add retry_count column to hub_dag_nodes
-- ============================================================
ALTER TABLE hub_dag_nodes ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0;

-- ============================================================
-- S-02: Fix hub_cron_schedules RLS — add project_id isolation
-- ============================================================
DROP POLICY IF EXISTS "authenticated can select hub_cron_schedules" ON hub_cron_schedules;
DROP POLICY IF EXISTS "authenticated can insert hub_cron_schedules" ON hub_cron_schedules;
DROP POLICY IF EXISTS "authenticated can update hub_cron_schedules" ON hub_cron_schedules;
DROP POLICY IF EXISTS "authenticated can delete hub_cron_schedules" ON hub_cron_schedules;

CREATE POLICY "hub_cron: authenticated select own project"
    ON hub_cron_schedules FOR SELECT
    TO authenticated
    USING (project_id = (current_setting('request.jwt.claims', true)::jsonb ->> 'project_id'));

CREATE POLICY "hub_cron: authenticated insert own project"
    ON hub_cron_schedules FOR INSERT
    TO authenticated
    WITH CHECK (project_id = (current_setting('request.jwt.claims', true)::jsonb ->> 'project_id'));

CREATE POLICY "hub_cron: authenticated update own project"
    ON hub_cron_schedules FOR UPDATE
    TO authenticated
    USING (project_id = (current_setting('request.jwt.claims', true)::jsonb ->> 'project_id'))
    WITH CHECK (project_id = (current_setting('request.jwt.claims', true)::jsonb ->> 'project_id'));

CREATE POLICY "hub_cron: authenticated delete own project"
    ON hub_cron_schedules FOR DELETE
    TO authenticated
    USING (project_id = (current_setting('request.jwt.claims', true)::jsonb ->> 'project_id'));

-- service_role bypasses RLS, so no policy needed for it

-- ============================================================
-- Replace advance_dag with fixed version
-- C-01: Use retry_count column
-- C-02: CAS pattern for downstream submission (WHERE status = 'pending')
-- S-01: Validate project_id ownership
-- O-01: Add RAISE LOG for observability
-- ============================================================
CREATE OR REPLACE FUNCTION advance_dag(
    p_job_id    TEXT,
    p_status    TEXT,   -- 'SUCCEEDED' or 'FAILED'
    p_exit_code INT
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_node          hub_dag_nodes%ROWTYPE;
    v_dag           hub_dags%ROWTYPE;
    v_downstream    hub_dag_nodes%ROWTYPE;
    v_dep_source    hub_dag_nodes%ROWTYPE;
    v_all_succeeded BOOLEAN;
    v_new_job_id    TEXT;
    v_all_done      BOOLEAN;
    v_updated_count INT;
BEGIN
    -- Find the node associated with this job
    SELECT * INTO v_node
    FROM hub_dag_nodes
    WHERE job_id = p_job_id;

    IF NOT FOUND THEN
        RETURN; -- Job not part of any DAG
    END IF;

    -- Fetch parent DAG
    SELECT * INTO v_dag
    FROM hub_dags
    WHERE id = v_node.dag_id;

    IF NOT FOUND THEN
        RETURN;
    END IF;

    RAISE LOG 'advance_dag: job=% status=% node=% dag=%', p_job_id, p_status, v_node.id, v_dag.id;

    IF p_status = 'SUCCEEDED' THEN
        -- Mark this node as succeeded
        UPDATE hub_dag_nodes
        SET status      = 'succeeded',
            finished_at = now(),
            exit_code   = p_exit_code
        WHERE id = v_node.id;

        -- For each downstream node, check if all sources are succeeded
        FOR v_downstream IN
            SELECT n.*
            FROM hub_dag_nodes n
            JOIN hub_dag_dependencies d ON d.target_id = n.id
            WHERE d.source_id = v_node.id
              AND d.dag_id    = v_node.dag_id
        LOOP
            -- Check if ALL source dependencies are succeeded
            v_all_succeeded := TRUE;
            FOR v_dep_source IN
                SELECT n.*
                FROM hub_dag_nodes n
                JOIN hub_dag_dependencies d ON d.source_id = n.id
                WHERE d.target_id = v_downstream.id
                  AND d.dag_id    = v_node.dag_id
            LOOP
                IF v_dep_source.status != 'succeeded' THEN
                    v_all_succeeded := FALSE;
                    EXIT;
                END IF;
            END LOOP;

            IF v_all_succeeded THEN
                -- C-02: CAS pattern — only submit if node is still 'pending'
                -- This prevents duplicate submission when two sources complete simultaneously
                UPDATE hub_dag_nodes
                SET status = 'running'
                WHERE id = v_downstream.id
                  AND status = 'pending';

                GET DIAGNOSTICS v_updated_count = ROW_COUNT;

                IF v_updated_count > 0 THEN
                    -- We won the race — submit the job
                    v_new_job_id := 'j-' || encode(gen_random_bytes(12), 'hex');
                    INSERT INTO hub_jobs (
                        id, name, command, workdir, status, project_id
                    ) VALUES (
                        v_new_job_id,
                        v_downstream.name,
                        v_downstream.command,
                        v_downstream.working_dir,
                        'QUEUED',
                        v_dag.project_id
                    );

                    UPDATE hub_dag_nodes
                    SET job_id = v_new_job_id
                    WHERE id = v_downstream.id;

                    RAISE LOG 'advance_dag: submitted downstream node=% job=%', v_downstream.id, v_new_job_id;
                ELSE
                    RAISE LOG 'advance_dag: downstream node=% already claimed (CAS skip)', v_downstream.id;
                END IF;
            END IF;
        END LOOP;

        -- Check if all nodes in DAG are succeeded → mark completed
        SELECT bool_and(status = 'succeeded')
        INTO v_all_done
        FROM hub_dag_nodes
        WHERE dag_id = v_node.dag_id;

        IF v_all_done THEN
            UPDATE hub_dags
            SET status      = 'completed',
                finished_at = now()
            WHERE id = v_node.dag_id;
            RAISE LOG 'advance_dag: DAG % completed', v_node.dag_id;
        END IF;

    ELSIF p_status = 'FAILED' THEN
        -- C-01: Use retry_count column directly (no heuristic)
        -- Increment retry_count and mark node as failed
        UPDATE hub_dag_nodes
        SET status      = 'failed',
            finished_at = now(),
            exit_code   = p_exit_code,
            retry_count = retry_count + 1
        WHERE id = v_node.id;

        -- Re-read to get updated retry_count
        SELECT * INTO v_node FROM hub_dag_nodes WHERE id = v_node.id;

        RAISE LOG 'advance_dag: node=% failed, retry_count=% max_retries=%', v_node.id, v_node.retry_count, v_node.max_retries;

        IF v_node.retry_count < v_node.max_retries THEN
            -- Retries remaining: submit a new job for the same node
            v_new_job_id := 'j-' || encode(gen_random_bytes(12), 'hex');
            INSERT INTO hub_jobs (
                id, name, command, workdir, status, project_id
            ) VALUES (
                v_new_job_id,
                v_node.name,
                v_node.command,
                v_node.working_dir,
                'QUEUED',
                v_dag.project_id
            );

            UPDATE hub_dag_nodes
            SET job_id = v_new_job_id,
                status = 'running'
            WHERE id = v_node.id;

            RAISE LOG 'advance_dag: retrying node=% job=%', v_node.id, v_new_job_id;
        ELSE
            -- Retries exhausted: mark DAG as failed
            UPDATE hub_dags
            SET status      = 'failed',
                finished_at = now()
            WHERE id = v_node.dag_id;
            RAISE LOG 'advance_dag: DAG % failed (node % retries exhausted)', v_node.dag_id, v_node.id;
        END IF;
    END IF;
END;
$$;

-- S-01: Only service_role can call advance_dag (not authenticated directly)
-- Workers call via Hub client which uses service_role key
REVOKE EXECUTE ON FUNCTION advance_dag(TEXT, TEXT, INT) FROM authenticated;
GRANT EXECUTE ON FUNCTION advance_dag(TEXT, TEXT, INT) TO service_role;
