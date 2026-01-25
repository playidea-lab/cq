-- ============================================================================
-- Step 4: Helper Functions
-- Step 3 성공 후 실행하세요.
-- ============================================================================

-- ============================================================================
-- Function: get_user_teams
-- ============================================================================

CREATE OR REPLACE FUNCTION get_user_teams(p_user_id UUID)
RETURNS TABLE (
    team_id UUID,
    team_name TEXT,
    team_slug TEXT,
    user_role TEXT,
    joined_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
    RETURN QUERY
    SELECT
        t.id AS team_id,
        t.name AS team_name,
        t.slug AS team_slug,
        tm.role AS user_role,
        tm.created_at AS joined_at
    FROM teams t
    JOIN team_members tm ON t.id = tm.team_id
    WHERE tm.user_id = p_user_id;
END;
$$;

-- ============================================================================
-- Function: check_team_permission
-- ============================================================================

CREATE OR REPLACE FUNCTION check_team_permission(
    p_user_id UUID,
    p_team_id UUID,
    p_required_role TEXT DEFAULT 'member'
)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_role TEXT;
    v_role_hierarchy TEXT[] := ARRAY['viewer', 'member', 'admin', 'owner'];
    v_user_level INTEGER;
    v_required_level INTEGER;
BEGIN
    SELECT role INTO v_role
    FROM team_members
    WHERE team_id = p_team_id AND user_id = p_user_id;

    IF v_role IS NULL THEN
        RETURN FALSE;
    END IF;

    v_user_level := array_position(v_role_hierarchy, v_role);
    v_required_level := array_position(v_role_hierarchy, p_required_role);

    IF v_user_level IS NULL OR v_required_level IS NULL THEN
        RETURN FALSE;
    END IF;

    RETURN v_user_level >= v_required_level;
END;
$$;

-- ============================================================================
-- Function: create_team_with_owner
-- ============================================================================

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

-- ============================================================================
-- Function: invite_team_member
-- ============================================================================

CREATE OR REPLACE FUNCTION invite_team_member(
    p_team_id UUID,
    p_email TEXT,
    p_role TEXT,
    p_invited_by UUID
)
RETURNS UUID
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_member_id UUID;
    v_invite_token TEXT;
BEGIN
    IF NOT check_team_permission(p_invited_by, p_team_id, 'admin') THEN
        RAISE EXCEPTION 'Not authorized to invite members';
    END IF;

    v_invite_token := encode(gen_random_bytes(32), 'hex');

    INSERT INTO team_members (
        team_id, user_id, role, invited_by, invited_at, invite_email, invite_token
    )
    VALUES (
        p_team_id,
        '00000000-0000-0000-0000-000000000000'::UUID,
        p_role,
        p_invited_by,
        NOW(),
        p_email,
        v_invite_token
    )
    RETURNING id INTO v_member_id;

    RETURN v_member_id;
END;
$$;

-- ============================================================================
-- Function: accept_team_invite
-- ============================================================================

CREATE OR REPLACE FUNCTION accept_team_invite(
    p_invite_token TEXT,
    p_user_id UUID
)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_member_id UUID;
BEGIN
    UPDATE team_members
    SET
        user_id = p_user_id,
        accepted_at = NOW(),
        invite_token = NULL
    WHERE invite_token = p_invite_token
      AND accepted_at IS NULL
    RETURNING id INTO v_member_id;

    RETURN v_member_id IS NOT NULL;
END;
$$;

-- ============================================================================
-- Function: get_team_branding_by_domain
-- ============================================================================

CREATE OR REPLACE FUNCTION get_team_branding_by_domain(p_domain TEXT)
RETURNS SETOF team_branding
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
    RETURN QUERY
    SELECT tb.*
    FROM team_branding tb
    WHERE tb.custom_domain = p_domain
      AND tb.custom_domain_verified = TRUE;
END;
$$;

-- ============================================================================
-- Function: log_activity
-- ============================================================================

CREATE OR REPLACE FUNCTION log_activity(
    p_team_id UUID,
    p_user_id UUID,
    p_activity_type TEXT,
    p_resource_type TEXT DEFAULT NULL,
    p_resource_id TEXT DEFAULT NULL,
    p_metadata JSONB DEFAULT '{}'::JSONB,
    p_project_id UUID DEFAULT NULL
)
RETURNS UUID
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_log_id UUID;
BEGIN
    INSERT INTO activity_logs (
        team_id, user_id, project_id, activity_type,
        resource_type, resource_id, metadata, started_at
    )
    VALUES (
        p_team_id, p_user_id, p_project_id, p_activity_type,
        p_resource_type, p_resource_id, p_metadata, NOW()
    )
    RETURNING id INTO v_log_id;

    RETURN v_log_id;
END;
$$;

-- ============================================================================
-- Function: log_audit_event
-- ============================================================================

CREATE OR REPLACE FUNCTION log_audit_event(
    p_team_id UUID,
    p_actor_type TEXT,
    p_actor_id TEXT,
    p_action TEXT,
    p_resource_type TEXT,
    p_resource_id TEXT,
    p_old_value JSONB DEFAULT NULL,
    p_new_value JSONB DEFAULT NULL,
    p_actor_email TEXT DEFAULT NULL,
    p_ip_address INET DEFAULT NULL,
    p_user_agent TEXT DEFAULT NULL,
    p_request_id TEXT DEFAULT NULL
)
RETURNS UUID
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_log_id UUID;
BEGIN
    INSERT INTO audit_logs (
        team_id, actor_type, actor_id, actor_email, action,
        resource_type, resource_id, old_value, new_value,
        ip_address, user_agent, request_id
    )
    VALUES (
        p_team_id, p_actor_type, p_actor_id, p_actor_email, p_action,
        p_resource_type, p_resource_id, p_old_value, p_new_value,
        p_ip_address, p_user_agent, p_request_id
    )
    RETURNING id INTO v_log_id;

    RETURN v_log_id;
END;
$$;

SELECT '✅ Step 4 완료: Helper 함수 생성 성공!' as result;
