-- Migration: 00042_c4_tasks_missing_columns
-- Adds 7 columns to c4_tasks that exist in SQLite but are missing in PostgreSQL.
-- Uses IF NOT EXISTS for idempotency (safe to run multiple times).

ALTER TABLE c4_tasks ADD COLUMN IF NOT EXISTS review_decision_evidence TEXT DEFAULT '';
ALTER TABLE c4_tasks ADD COLUMN IF NOT EXISTS failure_signature         TEXT DEFAULT '';
ALTER TABLE c4_tasks ADD COLUMN IF NOT EXISTS blocked_attempts          INTEGER DEFAULT 0;
ALTER TABLE c4_tasks ADD COLUMN IF NOT EXISTS last_error                TEXT DEFAULT '';
ALTER TABLE c4_tasks ADD COLUMN IF NOT EXISTS files_changed             TEXT DEFAULT '';
ALTER TABLE c4_tasks ADD COLUMN IF NOT EXISTS session_id                TEXT DEFAULT '';
ALTER TABLE c4_tasks ADD COLUMN IF NOT EXISTS superseded_by             TEXT DEFAULT '';
