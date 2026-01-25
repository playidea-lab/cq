-- =============================================================================
-- C4 Cloud Team Enhancements Schema
-- =============================================================================
-- Phase 3: Enhanced team management with billing and invitation workflow
-- Run with: supabase db push

-- =============================================================================
-- Extend Teams Table
-- =============================================================================

-- Add billing and plan fields to teams
ALTER TABLE teams
ADD COLUMN IF NOT EXISTS stripe_customer_id TEXT,
ADD COLUMN IF NOT EXISTS plan TEXT DEFAULT 'free'
    CHECK (plan IN ('free', 'pro', 'team', 'agency', 'enterprise'));

-- Index for Stripe lookups
CREATE INDEX IF NOT EXISTS idx_teams_stripe_customer
ON teams(stripe_customer_id)
WHERE stripe_customer_id IS NOT NULL;

-- =============================================================================
-- Extend Team Members Table
-- =============================================================================

-- Add viewer role to the check constraint (drop and recreate)
-- First, we need to handle the constraint update
DO $$
BEGIN
    -- Drop old constraint if exists
    ALTER TABLE team_members DROP CONSTRAINT IF EXISTS team_members_role_check;

    -- Add new constraint with viewer role
    ALTER TABLE team_members
    ADD CONSTRAINT team_members_role_check
    CHECK (role IN ('owner', 'admin', 'member', 'viewer'));
EXCEPTION
    WHEN undefined_object THEN
        NULL;  -- Constraint doesn't exist, that's fine
END $$;

-- Add invitation workflow fields
ALTER TABLE team_members
ADD COLUMN IF NOT EXISTS invited_by UUID REFERENCES auth.users(id),
ADD COLUMN IF NOT EXISTS invited_at TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS accepted_at TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS invite_email TEXT,
ADD COLUMN IF NOT EXISTS invite_token TEXT UNIQUE;

-- Rename joined_at to created_at for consistency (if it exists)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'team_members' AND column_name = 'joined_at'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'team_members' AND column_name = 'created_at'
    ) THEN
        ALTER TABLE team_members RENAME COLUMN joined_at TO created_at;
    END IF;
END $$;

-- Add created_at if neither exists
ALTER TABLE team_members
ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ DEFAULT NOW();

-- Index for pending invitations
CREATE INDEX IF NOT EXISTS idx_team_members_pending_invites
ON team_members(invite_email)
WHERE user_id IS NULL AND invite_token IS NOT NULL;

-- Index for invite token lookups
CREATE INDEX IF NOT EXISTS idx_team_members_invite_token
ON team_members(invite_token)
WHERE invite_token IS NOT NULL;

-- =============================================================================
-- Team Invitations View
-- =============================================================================

CREATE OR REPLACE VIEW pending_invitations AS
SELECT
    tm.id,
    tm.team_id,
    t.name as team_name,
    t.slug as team_slug,
    tm.role,
    tm.invite_email,
    tm.invited_by,
    u.email as inviter_email,
    tm.invited_at,
    tm.invite_token
FROM team_members tm
JOIN teams t ON t.id = tm.team_id
LEFT JOIN auth.users u ON u.id = tm.invited_by
WHERE tm.user_id IS NULL
  AND tm.invite_token IS NOT NULL;

-- =============================================================================
-- RLS Policy Updates
-- =============================================================================

-- Allow invitees to view their pending invitation by token
CREATE POLICY "Invitees can view pending invitation"
ON team_members FOR SELECT
USING (
    invite_token IS NOT NULL
    AND user_id IS NULL
);

-- Allow admins to manage members (insert/update/delete)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'team_members' AND policyname = 'Team admins can manage members'
    ) THEN
        CREATE POLICY "Team admins can manage members"
        ON team_members FOR ALL
        USING (
            team_id IN (
                SELECT team_id FROM team_members
                WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
            )
        );
    END IF;
END $$;

-- =============================================================================
-- Functions for Team Management
-- =============================================================================

-- Function to create a team with owner as first member
CREATE OR REPLACE FUNCTION create_team_with_owner(
    p_name TEXT,
    p_slug TEXT,
    p_owner_id UUID
) RETURNS UUID AS $$
DECLARE
    v_team_id UUID;
BEGIN
    -- Create team
    INSERT INTO teams (name, slug, owner_id)
    VALUES (p_name, p_slug, p_owner_id)
    RETURNING id INTO v_team_id;

    -- Add owner as member
    INSERT INTO team_members (team_id, user_id, role, accepted_at, created_at)
    VALUES (v_team_id, p_owner_id, 'owner', NOW(), NOW());

    RETURN v_team_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Function to create invitation
CREATE OR REPLACE FUNCTION create_team_invitation(
    p_team_id UUID,
    p_email TEXT,
    p_role TEXT,
    p_invited_by UUID
) RETURNS TEXT AS $$
DECLARE
    v_token TEXT;
BEGIN
    -- Generate secure token
    v_token := encode(gen_random_bytes(32), 'hex');

    -- Create pending member record
    INSERT INTO team_members (
        team_id,
        role,
        invite_email,
        invite_token,
        invited_by,
        invited_at,
        created_at
    )
    VALUES (
        p_team_id,
        p_role,
        p_email,
        v_token,
        p_invited_by,
        NOW(),
        NOW()
    )
    ON CONFLICT (team_id, user_id) DO NOTHING;  -- Prevent duplicate invites

    RETURN v_token;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Function to accept invitation
CREATE OR REPLACE FUNCTION accept_team_invitation(
    p_token TEXT,
    p_user_id UUID
) RETURNS BOOLEAN AS $$
DECLARE
    v_team_id UUID;
    v_role TEXT;
BEGIN
    -- Find and update the invitation
    UPDATE team_members
    SET user_id = p_user_id,
        accepted_at = NOW(),
        invite_token = NULL  -- Clear token after use
    WHERE invite_token = p_token
      AND user_id IS NULL
    RETURNING team_id, role INTO v_team_id, v_role;

    IF v_team_id IS NULL THEN
        RETURN FALSE;
    END IF;

    RETURN TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Function to get team with member count
CREATE OR REPLACE FUNCTION get_team_with_stats(p_team_id UUID)
RETURNS TABLE (
    id UUID,
    name TEXT,
    slug TEXT,
    owner_id UUID,
    plan TEXT,
    member_count BIGINT,
    created_at TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        t.id,
        t.name,
        t.slug,
        t.owner_id,
        t.plan,
        COUNT(tm.id) as member_count,
        t.created_at
    FROM teams t
    LEFT JOIN team_members tm ON tm.team_id = t.id AND tm.user_id IS NOT NULL
    WHERE t.id = p_team_id
    GROUP BY t.id, t.name, t.slug, t.owner_id, t.plan, t.created_at;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- =============================================================================
-- Comments for documentation
-- =============================================================================

COMMENT ON COLUMN teams.stripe_customer_id IS 'Stripe customer ID for billing';
COMMENT ON COLUMN teams.plan IS 'Subscription plan: free, pro, team, agency, enterprise';
COMMENT ON COLUMN team_members.invite_email IS 'Email address for pending invitation';
COMMENT ON COLUMN team_members.invite_token IS 'Secure token for accepting invitation';
COMMENT ON COLUMN team_members.invited_by IS 'User who sent the invitation';
COMMENT ON COLUMN team_members.invited_at IS 'When the invitation was sent';
COMMENT ON COLUMN team_members.accepted_at IS 'When the invitation was accepted';
