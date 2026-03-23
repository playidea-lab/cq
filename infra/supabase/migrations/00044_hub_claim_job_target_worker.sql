-- Migration: 00044_hub_claim_job_target_worker
-- Add target_worker filter to claim_job RPC.
-- Jobs with a non-empty target_worker are only claimable by that specific worker.
-- This fix activates the target_worker column that already exists in hub_jobs
-- but was previously not referenced in the WHERE clause.

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
      -- target_worker filter: empty = any worker can claim; otherwise only the named worker
      AND (j.target_worker = '' OR j.target_worker = p_worker_id)
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
