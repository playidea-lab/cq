-- Migration: 00052_telegram_notify_triggers
-- Creates pg_net-based triggers that call telegram-notify Edge Function
-- when c4_tasks, hub_jobs, or hub_workers reach terminal/notable states.

-- Ensure pg_net is available
CREATE EXTENSION IF NOT EXISTS pg_net WITH SCHEMA extensions;

-- Generic trigger function: POST old+new record as JSON to telegram-notify
CREATE OR REPLACE FUNCTION notify_telegram_on_update()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
  edge_url TEXT;
  payload  JSONB;
BEGIN
  -- NOTE: Update this URL if the Supabase project ref changes.
  edge_url := 'https://fhuomvsswxiwbfqjsgit.supabase.co/functions/v1/telegram-notify';

  payload := jsonb_build_object(
    'type',       TG_OP,
    'table',      TG_TABLE_NAME,
    'schema',     TG_TABLE_SCHEMA,
    'record',     to_jsonb(NEW),
    'old_record', to_jsonb(OLD)
  );

  PERFORM net.http_post(
    url     := edge_url,
    body    := payload,
    headers := jsonb_build_object(
      'Content-Type', 'application/json'
    )
  );

  RETURN NEW;
END;
$$;

-- Trigger on c4_tasks UPDATE
DROP TRIGGER IF EXISTS trg_telegram_notify_tasks ON c4_tasks;
CREATE TRIGGER trg_telegram_notify_tasks
  AFTER UPDATE ON c4_tasks
  FOR EACH ROW
  WHEN (NEW.status IN ('done', 'blocked') AND OLD.status IS DISTINCT FROM NEW.status)
  EXECUTE FUNCTION notify_telegram_on_update();

-- Trigger on hub_jobs UPDATE
DROP TRIGGER IF EXISTS trg_telegram_notify_jobs ON hub_jobs;
CREATE TRIGGER trg_telegram_notify_jobs
  AFTER UPDATE ON hub_jobs
  FOR EACH ROW
  WHEN (NEW.status IN ('COMPLETE', 'FAILED', 'CANCELLED') AND OLD.status IS DISTINCT FROM NEW.status)
  EXECUTE FUNCTION notify_telegram_on_update();

-- Trigger on hub_workers UPDATE
DROP TRIGGER IF EXISTS trg_telegram_notify_workers ON hub_workers;
CREATE TRIGGER trg_telegram_notify_workers
  AFTER UPDATE ON hub_workers
  FOR EACH ROW
  WHEN (NEW.status = 'offline' AND OLD.status = 'online')
  EXECUTE FUNCTION notify_telegram_on_update();
