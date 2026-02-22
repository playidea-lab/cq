-- Migration: add superseded_by column to c4_tasks
-- Used to mark stale R-tasks that have been replaced by a newer version
-- when REQUEST_CHANGES is called. This prevents stale reviews from being
-- picked up by AssignTask.

-- Up:
ALTER TABLE c4_tasks ADD COLUMN IF NOT EXISTS superseded_by TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_c4_tasks_superseded_by ON c4_tasks(superseded_by) WHERE superseded_by != '';

-- Down:
-- DROP INDEX IF EXISTS idx_c4_tasks_superseded_by;
-- ALTER TABLE c4_tasks DROP COLUMN superseded_by;
