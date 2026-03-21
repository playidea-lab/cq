-- Migration: 00039_hub_rpc
-- Atomic RPC functions for Hub operations.
-- All functions are SECURITY DEFINER to bypass RLS when called via PostgREST.
-- Pattern: Hub server calls these via service_role key.

-- ============================================================
-- claim_job
-- Atomically picks the highest-priority QUEUED job matching the
-- worker's capabilities, marks it RUNNING, and creates a lease.
-- Returns: jsonb {job: {...}, lease_id: TEXT} or NULL if no job available.
-- ============================================================
CREATE OR REPLACE FUNCTION claim_job(
    p_worker_id    TEXT,
    p_capabilities TEXT[],
    p_project_id   TEXT
)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_job        hub_jobs%ROWTYPE;
    v_lease_id   TEXT;
    v_now        TIMESTAMPTZ := now();
    v_expires_at TIMESTAMPTZ := now() + INTERVAL '5 minutes';
BEGIN
    -- Select the best QUEUED job with FOR UPDATE SKIP LOCKED to prevent
    -- concurrent workers from claiming the same job.
    SELECT j.*
    INTO v_job
    FROM hub_jobs j
    WHERE j.status = 'QUEUED'
      AND (p_project_id = '' OR j.project_id = p_project_id)
      -- capability filter: empty capability matches all; otherwise worker must have it
      AND (j.capability = '' OR j.capability = ANY(p_capabilities))
      -- required_tags: if job has required tags, worker capabilities must cover them all
      -- JSON array stored as TEXT; cast to jsonb for operator support
      AND (
          j.required_tags = '[]'
          OR j.required_tags = ''
          OR (
              SELECT bool_and(tag = ANY(p_capabilities))
              FROM jsonb_array_elements_text(j.required_tags::jsonb) AS tag
          )
      )
    ORDER BY j.priority DESC, j.created_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED;

    -- No matching job found
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;

    -- Claim the job: mark as RUNNING
    UPDATE hub_jobs
    SET status     = 'RUNNING',
        started_at = v_now,
        worker_id  = p_worker_id
    WHERE id = v_job.id
      AND status = 'QUEUED';

    -- Generate a lease ID (prefix 'l-' + random hex)
    v_lease_id := 'l-' || encode(gen_random_bytes(8), 'hex');

    -- Insert the lease
    INSERT INTO hub_leases (id, job_id, worker_id, created_at, expires_at)
    VALUES (v_lease_id, v_job.id, p_worker_id, v_now, v_expires_at);

    -- Update worker to busy (best-effort)
    UPDATE hub_workers
    SET status = 'busy'
    WHERE id = p_worker_id;

    -- Re-read the updated job row for accurate response
    SELECT * INTO v_job FROM hub_jobs WHERE id = v_job.id;

    RETURN jsonb_build_object(
        'lease_id', v_lease_id,
        'job',      row_to_json(v_job)::jsonb
    );
END;
$$;

-- ============================================================
-- release_lease_and_requeue
-- Rolls back a push dispatch failure: deletes the lease and
-- restores the job to QUEUED.
-- ============================================================
CREATE OR REPLACE FUNCTION release_lease_and_requeue(
    p_lease_id TEXT
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_job_id    TEXT;
    v_worker_id TEXT;
BEGIN
    -- Find the lease
    SELECT job_id, worker_id
    INTO v_job_id, v_worker_id
    FROM hub_leases
    WHERE id = p_lease_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'lease not found: %', p_lease_id;
    END IF;

    -- Restore job to QUEUED
    UPDATE hub_jobs
    SET status     = 'QUEUED',
        started_at = NULL,
        worker_id  = ''
    WHERE id = v_job_id
      AND status = 'RUNNING';

    -- Delete the lease
    DELETE FROM hub_leases WHERE id = p_lease_id;

    -- Restore worker to online (best-effort)
    UPDATE hub_workers
    SET status = 'online'
    WHERE id = v_worker_id;
END;
$$;

-- ============================================================
-- register_worker
-- UPSERTs a worker record and replaces its capabilities.
-- Returns: jsonb with the worker row.
-- ============================================================
CREATE OR REPLACE FUNCTION register_worker(
    p_worker_id    TEXT,
    p_hostname     TEXT,
    p_capabilities TEXT[],
    p_mcp_url      TEXT,
    p_project_id   TEXT
)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_now  TIMESTAMPTZ := now();
    v_cap  TEXT;
    v_worker hub_workers%ROWTYPE;
BEGIN
    -- UPSERT the worker record
    INSERT INTO hub_workers (
        id, hostname, status, mcp_url, project_id,
        last_heartbeat, registered_at
    )
    VALUES (
        p_worker_id, p_hostname, 'online', p_mcp_url, p_project_id,
        v_now, v_now
    )
    ON CONFLICT (id) DO UPDATE
        SET hostname       = EXCLUDED.hostname,
            status         = 'online',
            mcp_url        = EXCLUDED.mcp_url,
            project_id     = EXCLUDED.project_id,
            last_heartbeat = v_now;

    -- Replace capabilities: delete old ones for this worker, insert new ones
    DELETE FROM hub_capabilities WHERE worker_id = p_worker_id;

    FOREACH v_cap IN ARRAY p_capabilities LOOP
        INSERT INTO hub_capabilities (
            id, worker_id, name, project_id, updated_at
        )
        VALUES (
            'cap-' || encode(gen_random_bytes(8), 'hex'),
            p_worker_id,
            v_cap,
            p_project_id,
            v_now
        )
        ON CONFLICT (worker_id, name) DO UPDATE
            SET updated_at = v_now;
    END LOOP;

    SELECT * INTO v_worker FROM hub_workers WHERE id = p_worker_id;

    RETURN row_to_json(v_worker)::jsonb;
END;
$$;

-- ============================================================
-- renew_lease
-- Extends a lease's expires_at by p_duration_sec seconds.
-- ============================================================
CREATE OR REPLACE FUNCTION renew_lease(
    p_lease_id    TEXT,
    p_duration_sec INT
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
    UPDATE hub_leases
    SET expires_at = now() + (p_duration_sec || ' seconds')::INTERVAL
    WHERE id = p_lease_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'lease not found: %', p_lease_id;
    END IF;
END;
$$;

-- ============================================================
-- complete_job
-- Marks a job as finished (SUCCEEDED/FAILED/CANCELLED),
-- removes the lease, and sets the worker back to online.
-- ============================================================
CREATE OR REPLACE FUNCTION complete_job(
    p_job_id    TEXT,
    p_status    TEXT,
    p_exit_code INT,
    p_worker_id TEXT
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
    -- Update job status
    UPDATE hub_jobs
    SET status      = p_status,
        finished_at = now(),
        exit_code   = p_exit_code
    WHERE id = p_job_id
      AND status = 'RUNNING';

    IF NOT FOUND THEN
        RAISE EXCEPTION 'job % not found or not RUNNING', p_job_id;
    END IF;

    -- Remove any leases for this job
    DELETE FROM hub_leases WHERE job_id = p_job_id;

    -- Set worker back to online
    UPDATE hub_workers
    SET status      = 'online',
        last_job_at = now()
    WHERE id = p_worker_id;
END;
$$;

-- ============================================================
-- Grant execute to service_role and authenticated roles
-- (PostgREST exposes RPC to callers with those roles)
-- ============================================================
GRANT EXECUTE ON FUNCTION claim_job(TEXT, TEXT[], TEXT)           TO service_role, authenticated;
GRANT EXECUTE ON FUNCTION release_lease_and_requeue(TEXT)         TO service_role, authenticated;
GRANT EXECUTE ON FUNCTION register_worker(TEXT, TEXT, TEXT[], TEXT, TEXT) TO service_role, authenticated;
GRANT EXECUTE ON FUNCTION renew_lease(TEXT, INT)                  TO service_role, authenticated;
GRANT EXECUTE ON FUNCTION complete_job(TEXT, TEXT, INT, TEXT)     TO service_role, authenticated;
