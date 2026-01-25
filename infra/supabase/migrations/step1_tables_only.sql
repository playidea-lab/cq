-- ============================================================================
-- Step 1: 테이블만 생성 (FK, RLS 제외)
-- 이 스크립트를 먼저 실행해보세요.
-- ============================================================================

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================================
-- 1. TEAMS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    slug TEXT UNIQUE,
    owner_id UUID,
    settings JSONB DEFAULT '{}',
    stripe_customer_id TEXT,
    plan TEXT DEFAULT 'free',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- 2. TEAM_MEMBERS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS team_members (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    invited_by UUID,
    invited_at TIMESTAMPTZ,
    accepted_at TIMESTAMPTZ,
    invite_email TEXT,
    invite_token TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- 3. PROJECTS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT,
    repo_url TEXT,
    branch TEXT DEFAULT 'main',
    project_root TEXT DEFAULT '.',
    created_by UUID,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- 4. C4_STATE TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS c4_state (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL,
    current_status TEXT NOT NULL DEFAULT 'INIT',
    current_phase TEXT,
    priority_queue TEXT[] DEFAULT '{}',
    completed_tasks TEXT[] DEFAULT '{}',
    failed_tasks TEXT[] DEFAULT '{}',
    checkpoint_results JSONB DEFAULT '{}',
    validation_results JSONB DEFAULT '{}',
    context JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- 5. C4_TASKS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS c4_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    state_id UUID NOT NULL,
    task_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    scope TEXT,
    dod TEXT,
    priority INTEGER DEFAULT 0,
    dependencies TEXT[] DEFAULT '{}',
    status TEXT DEFAULT 'pending',
    assigned_worker TEXT,
    result JSONB,
    error TEXT,
    retries INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- 6. C4_WORKERS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS c4_workers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    state_id UUID NOT NULL,
    worker_id TEXT NOT NULL,
    status TEXT DEFAULT 'idle',
    current_task TEXT,
    started_at TIMESTAMPTZ,
    last_heartbeat TIMESTAMPTZ,
    metadata JSONB DEFAULT '{}'
);

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

-- ============================================================================
-- 8. INTEGRATION_PROVIDERS TABLE
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

-- ============================================================================
-- 9. TEAM_INTEGRATIONS TABLE
-- ============================================================================
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

-- ============================================================================
-- 10. ACTIVITY_LOGS TABLE
-- ============================================================================
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

-- ============================================================================
-- 11. AUDIT_LOGS TABLE
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

-- ============================================================================
-- 12. TEAM_SSO_CONFIGS TABLE
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

-- ============================================================================
-- 13. SSO_SESSIONS TABLE
-- ============================================================================
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

-- ============================================================================
-- 14. SSO_DOMAIN_VERIFICATIONS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS sso_domain_verifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL,
    domain TEXT NOT NULL UNIQUE,
    verification_token TEXT NOT NULL,
    verified BOOLEAN DEFAULT false,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- 15. TEAM_BRANDING TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS team_branding (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL UNIQUE,
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

SELECT '✅ Step 1 완료: 테이블 생성 성공!' as result;
