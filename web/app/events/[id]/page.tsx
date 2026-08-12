import React from 'react';
import { notFound } from 'next/navigation';
import type { Metadata } from 'next';
import Link from 'next/link';
import { getEventById } from '@/lib/api';
import { AiSummaryBox } from '@/components/AiBadge';
import { SourceList } from '@/components/SourceList';
import { formatTimeAgo } from '@/components/EventCard';
import { ArrowLeft, Clock, Layers, ShieldCheck, Tag } from 'lucide-react';

export const dynamic = 'force-dynamic';
export const revalidate = 0;

interface EventDetailPageProps {
  params: {
    id: string;
  };
}

export async function generateMetadata({
  params,
}: EventDetailPageProps): Promise<Metadata> {
  try {
    const event = await getEventById(params.id, { isServer: true });
    if (!event) {
      return { title: 'Event Not Found' };
    }
    const headline = (event.ai_headline && event.ai_headline.trim()) || event.canonical_title;
    return {
      title: `${headline} — Ethiopia Financial News`,
      description:
        event.ai_summary ||
        `Verified financial event aggregated from ${event.source_count} source reports.`,
    };
  } catch {
    return { title: 'Event Details' };
  }
}

export default async function EventDetailPage({ params }: EventDetailPageProps) {
  let event = null;
  let error: string | null = null;

  try {
    event = await getEventById(params.id, { isServer: true });
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

  const timeAgo = formatTimeAgo(event.last_updated_at || event.first_seen_at);
  const headline = (event.ai_headline && event.ai_headline.trim()) || event.canonical_title;

  return (
    <article style={{ maxWidth: '840px', margin: '0 auto' }}>
      <Link
        href="/"
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '0.4rem',
          fontSize: '0.875rem',
          color: 'var(--text-muted)',
          marginBottom: '1.5rem',
          transition: 'color 0.15s ease',
        }}
      >
        <ArrowLeft size={16} />
        <span>Back to News Feed</span>
      </Link>

      <header style={{ marginBottom: '1.75rem' }}>
        <div
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'center',
            gap: '0.6rem',
            marginBottom: '1rem',
          }}
        >
          {event.category && (
            <Link
              href={`/category/${event.category.slug}`}
              className="badge badge-category"
            >
              <Tag size={12} />
              <span>{event.category.name}</span>
            </Link>
          )}

          <span className="badge badge-sources">
            <Layers size={12} />
            <span>
              {event.source_count} {event.source_count === 1 ? 'source' : 'sources'}
            </span>
          </span>

          <span
            className="badge"
            style={{
              backgroundColor: 'rgba(59, 130, 246, 0.1)',
              color: '#93c5fd',
              border: '1px solid rgba(59, 130, 246, 0.2)',
            }}
          >
            <ShieldCheck size={12} />
            <span>Verified Event #{event.id}</span>
          </span>
        </div>

        <h1
          style={{
            fontSize: '2.125rem',
            fontWeight: 800,
            lineHeight: 1.3,
            color: 'var(--text-primary)',
            letterSpacing: '-0.02em',
            marginBottom: '1rem',
          }}
        >
          {headline}
        </h1>

        <div
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'center',
            gap: '1.25rem',
            fontSize: '0.875rem',
            color: 'var(--text-muted)',
            paddingBottom: '1.25rem',
            borderBottom: '1px solid var(--border-subtle)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.4rem' }}>
            <Clock size={14} />
            <span>Updated {timeAgo}</span>
          </div>
          <span>•</span>
          <div>
            First reported{' '}
            <time dateTime={event.first_seen_at}>
              {new Date(event.first_seen_at).toLocaleTimeString(undefined, {
                hour: '2-digit',
                minute: '2-digit',
                month: 'short',
                day: 'numeric',
              })}
            </time>
          </div>
        </div>
      </header>

      {/* AI Summary Box with mandatory transparency badge */}
      <AiSummaryBox
        summary={event.ai_summary}
        isAiGenerated={event.ai_summary_generated}
      />

      {/* Extracted Named Entities */}
      {event.entities && event.entities.length > 0 && (
        <section
          style={{
            margin: '2rem 0',
            padding: '1.25rem',
            backgroundColor: 'var(--bg-card)',
            borderRadius: 'var(--radius-md)',
            border: '1px solid var(--border-subtle)',
          }}
        >
          <h2
            style={{
              fontSize: '0.9375rem',
              fontWeight: 600,
              color: 'var(--text-secondary)',
              marginBottom: '0.75rem',
              textTransform: 'uppercase',
              letterSpacing: '0.05em',
            }}
          >
            Identified Entities & Organizations
          </h2>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.5rem' }}>
            {event.entities.map((entity) => (
              <span key={entity.id} className="badge badge-entity">
                <span style={{ fontWeight: 600 }}>{entity.name}</span>
                <span
                  style={{
                    fontSize: '0.7rem',
                    opacity: 0.6,
                    marginLeft: '0.35rem',
                  }}
                >
                  ({entity.type})
                </span>
              </span>
            ))}
          </div>
        </section>
      )}

      {/* Primary Sources with bounded excerpts */}
      <SourceList sources={event.sources} />
    </article>
  );
}
