-- Migration: 00051_notification_channels
-- Creates project_notification_channels and notification_pairings tables
-- for managing per-project notification routing and Telegram pairing codes.

-- ============================================================
-- project_notification_channels
-- ============================================================
CREATE TABLE IF NOT EXISTS project_notification_channels (  -- idempotent
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  TEXT        NOT NULL,
    channel_type TEXT       NOT NULL CHECK (channel_type IN ('telegram', 'slack', 'email')),
    config      JSONB       NOT NULL,
    events      TEXT[]      NOT NULL DEFAULT '{worker.offline,job.failed,job.complete,task.blocked}',
    created_by  UUID        REFERENCES auth.users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Prevent duplicate (project, channel_type, config) combos
    CONSTRAINT uq_project_channel_config UNIQUE (project_id, channel_type, config)
);

-- Index for fast lookup by project
CREATE INDEX IF NOT EXISTS idx_pnc_project_id
    ON project_notification_channels (project_id);

-- RLS
ALTER TABLE project_notification_channels ENABLE ROW LEVEL SECURITY;

-- Authenticated users can read channels in their own projects.
-- We allow any authenticated user full CRUD for simplicity; project-level
-- scoping can be layered on top via application logic or a project membership
-- table when it exists.
DROP POLICY IF EXISTS "auth_select_notification_channels" ON project_notification_channels;
CREATE POLICY "auth_select_notification_channels"
    ON project_notification_channels
    FOR SELECT
    TO authenticated
    USING (true);

DROP POLICY IF EXISTS "auth_insert_notification_channels" ON project_notification_channels;
CREATE POLICY "auth_insert_notification_channels"
    ON project_notification_channels
    FOR INSERT
    TO authenticated
    WITH CHECK (true);

DROP POLICY IF EXISTS "auth_update_notification_channels" ON project_notification_channels;
CREATE POLICY "auth_update_notification_channels"
    ON project_notification_channels
    FOR UPDATE
    TO authenticated
    USING (true)
    WITH CHECK (true);

DROP POLICY IF EXISTS "auth_delete_notification_channels" ON project_notification_channels;
CREATE POLICY "auth_delete_notification_channels"
    ON project_notification_channels
    FOR DELETE
    TO authenticated
    USING (true);

-- ============================================================
-- notification_pairings
-- One-time pairing codes that link a Telegram user ↔ project channel.
-- ============================================================
CREATE TABLE IF NOT EXISTS notification_pairings (
    code         TEXT        PRIMARY KEY,
    user_id      UUID        NOT NULL,
    project_id   TEXT        NOT NULL,
    channel_type TEXT        NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    used         BOOLEAN     NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for expiry cleanup and lookup
CREATE INDEX IF NOT EXISTS idx_np_user_id
    ON notification_pairings (user_id);

CREATE INDEX IF NOT EXISTS idx_np_expires_at
    ON notification_pairings (expires_at);

-- RLS
ALTER TABLE notification_pairings ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "auth_select_notification_pairings" ON notification_pairings;
CREATE POLICY "auth_select_notification_pairings"
    ON notification_pairings
    FOR SELECT
    TO authenticated
    USING (true);

DROP POLICY IF EXISTS "auth_insert_notification_pairings" ON notification_pairings;
CREATE POLICY "auth_insert_notification_pairings"
    ON notification_pairings
    FOR INSERT
    TO authenticated
    WITH CHECK (true);

DROP POLICY IF EXISTS "auth_update_notification_pairings" ON notification_pairings;
CREATE POLICY "auth_update_notification_pairings"
    ON notification_pairings
    FOR UPDATE
    TO authenticated
    USING (true)
    WITH CHECK (true);

DROP POLICY IF EXISTS "auth_delete_notification_pairings" ON notification_pairings;
CREATE POLICY "auth_delete_notification_pairings"
    ON notification_pairings
    FOR DELETE
    TO authenticated
    USING (true);
