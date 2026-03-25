-- Migration: 00050_worker_offline_cron
-- Marks stale hub_workers as offline when heartbeat exceeds 5 minutes.
-- This triggers the telegram-notify webhook (online→offline transition).
--
-- Requires: pg_cron extension (enabled by default on Supabase)
-- Also adds the hub_workers webhook for telegram-notify.

-- 1. Create function to mark stale workers as offline
CREATE OR REPLACE FUNCTION mark_stale_workers_offline()
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
  UPDATE hub_workers
  SET status = 'offline'
  WHERE status = 'online'
    AND last_heartbeat < now() - interval '5 minutes';
END;
$$;

-- 2. Schedule: run every minute
-- pg_cron is available on Supabase by default.
-- If this fails, enable via: CREATE EXTENSION IF NOT EXISTS pg_cron;
SELECT cron.schedule(
  'mark-stale-workers-offline',
  '* * * * *',
  $$SELECT mark_stale_workers_offline()$$
);

-- MANUAL STEP: Add hub_workers webhook in Supabase Dashboard
--
--   1. Go to Database → Webhooks → Create new webhook
--   2. Name: telegram-notify-workers
--      Table: hub_workers
--      Events: UPDATE
--      Type: Supabase Edge Function
--      Function: telegram-notify
--
-- This completes the pipeline:
--   heartbeat miss → pg_cron marks offline → webhook fires → telegram alert
