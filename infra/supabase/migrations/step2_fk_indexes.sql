-- ============================================================================
-- Step 2: Foreign Keys와 인덱스
-- Step 1 성공 후 실행하세요.
-- ============================================================================

-- ============================================================================
-- Foreign Keys (teams 참조)
-- ============================================================================

-- team_members → teams
ALTER TABLE team_members DROP CONSTRAINT IF EXISTS team_members_team_id_fkey;
ALTER TABLE team_members ADD CONSTRAINT team_members_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- projects → teams
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_team_id_fkey;
ALTER TABLE projects ADD CONSTRAINT projects_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- team_integrations → teams
ALTER TABLE team_integrations DROP CONSTRAINT IF EXISTS team_integrations_team_id_fkey;
ALTER TABLE team_integrations ADD CONSTRAINT team_integrations_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- team_integrations → integration_providers
ALTER TABLE team_integrations DROP CONSTRAINT IF EXISTS team_integrations_provider_id_fkey;
ALTER TABLE team_integrations ADD CONSTRAINT team_integrations_provider_id_fkey
    FOREIGN KEY (provider_id) REFERENCES integration_providers(id);

-- activity_logs → teams
ALTER TABLE activity_logs DROP CONSTRAINT IF EXISTS activity_logs_team_id_fkey;
ALTER TABLE activity_logs ADD CONSTRAINT activity_logs_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- activity_logs → projects
ALTER TABLE activity_logs DROP CONSTRAINT IF EXISTS activity_logs_project_id_fkey;
ALTER TABLE activity_logs ADD CONSTRAINT activity_logs_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;

-- audit_logs → teams
ALTER TABLE audit_logs DROP CONSTRAINT IF EXISTS audit_logs_team_id_fkey;
ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- team_sso_configs → teams
ALTER TABLE team_sso_configs DROP CONSTRAINT IF EXISTS team_sso_configs_team_id_fkey;
ALTER TABLE team_sso_configs ADD CONSTRAINT team_sso_configs_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- sso_sessions → teams
ALTER TABLE sso_sessions DROP CONSTRAINT IF EXISTS sso_sessions_team_id_fkey;
ALTER TABLE sso_sessions ADD CONSTRAINT sso_sessions_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- sso_domain_verifications → teams
ALTER TABLE sso_domain_verifications DROP CONSTRAINT IF EXISTS sso_domain_verifications_team_id_fkey;
ALTER TABLE sso_domain_verifications ADD CONSTRAINT sso_domain_verifications_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- team_branding → teams
ALTER TABLE team_branding DROP CONSTRAINT IF EXISTS team_branding_team_id_fkey;
ALTER TABLE team_branding ADD CONSTRAINT team_branding_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- ============================================================================
-- Foreign Keys (projects 참조)
-- ============================================================================

-- c4_state → projects
ALTER TABLE c4_state DROP CONSTRAINT IF EXISTS c4_state_project_id_fkey;
ALTER TABLE c4_state ADD CONSTRAINT c4_state_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- ============================================================================
-- Foreign Keys (c4_state 참조)
-- ============================================================================

-- c4_tasks → c4_state
ALTER TABLE c4_tasks DROP CONSTRAINT IF EXISTS c4_tasks_state_id_fkey;
ALTER TABLE c4_tasks ADD CONSTRAINT c4_tasks_state_id_fkey
    FOREIGN KEY (state_id) REFERENCES c4_state(id) ON DELETE CASCADE;

-- c4_workers → c4_state
ALTER TABLE c4_workers DROP CONSTRAINT IF EXISTS c4_workers_state_id_fkey;
ALTER TABLE c4_workers ADD CONSTRAINT c4_workers_state_id_fkey
    FOREIGN KEY (state_id) REFERENCES c4_state(id) ON DELETE CASCADE;

-- c4_events → c4_state
ALTER TABLE c4_events DROP CONSTRAINT IF EXISTS c4_events_state_id_fkey;
ALTER TABLE c4_events ADD CONSTRAINT c4_events_state_id_fkey
    FOREIGN KEY (state_id) REFERENCES c4_state(id) ON DELETE CASCADE;

-- ============================================================================
-- Unique Constraints
-- ============================================================================

ALTER TABLE team_members DROP CONSTRAINT IF EXISTS team_members_team_id_user_id_key;
ALTER TABLE team_members ADD CONSTRAINT team_members_team_id_user_id_key
    UNIQUE (team_id, user_id);

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_team_id_slug_key;
ALTER TABLE projects ADD CONSTRAINT projects_team_id_slug_key
    UNIQUE (team_id, slug);

ALTER TABLE c4_state DROP CONSTRAINT IF EXISTS c4_state_project_id_key;
ALTER TABLE c4_state ADD CONSTRAINT c4_state_project_id_key
    UNIQUE (project_id);

ALTER TABLE c4_tasks DROP CONSTRAINT IF EXISTS c4_tasks_state_id_task_id_key;
ALTER TABLE c4_tasks ADD CONSTRAINT c4_tasks_state_id_task_id_key
    UNIQUE (state_id, task_id);

ALTER TABLE team_integrations DROP CONSTRAINT IF EXISTS team_integrations_team_provider_external_key;
ALTER TABLE team_integrations ADD CONSTRAINT team_integrations_team_provider_external_key
    UNIQUE (team_id, provider_id, external_id);

ALTER TABLE team_sso_configs DROP CONSTRAINT IF EXISTS team_sso_configs_team_id_key;
ALTER TABLE team_sso_configs ADD CONSTRAINT team_sso_configs_team_id_key
    UNIQUE (team_id);

-- ============================================================================
-- Indexes
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_teams_owner ON teams(owner_id);
CREATE INDEX IF NOT EXISTS idx_teams_slug ON teams(slug);

CREATE INDEX IF NOT EXISTS idx_team_members_user ON team_members(user_id);
CREATE INDEX IF NOT EXISTS idx_team_members_team ON team_members(team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_invite_token ON team_members(invite_token) WHERE invite_token IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_projects_team ON projects(team_id);
CREATE INDEX IF NOT EXISTS idx_projects_slug ON projects(team_id, slug);

CREATE INDEX IF NOT EXISTS idx_c4_state_project ON c4_state(project_id);
CREATE INDEX IF NOT EXISTS idx_c4_tasks_state ON c4_tasks(state_id);
CREATE INDEX IF NOT EXISTS idx_c4_tasks_status ON c4_tasks(status);
CREATE INDEX IF NOT EXISTS idx_c4_workers_state ON c4_workers(state_id);
CREATE INDEX IF NOT EXISTS idx_c4_events_state ON c4_events(state_id);
CREATE INDEX IF NOT EXISTS idx_c4_events_type ON c4_events(event_type);

CREATE INDEX IF NOT EXISTS idx_integrations_provider_external ON team_integrations(provider_id, external_id);
CREATE INDEX IF NOT EXISTS idx_integrations_team ON team_integrations(team_id, status);

CREATE INDEX IF NOT EXISTS idx_activity_logs_team ON activity_logs(team_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_logs_user ON activity_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_activity_logs_type ON activity_logs(activity_type);

CREATE INDEX IF NOT EXISTS idx_audit_logs_team ON audit_logs(team_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);

CREATE INDEX IF NOT EXISTS idx_sso_configs_team ON team_sso_configs(team_id);
CREATE INDEX IF NOT EXISTS idx_sso_sessions_user ON sso_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sso_sessions_team ON sso_sessions(team_id);
CREATE INDEX IF NOT EXISTS idx_sso_sessions_expires ON sso_sessions(expires_at);

CREATE INDEX IF NOT EXISTS idx_sso_domain_team ON sso_domain_verifications(team_id);
CREATE INDEX IF NOT EXISTS idx_sso_domain_domain ON sso_domain_verifications(domain);

CREATE INDEX IF NOT EXISTS idx_team_branding_custom_domain ON team_branding(custom_domain) WHERE custom_domain IS NOT NULL AND custom_domain_verified = TRUE;
CREATE INDEX IF NOT EXISTS idx_team_branding_team_id ON team_branding(team_id);

SELECT '✅ Step 2 완료: FK와 인덱스 생성 성공!' as result;
