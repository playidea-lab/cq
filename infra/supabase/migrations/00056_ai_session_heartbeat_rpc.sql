-- Migration: 00056_ai_session_heartbeat_rpc
-- RPC function for atomic session heartbeat upsert.
-- Used by CF Worker on every MCP tool call.

CREATE OR REPLACE FUNCTION upsert_ai_session(
    p_owner_id text,
    p_tool text,
    p_title text DEFAULT NULL,
    p_summary text DEFAULT NULL,
    p_status text DEFAULT 'active'
) RETURNS uuid AS $$
DECLARE
    v_id uuid;
BEGIN
    -- Try to find existing active session for this owner+tool
    SELECT id INTO v_id FROM ai_sessions
    WHERE owner_id = p_owner_id AND tool = p_tool AND status = 'active'
    LIMIT 1;

    IF v_id IS NOT NULL THEN
        -- Update heartbeat + optional fields
        UPDATE ai_sessions SET
            last_seen = now(),
            title = COALESCE(p_title, title),
            summary = CASE WHEN p_summary IS NOT NULL THEN p_summary ELSE summary END,
            status = p_status,
            ended_at = CASE WHEN p_status = 'done' THEN now() ELSE ended_at END
        WHERE id = v_id;
        RETURN v_id;
    ELSE
        -- Insert new session
        INSERT INTO ai_sessions (owner_id, tool, title, summary, status, last_seen, started_at)
        VALUES (p_owner_id, p_tool, p_title, p_summary, p_status, now(), now())
        RETURNING id INTO v_id;
        RETURN v_id;
    END IF;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
