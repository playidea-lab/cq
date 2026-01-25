-- ============================================================================
-- C4 Combined Migration Script
-- ============================================================================
-- 이 스크립트는 모든 마이그레이션을 합친 것입니다.
-- 기존 테이블이 있어도 안전하게 실행됩니다.
-- Supabase Dashboard > SQL Editor에서 실행하세요.
-- ============================================================================

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================================
-- 1. TEAMS TABLE (기존 테이블 수정 또는 새로 생성)
-- ============================================================================

-- 테이블이 없으면 생성
CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 누락된 컬럼 추가 (foreign key 없이 먼저 추가)
ALTER TABLE teams ADD COLUMN IF NOT EXISTS slug TEXT;
ALTER TABLE teams ADD COLUMN IF NOT EXISTS owner_id UUID;
ALTER TABLE teams ADD COLUMN IF NOT EXISTS settings JSONB DEFAULT '{}';
ALTER TABLE teams ADD COLUMN IF NOT EXISTS stripe_customer_id TEXT;
ALTER TABLE teams ADD COLUMN IF NOT EXISTS plan TEXT DEFAULT 'free';

-- owner_id FK는 auth.users 접근 제한으로 인해 앱 레벨에서 검증
-- (Supabase에서 auth 스키마 FK 제약조건 추가 불가)

-- slug에 unique 제약 추가 (없으면)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'teams_slug_key') THEN
        ALTER TABLE teams ADD CONSTRAINT teams_slug_key UNIQUE (slug);
    END IF;
EXCEPTION WHEN others THEN
    NULL;
END $$;

-- 인덱스 생성
CREATE INDEX IF NOT EXISTS idx_teams_owner ON teams(owner_id);
CREATE INDEX IF NOT EXISTS idx_teams_slug ON teams(slug);

-- ============================================================================
-- 2. TEAM_MEMBERS TABLE
-- ============================================================================

-- 테이블 생성 (foreign key 없이)
CREATE TABLE IF NOT EXISTS team_members (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 누락된 컬럼 추가
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS invited_by UUID;
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS invited_at TIMESTAMPTZ;
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS accepted_at TIMESTAMPTZ;
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS invite_email TEXT;
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS invite_token TEXT;

-- Foreign keys 추가
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'team_members_team_id_fkey' AND table_name = 'team_members') THEN
        ALTER TABLE team_members ADD CONSTRAINT team_members_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

-- user_id, invited_by FK는 auth.users 접근 제한으로 앱 레벨에서 검증

-- unique 제약
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'team_members_team_id_user_id_key') THEN
        ALTER TABLE team_members ADD CONSTRAINT team_members_team_id_user_id_key UNIQUE (team_id, user_id);
    END IF;
EXCEPTION WHEN others THEN
    NULL;
END $$;

-- 인덱스
CREATE INDEX IF NOT EXISTS idx_team_members_user ON team_members(user_id);
CREATE INDEX IF NOT EXISTS idx_team_members_team ON team_members(team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_invite_token ON team_members(invite_token) WHERE invite_token IS NOT NULL;

-- ============================================================================
-- 3. PROJECTS TABLE
-- ============================================================================

-- 테이블 생성 (auth.users FK 없이)
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    repo_url TEXT,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_by UUID
);

-- Foreign keys 추가
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'projects_team_id_fkey' AND table_name = 'projects') THEN
        ALTER TABLE projects ADD CONSTRAINT projects_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

-- created_by FK는 auth.users 접근 제한으로 앱 레벨에서 검증

CREATE INDEX IF NOT EXISTS idx_projects_team ON projects(team_id);

-- ============================================================================
-- 4. C4_STATE TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS c4_state (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'INIT',
    current_phase TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- FK 추가 (별도 블록)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'c4_state_project_id_fkey' AND table_name = 'c4_state') THEN
        ALTER TABLE c4_state ADD CONSTRAINT c4_state_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_c4_state_project ON c4_state(project_id);

-- ============================================================================
-- 5. C4_TASKS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS c4_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    state_id UUID NOT NULL,
    task_id TEXT NOT NULL,
    title TEXT NOT NULL,
    scope TEXT,
    dod TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    priority INTEGER DEFAULT 0,
    dependencies TEXT[],
    assigned_worker TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- FK 추가 (별도 블록)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'c4_tasks_state_id_fkey' AND table_name = 'c4_tasks') THEN
        ALTER TABLE c4_tasks ADD CONSTRAINT c4_tasks_state_id_fkey FOREIGN KEY (state_id) REFERENCES c4_state(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'c4_tasks_state_id_task_id_key') THEN
        ALTER TABLE c4_tasks ADD CONSTRAINT c4_tasks_state_id_task_id_key UNIQUE (state_id, task_id);
    END IF;
EXCEPTION WHEN others THEN
    NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_c4_tasks_state ON c4_tasks(state_id);
CREATE INDEX IF NOT EXISTS idx_c4_tasks_status ON c4_tasks(status);

-- ============================================================================
-- 6. C4_WORKERS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS c4_workers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    state_id UUID NOT NULL,
    worker_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'idle',
    current_task TEXT,
    metadata JSONB DEFAULT '{}',
    last_heartbeat TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- FK 추가 (별도 블록)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'c4_workers_state_id_fkey' AND table_name = 'c4_workers') THEN
        ALTER TABLE c4_workers ADD CONSTRAINT c4_workers_state_id_fkey FOREIGN KEY (state_id) REFERENCES c4_state(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'c4_workers_state_id_worker_id_key') THEN
        ALTER TABLE c4_workers ADD CONSTRAINT c4_workers_state_id_worker_id_key UNIQUE (state_id, worker_id);
    END IF;
EXCEPTION WHEN others THEN
    NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_c4_workers_state ON c4_workers(state_id);

-- ============================================================================
-- 7. C4_EVENTS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS c4_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    state_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- FK 추가 (별도 블록)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'c4_events_state_id_fkey' AND table_name = 'c4_events') THEN
        ALTER TABLE c4_events ADD CONSTRAINT c4_events_state_id_fkey FOREIGN KEY (state_id) REFERENCES c4_state(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_c4_events_state ON c4_events(state_id);
CREATE INDEX IF NOT EXISTS idx_c4_events_type ON c4_events(event_type);

-- ============================================================================
-- 8. INTEGRATION_PROVIDERS TABLE (시스템 테이블)
-- ============================================================================

CREATE TABLE IF NOT EXISTS integration_providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    oauth_url TEXT,
    webhook_path TEXT,
    icon_url TEXT,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 초기 프로바이더 데이터
INSERT INTO integration_providers (id, name, category, oauth_url, webhook_path, icon_url, enabled)
VALUES
    ('github', 'GitHub', 'source_control', 'https://github.com/login/oauth/authorize', '/webhooks/github', 'https://github.githubassets.com/images/modules/logos_page/GitHub-Mark.png', true),
    ('discord', 'Discord', 'messaging', 'https://discord.com/api/oauth2/authorize', '/webhooks/discord', 'https://assets-global.website-files.com/6257adef93867e50d84d30e2/636e0a6a49cf127bf92de1e2_icon_clyde_blurple_RGB.png', true),
    ('dooray', 'Dooray', 'collaboration', 'https://auth.dooray.com/oauth/authorize', '/webhooks/dooray', 'https://door.dooray.com/assets/images/dooray_logo.png', true),
    ('slack', 'Slack', 'messaging', 'https://slack.com/oauth/v2/authorize', '/webhooks/slack', 'https://a.slack-edge.com/80588/marketing/img/icons/icon_slack_hash_colored.png', true)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    category = EXCLUDED.category,
    oauth_url = EXCLUDED.oauth_url,
    webhook_path = EXCLUDED.webhook_path,
    icon_url = EXCLUDED.icon_url;

-- ============================================================================
-- 9. TEAM_INTEGRATIONS TABLE
-- ============================================================================

-- 테이블 생성 (FK 없이)
CREATE TABLE IF NOT EXISTS team_integrations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL,
    provider_id TEXT NOT NULL,
    external_id TEXT NOT NULL,
    external_name TEXT,
    credentials JSONB,
    settings JSONB DEFAULT '{}',
    status TEXT DEFAULT 'active',
    connected_by UUID,
    connected_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

-- Foreign keys 추가
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'team_integrations_team_id_fkey' AND table_name = 'team_integrations') THEN
        ALTER TABLE team_integrations ADD CONSTRAINT team_integrations_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'team_integrations_provider_id_fkey' AND table_name = 'team_integrations') THEN
        ALTER TABLE team_integrations ADD CONSTRAINT team_integrations_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES integration_providers(id);
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

-- connected_by FK는 auth.users 접근 제한으로 앱 레벨에서 검증

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'team_integrations_team_provider_external_key') THEN
        ALTER TABLE team_integrations ADD CONSTRAINT team_integrations_team_provider_external_key UNIQUE (team_id, provider_id, external_id);
    END IF;
EXCEPTION WHEN others THEN
    NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_integrations_provider_external ON team_integrations(provider_id, external_id);
CREATE INDEX IF NOT EXISTS idx_integrations_team ON team_integrations(team_id, status);

-- ============================================================================
-- 10. ACTIVITY_LOGS TABLE (타임트래킹)
-- ============================================================================

-- 테이블 생성 (FK 없이)
CREATE TABLE IF NOT EXISTS activity_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL,
    user_id UUID,
    project_id UUID,
    activity_type TEXT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    metadata JSONB DEFAULT '{}',
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Foreign keys 추가
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'activity_logs_team_id_fkey' AND table_name = 'activity_logs') THEN
        ALTER TABLE activity_logs ADD CONSTRAINT activity_logs_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

-- user_id FK는 auth.users 접근 제한으로 앱 레벨에서 검증

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'activity_logs_project_id_fkey' AND table_name = 'activity_logs') THEN
        ALTER TABLE activity_logs ADD CONSTRAINT activity_logs_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

-- duration_seconds는 generated column으로 추가 시도 (이미 있으면 무시)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'activity_logs' AND column_name = 'duration_seconds') THEN
        ALTER TABLE activity_logs ADD COLUMN duration_seconds INTEGER GENERATED ALWAYS AS (
            CASE WHEN ended_at IS NOT NULL THEN
                EXTRACT(EPOCH FROM (ended_at - started_at))::INTEGER
            ELSE NULL END
        ) STORED;
    END IF;
EXCEPTION WHEN others THEN
    NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_activity_logs_team ON activity_logs(team_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_logs_user ON activity_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_activity_logs_type ON activity_logs(activity_type);

-- ============================================================================
-- 11. AUDIT_LOGS TABLE (감사 로그)
-- ============================================================================

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    actor_email TEXT,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    old_value JSONB,
    new_value JSONB,
    ip_address INET,
    user_agent TEXT,
    request_id TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- FK 추가 (별도 블록)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'audit_logs_team_id_fkey' AND table_name = 'audit_logs') THEN
        ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

-- hash 컬럼은 generated column으로 추가 시도 (이미 있으면 무시)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'audit_logs' AND column_name = 'hash') THEN
        ALTER TABLE audit_logs ADD COLUMN hash TEXT GENERATED ALWAYS AS (
            encode(sha256(
                (id::text || actor_id || action || resource_id || created_at::text)::bytea
            ), 'hex')
        ) STORED;
    END IF;
EXCEPTION WHEN others THEN
    NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_audit_logs_team ON audit_logs(team_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);

-- ============================================================================
-- 12. TEAM_USAGE_SUMMARY VIEW
-- ============================================================================

CREATE OR REPLACE VIEW team_usage_summary AS
SELECT
    team_id,
    DATE_TRUNC('day', started_at) as date,
    activity_type,
    COUNT(*) as count,
    SUM(duration_seconds) as total_seconds
FROM activity_logs
WHERE ended_at IS NOT NULL
GROUP BY team_id, DATE_TRUNC('day', started_at), activity_type;

-- ============================================================================
-- 13. TEAM_SSO_CONFIGS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS team_sso_configs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL,
    provider TEXT NOT NULL,
    client_id TEXT,
    client_secret_encrypted TEXT,
    issuer_url TEXT,
    entity_id TEXT,
    sso_url TEXT,
    certificate TEXT,
    auto_provision BOOLEAN DEFAULT true,
    default_role TEXT DEFAULT 'member',
    allowed_domains TEXT[],
    enabled BOOLEAN DEFAULT false,
    verified BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- FK 추가 (별도 블록)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'team_sso_configs_team_id_fkey' AND table_name = 'team_sso_configs') THEN
        ALTER TABLE team_sso_configs ADD CONSTRAINT team_sso_configs_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'team_sso_configs_team_id_key') THEN
        ALTER TABLE team_sso_configs ADD CONSTRAINT team_sso_configs_team_id_key UNIQUE (team_id);
    END IF;
EXCEPTION WHEN others THEN
    NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_sso_configs_team ON team_sso_configs(team_id);

-- ============================================================================
-- 14. SSO_SESSIONS TABLE
-- ============================================================================

-- 테이블 생성 (FK 없이)
CREATE TABLE IF NOT EXISTS sso_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    team_id UUID NOT NULL,
    provider TEXT NOT NULL,
    provider_user_id TEXT,
    assertion_hash TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Foreign keys 추가
-- user_id FK는 auth.users 접근 제한으로 앱 레벨에서 검증

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'sso_sessions_team_id_fkey' AND table_name = 'sso_sessions') THEN
        ALTER TABLE sso_sessions ADD CONSTRAINT sso_sessions_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_sso_sessions_user ON sso_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sso_sessions_team ON sso_sessions(team_id);
CREATE INDEX IF NOT EXISTS idx_sso_sessions_expires ON sso_sessions(expires_at);

-- ============================================================================
-- 15. SSO_DOMAIN_VERIFICATIONS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS sso_domain_verifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL,
    domain TEXT NOT NULL,
    verification_token TEXT NOT NULL,
    verified BOOLEAN DEFAULT false,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- FK 추가 (별도 블록)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'sso_domain_verifications_team_id_fkey' AND table_name = 'sso_domain_verifications') THEN
        ALTER TABLE sso_domain_verifications ADD CONSTRAINT sso_domain_verifications_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'sso_domain_verifications_domain_key') THEN
        ALTER TABLE sso_domain_verifications ADD CONSTRAINT sso_domain_verifications_domain_key UNIQUE (domain);
    END IF;
EXCEPTION WHEN others THEN
    NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_sso_domain_team ON sso_domain_verifications(team_id);
CREATE INDEX IF NOT EXISTS idx_sso_domain_domain ON sso_domain_verifications(domain);

-- ============================================================================
-- 16. TEAM_BRANDING TABLE
-- ============================================================================

-- 테이블 생성 (FK 없이)
CREATE TABLE IF NOT EXISTS team_branding (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL,
    logo_url TEXT,
    logo_dark_url TEXT,
    favicon_url TEXT,
    brand_name TEXT,
    primary_color TEXT DEFAULT '#2563EB',
    secondary_color TEXT DEFAULT '#64748B',
    accent_color TEXT DEFAULT '#F59E0B',
    background_color TEXT DEFAULT '#FFFFFF',
    text_color TEXT DEFAULT '#1F2937',
    heading_font TEXT,
    body_font TEXT,
    font_scale DECIMAL(3,2) DEFAULT 1.0,
    custom_domain TEXT UNIQUE,
    custom_domain_verified BOOLEAN DEFAULT FALSE,
    custom_domain_verification_token TEXT,
    custom_domain_verified_at TIMESTAMPTZ,
    email_from_name TEXT,
    email_footer_text TEXT,
    email_header_html TEXT,
    custom_css TEXT,
    meta_description TEXT,
    social_preview_image_url TEXT,
    hide_powered_by BOOLEAN DEFAULT FALSE,
    custom_login_background_url TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_by UUID
);

-- Foreign keys 추가
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'team_branding_team_id_fkey' AND table_name = 'team_branding') THEN
        ALTER TABLE team_branding ADD CONSTRAINT team_branding_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
    END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

-- created_by FK는 auth.users 접근 제한으로 앱 레벨에서 검증

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'team_branding_team_id_key') THEN
        ALTER TABLE team_branding ADD CONSTRAINT team_branding_team_id_key UNIQUE (team_id);
    END IF;
EXCEPTION WHEN others THEN
    NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_team_branding_custom_domain ON team_branding(custom_domain) WHERE custom_domain IS NOT NULL AND custom_domain_verified = TRUE;
CREATE INDEX IF NOT EXISTS idx_team_branding_team_id ON team_branding(team_id);

-- ============================================================================
-- 17. ROW LEVEL SECURITY (RLS)
-- ============================================================================

-- Enable RLS on all tables
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

-- Drop existing policies and recreate
DO $$
BEGIN
    -- Teams policies
    DROP POLICY IF EXISTS "Team members can view team" ON teams;
    DROP POLICY IF EXISTS "Team admins can update team" ON teams;
    DROP POLICY IF EXISTS "Users can create teams" ON teams;

    -- Team members policies
    DROP POLICY IF EXISTS "Members can view team members" ON team_members;
    DROP POLICY IF EXISTS "Admins can manage team members" ON team_members;

    -- Projects policies
    DROP POLICY IF EXISTS "Team members can view projects" ON projects;
    DROP POLICY IF EXISTS "Team members can manage projects" ON projects;

    -- C4 state policies
    DROP POLICY IF EXISTS "Team members can view c4 state" ON c4_state;
    DROP POLICY IF EXISTS "Team members can manage c4 state" ON c4_state;

    -- Integrations policies
    DROP POLICY IF EXISTS "Team members can view integrations" ON team_integrations;
    DROP POLICY IF EXISTS "Team admins can manage integrations" ON team_integrations;

    -- Activity logs policies
    DROP POLICY IF EXISTS "Team members can view activity logs" ON activity_logs;
    DROP POLICY IF EXISTS "System can insert activity logs" ON activity_logs;

    -- Audit logs policies
    DROP POLICY IF EXISTS "Team admins can view audit logs" ON audit_logs;
    DROP POLICY IF EXISTS "System can insert audit logs" ON audit_logs;

    -- SSO policies
    DROP POLICY IF EXISTS "Team admins can manage SSO config" ON team_sso_configs;
    DROP POLICY IF EXISTS "Users can view own SSO sessions" ON sso_sessions;

    -- Branding policies
    DROP POLICY IF EXISTS "Team members can view branding" ON team_branding;
    DROP POLICY IF EXISTS "Team admins can manage branding" ON team_branding;
    DROP POLICY IF EXISTS "Public can view branding by custom domain" ON team_branding;
EXCEPTION WHEN others THEN
    NULL;
END $$;

-- Teams RLS
CREATE POLICY "Team members can view team" ON teams FOR SELECT
USING (id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid()));

CREATE POLICY "Team admins can update team" ON teams FOR UPDATE
USING (id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid() AND role IN ('owner', 'admin')));

CREATE POLICY "Users can create teams" ON teams FOR INSERT
WITH CHECK (auth.uid() IS NOT NULL);

-- Team members RLS
CREATE POLICY "Members can view team members" ON team_members FOR SELECT
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid()));

CREATE POLICY "Admins can manage team members" ON team_members FOR ALL
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid() AND role IN ('owner', 'admin')));

-- Projects RLS
CREATE POLICY "Team members can view projects" ON projects FOR SELECT
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid()));

CREATE POLICY "Team members can manage projects" ON projects FOR ALL
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid()));

-- C4 State RLS
CREATE POLICY "Team members can view c4 state" ON c4_state FOR SELECT
USING (project_id IN (
    SELECT p.id FROM projects p
    JOIN team_members tm ON p.team_id = tm.team_id
    WHERE tm.user_id = auth.uid()
));

CREATE POLICY "Team members can manage c4 state" ON c4_state FOR ALL
USING (project_id IN (
    SELECT p.id FROM projects p
    JOIN team_members tm ON p.team_id = tm.team_id
    WHERE tm.user_id = auth.uid()
));

-- Team Integrations RLS
CREATE POLICY "Team members can view integrations" ON team_integrations FOR SELECT
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid()));

CREATE POLICY "Team admins can manage integrations" ON team_integrations FOR ALL
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid() AND role IN ('owner', 'admin')));

-- Activity Logs RLS
CREATE POLICY "Team members can view activity logs" ON activity_logs FOR SELECT
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid()));

CREATE POLICY "System can insert activity logs" ON activity_logs FOR INSERT
WITH CHECK (true);

-- Audit Logs RLS
CREATE POLICY "Team admins can view audit logs" ON audit_logs FOR SELECT
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid() AND role IN ('owner', 'admin')));

CREATE POLICY "System can insert audit logs" ON audit_logs FOR INSERT
WITH CHECK (true);

-- SSO Config RLS
CREATE POLICY "Team admins can manage SSO config" ON team_sso_configs FOR ALL
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid() AND role IN ('owner', 'admin')));

-- SSO Sessions RLS
CREATE POLICY "Users can view own SSO sessions" ON sso_sessions FOR SELECT
USING (user_id = auth.uid());

-- Team Branding RLS
CREATE POLICY "Team members can view branding" ON team_branding FOR SELECT
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid()));

CREATE POLICY "Team admins can manage branding" ON team_branding FOR ALL
USING (team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid() AND role IN ('owner', 'admin')));

CREATE POLICY "Public can view branding by custom domain" ON team_branding FOR SELECT
USING (custom_domain IS NOT NULL AND custom_domain_verified = TRUE);

-- ============================================================================
-- 18. HELPER FUNCTIONS
-- ============================================================================

-- Create team with owner
CREATE OR REPLACE FUNCTION create_team_with_owner(
    p_name TEXT,
    p_slug TEXT,
    p_owner_id UUID
)
RETURNS UUID
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_team_id UUID;
BEGIN
    INSERT INTO teams (name, slug, owner_id)
    VALUES (p_name, p_slug, p_owner_id)
    RETURNING id INTO v_team_id;

    INSERT INTO team_members (team_id, user_id, role, accepted_at)
    VALUES (v_team_id, p_owner_id, 'owner', NOW());

    RETURN v_team_id;
END;
$$;

-- Create team invitation
CREATE OR REPLACE FUNCTION create_team_invitation(
    p_team_id UUID,
    p_email TEXT,
    p_role TEXT,
    p_invited_by UUID
)
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_token TEXT;
BEGIN
    v_token := encode(gen_random_bytes(32), 'hex');

    INSERT INTO team_members (team_id, user_id, role, invite_email, invite_token, invited_by, invited_at)
    VALUES (p_team_id, '00000000-0000-0000-0000-000000000000'::UUID, p_role, p_email, v_token, p_invited_by, NOW())
    ON CONFLICT (team_id, user_id) DO NOTHING;

    RETURN v_token;
END;
$$;

-- Accept team invitation
CREATE OR REPLACE FUNCTION accept_team_invitation(
    p_token TEXT,
    p_user_id UUID
)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_member team_members;
BEGIN
    SELECT * INTO v_member
    FROM team_members
    WHERE invite_token = p_token AND accepted_at IS NULL;

    IF v_member.id IS NULL THEN
        RETURN FALSE;
    END IF;

    UPDATE team_members
    SET user_id = p_user_id,
        accepted_at = NOW(),
        invite_token = NULL
    WHERE id = v_member.id;

    RETURN TRUE;
END;
$$;

-- Get branding by domain
CREATE OR REPLACE FUNCTION get_branding_by_domain(domain TEXT)
RETURNS SETOF team_branding
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
    RETURN QUERY
    SELECT *
    FROM team_branding
    WHERE custom_domain = domain
    AND custom_domain_verified = TRUE;
END;
$$;

-- Get or create team branding
CREATE OR REPLACE FUNCTION get_or_create_team_branding(p_team_id UUID)
RETURNS team_branding
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_branding team_branding;
    v_team teams;
BEGIN
    SELECT * INTO v_branding
    FROM team_branding
    WHERE team_id = p_team_id;

    IF v_branding.id IS NULL THEN
        SELECT * INTO v_team FROM teams WHERE id = p_team_id;

        INSERT INTO team_branding (team_id, brand_name, created_by)
        VALUES (p_team_id, v_team.name, auth.uid())
        RETURNING * INTO v_branding;
    END IF;

    RETURN v_branding;
END;
$$;

-- Initiate domain verification
CREATE OR REPLACE FUNCTION initiate_domain_verification(
    p_team_id UUID,
    p_domain TEXT
)
RETURNS JSON
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_token TEXT;
    v_branding team_branding;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM team_members
        WHERE team_id = p_team_id
        AND user_id = auth.uid()
        AND role IN ('owner', 'admin')
    ) THEN
        RETURN json_build_object('success', false, 'error', 'Not authorized');
    END IF;

    IF p_domain !~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$' THEN
        RETURN json_build_object('success', false, 'error', 'Invalid domain format');
    END IF;

    IF EXISTS (
        SELECT 1 FROM team_branding
        WHERE custom_domain = p_domain
        AND team_id != p_team_id
    ) THEN
        RETURN json_build_object('success', false, 'error', 'Domain already in use');
    END IF;

    v_token := 'c4-verify-' || encode(gen_random_bytes(16), 'hex');

    INSERT INTO team_branding (team_id, custom_domain, custom_domain_verification_token, custom_domain_verified)
    VALUES (p_team_id, p_domain, v_token, FALSE)
    ON CONFLICT (team_id) DO UPDATE SET
        custom_domain = p_domain,
        custom_domain_verification_token = v_token,
        custom_domain_verified = FALSE,
        custom_domain_verified_at = NULL,
        updated_at = NOW()
    RETURNING * INTO v_branding;

    RETURN json_build_object(
        'success', true,
        'verification_token', v_token,
        'instructions', json_build_object(
            'type', 'TXT',
            'name', '_c4-verification.' || p_domain,
            'value', v_token
        )
    );
END;
$$;

-- Update team branding
CREATE OR REPLACE FUNCTION update_team_branding(
    p_team_id UUID,
    p_updates JSONB
)
RETURNS team_branding
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_branding team_branding;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM team_members
        WHERE team_id = p_team_id
        AND user_id = auth.uid()
        AND role IN ('owner', 'admin')
    ) THEN
        RAISE EXCEPTION 'Not authorized';
    END IF;

    INSERT INTO team_branding (team_id, created_by)
    VALUES (p_team_id, auth.uid())
    ON CONFLICT (team_id) DO NOTHING;

    UPDATE team_branding
    SET
        logo_url = COALESCE(p_updates->>'logo_url', logo_url),
        logo_dark_url = COALESCE(p_updates->>'logo_dark_url', logo_dark_url),
        favicon_url = COALESCE(p_updates->>'favicon_url', favicon_url),
        brand_name = COALESCE(p_updates->>'brand_name', brand_name),
        primary_color = COALESCE(p_updates->>'primary_color', primary_color),
        secondary_color = COALESCE(p_updates->>'secondary_color', secondary_color),
        accent_color = COALESCE(p_updates->>'accent_color', accent_color),
        background_color = COALESCE(p_updates->>'background_color', background_color),
        text_color = COALESCE(p_updates->>'text_color', text_color),
        heading_font = COALESCE(p_updates->>'heading_font', heading_font),
        body_font = COALESCE(p_updates->>'body_font', body_font),
        email_from_name = COALESCE(p_updates->>'email_from_name', email_from_name),
        email_footer_text = COALESCE(p_updates->>'email_footer_text', email_footer_text),
        meta_description = COALESCE(p_updates->>'meta_description', meta_description),
        social_preview_image_url = COALESCE(p_updates->>'social_preview_image_url', social_preview_image_url),
        custom_login_background_url = COALESCE(p_updates->>'custom_login_background_url', custom_login_background_url),
        updated_at = NOW()
    WHERE team_id = p_team_id
    RETURNING * INTO v_branding;

    RETURN v_branding;
END;
$$;

-- ============================================================================
-- 19. TRIGGERS
-- ============================================================================

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply trigger to tables with updated_at
DO $$
DECLARE
    t TEXT;
BEGIN
    FOR t IN SELECT unnest(ARRAY['teams', 'projects', 'c4_state', 'c4_tasks', 'team_sso_configs', 'team_branding'])
    LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', 'update_' || t || '_updated_at', t);
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION update_updated_at_column()', 'update_' || t || '_updated_at', t);
    END LOOP;
END $$;

-- ============================================================================
-- 20. COMMENTS
-- ============================================================================

COMMENT ON TABLE teams IS 'Multi-tenant teams/organizations';
COMMENT ON TABLE team_members IS 'Team membership with roles';
COMMENT ON TABLE projects IS 'Projects within a team';
COMMENT ON TABLE c4_state IS 'C4 orchestration state machine';
COMMENT ON TABLE c4_tasks IS 'C4 tasks/todos';
COMMENT ON TABLE c4_workers IS 'C4 worker processes';
COMMENT ON TABLE c4_events IS 'C4 event log';
COMMENT ON TABLE integration_providers IS 'Available integration providers (system table)';
COMMENT ON TABLE team_integrations IS 'Team-specific integration connections';
COMMENT ON TABLE activity_logs IS 'Activity tracking for time/usage analytics';
COMMENT ON TABLE audit_logs IS 'Immutable audit trail for compliance';
COMMENT ON TABLE team_sso_configs IS 'SSO/SAML configuration per team';
COMMENT ON TABLE sso_sessions IS 'Active SSO sessions';
COMMENT ON TABLE team_branding IS 'White-label branding configuration';

-- ============================================================================
-- MIGRATION COMPLETE!
-- ============================================================================

SELECT '✅ Combined migration completed successfully!' as result;
