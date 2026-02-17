-- Migration: 00019_storage_rls_and_schema_fix
-- 1. Storage RLS: project membership verification (security fix)
-- 2. c4_tasks: remove unused 'review' from status CHECK

-- ============================================================
-- 1. Fix storage.objects RLS policies
--    BEFORE: only checks auth.uid() IS NOT NULL
--    AFTER:  joins c4_drive_files to verify project membership
-- ============================================================

-- Helper function: check if authenticated user is a member of the project
-- that owns the storage object (by extracting project_id from storage path).
-- Storage path format: {project_id}/{hash_prefix}/{filename}
CREATE OR REPLACE FUNCTION c4_storage_object_member_check(object_name TEXT)
RETURNS BOOLEAN AS $$
DECLARE
    v_project_id UUID;
BEGIN
    -- Extract project_id from the first path segment
    v_project_id := split_part(object_name, '/', 1)::UUID;
    RETURN c4_is_project_member(v_project_id);
EXCEPTION
    WHEN invalid_text_representation THEN
        -- object_name doesn't start with a valid UUID
        RETURN FALSE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER STABLE;

-- Drop old permissive policies
DROP POLICY IF EXISTS "Project members can upload to drive" ON storage.objects;
DROP POLICY IF EXISTS "Project members can read from drive" ON storage.objects;
DROP POLICY IF EXISTS "Project members can delete from drive" ON storage.objects;
DROP POLICY IF EXISTS "Project members can update drive objects" ON storage.objects;

-- Recreate with project membership verification
CREATE POLICY "Project members can upload to drive"
    ON storage.objects FOR INSERT
    WITH CHECK (
        bucket_id = 'c4-drive'
        AND auth.uid() IS NOT NULL
        AND c4_storage_object_member_check(name)
    );

CREATE POLICY "Project members can read from drive"
    ON storage.objects FOR SELECT
    USING (
        bucket_id = 'c4-drive'
        AND auth.uid() IS NOT NULL
        AND c4_storage_object_member_check(name)
    );

CREATE POLICY "Project members can delete from drive"
    ON storage.objects FOR DELETE
    USING (
        bucket_id = 'c4-drive'
        AND auth.uid() IS NOT NULL
        AND c4_storage_object_member_check(name)
    );

CREATE POLICY "Project members can update drive objects"
    ON storage.objects FOR UPDATE
    USING (
        bucket_id = 'c4-drive'
        AND auth.uid() IS NOT NULL
        AND c4_storage_object_member_check(name)
    )
    WITH CHECK (
        bucket_id = 'c4-drive'
        AND auth.uid() IS NOT NULL
        AND c4_storage_object_member_check(name)
    );

-- ============================================================
-- 2. Remove unused 'review' from c4_tasks status CHECK
--    Go code only uses: pending, in_progress, done, blocked
-- ============================================================

-- Drop the old constraint and add corrected one
ALTER TABLE c4_tasks DROP CONSTRAINT IF EXISTS c4_tasks_status_check;
ALTER TABLE c4_tasks ADD CONSTRAINT c4_tasks_status_check
    CHECK (status IN ('pending', 'in_progress', 'done', 'blocked'));
