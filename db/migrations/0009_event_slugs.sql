-- Migration 0009: Add slug to news_events with partial unique index for V8 SEO
-- Slugs are generated once, immutable thereafter, enabling canonical URLs.

ALTER TABLE news_events ADD COLUMN IF NOT EXISTS slug TEXT;

-- Backfill existing events from ai_headline (or fallback to canonical_title)
UPDATE news_events
SET slug = LOWER(
    REGEXP_REPLACE(
        REGEXP_REPLACE(
            COALESCE(NULLIF(TRIM(ai_headline), ''), canonical_title),
            '[^a-zA-Z0-9\u1200-\u137F]+', '-', 'g'
        ),
        '^-+|-+$', '', 'g'
    )
)
WHERE slug IS NULL;

-- Ensure any empty slugs receive a fallback identifier
UPDATE news_events
SET slug = 'event-' || id
WHERE slug IS NULL OR slug = '';

-- Ensure absolute uniqueness across backfilled events by appending ID if identical slugs exist
WITH numbered AS (
    SELECT id, slug, ROW_NUMBER() OVER (PARTITION BY slug ORDER BY id ASC) as rn
    FROM news_events
    WHERE slug IS NOT NULL
)
UPDATE news_events ne
SET slug = ne.slug || '-' || ne.id
FROM numbered n
WHERE ne.id = n.id AND n.rn > 1;

-- Partial unique index for fast lookups and collision prevention
CREATE UNIQUE INDEX IF NOT EXISTS idx_news_events_slug ON news_events(slug) WHERE slug IS NOT NULL;
