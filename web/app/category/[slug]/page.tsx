import React from 'react';
import { notFound } from 'next/navigation';
import type { Metadata } from 'next';
import Link from 'next/link';
import { getEvents, getCategories, Category, NewsEvent } from '@/lib/api';
import { EventCard } from '@/components/EventCard';
import { CategoryNav } from '@/components/CategoryNav';
import { SearchBar } from '@/components/SearchBar';
import { LiveFeedUpdater } from '@/components/LiveFeedUpdater';
import { ArrowLeft, Tag } from 'lucide-react';

import { buildCategoryMetadata, DEFAULT_SITE_URL } from '@/lib/seo';

export const dynamic = 'force-dynamic';
export const revalidate = 0;

interface CategoryPageProps {
  params: {
    slug: string;
  };
}

export async function generateMetadata({
  params,
}: CategoryPageProps): Promise<Metadata> {
  try {
    const categories = await getCategories({ isServer: true });
    const currentCategory = categories.find((c) => c.slug === params.slug);
    if (!currentCategory) {
      return { title: 'Category Not Found' };
    }
    const canonicalUrl = `${DEFAULT_SITE_URL}/category/${currentCategory.slug}`;
    return buildCategoryMetadata(currentCategory, canonicalUrl);
  } catch {
    return { title: 'Category News — Ethiopia Financial Insights' };
  }
}

export default async function CategoryPage({ params }: CategoryPageProps) {
  let categories: Category[] = [];
  try {
    categories = await getCategories({ isServer: true });
  } catch {
    // Continue with empty
  }

  const currentCategory = categories.find((c) => c.slug === params.slug);
  if (categories.length > 0 && !currentCategory) {
    notFound();
  }

  let events: NewsEvent[] = [];
  let total = 0;
  let error: string | null = null;

  try {
    const res = await getEvents({
      category: params.slug,
      limit: 25,
      isServer: true,
    });
    events = res.events || [];
    total = res.total || 0;
  } catch (err: unknown) {
    error =
      err instanceof Error
        ? err.message
        : 'Failed to load news for this category.';
  }

  const latestTimestamp =
    events.length > 0
      ? events[0].last_updated_at || events[0].first_seen_at
      : undefined;

  const displayName = currentCategory?.name || params.slug;

  return (
    <div>
      <div style={{ marginBottom: '1.5rem' }}>
        <Link
          href="/"
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '0.4rem',
            fontSize: '0.875rem',
            color: 'var(--text-muted)',
            marginBottom: '1rem',
          }}
        >
          <ArrowLeft size={16} />
          <span>Back to All News</span>
        </Link>

        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            flexWrap: 'wrap',
            gap: '1rem',
            marginBottom: '1.25rem',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
            <div
              style={{
                width: '2.75rem',
                height: '2.75rem',
                borderRadius: 'var(--radius-md)',
                backgroundColor: 'rgba(59, 130, 246, 0.12)',
                border: '1px solid rgba(59, 130, 246, 0.3)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#60a5fa',
              }}
            >
              <Tag size={20} />
            </div>
            <div>
              <h1
                style={{
                  fontSize: '1.75rem',
                  fontWeight: 800,
                  color: 'var(--text-primary)',
                  letterSpacing: '-0.02em',
                }}
              >
                {displayName}
              </h1>
              <p
                style={{
                  color: 'var(--text-secondary)',
                  fontSize: '0.875rem',
                  marginTop: '0.2rem',
                }}
              >
                {total} verified {total === 1 ? 'event' : 'events'} aggregated in this topic
              </p>
            </div>
          </div>

          <SearchBar />
        </div>

        {categories.length > 0 && (
          <CategoryNav
            categories={categories}
            activeSlug={params.slug}
            totalEvents={total}
          />
        )}
      </div>

      <LiveFeedUpdater
        initialLatestTimestamp={latestTimestamp}
        category={params.slug}
      />

      {error ? (
        <div className="error-banner">
          <h2 className="error-title">Category Feed Unavailable</h2>
          <p className="error-desc">{error}</p>
          <a href={`/category/${params.slug}`} className="btn-retry">
            Retry
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
          <h2
            style={{
              fontSize: '1.25rem',
              fontWeight: 600,
              marginBottom: '0.5rem',
            }}
          >
            No Events in {displayName}
          </h2>
          <p
            style={{
              color: 'var(--text-muted)',
              fontSize: '0.9375rem',
              maxWidth: '400px',
              margin: '0 auto',
            }}
          >
            No active events have been categorized under this topic yet. Check back
            as new reports are ingested.
          </p>
        </div>
      ) : (
        <section aria-label={`${displayName} News Events`}>
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
