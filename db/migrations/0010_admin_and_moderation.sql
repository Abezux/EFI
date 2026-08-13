-- Migration: 0010_admin_and_moderation.sql
-- Description: Create users, moderation_actions tables, add news_events.is_hidden, and configure efi_admin role.

-- 1. Users table for authentication
CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('admin', 'moderator')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2. Moderation actions audit trail
CREATE TABLE IF NOT EXISTS moderation_actions (
  id BIGSERIAL PRIMARY KEY,
  actor_user_id BIGINT NOT NULL REFERENCES users(id),
  action_type TEXT NOT NULL,   -- 'hide_event' | 'restore_event' | 'detach_source' | 'toggle_channel' | 'add_channel' | 'resolve_needs_review'
  target_type TEXT NOT NULL,   -- 'event' | 'source' | 'channel' | 'raw_post'
  target_id BIGINT NOT NULL,
  reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_moderation_actions_created_at ON moderation_actions(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_moderation_actions_target ON moderation_actions(target_type, target_id);

-- 3. Add is_hidden to news_events
ALTER TABLE news_events ADD COLUMN IF NOT EXISTS is_hidden BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS idx_news_events_is_hidden ON news_events(is_hidden) WHERE is_hidden = true;

-- 4. Create efi_admin role idempotently
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'efi_admin') THEN
    CREATE ROLE efi_admin WITH LOGIN PASSWORD 'efi_admin_pass';
  END IF;
END
$$;

-- Grant connect on current database
DO $$
BEGIN
  EXECUTE format('GRANT CONNECT ON DATABASE %I TO efi_admin', current_database());
END
$$;

-- Grant schema usage
GRANT USAGE ON SCHEMA public TO efi_admin;

-- Grant SELECT, INSERT, UPDATE ONLY on required tables (strictly NO DELETE privilege anywhere)
GRANT SELECT, INSERT, UPDATE ON users, moderation_actions, news_events, channels, event_sources, raw_posts, processing_audit, categories, entities, event_entities TO efi_admin;

-- Grant usage on sequences for inserts
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO efi_admin;
