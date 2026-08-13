import React from 'react';
import { NewsEvent } from '../lib/api';
import { getCanonicalEventUrl } from '../lib/seo';

interface StructuredDataProps {
  event: NewsEvent;
  canonicalUrl?: string;
}

export function StructuredData({ event, canonicalUrl }: StructuredDataProps) {
  const url = canonicalUrl || getCanonicalEventUrl(event);
  const headline = (event.ai_headline && event.ai_headline.trim()) || event.canonical_title;

  const sourcesCitations =
    event.sources?.map((s) => ({
      '@type': 'NewsMediaOrganization',
      name: s.channel_name,
      url: s.channel_handle
        ? `https://t.me/${s.channel_handle.replace(/^@/, '')}`
        : undefined,
    })) || [];

  const entitiesAbout =
    event.entities?.map((ent) => ({
      '@type': ent.type === 'person' ? 'Person' : ent.type === 'place' ? 'Place' : 'Organization',
      name: ent.name,
    })) || [];

  const jsonLd = {
    '@context': 'https://schema.org',
    '@type': 'NewsArticle',
    mainEntityOfPage: {
      '@type': 'WebPage',
      '@id': url,
    },
    headline,
    description: event.ai_summary,
    url,
    datePublished: event.first_seen_at,
    dateModified: event.last_updated_at,
    articleSection: event.category?.name || 'Economy',
    keywords: event.entities?.map((e) => e.name).join(', '),
    author: {
      '@type': 'Organization',
      name: 'Ethiopia Financial Insights',
      url: 'https://efi.et',
    },
    publisher: {
      '@type': 'Organization',
      name: 'Ethiopia Financial Insights',
      url: 'https://efi.et',
    },
    isBasedOn: sourcesCitations.length > 0 ? sourcesCitations : undefined,
    about: entitiesAbout.length > 0 ? entitiesAbout : undefined,
  };

  return (
    <script
      type="application/ld+json"
      dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
    />
  );
}
