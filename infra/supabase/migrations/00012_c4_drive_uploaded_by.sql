-- Migration: 00012_c4_drive_uploaded_by
-- Set uploaded_by column default to auth.uid() so PostgREST auto-fills from JWT.

ALTER TABLE c4_drive_files ALTER COLUMN uploaded_by SET DEFAULT auth.uid();
