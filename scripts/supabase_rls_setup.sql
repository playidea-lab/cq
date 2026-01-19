-- C4 Supabase RLS Setup
-- Run this in Supabase SQL Editor (Dashboard > SQL Editor)

-- ============================================================================
-- Step 1: Add team_id column to existing tables
-- ============================================================================

-- Add team_id to c4_state
ALTER TABLE c4_state ADD COLUMN IF NOT EXISTS team_id UUID;

-- Add team_id to c4_locks
ALTER TABLE c4_locks ADD COLUMN IF NOT EXISTS team_id UUID;

-- ============================================================================
-- Step 2: Create teams and team_members tables
-- ============================================================================

-- Teams table
CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Team members table (links users to teams)
CREATE TABLE IF NOT EXISTS team_members (
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member', 'viewer')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);

-- ============================================================================
-- Step 3: Enable Row Level Security
-- ============================================================================

-- Enable RLS on c4_state
ALTER TABLE c4_state ENABLE ROW LEVEL SECURITY;

-- Enable RLS on c4_locks
ALTER TABLE c4_locks ENABLE ROW LEVEL SECURITY;

-- Enable RLS on teams
ALTER TABLE teams ENABLE ROW LEVEL SECURITY;

-- Enable RLS on team_members
ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;

-- ============================================================================
-- Step 4: Create RLS Policies
-- ============================================================================

-- c4_state: Users can only access their team's state
DROP POLICY IF EXISTS "team_state_access" ON c4_state;
CREATE POLICY "team_state_access" ON c4_state
    FOR ALL
    USING (
        team_id IS NULL  -- Allow access if no team_id (backward compatibility)
        OR team_id IN (
            SELECT tm.team_id FROM team_members tm
            WHERE tm.user_id = auth.uid()
        )
    );

-- c4_locks: Users can only access their team's locks
DROP POLICY IF EXISTS "team_locks_access" ON c4_locks;
CREATE POLICY "team_locks_access" ON c4_locks
    FOR ALL
    USING (
        team_id IS NULL  -- Allow access if no team_id (backward compatibility)
        OR team_id IN (
            SELECT tm.team_id FROM team_members tm
            WHERE tm.user_id = auth.uid()
        )
    );

-- teams: Users can view teams they belong to
DROP POLICY IF EXISTS "team_view" ON teams;
CREATE POLICY "team_view" ON teams
    FOR SELECT
    USING (
        id IN (
            SELECT tm.team_id FROM team_members tm
            WHERE tm.user_id = auth.uid()
        )
    );

-- teams: Only admins can update team info
DROP POLICY IF EXISTS "team_update" ON teams;
CREATE POLICY "team_update" ON teams
    FOR UPDATE
    USING (
        id IN (
            SELECT tm.team_id FROM team_members tm
            WHERE tm.user_id = auth.uid() AND tm.role = 'admin'
        )
    );

-- team_members: Users can view members of their teams
DROP POLICY IF EXISTS "team_members_view" ON team_members;
CREATE POLICY "team_members_view" ON team_members
    FOR SELECT
    USING (
        team_id IN (
            SELECT tm.team_id FROM team_members tm
            WHERE tm.user_id = auth.uid()
        )
    );

-- team_members: Only admins can manage team members
DROP POLICY IF EXISTS "team_members_manage" ON team_members;
CREATE POLICY "team_members_manage" ON team_members
    FOR ALL
    USING (
        team_id IN (
            SELECT tm.team_id FROM team_members tm
            WHERE tm.user_id = auth.uid() AND tm.role = 'admin'
        )
    );

-- ============================================================================
-- Step 5: Create indexes for performance
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_c4_state_team_id ON c4_state(team_id);
CREATE INDEX IF NOT EXISTS idx_c4_locks_team_id ON c4_locks(team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_user_id ON team_members(user_id);
CREATE INDEX IF NOT EXISTS idx_team_members_team_id ON team_members(team_id);

-- ============================================================================
-- Step 6: Helper function to create a team with first admin
-- ============================================================================

CREATE OR REPLACE FUNCTION create_team_with_admin(
    team_name TEXT,
    admin_user_id UUID DEFAULT auth.uid()
)
RETURNS UUID
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    new_team_id UUID;
BEGIN
    -- Create team
    INSERT INTO teams (name) VALUES (team_name)
    RETURNING id INTO new_team_id;

    -- Add creator as admin
    INSERT INTO team_members (team_id, user_id, role)
    VALUES (new_team_id, admin_user_id, 'admin');

    RETURN new_team_id;
END;
$$;

-- ============================================================================
-- Done!
-- ============================================================================
--
-- Usage:
-- 1. Create a team: SELECT create_team_with_admin('My Team');
-- 2. Add member: INSERT INTO team_members (team_id, user_id, role) VALUES (...);
-- 3. Use team_id in C4 config:
--
--    .env:
--    C4_TEAM_ID=your-team-uuid
--
--    Or config.yaml:
--    store:
--      backend: supabase
--      team_id: your-team-uuid
--
