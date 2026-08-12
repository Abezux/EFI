import React from 'react';
import { getEvents, getCategories, NewsEvent, Category } from '@/lib/api';
import { EventCard } from '@/components/EventCard';
import { CategoryNav } from '@/components/CategoryNav';
import { SearchBar } from '@/components/SearchBar';
import { LiveFeedUpdater } from '@/components/LiveFeedUpdater';
import { AlertTriangle, Radio } from 'lucide-react';

// Force dynamic server rendering for fresh news data
export const dynamic = 'force-dynamic';
export const revalidate = 0;

export default async function HomePage() {
  let events: NewsEvent[] = [];
  let total = 0;
  let categories: Category[] = [];
  let error: string | null = null;

  try {
    const [eventsRes, catsRes] = await Promise.all([
      getEvents({ limit: 25, isServer: true }),
      getCategories({ isServer: true }),
    ]);
    events = eventsRes.events || [];
    total = eventsRes.total || 0;
    categories = catsRes || [];
  } catch (err: unknown) {
    error =
      err instanceof Error
        ? err.message
        : 'Failed to connect to the news aggregation service.';
  }

  const latestTimestamp =
    events.length > 0
      ? events[0].last_updated_at || events[0].first_seen_at
      : undefined;

  return (
    <div>
      <div style={{ marginBottom: '2rem', display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '1rem' }}>
          <div>
            <h1 style={{ fontSize: '1.875rem', fontWeight: 800, color: 'var(--text-primary)', letterSpacing: '-0.02em' }}>
              Ethiopia Financial News
            </h1>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.9375rem', marginTop: '0.25rem' }}>
              Aggregated, verified, and AI-summarized in real time from monitored channels.
            </p>
          </div>
          <SearchBar />
        </div>

        {categories.length > 0 && (
          <CategoryNav categories={categories} totalEvents={total} />
        )}
      </div>

      <LiveFeedUpdater initialLatestTimestamp={latestTimestamp} />

      {error ? (
        <div className="error-banner">
          <div style={{ display: 'flex', justifyContent: 'center', marginBottom: '0.75rem' }}>
            <AlertTriangle size={32} color="#ef4444" />
          </div>
          <h2 className="error-title">News Feed Temporarily Unavailable</h2>
          <p className="error-desc">{error}</p>
          <a href="/" className="btn-retry">
            Reload Feed
          </a>
        </div>
      ) : events.length === 0 ? (
        <div
          style={{
            textAlign: 'center',
            padding: '4rem 1rem',
            backgroundColor: 'var(--bg-card)',
            borderRadius: 'var(--radius-lg)',
            border: '1px solid var(--border-subtle)',
          }}
        >
          <Radio size={36} color="#64748b" style={{ margin: '0 auto 1rem' }} />
          <h2 style={{ fontSize: '1.25rem', fontWeight: 600, marginBottom: '0.5rem' }}>
            No Active Events Yet
          </h2>
          <p style={{ color: 'var(--text-muted)', fontSize: '0.9375rem', maxWidth: '400px', margin: '0 auto' }}>
            The ingestion listener is monitoring channels. Once events are clustered and enriched, they will appear here.
          </p>
        </div>
      ) : (
        <section aria-label="Latest News Events">
          <div className="feed-grid">
            {events.map((event) => (
              <EventCard key={event.id} event={event} />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
