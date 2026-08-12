-- Migration: 0007_ai_headline.sql
-- Description: Add nullable ai_headline column to news_events table for full-context AI enrichment.

ALTER TABLE news_events ADD COLUMN ai_headline TEXT;
