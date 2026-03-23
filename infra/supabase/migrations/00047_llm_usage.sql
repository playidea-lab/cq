-- Migration: 00047_llm_usage
-- Per-user monthly LLM proxy usage counter for freemium rate limiting.

CREATE TABLE IF NOT EXISTS llm_usage (
    user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
    month   TEXT NOT NULL,  -- format: '2026-03'
    count   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, month)
);

-- RLS: Edge Function uses service_role key, so no user-facing RLS needed.
-- But enable RLS to prevent anonymous access via anon key.
ALTER TABLE llm_usage ENABLE ROW LEVEL SECURITY;

-- Service role has full access (Edge Function uses SUPABASE_SERVICE_ROLE_KEY).
-- No user-facing policies — this table is only accessed by the llm-proxy Edge Function.

-- Atomic upsert: insert or increment count. Returns new count.
CREATE OR REPLACE FUNCTION increment_llm_usage(p_user_id UUID, p_month TEXT)
RETURNS INTEGER
LANGUAGE sql
SECURITY DEFINER
AS $$
  INSERT INTO llm_usage (user_id, month, count)
  VALUES (p_user_id, p_month, 1)
  ON CONFLICT (user_id, month)
  DO UPDATE SET count = llm_usage.count + 1
  RETURNING count;
$$;
