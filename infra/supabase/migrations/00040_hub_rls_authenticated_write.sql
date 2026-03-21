-- Migration: 00040_hub_rls_authenticated_write
-- Allow authenticated users to INSERT/UPDATE jobs and workers.
-- Previously only service_role could write; now authenticated users
-- (with valid JWT from cq auth login) can submit jobs and register workers.

-- hub_jobs: authenticated can INSERT (submit jobs) and UPDATE (cancel)
CREATE POLICY "hub_jobs: authenticated can insert"
    ON hub_jobs FOR INSERT
    WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "hub_jobs: authenticated can update"
    ON hub_jobs FOR UPDATE
    USING (auth.role() = 'authenticated')
    WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "hub_jobs: authenticated can select all"
    ON hub_jobs FOR SELECT
    USING (auth.role() = 'authenticated');

-- hub_workers: authenticated can INSERT/UPDATE (register, heartbeat)
CREATE POLICY "hub_workers: authenticated can insert"
    ON hub_workers FOR INSERT
    WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "hub_workers: authenticated can update"
    ON hub_workers FOR UPDATE
    USING (auth.role() = 'authenticated')
    WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "hub_workers: authenticated can select all"
    ON hub_workers FOR SELECT
    USING (auth.role() = 'authenticated');

-- hub_leases: authenticated can INSERT/UPDATE/SELECT (claim, renew)
CREATE POLICY "hub_leases: authenticated can insert"
    ON hub_leases FOR INSERT
    WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "hub_leases: authenticated can update"
    ON hub_leases FOR UPDATE
    USING (auth.role() = 'authenticated')
    WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "hub_leases: authenticated can select"
    ON hub_leases FOR SELECT
    USING (auth.role() = 'authenticated');

CREATE POLICY "hub_leases: authenticated can delete"
    ON hub_leases FOR DELETE
    USING (auth.role() = 'authenticated');

-- hub_capabilities: authenticated can INSERT/SELECT
CREATE POLICY "hub_capabilities: authenticated can insert"
    ON hub_capabilities FOR INSERT
    WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "hub_capabilities: authenticated can select all"
    ON hub_capabilities FOR SELECT
    USING (auth.role() = 'authenticated');

CREATE POLICY "hub_capabilities: authenticated can delete"
    ON hub_capabilities FOR DELETE
    USING (auth.role() = 'authenticated');

-- hub_job_logs: authenticated can INSERT/SELECT
CREATE POLICY "hub_job_logs: authenticated can insert"
    ON hub_job_logs FOR INSERT
    WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "hub_job_logs: authenticated can select"
    ON hub_job_logs FOR SELECT
    USING (auth.role() = 'authenticated');
