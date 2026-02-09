-- Migration: 00001_c4_projects
-- Creates the projects table and project_members join table.
-- RLS: Users can only see projects they are members of.

-- ============================================================
-- c4_projects
-- ============================================================
CREATE TABLE c4_projects (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    owner_id   UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_c4_projects_owner ON c4_projects(owner_id);

-- ============================================================
-- c4_project_members (join table)
-- ============================================================
CREATE TABLE c4_project_members (
    project_id UUID NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member'
                    CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX idx_c4_project_members_user ON c4_project_members(user_id);

-- ============================================================
-- Helper function: check project membership
-- ============================================================
CREATE OR REPLACE FUNCTION c4_is_project_member(p_project_id UUID)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
AS $$
    SELECT EXISTS (
        SELECT 1 FROM c4_project_members
        WHERE project_id = p_project_id
          AND user_id = auth.uid()
    );
$$;

-- ============================================================
-- Trigger: auto-add owner as member on project creation
-- ============================================================
CREATE OR REPLACE FUNCTION c4_auto_add_owner_member()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
    INSERT INTO c4_project_members (project_id, user_id, role)
    VALUES (NEW.id, NEW.owner_id, 'owner')
    ON CONFLICT (project_id, user_id) DO NOTHING;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_c4_projects_auto_member
    AFTER INSERT ON c4_projects
    FOR EACH ROW
    EXECUTE FUNCTION c4_auto_add_owner_member();

-- ============================================================
-- Trigger: update updated_at on modification
-- ============================================================
CREATE OR REPLACE FUNCTION c4_set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_c4_projects_updated_at
    BEFORE UPDATE ON c4_projects
    FOR EACH ROW
    EXECUTE FUNCTION c4_set_updated_at();

-- ============================================================
-- RLS: c4_projects
-- ============================================================
ALTER TABLE c4_projects ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view projects they belong to"
    ON c4_projects FOR SELECT
    USING (c4_is_project_member(id));

CREATE POLICY "Users can create projects"
    ON c4_projects FOR INSERT
    WITH CHECK (auth.uid() = owner_id);

CREATE POLICY "Project owners can update their projects"
    ON c4_projects FOR UPDATE
    USING (owner_id = auth.uid())
    WITH CHECK (owner_id = auth.uid());

CREATE POLICY "Project owners can delete their projects"
    ON c4_projects FOR DELETE
    USING (owner_id = auth.uid());

-- ============================================================
-- RLS: c4_project_members
-- ============================================================
ALTER TABLE c4_project_members ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view fellow members"
    ON c4_project_members FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Owners and admins can manage members"
    ON c4_project_members FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1 FROM c4_project_members
            WHERE project_id = c4_project_members.project_id
              AND user_id = auth.uid()
              AND role IN ('owner', 'admin')
        )
    );

CREATE POLICY "Owners and admins can remove members"
    ON c4_project_members FOR DELETE
    USING (
        EXISTS (
            SELECT 1 FROM c4_project_members pm
            WHERE pm.project_id = c4_project_members.project_id
              AND pm.user_id = auth.uid()
              AND pm.role IN ('owner', 'admin')
        )
    );
