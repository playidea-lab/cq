-- Add execution_mode column to c4_tasks (align with SQLite schema)
-- Values: 'worker' (default), 'direct'

ALTER TABLE c4_tasks
ADD COLUMN IF NOT EXISTS execution_mode TEXT NOT NULL DEFAULT 'worker';

COMMENT ON COLUMN c4_tasks.execution_mode IS 'Task execution mode: worker (isolated worktree) or direct (in-place)';
