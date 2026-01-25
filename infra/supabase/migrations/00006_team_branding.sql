-- Team Branding Schema for White-label Functionality
-- Migration: 00006_team_branding.sql
-- Description: Adds team branding customization for agency white-label support

-- ============================================================================
-- TEAM BRANDING TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS team_branding (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- Basic Branding
    logo_url TEXT,                          -- Main logo (light background)
    logo_dark_url TEXT,                     -- Logo for dark background
    favicon_url TEXT,                       -- Favicon/browser icon
    brand_name TEXT,                        -- Display name (overrides team name)

    -- Color Scheme
    primary_color TEXT DEFAULT '#2563EB',   -- Main brand color
    secondary_color TEXT DEFAULT '#64748B', -- Secondary color
    accent_color TEXT DEFAULT '#F59E0B',    -- Accent/highlight color
    background_color TEXT DEFAULT '#FFFFFF',-- Background color
    text_color TEXT DEFAULT '#1F2937',      -- Primary text color

    -- Typography
    heading_font TEXT,                      -- Font for headings (Google Fonts name)
    body_font TEXT,                         -- Font for body text
    font_scale DECIMAL(3,2) DEFAULT 1.0,    -- Font size multiplier

    -- Custom Domain (Enterprise feature)
    custom_domain TEXT UNIQUE,              -- e.g., 'projects.agency.com'
    custom_domain_verified BOOLEAN DEFAULT FALSE,
    custom_domain_verification_token TEXT,
    custom_domain_verified_at TIMESTAMPTZ,

    -- Email Branding
    email_from_name TEXT,                   -- "From" name in emails
    email_footer_text TEXT,                 -- Custom email footer
    email_header_html TEXT,                 -- Custom email header HTML

    -- Advanced Customization
    custom_css TEXT,                        -- Additional CSS (Enterprise)
    meta_description TEXT,                  -- SEO meta description
    social_preview_image_url TEXT,          -- OG image for link previews

    -- Feature Flags
    hide_powered_by BOOLEAN DEFAULT FALSE,  -- Hide "Powered by C4" (Enterprise)
    custom_login_background_url TEXT,       -- Custom login page background

    -- Metadata
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_by UUID REFERENCES auth.users(id),

    -- Ensure one branding per team
    UNIQUE(team_id)
);

-- Index for custom domain lookups
CREATE INDEX IF NOT EXISTS idx_team_branding_custom_domain
ON team_branding(custom_domain)
WHERE custom_domain IS NOT NULL AND custom_domain_verified = TRUE;

-- Index for team lookups
CREATE INDEX IF NOT EXISTS idx_team_branding_team_id ON team_branding(team_id);

-- ============================================================================
-- ROW LEVEL SECURITY
-- ============================================================================

ALTER TABLE team_branding ENABLE ROW LEVEL SECURITY;

-- Team members can view their team's branding
CREATE POLICY "Team members can view branding"
ON team_branding FOR SELECT
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid()
    )
);

-- Team admins/owners can update branding
CREATE POLICY "Team admins can manage branding"
ON team_branding FOR ALL
USING (
    team_id IN (
        SELECT team_id FROM team_members
        WHERE user_id = auth.uid()
        AND role IN ('owner', 'admin')
    )
);

-- Public access to branding via custom domain (for login pages)
CREATE POLICY "Public can view branding by custom domain"
ON team_branding FOR SELECT
USING (
    custom_domain IS NOT NULL
    AND custom_domain_verified = TRUE
);

-- ============================================================================
-- HELPER FUNCTIONS
-- ============================================================================

-- Function to get branding by custom domain
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

-- Function to get or create default branding for a team
CREATE OR REPLACE FUNCTION get_or_create_team_branding(p_team_id UUID)
RETURNS team_branding
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_branding team_branding;
    v_team teams;
BEGIN
    -- Try to get existing branding
    SELECT * INTO v_branding
    FROM team_branding
    WHERE team_id = p_team_id;

    -- If not found, create default branding
    IF v_branding.id IS NULL THEN
        -- Get team info for defaults
        SELECT * INTO v_team FROM teams WHERE id = p_team_id;

        INSERT INTO team_branding (team_id, brand_name, created_by)
        VALUES (p_team_id, v_team.name, auth.uid())
        RETURNING * INTO v_branding;
    END IF;

    RETURN v_branding;
END;
$$;

-- Function to initiate custom domain verification
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
    -- Check user has admin access
    IF NOT EXISTS (
        SELECT 1 FROM team_members
        WHERE team_id = p_team_id
        AND user_id = auth.uid()
        AND role IN ('owner', 'admin')
    ) THEN
        RETURN json_build_object(
            'success', false,
            'error', 'Not authorized'
        );
    END IF;

    -- Check domain format (basic validation)
    IF p_domain !~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$' THEN
        RETURN json_build_object(
            'success', false,
            'error', 'Invalid domain format'
        );
    END IF;

    -- Check domain not already in use
    IF EXISTS (
        SELECT 1 FROM team_branding
        WHERE custom_domain = p_domain
        AND team_id != p_team_id
    ) THEN
        RETURN json_build_object(
            'success', false,
            'error', 'Domain already in use'
        );
    END IF;

    -- Generate verification token
    v_token := 'c4-verify-' || encode(gen_random_bytes(16), 'hex');

    -- Update or insert branding with verification token
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

-- Function to verify custom domain (called after DNS is set up)
CREATE OR REPLACE FUNCTION verify_custom_domain(p_team_id UUID)
RETURNS JSON
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_branding team_branding;
BEGIN
    -- Get current branding
    SELECT * INTO v_branding
    FROM team_branding
    WHERE team_id = p_team_id;

    IF v_branding.id IS NULL THEN
        RETURN json_build_object(
            'success', false,
            'error', 'No branding found'
        );
    END IF;

    IF v_branding.custom_domain IS NULL THEN
        RETURN json_build_object(
            'success', false,
            'error', 'No custom domain configured'
        );
    END IF;

    -- Note: Actual DNS verification should be done in application code
    -- This function assumes verification was done externally
    -- Mark as verified
    UPDATE team_branding
    SET custom_domain_verified = TRUE,
        custom_domain_verified_at = NOW(),
        updated_at = NOW()
    WHERE team_id = p_team_id;

    RETURN json_build_object(
        'success', true,
        'domain', v_branding.custom_domain,
        'verified_at', NOW()
    );
END;
$$;

-- Function to update team branding
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
    -- Check user has admin access
    IF NOT EXISTS (
        SELECT 1 FROM team_members
        WHERE team_id = p_team_id
        AND user_id = auth.uid()
        AND role IN ('owner', 'admin')
    ) THEN
        RAISE EXCEPTION 'Not authorized';
    END IF;

    -- Ensure branding exists
    INSERT INTO team_branding (team_id, created_by)
    VALUES (p_team_id, auth.uid())
    ON CONFLICT (team_id) DO NOTHING;

    -- Update allowed fields only
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
-- TRIGGER FOR UPDATED_AT
-- ============================================================================

CREATE OR REPLACE FUNCTION update_team_branding_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER team_branding_updated_at
    BEFORE UPDATE ON team_branding
    FOR EACH ROW
    EXECUTE FUNCTION update_team_branding_updated_at();

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE team_branding IS 'White-label branding configuration for teams/agencies';
COMMENT ON COLUMN team_branding.custom_domain IS 'Custom domain for white-label access (e.g., projects.agency.com)';
COMMENT ON COLUMN team_branding.hide_powered_by IS 'Enterprise feature: hide C4 branding';
COMMENT ON COLUMN team_branding.custom_css IS 'Enterprise feature: additional CSS customization';
