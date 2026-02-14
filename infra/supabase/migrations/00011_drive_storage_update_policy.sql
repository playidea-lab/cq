-- Add UPDATE policy for storage.objects (needed for upsert via x-upsert header)
CREATE POLICY "Project members can update drive objects"
    ON storage.objects FOR UPDATE
    USING (bucket_id = 'c4-drive' AND auth.uid() IS NOT NULL)
    WITH CHECK (bucket_id = 'c4-drive' AND auth.uid() IS NOT NULL);
