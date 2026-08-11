-- Migration 0005: AI Enrichment & Audit Infrastructure
-- Adds processing_audit for AI decision traceability, categories, entities, and news_events enrichment fields.

-- 1. AI Processing Audit Trail
CREATE TABLE processing_audit (
    id BIGSERIAL PRIMARY KEY,
    raw_post_id BIGINT REFERENCES raw_posts(id) ON DELETE SET NULL,
    news_event_id BIGINT REFERENCES news_events(id) ON DELETE SET NULL,
    stage TEXT NOT NULL,              -- 'verification' | 'enrichment'
    decision TEXT,                    -- e.g. 'same_event' | 'different_event' | 'summary_generated'
    confidence REAL,
    model_used TEXT NOT NULL,         -- e.g. 'gemini-flash-latest'
    raw_response TEXT,                -- raw output for auditability and verification
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2. Fixed Category Taxonomy (Finance-focused + General & Politics catch-all)
CREATE TABLE categories (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE
);

INSERT INTO categories (name, slug) VALUES
    ('Banking & Finance', 'banking-finance'),
    ('Macroeconomy & Policy', 'macroeconomy-policy'),
    ('Forex & Trade', 'forex-trade'),
    ('Capital Markets', 'capital-markets'),
    ('Commodities & Agriculture', 'commodities-agriculture'),
    ('Corporate & Tech', 'corporate-tech'),
    ('Energy & Infrastructure', 'energy-infrastructure'),
    ('General Economy', 'general-economy'),
    ('Politics & Security', 'politics-security');

-- 3. Enrichment fields on news_events
ALTER TABLE news_events
    ADD COLUMN ai_summary TEXT,
    ADD COLUMN category_id INTEGER REFERENCES categories(id),
    ADD COLUMN last_enriched_at TIMESTAMPTZ;

-- 4. Named Entities & Event Entity Associations
CREATE TABLE entities (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,               -- 'person' | 'place' | 'organization'
    UNIQUE (name, type)
);

CREATE TABLE event_entities (
    event_id BIGINT NOT NULL REFERENCES news_events(id) ON DELETE CASCADE,
    entity_id BIGINT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    PRIMARY KEY (event_id, entity_id)
);

-- 5. Audit & Query Indexes
CREATE INDEX idx_processing_audit_raw_post_id ON processing_audit(raw_post_id);
CREATE INDEX idx_processing_audit_news_event_id ON processing_audit(news_event_id);
CREATE INDEX idx_news_events_category_id ON news_events(category_id);
CREATE INDEX idx_event_entities_entity_id ON event_entities(entity_id);
