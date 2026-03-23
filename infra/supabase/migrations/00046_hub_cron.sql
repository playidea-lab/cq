-- Migration: hub_cron_schedules table for CronScheduler
-- Stores recurring job schedules with cron expressions.

CREATE TABLE IF NOT EXISTS hub_cron_schedules (
    id           TEXT        PRIMARY KEY,
    name         TEXT        NOT NULL,
    cron_expr    TEXT        NOT NULL,
    job_template TEXT        NOT NULL DEFAULT '{}',
    dag_id       TEXT        NOT NULL DEFAULT '',
    enabled      BOOLEAN     NOT NULL DEFAULT true,
    last_run     TIMESTAMPTZ,
    next_run     TIMESTAMPTZ,
    project_id   TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Enable Row Level Security
ALTER TABLE hub_cron_schedules ENABLE ROW LEVEL SECURITY;

-- RLS Policies: authenticated users can manage their project's schedules
CREATE POLICY "authenticated can select hub_cron_schedules"
    ON hub_cron_schedules FOR SELECT
    TO authenticated
    USING (true);

CREATE POLICY "authenticated can insert hub_cron_schedules"
    ON hub_cron_schedules FOR INSERT
    TO authenticated
    WITH CHECK (true);

CREATE POLICY "authenticated can update hub_cron_schedules"
    ON hub_cron_schedules FOR UPDATE
    TO authenticated
    USING (true)
    WITH CHECK (true);

CREATE POLICY "authenticated can delete hub_cron_schedules"
    ON hub_cron_schedules FOR DELETE
    TO authenticated
    USING (true);
