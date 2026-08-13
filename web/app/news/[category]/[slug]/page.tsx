import React from 'react';
import { notFound, permanentRedirect } from 'next/navigation';
import type { Metadata } from 'next';
import Link from 'next/link';
import { getEventById } from '@/lib/api';
import {
  extractIdFromSlug,
  getCategorySlug,
  getEventSlug,
  getCanonicalEventPath,
  getCanonicalEventUrl,
  buildEventMetadata,
} from '@/lib/seo';
import { StructuredData } from '@/components/StructuredData';
import { AiSummaryBox } from '@/components/AiBadge';
import { SourceList } from '@/components/SourceList';
import { formatTimeAgo } from '@/components/EventCard';
import { ArrowLeft, Clock, Layers, ShieldCheck, Tag } from 'lucide-react';

export const dynamic = 'force-dynamic';
export const revalidate = 0;

interface NewsDetailPageProps {
  params: {
    category: string;
    slug: string;
  };
}

export async function generateMetadata({
  params,
}: NewsDetailPageProps): Promise<Metadata> {
  const eventId = extractIdFromSlug(params.slug);
  if (!eventId) {
    return { title: 'Not Found' };
  }

  try {
    const event = await getEventById(eventId, { isServer: true });
    if (!event) {
      return { title: 'Not Found' };
    }
    const canonicalUrl = getCanonicalEventUrl(event);
    return buildEventMetadata(event, canonicalUrl);
  } catch {
    return { title: 'Event Details — Ethiopia Financial Insights' };
  }
}

export default async function NewsDetailPage({ params }: NewsDetailPageProps) {
  const eventId = extractIdFromSlug(params.slug);
  if (!eventId) {
    notFound();
  }

  let event = null;
  let error: string | null = null;

  try {
    event = await getEventById(eventId, { isServer: true });
  } catch (err: unknown) {
    error =
      err instanceof Error
        ? err.message
        : 'Failed to load event details from API.';
  }

  if (error) {
    return (
      <div className="error-banner">
        <h2 className="error-title">Error Loading Event</h2>
        <p className="error-desc">{error}</p>
        <Link href="/" className="btn-retry">
          Back to Feed
        </Link>
      </div>
    );
  }

  if (!event) {
    notFound();
  }

  // Canonical URL Enforcement:
  // If the requested URL category or slug doesn't match the event's canonical path, 301 redirect.
  const expectedCategory = getCategorySlug(event);
  const expectedSlug = `${getEventSlug(event)}-${event.id}`;
  if (params.category !== expectedCategory || params.slug !== expectedSlug) {
    permanentRedirect(getCanonicalEventPath(event));
  }

  const headline =
    (event.ai_headline && event.ai_headline.trim()) || event.canonical_title;
  const canonicalUrl = getCanonicalEventUrl(event);

  return (
    <div className="event-detail-page">
      {/* Schema.org NewsArticle JSON-LD */}
      <StructuredData event={event} canonicalUrl={canonicalUrl} />

      <div className="page-nav">
        <Link href="/" className="back-link">
          <ArrowLeft className="w-4 h-4 mr-1 inline" />
          Back to Live Feed
        </Link>
      </div>

      <article className="event-detail-card">
        {/* Header */}
        <header className="event-detail-header">
          <div className="event-header-meta">
            {event.category && (
              <span className="category-pill">{event.category.name}</span>
            )}
            <span className="meta-time">
              <Clock className="w-3.5 h-3.5 mr-1 inline opacity-70" />
              First reported {formatTimeAgo(event.first_seen_at)}
            </span>
          </div>

          <h1 className="event-detail-title">{headline}</h1>

          <div className="event-stats-row">
            <span className="stat-badge">
              <Layers className="w-4 h-4 mr-1 inline text-primary-light" />
              {event.source_count} {event.source_count === 1 ? 'Source Report' : 'Source Reports'}
            </span>
            <span className="stat-badge">
              <ShieldCheck className="w-4 h-4 mr-1 inline text-emerald-400" />
              Verified Multi-Source Event
            </span>
          </div>
        </header>

        {/* AI-Synthesized Multi-Paragraph Summary */}
        <section className="event-summary-section">
          <AiSummaryBox
            summary={event.ai_summary}
            isAiGenerated={event.ai_summary_generated}
          />
        </section>

        {/* Extracted Entities */}
        {event.entities && event.entities.length > 0 && (
          <section className="event-entities-section">
            <h3 className="section-subtitle">
              <Tag className="w-4 h-4 mr-1.5 inline opacity-70" />
              Mentioned Entities & Topics
            </h3>
            <div className="entities-chip-list">
              {event.entities.map((entity) => (
                <span
                  key={entity.id}
                  className={`entity-chip entity-chip-${entity.type.toLowerCase()}`}
                >
                  <span className="entity-type-tag">{entity.type}</span>
                  <span className="entity-name">{entity.name}</span>
                </span>
              ))}
            </div>
          </section>
        )}

        {/* Source Channel Reports */}
        <section className="event-sources-section">
          <h3 className="section-subtitle">
            <Layers className="w-4 h-4 mr-1.5 inline opacity-70" />
            Aggregated Source Reports ({event.sources?.length || 0})
          </h3>
          <p className="sources-disclaimer">
            Individual source excerpts are strictly bounded (under 160 characters)
            with primary channel attribution. Click below to view the original Telegram post.
          </p>
          <SourceList sources={event.sources || []} />
        </section>
      </article>
    </div>
  );
}
