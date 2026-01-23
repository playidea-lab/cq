-- Migration: 00004_sso_config.sql
-- Description: SSO/SAML configuration for Enterprise authentication
-- Phase 5: SSO/SAML implementation

-- ============================================================================
-- SSO Configuration Table
-- ============================================================================

-- Team SSO configuration (one per team)
CREATE TABLE IF NOT EXISTS team_sso_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- Provider type
    provider TEXT NOT NULL,  -- 'google', 'microsoft', 'okta', 'saml'

    -- OIDC settings (Google, Microsoft)
    client_id TEXT,
    client_secret_encrypted TEXT,  -- Encrypted with app key
    issuer_url TEXT,

    -- SAML settings (Okta, OneLogin, custom)
    entity_id TEXT,
    sso_url TEXT,
    slo_url TEXT,  -- Single Logout URL
    certificate TEXT,  -- X.509 certificate for signature validation

    -- Common settings
    auto_provision BOOLEAN DEFAULT true,  -- JIT user provisioning
    default_role TEXT DEFAULT 'member',  -- Default role for new users
    allowed_domains TEXT[],  -- Allowed email domains (e.g., ['company.com'])

    -- Status
    enabled BOOLEAN DEFAULT false,
    verified BOOLEAN DEFAULT false,  -- Configuration verified working

    -- Metadata
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_by UUID REFERENCES auth.users(id),

    -- One SSO config per team
    UNIQUE(team_id)
);

-- ============================================================================
-- SSO Sessions Table
-- ============================================================================

-- Track SSO sessions for audit and logout
CREATE TABLE IF NOT EXISTS sso_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- SSO provider info
    provider TEXT NOT NULL,
    provider_user_id TEXT,  -- User ID from provider
    provider_email TEXT,

    -- Session data
    session_token_hash TEXT,  -- Hash of session token

    -- SAML assertion info (for audit)
    assertion_id TEXT,  -- SAML assertion ID
    assertion_hash TEXT,  -- Hash for integrity verification

    -- Timestamps
    authenticated_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    last_activity_at TIMESTAMPTZ DEFAULT NOW(),

    -- Status
    revoked BOOLEAN DEFAULT false,
    revoked_at TIMESTAMPTZ,
    revoked_reason TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- SSO Domain Verification Table
-- ============================================================================

-- Domain ownership verification for SSO
CREATE TABLE IF NOT EXISTS sso_domain_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    domain TEXT NOT NULL,

    -- Verification method
    verification_method TEXT NOT NULL,  -- 'dns_txt', 'dns_cname', 'meta_tag'
    verification_token TEXT NOT NULL,

    -- Status
    verified BOOLEAN DEFAULT false,
    verified_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,  -- Verification token expiry

    UNIQUE(team_id, domain)
);

-- ============================================================================
-- Indexes
-- ============================================================================

-- SSO config lookup
CREATE INDEX IF NOT EXISTS idx_sso_configs_team ON team_sso_configs(team_id);
CREATE INDEX IF NOT EXISTS idx_sso_configs_provider ON team_sso_configs(provider);
CREATE INDEX IF NOT EXISTS idx_sso_configs_enabled ON team_sso_configs(enabled) WHERE enabled = true;

-- SSO sessions lookup
CREATE INDEX IF NOT EXISTS idx_sso_sessions_user ON sso_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sso_sessions_team ON sso_sessions(team_id);
CREATE INDEX IF NOT EXISTS idx_sso_sessions_expires ON sso_sessions(expires_at) WHERE revoked = false;
CREATE INDEX IF NOT EXISTS idx_sso_sessions_provider_user ON sso_sessions(provider, provider_user_id);

-- Domain verification
CREATE INDEX IF NOT EXISTS idx_sso_domains_team ON sso_domain_verifications(team_id);
CREATE INDEX IF NOT EXISTS idx_sso_domains_domain ON sso_domain_verifications(domain);

-- ============================================================================
-- Row Level Security
-- ============================================================================

ALTER TABLE team_sso_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE sso_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sso_domain_verifications ENABLE ROW LEVEL SECURITY;

-- SSO configs: Only team admins/owners can view and manage
CREATE POLICY "Team admins can view SSO config"
ON team_sso_configs FOR SELECT
USING (team_id IN (
    SELECT team_id FROM team_members
    WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
));

CREATE POLICY "Team admins can manage SSO config"
ON team_sso_configs FOR ALL
USING (team_id IN (
    SELECT team_id FROM team_members
    WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
));

-- SSO sessions: Users can view their own, admins can view all team sessions
CREATE POLICY "Users can view own SSO sessions"
ON sso_sessions FOR SELECT
USING (user_id = auth.uid());

CREATE POLICY "Team admins can view team SSO sessions"
ON sso_sessions FOR SELECT
USING (team_id IN (
    SELECT team_id FROM team_members
    WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
));

CREATE POLICY "Team admins can revoke SSO sessions"
ON sso_sessions FOR UPDATE
USING (team_id IN (
    SELECT team_id FROM team_members
    WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
));

-- Domain verifications: Team admins only
CREATE POLICY "Team admins can manage domain verifications"
ON sso_domain_verifications FOR ALL
USING (team_id IN (
    SELECT team_id FROM team_members
    WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
));

-- ============================================================================
-- Triggers
-- ============================================================================

-- Update updated_at on SSO config changes
CREATE OR REPLACE FUNCTION update_sso_config_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_sso_config_updated_at
    BEFORE UPDATE ON team_sso_configs
    FOR EACH ROW
    EXECUTE FUNCTION update_sso_config_updated_at();

-- Update last_activity_at on session activity
CREATE OR REPLACE FUNCTION update_sso_session_activity()
RETURNS TRIGGER AS $$
BEGIN
    NEW.last_activity_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_sso_session_activity
    BEFORE UPDATE ON sso_sessions
    FOR EACH ROW
    WHEN (OLD.revoked = false AND NEW.revoked = false)
    EXECUTE FUNCTION update_sso_session_activity();

-- ============================================================================
-- Helper Functions
-- ============================================================================

-- Get SSO config for a team (with decryption placeholder)
CREATE OR REPLACE FUNCTION get_team_sso_config(p_team_id UUID)
RETURNS TABLE (
    id UUID,
    provider TEXT,
    client_id TEXT,
    issuer_url TEXT,
    entity_id TEXT,
    sso_url TEXT,
    auto_provision BOOLEAN,
    default_role TEXT,
    allowed_domains TEXT[],
    enabled BOOLEAN,
    verified BOOLEAN
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        c.id,
        c.provider,
        c.client_id,
        c.issuer_url,
        c.entity_id,
        c.sso_url,
        c.auto_provision,
        c.default_role,
        c.allowed_domains,
        c.enabled,
        c.verified
    FROM team_sso_configs c
    WHERE c.team_id = p_team_id AND c.enabled = true;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Check if email domain is allowed for SSO
CREATE OR REPLACE FUNCTION is_sso_domain_allowed(p_team_id UUID, p_email TEXT)
RETURNS BOOLEAN AS $$
DECLARE
    v_domain TEXT;
    v_allowed_domains TEXT[];
BEGIN
    -- Extract domain from email
    v_domain := split_part(p_email, '@', 2);

    -- Get allowed domains
    SELECT allowed_domains INTO v_allowed_domains
    FROM team_sso_configs
    WHERE team_id = p_team_id AND enabled = true;

    -- If no config or no domains specified, allow all
    IF v_allowed_domains IS NULL OR array_length(v_allowed_domains, 1) IS NULL THEN
        RETURN true;
    END IF;

    -- Check if domain is in allowed list
    RETURN v_domain = ANY(v_allowed_domains);
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Clean up expired SSO sessions
CREATE OR REPLACE FUNCTION cleanup_expired_sso_sessions()
RETURNS INTEGER AS $$
DECLARE
    v_count INTEGER;
BEGIN
    WITH deleted AS (
        DELETE FROM sso_sessions
        WHERE expires_at < NOW() AND revoked = false
        RETURNING id
    )
    SELECT COUNT(*) INTO v_count FROM deleted;

    RETURN v_count;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- ============================================================================
-- Comments
-- ============================================================================

COMMENT ON TABLE team_sso_configs IS 'SSO configuration per team (OIDC/SAML)';
COMMENT ON TABLE sso_sessions IS 'Active SSO sessions for audit and logout';
COMMENT ON TABLE sso_domain_verifications IS 'Domain ownership verification for SSO';

COMMENT ON COLUMN team_sso_configs.client_secret_encrypted IS 'Encrypted OIDC client secret';
COMMENT ON COLUMN team_sso_configs.certificate IS 'X.509 certificate for SAML signature validation';
COMMENT ON COLUMN team_sso_configs.auto_provision IS 'Automatically create users on first SSO login';
COMMENT ON COLUMN team_sso_configs.allowed_domains IS 'Email domains allowed to use SSO';

COMMENT ON COLUMN sso_sessions.assertion_hash IS 'SHA256 hash of SAML assertion for integrity';
COMMENT ON COLUMN sso_sessions.revoked IS 'Session manually revoked by admin';
