-- Migration: 00049_telegram_notify_webhook
-- Registers DB webhooks that call the telegram-notify Edge Function
-- when c4_tasks or hub_jobs reach terminal states.
--
-- Supabase provides two webhook mechanisms:
--   1. Dashboard → Database → Webhooks (recommended, no migration needed)
--   2. supabase_functions.hooks table (used below for code-managed setup)
--
-- This migration uses approach #2 for reproducibility.
-- The Edge Function URL is: https://<project-ref>.supabase.co/functions/v1/telegram-notify
--
-- MANUAL STEP REQUIRED:
--   Set Edge Function secrets via Supabase Dashboard or CLI:
--     supabase secrets set TELEGRAM_BOT_TOKEN=<bot-token>
--     supabase secrets set TELEGRAM_CHAT_ID=<chat-id>

-- Ensure the c4_tasks webhook exists (idempotent)
INSERT INTO supabase_functions.hooks (hook_table_id, hook_name, hook_type, hook_function_id, request_id)
SELECT
  c.oid,
  'telegram_notify_tasks',
  'http_request',
  NULL,
  NULL
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relname = 'c4_tasks'
ON CONFLICT DO NOTHING;

-- NOTE: The above is a placeholder showing intent.
-- Supabase DB webhooks are best configured via Dashboard:
--
--   1. Go to Database → Webhooks → Create new webhook
--   2. Name: telegram-notify-tasks
--      Table: c4_tasks
--      Events: UPDATE
--      Type: Supabase Edge Function
--      Function: telegram-notify
--
--   3. Name: telegram-notify-jobs
--      Table: hub_jobs
--      Events: UPDATE
--      Type: Supabase Edge Function
--      Function: telegram-notify
--
-- The Edge Function handles filtering (only terminal states trigger messages).
