-- Migration: 0008_clustering_anti_drift.sql
-- Description: Add founding_raw_post_id to news_events, backfill from earliest event_sources, and add first_seen_at index.

ALTER TABLE news_events ADD COLUMN IF NOT EXISTS founding_raw_post_id BIGINT REFERENCES raw_posts(id);

-- Backfill for existing events: set to the earliest-added_at source in event_sources
UPDATE news_events ne
SET founding_raw_post_id = sub.raw_post_id
FROM (
    SELECT DISTINCT ON (event_id) event_id, raw_post_id
    FROM event_sources
    ORDER BY event_id, added_at ASC
) sub
WHERE ne.id = sub.event_id AND ne.founding_raw_post_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_news_events_first_seen_at ON news_events(first_seen_at);
CREATE INDEX IF NOT EXISTS idx_news_events_founding_raw_post_id ON news_events(founding_raw_post_id);
