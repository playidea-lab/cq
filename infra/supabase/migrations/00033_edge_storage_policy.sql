-- Migration: 00033_edge_storage_policy
-- Allow authenticated edge agents to upload/read files under the edges/ path.
-- Edge upload path format: edges/{edgeID}/{filename}
-- The existing project-member policy checks first segment as UUID (project_id),
-- which fails for the "edges" prefix. This policy covers that gap.

CREATE POLICY "Authenticated users can upload to edges path"
    ON storage.objects FOR INSERT
    WITH CHECK (
        bucket_id = 'c4-drive'
        AND auth.uid() IS NOT NULL
        AND name LIKE 'edges/%'
    );

CREATE POLICY "Authenticated users can read from edges path"
    ON storage.objects FOR SELECT
    USING (
        bucket_id = 'c4-drive'
        AND auth.uid() IS NOT NULL
        AND name LIKE 'edges/%'
    );
