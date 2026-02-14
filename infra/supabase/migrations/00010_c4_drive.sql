-- Migration: 00010_c4_drive
-- Creates drive file metadata table and Supabase Storage bucket for file sharing.
-- RLS: Only project members can access drive files.

-- ============================================================
-- Supabase Storage bucket
-- ============================================================
INSERT INTO storage.buckets (id, name, public)
VALUES ('c4-drive', 'c4-drive', false)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- c4_drive_files — file metadata
-- ============================================================
CREATE TABLE c4_drive_files (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    UUID NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    path          TEXT NOT NULL,
    storage_path  TEXT NOT NULL,
    size_bytes    BIGINT NOT NULL DEFAULT 0,
    content_hash  TEXT NOT NULL DEFAULT '',
    content_type  TEXT NOT NULL DEFAULT 'application/octet-stream',
    uploaded_by   UUID REFERENCES auth.users(id),
    is_folder     BOOLEAN NOT NULL DEFAULT FALSE,
    metadata      JSONB DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_drive_project ON c4_drive_files(project_id);
CREATE UNIQUE INDEX idx_drive_path ON c4_drive_files(project_id, path);

-- ============================================================
-- Trigger: auto-update updated_at
-- ============================================================
CREATE TRIGGER trg_c4_drive_files_updated_at
    BEFORE UPDATE ON c4_drive_files
    FOR EACH ROW
    EXECUTE FUNCTION c4_set_updated_at();

-- ============================================================
-- RLS: c4_drive_files
-- ============================================================
ALTER TABLE c4_drive_files ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view drive files"
    ON c4_drive_files FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can upload drive files"
    ON c4_drive_files FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update drive files"
    ON c4_drive_files FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can delete drive files"
    ON c4_drive_files FOR DELETE
    USING (c4_is_project_member(project_id));

-- ============================================================
-- RLS: Supabase Storage objects for c4-drive bucket
-- ============================================================
CREATE POLICY "Project members can upload to drive"
    ON storage.objects FOR INSERT
    WITH CHECK (
        bucket_id = 'c4-drive'
        AND auth.uid() IS NOT NULL
    );

CREATE POLICY "Project members can read from drive"
    ON storage.objects FOR SELECT
    USING (
        bucket_id = 'c4-drive'
        AND auth.uid() IS NOT NULL
    );

CREATE POLICY "Project members can delete from drive"
    ON storage.objects FOR DELETE
    USING (
        bucket_id = 'c4-drive'
        AND auth.uid() IS NOT NULL
    );
