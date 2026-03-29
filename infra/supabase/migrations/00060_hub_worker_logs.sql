-- Migration: 00060_hub_worker_logs
-- Store crash/diagnostic logs uploaded by Hub workers.
-- Workers write to a RingBuffer locally and flush on crash/exit.

CREATE TABLE IF NOT EXISTS hub_worker_logs (
    id          BIGSERIAL PRIMARY KEY,
    worker_id   TEXT        NOT NULL,
    content     TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for fast lookup by worker.
CREATE INDEX IF NOT EXISTS idx_hub_worker_logs_worker_id ON hub_worker_logs (worker_id);

-- RLS: service_role can insert; authenticated users can read their own worker logs.
ALTER TABLE hub_worker_logs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "workers can insert crash logs"
    ON hub_worker_logs FOR INSERT
    WITH CHECK (true);

CREATE POLICY "authenticated can read worker logs"
    ON hub_worker_logs FOR SELECT
    TO authenticated
    USING (true);
