-- Fix: Realtime postgres_changes with SECURITY DEFINER functions may not correctly
-- receive auth.uid() context in Supabase Realtime's RLS check.
-- Add a direct policy using auth.uid() without SECURITY DEFINER to ensure
-- Realtime can properly filter events per subscriber.

-- Drop the existing SELECT policy
DROP POLICY IF EXISTS "Members can view messages" ON c1_messages;

-- Recreate without SECURITY DEFINER dependency — use inline join instead
-- This ensures auth.uid() is evaluated in the caller's security context (Realtime-compatible)
CREATE POLICY "Members can view messages"
    ON c1_messages FOR SELECT
    USING (
        EXISTS (
            SELECT 1 FROM c4_project_members
            WHERE project_id = c1_messages.project_id
              AND user_id = auth.uid()
        )
    );
