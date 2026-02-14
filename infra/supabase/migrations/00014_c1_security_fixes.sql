-- Migration: 00014_c1_security_fixes
-- Security improvements for c1_messages:
-- 1. System messages cannot be deleted
-- 2. Users can only update their own messages

-- ============================================================
-- Fix DELETE policy: prevent deletion of system messages
-- ============================================================
DROP POLICY IF EXISTS "Members can delete own messages" ON c1_messages;

CREATE POLICY "Members can delete own messages"
    ON c1_messages FOR DELETE
    USING (project_id IN (SELECT project_id FROM c1_participants WHERE user_id = auth.uid())
           AND sender_type != 'system');

-- ============================================================
-- Add UPDATE policy: users can only update their own messages
-- ============================================================
CREATE POLICY "Members can update own messages"
    ON c1_messages FOR UPDATE
    USING (sender_id = auth.uid()::text);
