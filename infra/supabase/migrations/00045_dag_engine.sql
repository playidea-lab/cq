-- Migration: 00045_dag_engine
-- advance_dag RPC: called after a job completes to automatically progress
-- the DAG by submitting downstream nodes and handling retries.

-- ============================================================
-- advance_dag
-- Called when a hub_job associated with a DAG node finishes.
-- Handles: downstream submission, retry logic, DAG completion/failure.
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
    v_retry_count   INT;
    v_new_job_id    TEXT;
    v_all_done      BOOLEAN;
    v_any_failed    BOOLEAN;
BEGIN
    -- Find the node associated with this job
    SELECT * INTO v_node
    FROM hub_dag_nodes
    WHERE job_id = p_job_id;

    IF NOT FOUND THEN
        -- Job not part of any DAG — nothing to do
        RETURN;
    END IF;

    -- Fetch parent DAG
    SELECT * INTO v_dag
    FROM hub_dags
    WHERE id = v_node.dag_id;

    IF NOT FOUND THEN
        RETURN;
    END IF;

    IF p_status = 'SUCCEEDED' THEN
        -- Mark this node as succeeded
        UPDATE hub_dag_nodes
        SET status      = 'succeeded',
            finished_at = now(),
            exit_code   = p_exit_code
        WHERE id = v_node.id;

        -- For each downstream node (nodes that depend on this node as source)
        FOR v_downstream IN
            SELECT n.*
            FROM hub_dag_nodes n
            JOIN hub_dag_dependencies d ON d.target_id = n.id
            WHERE d.source_id = v_node.id
              AND d.dag_id    = v_node.dag_id
        LOOP
            -- Check if ALL source dependencies of this downstream node are succeeded
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
                -- Submit a new hub_job for this downstream node
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

                -- Link the node to the new job and mark it running
                UPDATE hub_dag_nodes
                SET job_id = v_new_job_id,
                    status = 'running'
                WHERE id = v_downstream.id;
            END IF;
        END LOOP;

        -- Check if all nodes in the DAG are now succeeded → mark DAG completed
        SELECT
            bool_and(status = 'succeeded'),
            bool_or(status = 'failed')
        INTO v_all_done, v_any_failed
        FROM hub_dag_nodes
        WHERE dag_id = v_node.dag_id;

        IF v_all_done THEN
            UPDATE hub_dags
            SET status      = 'completed',
                finished_at = now()
            WHERE id = v_node.dag_id;
        END IF;

    ELSIF p_status = 'FAILED' THEN
        -- Count how many FAILED jobs exist for this node (= retry count so far)
        SELECT COUNT(*)
        INTO v_retry_count
        FROM hub_jobs j
        JOIN hub_dag_nodes n ON n.job_id = j.id
        WHERE n.id = v_node.id
          AND j.status IN ('FAILED', 'CANCELLED');

        -- Also count historical failed jobs for this node via a separate approach:
        -- The retry count = total failed jobs for this dag_node_id
        SELECT COUNT(*)
        INTO v_retry_count
        FROM hub_jobs
        WHERE id IN (
            SELECT job_id FROM hub_dag_nodes WHERE id = v_node.id
        ) AND status = 'FAILED';

        -- A simpler heuristic: count how many times the node has been attempted
        -- by counting all hub_jobs rows that were created for this node.
        -- We track this by finding jobs with the same name in the same dag context.
        -- Since job_id column on hub_dag_nodes is overwritten each retry,
        -- we use a direct count of FAILED jobs for this node's current job_id history.
        -- The design note says: retry_count = number of FAILED jobs for that node.
        -- We count via checking how many times this node has failed:
        SELECT COUNT(*)
        INTO v_retry_count
        FROM hub_jobs hj
        WHERE hj.status = 'FAILED'
          AND hj.id = p_job_id;

        -- Since we just got a FAILED signal for p_job_id, count total failures
        -- across all jobs ever linked to this node requires audit table.
        -- Simpler: use node's existing exit_code/status history isn't stored.
        -- Use the approach from the task spec: count FAILED jobs for this node
        -- by matching on name + dag context via a subquery on hub_dag_nodes history.
        -- Given the constraint that job_id is overwritten, we count using:
        -- "how many times has this node been in failed state" via a dedicated counter.
        -- For now, implement as: retry_count = (node's current attempt number - 1)
        -- tracked by counting total job submissions where command+workdir matches
        -- within this dag. This is approximate; a dedicated retry_count column would
        -- be ideal but is not in scope.

        -- Pragmatic implementation: count FAILED jobs matching this node's command
        -- within the same project, submitted since the DAG started.
        SELECT COUNT(*)
        INTO v_retry_count
        FROM hub_jobs hj
        WHERE hj.command    = v_node.command
          AND hj.workdir    = v_node.working_dir
          AND hj.project_id = v_dag.project_id
          AND hj.status     = 'FAILED'
          AND hj.created_at >= v_dag.started_at;

        -- Mark current job as failed on the node
        UPDATE hub_dag_nodes
        SET status      = 'failed',
            finished_at = now(),
            exit_code   = p_exit_code
        WHERE id = v_node.id;

        IF v_retry_count < v_node.max_retries THEN
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

            -- Update node to point to retry job
            UPDATE hub_dag_nodes
            SET job_id = v_new_job_id,
                status = 'running'
            WHERE id = v_node.id;
        ELSE
            -- Retries exhausted: mark DAG as failed
            UPDATE hub_dags
            SET status      = 'failed',
                finished_at = now()
            WHERE id = v_node.dag_id;
        END IF;
    END IF;
END;
$$;

-- ============================================================
-- Grant execute to service_role and authenticated roles
-- ============================================================
GRANT EXECUTE ON FUNCTION advance_dag(TEXT, TEXT, INT) TO service_role, authenticated;
