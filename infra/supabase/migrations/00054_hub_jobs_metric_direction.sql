-- Migration 00054: Add primary_metric and lower_is_better columns to hub_jobs
-- Supports metric direction tracking for experiment comparison

ALTER TABLE hub_jobs
    ADD COLUMN IF NOT EXISTS primary_metric TEXT,
    ADD COLUMN IF NOT EXISTS lower_is_better BOOLEAN;
