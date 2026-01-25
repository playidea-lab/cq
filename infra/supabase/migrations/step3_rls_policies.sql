-- ============================================================================
-- Step 3: RLS 정책
-- Step 2 성공 후 실행하세요.
-- ============================================================================

-- ============================================================================
-- Enable RLS on all tables
-- ============================================================================

ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE c4_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE c4_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE c4_workers ENABLE ROW LEVEL SECURITY;
ALTER TABLE c4_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_integrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_sso_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE sso_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sso_domain_verifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_branding ENABLE ROW LEVEL SECURITY;

-- ============================================================================
-- Teams RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team members can view team" ON teams;
CREATE POLICY "Team members can view team" ON teams FOR SELECT
USING (
    id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

DROP POLICY IF EXISTS "Team admins can update team" ON teams;
CREATE POLICY "Team admins can update team" ON teams FOR UPDATE
USING (
    id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

DROP POLICY IF EXISTS "Users can create teams" ON teams;
CREATE POLICY "Users can create teams" ON teams FOR INSERT
WITH CHECK (owner_id = auth.uid());

DROP POLICY IF EXISTS "Team owners can delete team" ON teams;
CREATE POLICY "Team owners can delete team" ON teams FOR DELETE
USING (
    id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role = 'owner'
    )
);

-- ============================================================================
-- Team Members RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Members can view team members" ON team_members;
CREATE POLICY "Members can view team members" ON team_members FOR SELECT
USING (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

DROP POLICY IF EXISTS "Admins can manage team members" ON team_members;
CREATE POLICY "Admins can manage team members" ON team_members FOR ALL
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

-- ============================================================================
-- Projects RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team members can view projects" ON projects;
CREATE POLICY "Team members can view projects" ON projects FOR SELECT
USING (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

DROP POLICY IF EXISTS "Team members can create projects" ON projects;
CREATE POLICY "Team members can create projects" ON projects FOR INSERT
WITH CHECK (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

DROP POLICY IF EXISTS "Team admins can manage projects" ON projects;
CREATE POLICY "Team admins can manage projects" ON projects FOR ALL
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin', 'member')
    )
);

-- ============================================================================
-- C4 State RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team members can view c4_state" ON c4_state;
CREATE POLICY "Team members can view c4_state" ON c4_state FOR SELECT
USING (
    project_id IN (
        SELECT p.id FROM projects p
        JOIN team_members tm ON p.team_id = tm.team_id
        WHERE tm.user_id = auth.uid()
    )
);

DROP POLICY IF EXISTS "Team members can manage c4_state" ON c4_state;
CREATE POLICY "Team members can manage c4_state" ON c4_state FOR ALL
USING (
    project_id IN (
        SELECT p.id FROM projects p
        JOIN team_members tm ON p.team_id = tm.team_id
        WHERE tm.user_id = auth.uid()
    )
);

-- ============================================================================
-- C4 Tasks RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team members can view c4_tasks" ON c4_tasks;
CREATE POLICY "Team members can view c4_tasks" ON c4_tasks FOR SELECT
USING (
    state_id IN (
        SELECT cs.id FROM c4_state cs
        JOIN projects p ON cs.project_id = p.id
        JOIN team_members tm ON p.team_id = tm.team_id
        WHERE tm.user_id = auth.uid()
    )
);

DROP POLICY IF EXISTS "Team members can manage c4_tasks" ON c4_tasks;
CREATE POLICY "Team members can manage c4_tasks" ON c4_tasks FOR ALL
USING (
    state_id IN (
        SELECT cs.id FROM c4_state cs
        JOIN projects p ON cs.project_id = p.id
        JOIN team_members tm ON p.team_id = tm.team_id
        WHERE tm.user_id = auth.uid()
    )
);

-- ============================================================================
-- C4 Workers RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team members can view c4_workers" ON c4_workers;
CREATE POLICY "Team members can view c4_workers" ON c4_workers FOR SELECT
USING (
    state_id IN (
        SELECT cs.id FROM c4_state cs
        JOIN projects p ON cs.project_id = p.id
        JOIN team_members tm ON p.team_id = tm.team_id
        WHERE tm.user_id = auth.uid()
    )
);

DROP POLICY IF EXISTS "Team members can manage c4_workers" ON c4_workers;
CREATE POLICY "Team members can manage c4_workers" ON c4_workers FOR ALL
USING (
    state_id IN (
        SELECT cs.id FROM c4_state cs
        JOIN projects p ON cs.project_id = p.id
        JOIN team_members tm ON p.team_id = tm.team_id
        WHERE tm.user_id = auth.uid()
    )
);

-- ============================================================================
-- C4 Events RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team members can view c4_events" ON c4_events;
CREATE POLICY "Team members can view c4_events" ON c4_events FOR SELECT
USING (
    state_id IN (
        SELECT cs.id FROM c4_state cs
        JOIN projects p ON cs.project_id = p.id
        JOIN team_members tm ON p.team_id = tm.team_id
        WHERE tm.user_id = auth.uid()
    )
);

DROP POLICY IF EXISTS "Team members can insert c4_events" ON c4_events;
CREATE POLICY "Team members can insert c4_events" ON c4_events FOR INSERT
WITH CHECK (
    state_id IN (
        SELECT cs.id FROM c4_state cs
        JOIN projects p ON cs.project_id = p.id
        JOIN team_members tm ON p.team_id = tm.team_id
        WHERE tm.user_id = auth.uid()
    )
);

-- ============================================================================
-- Team Integrations RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team members can view integrations" ON team_integrations;
CREATE POLICY "Team members can view integrations" ON team_integrations FOR SELECT
USING (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

DROP POLICY IF EXISTS "Team admins can manage integrations" ON team_integrations;
CREATE POLICY "Team admins can manage integrations" ON team_integrations FOR ALL
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

-- ============================================================================
-- Activity Logs RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team members can view activity_logs" ON activity_logs;
CREATE POLICY "Team members can view activity_logs" ON activity_logs FOR SELECT
USING (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

DROP POLICY IF EXISTS "Team members can insert activity_logs" ON activity_logs;
CREATE POLICY "Team members can insert activity_logs" ON activity_logs FOR INSERT
WITH CHECK (
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

-- ============================================================================
-- Audit Logs RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team admins can view audit_logs" ON audit_logs;
CREATE POLICY "Team admins can view audit_logs" ON audit_logs FOR SELECT
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

DROP POLICY IF EXISTS "System can insert audit_logs" ON audit_logs;
CREATE POLICY "System can insert audit_logs" ON audit_logs FOR INSERT
WITH CHECK (true);

-- ============================================================================
-- Team SSO Configs RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team admins can view sso_configs" ON team_sso_configs;
CREATE POLICY "Team admins can view sso_configs" ON team_sso_configs FOR SELECT
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

DROP POLICY IF EXISTS "Team admins can manage sso_configs" ON team_sso_configs;
CREATE POLICY "Team admins can manage sso_configs" ON team_sso_configs FOR ALL
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

-- ============================================================================
-- SSO Sessions RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Users can view own sso_sessions" ON sso_sessions;
CREATE POLICY "Users can view own sso_sessions" ON sso_sessions FOR SELECT
USING (user_id = auth.uid());

DROP POLICY IF EXISTS "System can manage sso_sessions" ON sso_sessions;
CREATE POLICY "System can manage sso_sessions" ON sso_sessions FOR ALL
USING (true);

-- ============================================================================
-- SSO Domain Verifications RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Team admins can view domain_verifications" ON sso_domain_verifications;
CREATE POLICY "Team admins can view domain_verifications" ON sso_domain_verifications FOR SELECT
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

DROP POLICY IF EXISTS "Team admins can manage domain_verifications" ON sso_domain_verifications;
CREATE POLICY "Team admins can manage domain_verifications" ON sso_domain_verifications FOR ALL
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

-- ============================================================================
-- Team Branding RLS Policies
-- ============================================================================

DROP POLICY IF EXISTS "Anyone can view branding" ON team_branding;
CREATE POLICY "Anyone can view branding" ON team_branding FOR SELECT
USING (true);

DROP POLICY IF EXISTS "Team admins can manage branding" ON team_branding;
CREATE POLICY "Team admins can manage branding" ON team_branding FOR ALL
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
    )
);

SELECT '✅ Step 3 완료: RLS 정책 생성 성공!' as result;
