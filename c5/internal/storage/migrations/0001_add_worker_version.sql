-- Migration: add version column to workers table.
-- Applied inline in store/sqlite.go migrate() for existing deployments.
-- IF NOT EXISTS variant not supported by SQLite ALTER TABLE;
-- the duplicate-column error is suppressed in the migration loop.
ALTER TABLE workers ADD COLUMN version TEXT NOT NULL DEFAULT '';
