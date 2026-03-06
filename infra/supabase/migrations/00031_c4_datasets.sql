-- Migration: 00031_c4_datasets
-- Creates c4_datasets table for content-addressable dataset versioning.
-- RLS: project_id based (same pattern as c4_drive_files).

-- ============================================================
-- c4_datasets — dataset manifest metadata
-- ============================================================
CREATE TABLE c4_datasets (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       UUID NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    version_hash     TEXT NOT NULL,  -- first 16 chars of manifest SHA256
    manifest         JSONB NOT NULL, -- [{"path":"rel/path","hash":"sha256hex","size":N}]
    total_size_bytes BIGINT DEFAULT 0,
    file_count       INT    DEFAULT 0,
    created_at       TIMESTAMPTZ DEFAULT now(),
    UNIQUE(project_id, name, version_hash)
);

CREATE INDEX ON c4_datasets(project_id, name, created_at DESC);

-- ============================================================
-- RLS: c4_datasets
-- ============================================================
ALTER TABLE c4_datasets ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view datasets"
    ON c4_datasets FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can insert datasets"
    ON c4_datasets FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update datasets"
    ON c4_datasets FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can delete datasets"
    ON c4_datasets FOR DELETE
    USING (c4_is_project_member(project_id));
