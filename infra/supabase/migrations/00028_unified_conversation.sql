-- Migration: 00028_unified_conversation
-- Unifies conversation storage under c1_* tables:
--   1. Make project_id nullable (bot/event channels have no c4_projects entry)
--   2. Add tenant_id, platform, platform_id columns
--   3. Extend channel_type CHECK to include 'bot' and 'event'
--   4. Partial unique indexes for bot/event channels and platform members
--   5. Migrate conversations → c1_messages, create backward-compat view
--   6. Knowledge ingestion trigger (bot-channel assistant replies → c4_documents)

-- ============================================================
-- 1. Make project_id nullable in c1_channels, c1_messages, c1_members
--    Bot and event channels are not owned by a c4_projects entry.
-- ============================================================
ALTER TABLE c1_channels ALTER COLUMN project_id DROP NOT NULL;
ALTER TABLE c1_messages ALTER COLUMN project_id DROP NOT NULL;
ALTER TABLE c1_members  ALTER COLUMN project_id DROP NOT NULL;

-- ============================================================
-- 2. tenant_id — single-tenant for now; value defaults to 'default'
-- ============================================================
ALTER TABLE c1_channels ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE c1_members  ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';

-- ============================================================
-- 3. platform columns
--    c1_channels.platform : '' (native), 'dooray', 'discord', 'slack'
--    c1_members.platform  : '' (native), 'dooray', 'session'
--    c1_members.platform_id : external identifier (e.g. Dooray user ID)
-- ============================================================
ALTER TABLE c1_channels ADD COLUMN IF NOT EXISTS platform    TEXT NOT NULL DEFAULT '';
ALTER TABLE c1_members  ADD COLUMN IF NOT EXISTS platform    TEXT NOT NULL DEFAULT '';
ALTER TABLE c1_members  ADD COLUMN IF NOT EXISTS platform_id TEXT NOT NULL DEFAULT '';

-- ============================================================
-- 4. Extend channel_type CHECK constraint to include 'bot' and 'event'
-- ============================================================
ALTER TABLE c1_channels DROP CONSTRAINT IF EXISTS chk_channel_type;
ALTER TABLE c1_channels ADD CONSTRAINT chk_channel_type
  CHECK (channel_type IN ('general', 'project', 'knowledge', 'session', 'dm', 'bot', 'event'));

-- ============================================================
-- 5. Partial unique index: bot/event channels keyed by (tenant, platform, name)
--    The existing UNIQUE (project_id, name) allows NULL+NULL duplicates, so we
--    need an explicit index to enforce uniqueness for bot/event channels.
-- ============================================================
CREATE UNIQUE INDEX IF NOT EXISTS uniq_c1_channels_bot
  ON c1_channels(tenant_id, platform, name)
  WHERE channel_type IN ('bot', 'event');

-- Partial unique index: platform members keyed by (tenant, platform, platform_id)
CREATE UNIQUE INDEX IF NOT EXISTS uniq_c1_members_platform
  ON c1_members(tenant_id, platform, platform_id)
  WHERE platform != '';

-- ============================================================
-- 6. General indexes
-- ============================================================
-- Covering index for the conv_to_knowledge trigger subquery:
--   SELECT channel_type FROM c1_channels WHERE id = NEW.channel_id
-- Makes the trigger an index-only scan instead of a heap fetch.
CREATE INDEX IF NOT EXISTS idx_c1_channels_id_type
  ON c1_channels(id, channel_type);

CREATE INDEX IF NOT EXISTS idx_c1_channels_tenant
  ON c1_channels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_c1_channels_platform
  ON c1_channels(tenant_id, platform, channel_type);
CREATE INDEX IF NOT EXISTS idx_c1_members_tenant
  ON c1_members(tenant_id);

-- ============================================================
-- 7. Migrate conversations → c1_channels + c1_messages
--    Creates one bot channel per distinct conversations.channel_id and
--    moves all messages into c1_messages.
-- ============================================================
DO $$
DECLARE
  ch_id   UUID;
  grp     RECORD;
  plat    TEXT;
  ch_name TEXT;
BEGIN
  -- Skip if conversations table doesn't exist (idempotent re-run safety).
  IF NOT EXISTS (SELECT 1 FROM information_schema.tables
                 WHERE table_name = 'conversations'
                   AND table_schema = 'public') THEN
    RETURN;
  END IF;

  FOR grp IN (
    SELECT
      channel_id,
      COALESCE(NULLIF(MAX(platform), ''), 'dooray') AS platform
    FROM conversations
    GROUP BY channel_id
  ) LOOP
    plat    := grp.platform;
    ch_name := plat || '-' || grp.channel_id;

    -- Upsert bot channel (ignore conflict = already migrated).
    -- Explicit partial-index conflict target: uniq_c1_channels_bot (tenant_id, platform, name)
    -- WHERE channel_type IN ('bot','event'). Using ON CONFLICT without target is valid
    -- PostgreSQL syntax for DO NOTHING, but the explicit target is more self-documenting.
    INSERT INTO c1_channels (project_id, name, channel_type, platform, tenant_id)
    VALUES (NULL, ch_name, 'bot', plat, 'default')
    ON CONFLICT (tenant_id, platform, name) WHERE channel_type IN ('bot', 'event') DO NOTHING;

    SELECT id INTO ch_id
    FROM c1_channels
    WHERE tenant_id    = 'default'
      AND platform     = plat
      AND name         = ch_name
      AND channel_type IN ('bot', 'event')
    LIMIT 1;

    CONTINUE WHEN ch_id IS NULL;

    -- Migrate messages (simple INSERT; re-running will create duplicates — acceptable
    -- since this block only runs once when conversations table still exists).
    INSERT INTO c1_messages (channel_id, project_id, sender_name, sender_type, content, created_at)
    SELECT
      ch_id,
      NULL,
      CASE WHEN c.role = 'user' THEN 'dooray-user' ELSE 'c5-assistant' END,
      CASE WHEN c.role = 'user' THEN 'user'        ELSE 'system'       END,
      c.content,
      c.created_at
    FROM conversations c
    WHERE c.channel_id = grp.channel_id;

  END LOOP;
END $$;

-- ============================================================
-- 8. Backward-compatible conversations view
--    Rename the original table; create a read-only view that maps
--    c1_messages + c1_channels back to the original conversations shape.
-- ============================================================
ALTER TABLE IF EXISTS conversations RENAME TO conversations_legacy;

CREATE OR REPLACE VIEW conversations AS
  SELECT
    m.id                                                                       AS id,
    -- Recover original channelID by stripping the 'platform-' prefix from ch.name.
    CASE
      WHEN ch.platform != ''
      THEN SUBSTRING(ch.name FROM LENGTH(ch.platform) + 2)
      ELSE ch.name
    END                                                                        AS channel_id,
    ch.platform                                                                AS platform,
    COALESCE(m.project_id::text, '')                                           AS project_id,
    CASE WHEN m.sender_type = 'user' THEN 'user' ELSE 'assistant' END         AS role,
    m.content,
    m.created_at
  FROM c1_messages m
  JOIN c1_channels ch ON m.channel_id = ch.id
  WHERE ch.channel_type = 'bot';

-- ============================================================
-- 9. Knowledge ingestion trigger
--    When an assistant reply is inserted into a bot channel,
--    automatically record it in c4_documents for C9 search.
--    Only fires when project_id is known (non-NULL).
-- ============================================================
CREATE OR REPLACE FUNCTION conv_to_knowledge() RETURNS TRIGGER AS $$
BEGIN
  IF NEW.sender_type != 'user'
     AND NEW.project_id IS NOT NULL
     AND (SELECT channel_type FROM c1_channels WHERE id = NEW.channel_id) = 'bot'
  THEN
    INSERT INTO c4_documents (doc_id, project_id, doc_type, title, domain, body)
    VALUES (
      gen_random_uuid()::text,
      NEW.project_id,
      'insight',
      '대화: ' || NEW.channel_id || ' ' || (NEW.created_at AT TIME ZONE 'UTC')::date,
      'conversation',
      NEW.content
    );
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_conv_knowledge ON c1_messages;
CREATE TRIGGER trg_conv_knowledge
  AFTER INSERT ON c1_messages
  FOR EACH ROW EXECUTE FUNCTION conv_to_knowledge();
