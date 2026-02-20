-- Enable Supabase Realtime for C1 tables
-- Allows WebSocket subscribers to receive INSERT/UPDATE/DELETE events

ALTER PUBLICATION supabase_realtime ADD TABLE c1_messages;
ALTER PUBLICATION supabase_realtime ADD TABLE c1_channels;
ALTER PUBLICATION supabase_realtime ADD TABLE c1_members;
ALTER PUBLICATION supabase_realtime ADD TABLE c1_channel_summaries;
