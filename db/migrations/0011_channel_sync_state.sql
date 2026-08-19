-- Migration: 0011_channel_sync_state.sql
-- Description: Add last_synced_message_id to channels for bounded backfill and gap recovery (V9.2).

ALTER TABLE channels ADD COLUMN IF NOT EXISTS last_synced_message_id BIGINT;

-- Comment: Nullable. Existing channels start with NULL.
-- The first live-ingested or backfilled message sets it going forward.
COMMENT ON COLUMN channels.last_synced_message_id IS 'Highest Telegram message ID successfully ingested (via live listening or backfill)';
