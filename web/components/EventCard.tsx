import React from 'react';
import Link from 'next/link';
import { NewsEvent } from '@/lib/api';
import { getCanonicalEventPath } from '@/lib/seo';
import { AiBadge } from './AiBadge';
import { Clock, Layers } from 'lucide-react';

interface EventCardProps {
  event: NewsEvent;
}

export function formatTimeAgo(dateString: string): string {
  try {
    const date = new Date(dateString);
    if (isNaN(date.getTime())) return dateString;

    const now = new Date();
    const diffSec = Math.floor((now.getTime() - date.getTime()) / 1000);

    if (diffSec < 60) return 'just now';
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}m ago`;
    const diffHours = Math.floor(diffMin / 60);
    if (diffHours < 24) return `${diffHours}h ago`;
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 7) return `${diffDays}d ago`;

    return date.toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
      year: date.getFullYear() !== now.getFullYear() ? 'numeric' : undefined,
    });
  } catch {
    return dateString;
  }
}

export function EventCard({ event }: EventCardProps) {
  const timeAgo = formatTimeAgo(event.last_updated_at || event.first_seen_at);
  const headline = (event.ai_headline && event.ai_headline.trim()) || event.canonical_title;
  const canonicalPath = getCanonicalEventPath(event);

  return (
    <article className="event-card" data-testid={`event-card-${event.id}`}>
      <div className="event-card-header">
        <div className="event-card-badges">
          {event.category && (
            <Link
              href={`/category/${event.category.slug}`}
              className="badge badge-category"
            >
              {event.category.name}
            </Link>
          )}
          <span className="badge badge-sources">
            <Layers size={12} />
            <span>
              {event.source_count} {event.source_count === 1 ? 'source' : 'sources'}
            </span>
          </span>
          {event.ai_summary_generated && <AiBadge />}
        </div>

        <div className="event-card-time" title={event.last_updated_at}>
          <Clock size={13} />
          <time dateTime={event.last_updated_at}>{timeAgo}</time>
        </div>
      </div>

      <h2 className="event-card-title">
        <Link href={canonicalPath} className="event-card-title-link">
          {headline}
        </Link>
      </h2>

      {event.ai_summary && (
        <p className="event-card-summary-preview">{event.ai_summary}</p>
      )}

      <div className="event-card-footer">
        <div className="event-card-entities">
          {event.entities && event.entities.length > 0 ? (
            event.entities.slice(0, 4).map((ent) => (
              <span key={ent.id} className="badge badge-entity">
                {ent.name}
              </span>
            ))
          ) : (
            <span style={{ fontSize: '0.8125rem', color: 'var(--text-muted)' }}>
              Verified Event #{event.id}
            </span>
          )}
        </div>

        <Link
          href={canonicalPath}
          style={{
            fontSize: '0.875rem',
            fontWeight: 600,
            color: 'var(--accent-primary)',
          }}
        >
          View Details →
        </Link>
      </div>
    </article>
  );
}
