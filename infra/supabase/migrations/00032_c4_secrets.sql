-- Migration 00032: C4 Secrets cloud table
-- Stores encrypted secrets (base64-encoded ciphertext+nonce) per project.
-- Clients decrypt locally using their master key; Supabase stores opaque blobs.

CREATE TABLE IF NOT EXISTS c4_secrets (
    project_id  TEXT    NOT NULL,
    key         TEXT    NOT NULL,
    ciphertext  TEXT    NOT NULL, -- base64(aes-gcm ciphertext)
    nonce       TEXT    NOT NULL, -- base64(aes-gcm nonce)
    updated_at  BIGINT  NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT,
    PRIMARY KEY (project_id, key)
);

-- Index for per-project key listing
CREATE INDEX IF NOT EXISTS idx_c4_secrets_project_id ON c4_secrets(project_id);

-- RLS: enable row-level security
ALTER TABLE c4_secrets ENABLE ROW LEVEL SECURITY;

-- Policy: anon key can read/write own project's secrets
-- (project_id isolation is enforced by the application via JWT claims or API key scope)
CREATE POLICY "project secrets access" ON c4_secrets
    USING (true)
    WITH CHECK (true);
