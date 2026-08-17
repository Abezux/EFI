import type { Metadata } from 'next';
import { NewsEvent, Category } from './api';

export const SITE_NAME = 'Ethiopia Financial Insights';
export const DEFAULT_DESCRIPTION =
  'Verified, multi-source financial and economic news aggregation for Ethiopia with real-time AI synthesis.';
export const DEFAULT_SITE_URL =
  process.env.NEXT_PUBLIC_SITE_URL || 'https://efi.et';

/**
 * Deterministically creates a clean URL slug from any title or headline.
 * Supports alphanumeric English characters, numbers, and Ethiopic unicode range (\u1200-\u137F).
 */
export function slugify(text: string): string {
  if (!text) return 'event';
  const clean = text
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9\u1200-\u137F]+/g, '-')
    .replace(/^-+|-+$/g, '');

  if (!clean) return 'event';

  if (clean.length > 80) {
    const truncated = clean.substring(0, 80);
    const lastDash = truncated.lastIndexOf('-');
    return (lastDash > 40 ? truncated.substring(0, lastDash) : truncated).replace(
      /-+$/,
      ''
    );
  }

  return clean;
}

/**
 * Extracts the trailing numeric event ID from a slug parameter.
 * Example: "cbe-fx-directive-101" -> 101, "101" -> 101, "invalid-slug" -> null
 */
export function extractIdFromSlug(slugParam: string): number | null {
  if (!slugParam) return null;
  const match = slugParam.match(/-?(\d+)$/);
  if (!match) return null;
  const id = parseInt(match[1], 10);
  return Number.isNaN(id) || id <= 0 ? null : id;
}

/**
 * Returns category slug for canonical routing, defaulting to 'general'.
 */
export function getCategorySlug(event: NewsEvent): string {
  return event.category?.slug?.trim() || 'general';
}

/**
 * Returns the immutable event slug, falling back to slugified headline/title.
 */
export function getEventSlug(event: NewsEvent): string {
  if (event.slug && event.slug.trim() !== '') {
    return event.slug.trim();
  }
  const title = (event.ai_headline && event.ai_headline.trim()) || event.canonical_title;
  return slugify(title);
}

/**
 * Returns the canonical relative path: /news/{category-slug}/{event-slug}-{id}
 */
export function getCanonicalEventPath(event: NewsEvent): string {
  const categorySlug = getCategorySlug(event);
  const eventSlug = getEventSlug(event);
  return `/news/${categorySlug}/${eventSlug}-${event.id}`;
}

/**
 * Returns absolute canonical URL.
 */
export function getCanonicalEventUrl(event: NewsEvent, baseUrl?: string): string {
  const base = (baseUrl || DEFAULT_SITE_URL).replace(/\/$/, '');
  return `${base}${getCanonicalEventPath(event)}`;
}

/**
 * Builds standard Open Graph and Twitter Card metadata for an event.
 */
export function buildEventMetadata(
  event: NewsEvent,
  canonicalUrl: string
): Metadata {
  const headline = (event.ai_headline && event.ai_headline.trim()) || event.canonical_title;
  const title = `${headline} — ${SITE_NAME}`;
  const description =
    event.ai_summary && event.ai_summary.trim() !== ''
      ? event.ai_summary.slice(0, 200).trim() + '...'
      : `Verified financial report aggregated from ${event.source_count} primary sources.`;

  const tags = event.entities?.map((e) => e.name) || [];

  return {
    title,
    description,
    alternates: {
      canonical: canonicalUrl,
    },
    openGraph: {
      type: 'article',
      siteName: SITE_NAME,
      title,
      description,
      url: canonicalUrl,
      publishedTime: event.first_seen_at,
      modifiedTime: event.last_updated_at,
      section: event.category?.name || 'Economy',
      tags,
    },
    twitter: {
      card: 'summary_large_image',
      title,
      description,
    },
  };
}

/**
 * Builds metadata for a category page.
 */
export function buildCategoryMetadata(
  category: Category,
  canonicalUrl: string
): Metadata {
  const title = `${category.name} News & Economic Updates — ${SITE_NAME}`;
  const description = `Live financial news, policy directives, and verified reports covering ${category.name} in Ethiopia.`;

  return {
    title,
    description,
    alternates: {
      canonical: canonicalUrl,
    },
    openGraph: {
      type: 'website',
      siteName: SITE_NAME,
      title,
      description,
      url: canonicalUrl,
    },
    twitter: {
      card: 'summary',
      title,
      description,
    },
  };
}
