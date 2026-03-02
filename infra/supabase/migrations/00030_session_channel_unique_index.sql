-- Migration: 00030_session_channel_unique_index
-- Creates a partial unique index on c1_channels for session-type channels.
-- This ensures EnsureChannel is idempotent: each (tenant_id, platform, name)
-- combination is unique for session channels, allowing ON CONFLICT DO NOTHING.

CREATE UNIQUE INDEX IF NOT EXISTS uniq_c1_channels_session_platform
  ON c1_channels (tenant_id, platform, name)
  WHERE channel_type = 'session';
