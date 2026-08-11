-- Migration: 0003_events_and_clustering.sql
-- Description: Add simhash column to raw_posts and create news_events and event_sources tables for text-similarity clustering.

ALTER TABLE raw_posts ADD COLUMN IF NOT EXISTS simhash BIGINT;

CREATE TABLE IF NOT EXISTS news_events (
  id BIGSERIAL PRIMARY KEY,
  canonical_title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  first_seen_at TIMESTAMPTZ NOT NULL,
  last_updated_at TIMESTAMPTZ NOT NULL,
  source_count INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS event_sources (
  event_id BIGINT NOT NULL REFERENCES news_events(id) ON DELETE CASCADE,
  raw_post_id BIGINT NOT NULL REFERENCES raw_posts(id) ON DELETE RESTRICT,
  similarity_score INTEGER,
  added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (event_id, raw_post_id)
);

CREATE INDEX IF NOT EXISTS idx_raw_posts_processing_status ON raw_posts(processing_status);
CREATE INDEX IF NOT EXISTS idx_news_events_last_updated_at ON news_events(last_updated_at);

-- Explicitly grant permissions to the least-privilege application role
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'efi_app') THEN
    GRANT SELECT, INSERT, UPDATE ON TABLE news_events TO efi_app;
    GRANT SELECT, INSERT, UPDATE ON TABLE event_sources TO efi_app;
    GRANT USAGE, SELECT ON SEQUENCE news_events_id_seq TO efi_app;
  END IF;
END $$;
