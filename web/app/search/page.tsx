import React from 'react';
import type { Metadata } from 'next';
import Link from 'next/link';
import { searchEvents, NewsEvent } from '@/lib/api';
import { EventCard } from '@/components/EventCard';
import { SearchBar } from '@/components/SearchBar';
import { ArrowLeft, Search as SearchIcon } from 'lucide-react';

export const dynamic = 'force-dynamic';
export const revalidate = 0;

interface SearchPageProps {
  searchParams: {
    q?: string;
  };
}

export function generateMetadata({ searchParams }: SearchPageProps): Metadata {
  const query = searchParams.q?.trim();
  return {
    title: query
      ? `Search results for "${query}" — Ethiopia Financial News`
      : 'Search News — Ethiopia Financial Platform',
    description:
      'Search aggregated Ethiopian economic and financial news in English and Amharic.',
  };
}

export default async function SearchPage({ searchParams }: SearchPageProps) {
  const query = searchParams.q?.trim() || '';
  let events: NewsEvent[] = [];
  let total = 0;
  let error: string | null = null;

  if (query) {
    try {
      const res = await searchEvents(query, { limit: 30, isServer: true });
      events = res.events || [];
      total = res.total || 0;
    } catch (err: unknown) {
      error =
        err instanceof Error
          ? err.message
          : 'Failed to complete search query.';
    }
  }

  return (
    <div>
      <div style={{ marginBottom: '2rem' }}>
        <Link
          href="/"
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '0.4rem',
            fontSize: '0.875rem',
            color: 'var(--text-muted)',
            marginBottom: '1.25rem',
          }}
        >
          <ArrowLeft size={16} />
          <span>Back to News Feed</span>
        </Link>

        <h1
          style={{
            fontSize: '1.875rem',
            fontWeight: 800,
            color: 'var(--text-primary)',
            letterSpacing: '-0.02em',
            marginBottom: '1rem',
          }}
        >
          Search News Events
        </h1>

        <div style={{ maxWidth: '700px', marginBottom: '1.5rem' }}>
          <SearchBar initialQuery={query} />
        </div>

        {query && !error && (
          <p style={{ color: 'var(--text-secondary)', fontSize: '0.9375rem' }}>
            Found <strong style={{ color: 'var(--text-primary)' }}>{total}</strong>{' '}
            {total === 1 ? 'event' : 'events'} matching &ldquo;
            <span style={{ color: 'var(--accent-primary)' }}>{query}</span>&rdquo;
          </p>
        )}
      </div>

      {error ? (
        <div className="error-banner">
          <h2 className="error-title">Search Service Error</h2>
          <p className="error-desc">{error}</p>
        </div>
      ) : !query ? (
        <div
          style={{
            textAlign: 'center',
            padding: '4rem 1rem',
            backgroundColor: 'var(--bg-card)',
            borderRadius: 'var(--radius-lg)',
            border: '1px solid var(--border-subtle)',
          }}
        >
          <SearchIcon
            size={36}
            color="#64748b"
            style={{ margin: '0 auto 1rem' }}
          />
          <h2
            style={{
              fontSize: '1.25rem',
              fontWeight: 600,
              marginBottom: '0.5rem',
            }}
          >
            Explore Ethiopian Financial Topics
          </h2>
          <p
            style={{
              color: 'var(--text-muted)',
              fontSize: '0.9375rem',
              maxWidth: '480px',
              margin: '0 auto',
            }}
          >
            Type keywords in English or Amharic (e.g. inflation, CBE, ባንክ, የውጭ
            ምንዛሬ, ነዳጅ) to find verified news events.
          </p>
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
          <h2
            style={{
              fontSize: '1.25rem',
              fontWeight: 600,
              marginBottom: '0.5rem',
            }}
          >
            No Results Found
          </h2>
          <p
            style={{
              color: 'var(--text-muted)',
              fontSize: '0.9375rem',
              maxWidth: '400px',
              margin: '0 auto',
            }}
          >
            No active news events matched &ldquo;{query}&rdquo;. Try using different
            or broader keywords.
          </p>
        </div>
      ) : (
        <section aria-label="Search Results">
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
