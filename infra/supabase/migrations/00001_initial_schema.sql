-- =============================================================================
-- C4 Cloud Initial Schema
-- =============================================================================
-- Run with: supabase db push
-- Or in Supabase Dashboard: SQL Editor

-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Teams & Members
-- =============================================================================

CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    owner_id UUID NOT NULL REFERENCES auth.users(id),
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS team_members (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(team_id, user_id)
);

-- =============================================================================
-- Projects
-- =============================================================================

CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    owner_id UUID REFERENCES auth.users(id),
    git_repo TEXT,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    -- Either team_id or owner_id must be set
    CHECK (team_id IS NOT NULL OR owner_id IS NOT NULL),
    UNIQUE(team_id, slug),
    UNIQUE(owner_id, slug)
);

-- =============================================================================
-- C4 State (replaces local SQLite c4_state table)
-- =============================================================================

CREATE TABLE IF NOT EXISTS c4_state (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'INIT',
    execution_mode TEXT,
    current_checkpoint TEXT,
    queue JSONB DEFAULT '{"pending": [], "in_progress": {}, "done": []}',
    config JSONB DEFAULT '{}',
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(project_id)
);

-- =============================================================================
-- Tasks
-- =============================================================================

CREATE TABLE IF NOT EXISTS c4_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    task_id TEXT NOT NULL,
    title TEXT NOT NULL,
    dod TEXT,
    scope TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    assigned_worker TEXT,
    original_worker TEXT,  -- For repair tasks: original worker
    branch TEXT,
    commit_sha TEXT,
    validation_results JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(project_id, task_id)
);

-- =============================================================================
-- Workers
-- =============================================================================

CREATE TABLE IF NOT EXISTS c4_workers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    worker_id TEXT NOT NULL,
    user_id UUID REFERENCES auth.users(id),
    state TEXT NOT NULL DEFAULT 'idle',
    task_id TEXT,
    last_seen TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(project_id, worker_id)
);

-- =============================================================================
-- Events (for audit trail and real-time)
-- =============================================================================

CREATE TABLE IF NOT EXISTS c4_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    actor TEXT,
    payload JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Index for efficient event queries
CREATE INDEX IF NOT EXISTS idx_c4_events_project_created 
ON c4_events(project_id, created_at DESC);

-- =============================================================================
-- Row Level Security (RLS)
-- =============================================================================

-- Enable RLS on all tables
ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE c4_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE c4_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE c4_workers ENABLE ROW LEVEL SECURITY;
ALTER TABLE c4_events ENABLE ROW LEVEL SECURITY;

-- Teams: Members can view their teams
CREATE POLICY "Team members can view team"
ON teams FOR SELECT
USING (
    id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

-- Teams: Only owners can update
CREATE POLICY "Team owners can update"
ON teams FOR UPDATE
USING (owner_id = auth.uid());

-- Team members: Can view members of their teams
CREATE POLICY "View team members"
ON team_members FOR SELECT
USING (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

-- Projects: Team projects visible to team members
CREATE POLICY "Team projects visible to members"
ON projects FOR SELECT
USING (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
    OR owner_id = auth.uid()
);

-- Projects: Users can create personal projects
CREATE POLICY "Users can create personal projects"
ON projects FOR INSERT
WITH CHECK (owner_id = auth.uid() AND team_id IS NULL);

-- C4 State: Visible to project members
CREATE POLICY "C4 state visible to project members"
ON c4_state FOR SELECT
USING (
    project_id IN (
        SELECT id FROM projects 
        WHERE owner_id = auth.uid()
        OR team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
    )
);

-- C4 State: Project members can update
CREATE POLICY "C4 state updatable by project members"
ON c4_state FOR UPDATE
USING (
    project_id IN (
        SELECT id FROM projects 
        WHERE owner_id = auth.uid()
        OR team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
    )
);

-- Tasks: Visible to project members
CREATE POLICY "Tasks visible to project members"
ON c4_tasks FOR SELECT
USING (
    project_id IN (
        SELECT id FROM projects 
        WHERE owner_id = auth.uid()
        OR team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
    )
);

-- Tasks: Project members can insert/update
CREATE POLICY "Tasks manageable by project members"
ON c4_tasks FOR ALL
USING (
    project_id IN (
        SELECT id FROM projects 
        WHERE owner_id = auth.uid()
        OR team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
    )
);

-- Workers: Visible to project members
CREATE POLICY "Workers visible to project members"
ON c4_workers FOR SELECT
USING (
    project_id IN (
        SELECT id FROM projects 
        WHERE owner_id = auth.uid()
        OR team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
    )
);

-- Events: Visible to project members
CREATE POLICY "Events visible to project members"
ON c4_events FOR SELECT
USING (
    project_id IN (
        SELECT id FROM projects 
        WHERE owner_id = auth.uid()
        OR team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
    )
);

-- =============================================================================
-- Functions for real-time subscriptions
-- =============================================================================

-- Function to notify on state changes
CREATE OR REPLACE FUNCTION notify_state_change()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify(
        'c4_state_change',
        json_build_object(
            'project_id', NEW.project_id,
            'status', NEW.status,
            'updated_at', NEW.updated_at
        )::text
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for state changes
DROP TRIGGER IF EXISTS on_state_change ON c4_state;
CREATE TRIGGER on_state_change
AFTER UPDATE ON c4_state
FOR EACH ROW
EXECUTE FUNCTION notify_state_change();

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply updated_at trigger to relevant tables
CREATE TRIGGER update_teams_updated_at
BEFORE UPDATE ON teams
FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_projects_updated_at
BEFORE UPDATE ON projects
FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_c4_state_updated_at
BEFORE UPDATE ON c4_state
FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_c4_tasks_updated_at
BEFORE UPDATE ON c4_tasks
FOR EACH ROW EXECUTE FUNCTION update_updated_at();
