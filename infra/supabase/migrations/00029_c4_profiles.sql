-- Migration: 00029_c4_profiles
-- Creates c4_profiles and c4_pending_invitations tables with SECURITY DEFINER RPCs.
-- c4_profiles: stores user identity linked to Supabase auth.users via UpsertProfile.
-- c4_pending_invitations: tracks email-based invitations for users not yet registered.
-- Access: SECURITY DEFINER RPCs only (c4_invite_or_pend, c4_resolve_pending_invitations).

-- ============================================================
-- c4_profiles
-- ============================================================
CREATE TABLE c4_profiles (
    user_id      UUID PRIMARY KEY,
    email        TEXT UNIQUE NOT NULL,
    display_name TEXT,
    avatar_url   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_c4_profiles_email ON c4_profiles(email);

-- ============================================================
-- c4_pending_invitations
-- ============================================================
CREATE TABLE c4_pending_invitations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    email      TEXT NOT NULL,
    invited_by UUID NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '7 days',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, email)
);

CREATE INDEX idx_c4_pending_invitations_email ON c4_pending_invitations(email);
CREATE INDEX idx_c4_pending_invitations_project ON c4_pending_invitations(project_id);

-- ============================================================
-- RLS: c4_profiles
-- ============================================================
ALTER TABLE c4_profiles ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view their own profile"
    ON c4_profiles FOR SELECT
    USING (user_id = auth.uid());

CREATE POLICY "Users can insert their own profile"
    ON c4_profiles FOR INSERT
    WITH CHECK (auth.uid() = user_id);

CREATE POLICY "Users can update their own profile"
    ON c4_profiles FOR UPDATE
    USING (user_id = auth.uid());

-- ============================================================
-- RLS: c4_pending_invitations
-- Access only via SECURITY DEFINER RPCs below.
-- ============================================================
ALTER TABLE c4_pending_invitations ENABLE ROW LEVEL SECURITY;

REVOKE ALL ON c4_pending_invitations FROM authenticated, anon;

-- ============================================================
-- RPC: c4_invite_or_pend
-- Called by project admins to invite a user by email.
-- If the user has a profile → add as member immediately.
-- Otherwise → create a pending invitation.
-- Returns: 'added' | 'invited'
-- ============================================================
CREATE OR REPLACE FUNCTION c4_invite_or_pend(
    p_project_id UUID,
    p_email      TEXT
) RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_invited_by UUID;
BEGIN
    v_invited_by := auth.uid();

    IF EXISTS (SELECT 1 FROM c4_profiles WHERE email = p_email) THEN
        INSERT INTO c4_project_members (project_id, user_id, role)
            SELECT p_project_id, user_id, 'member'
            FROM c4_profiles
            WHERE email = p_email
            ON CONFLICT DO NOTHING;
        RETURN 'added';
    ELSE
        INSERT INTO c4_pending_invitations (project_id, email, invited_by)
            VALUES (p_project_id, p_email, v_invited_by)
            ON CONFLICT DO NOTHING;
        RETURN 'invited';
    END IF;
END;
$$;

-- ============================================================
-- RPC: c4_resolve_pending_invitations
-- Called after a new user signs up (via UpsertProfile).
-- Converts any pending invitations for their email into memberships.
-- ============================================================
CREATE OR REPLACE FUNCTION c4_resolve_pending_invitations(
    p_user_id UUID,
    p_email   TEXT
) RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
    INSERT INTO c4_project_members (project_id, user_id, role)
        SELECT project_id, p_user_id, 'member'
        FROM c4_pending_invitations
        WHERE email = p_email
        ON CONFLICT DO NOTHING;

    DELETE FROM c4_pending_invitations
    WHERE email = p_email;
END;
$$;
