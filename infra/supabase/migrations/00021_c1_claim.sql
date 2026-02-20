-- Migration: 00021_c1_claim
-- Adds atomic claim dispatch for @cq worker messages.
-- Workers poll #cq channel and claim unclaimed @cq mentions.

-- ============================================================
-- Add claim columns to c1_messages
-- ============================================================
ALTER TABLE c1_messages ADD COLUMN IF NOT EXISTS claimed_by   TEXT;
ALTER TABLE c1_messages ADD COLUMN IF NOT EXISTS claimed_at   TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_c1_messages_claimed ON c1_messages(channel_id, claimed_by, created_at);

-- ============================================================
-- Atomic claim function — returns claimed row or empty set
-- ============================================================
CREATE OR REPLACE FUNCTION claim_message(p_message_id UUID, p_worker_id TEXT)
RETURNS SETOF c1_messages AS $$
BEGIN
    RETURN QUERY
    UPDATE c1_messages
    SET claimed_by = p_worker_id,
        claimed_at = now()
    WHERE id = p_message_id
      AND claimed_by IS NULL
    RETURNING *;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
