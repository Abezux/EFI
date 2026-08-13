import React from 'react';
import { render } from '@testing-library/react';
import { StructuredData } from '../StructuredData';
import { NewsEvent } from '@/lib/api';

describe('StructuredData Component', () => {
  const sampleEvent: NewsEvent = {
    id: 101,
    canonical_title: 'Raw Title',
    ai_headline: 'Commercial Bank of Ethiopia FX Directive',
    slug: 'commercial-bank-of-ethiopia-fx-directive',
    ai_summary: 'Central bank launched new FX policies across major retail banks.',
    ai_summary_generated: true,
    category: {
      id: 1,
      name: 'Banking & Finance',
      slug: 'banking-finance',
    },
    entities: [{ id: 1, name: 'CBE', type: 'ORGANIZATION' }],
    sources: [
      {
        channel_name: 'Tikvah Ethiopia',
        channel_handle: 'tikvahethiopia',
        telegram_message_id: 8881,
        posted_at: '2026-08-11T12:00:00Z',
        excerpt: 'CBE implemented directive today...',
      },
    ],
    source_count: 2,
    first_seen_at: '2026-08-11T12:00:00Z',
    last_updated_at: '2026-08-11T14:00:00Z',
  };

  it('renders application/ld+json script with valid NewsArticle schema', () => {
    const { container } = render(
      <StructuredData
        event={sampleEvent}
        canonicalUrl="https://efi.et/news/banking-finance/commercial-bank-of-ethiopia-fx-directive-101"
      />
    );

    const script = container.querySelector('script[type="application/ld+json"]');
    expect(script).not.toBeNull();

    const parsed = JSON.parse(script?.innerHTML || '{}');
    expect(parsed['@context']).toBe('https://schema.org');
    expect(parsed['@type']).toBe('NewsArticle');
    expect(parsed.headline).toBe('Commercial Bank of Ethiopia FX Directive');
    expect(parsed.description).toBe(
      'Central bank launched new FX policies across major retail banks.'
    );
    expect(parsed.url).toBe(
      'https://efi.et/news/banking-finance/commercial-bank-of-ethiopia-fx-directive-101'
    );
    expect(parsed.author.name).toBe('Ethiopia Financial Insights');
    expect(parsed.publisher.name).toBe('Ethiopia Financial Insights');
    expect(parsed.articleSection).toBe('Banking & Finance');
    expect(parsed.isBasedOn).toHaveLength(1);
    expect(parsed.isBasedOn[0].name).toBe('Tikvah Ethiopia');
    expect(parsed.isBasedOn[0].url).toBe('https://t.me/tikvahethiopia');
  });
});
