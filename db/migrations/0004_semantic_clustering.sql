-- Migration: 0004_semantic_clustering.sql
-- Description: Configure 768-dimensional pgvector embeddings for Gemini text-embedding-004, add embedding_centroid to news_events with HNSW index.

-- Ensure pgvector extension is available
CREATE EXTENSION IF NOT EXISTS vector;

-- Alter raw_posts.embedding to match the chosen 768-dimensional Gemini embedding model
ALTER TABLE raw_posts ALTER COLUMN embedding TYPE vector(768);

-- Add embedding_centroid column to news_events table
ALTER TABLE news_events ADD COLUMN IF NOT EXISTS embedding_centroid vector(768);

-- Create HNSW index for fast approximate nearest neighbor (ANN) cosine similarity search
CREATE INDEX IF NOT EXISTS idx_news_events_embedding_centroid
  ON news_events USING hnsw (embedding_centroid vector_cosine_ops);

-- Grant least-privilege permissions to efi_app
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'efi_app') THEN
    GRANT SELECT, INSERT, UPDATE ON TABLE news_events TO efi_app;
    GRANT SELECT, INSERT, UPDATE ON TABLE raw_posts TO efi_app;
  END IF;
END $$;
