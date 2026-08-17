import {
  slugify,
  extractIdFromSlug,
  getCategorySlug,
  getEventSlug,
  getCanonicalEventPath,
  getCanonicalEventUrl,
  buildEventMetadata,
  buildCategoryMetadata,
  DEFAULT_SITE_URL,
} from '../seo';
import { NewsEvent, Category } from '../api';

describe('SEO Helper Functions', () => {
  describe('slugify', () => {
    it('handles standard English text', () => {
      expect(slugify('Commercial Bank of Ethiopia FX Directive')).toBe(
        'commercial-bank-of-ethiopia-fx-directive'
      );
    });

    it('handles punctuation and special characters', () => {
      expect(slugify('New $250 Million Bond Fund Launched! (2026 Update)')).toBe(
        'new-250-million-bond-fund-launched-2026-update'
      );
    });

    it('preserves Ethiopic characters', () => {
      expect(slugify('የአፍሪካ ሀገራትን የብድር ጫና ለመቀነስ')).toBe(
        'የአፍሪካ-ሀገራትን-የብድር-ጫና-ለመቀነስ'
      );
    });

    it('handles empty and whitespace-only strings gracefully', () => {
      expect(slugify('')).toBe('event');
      expect(slugify('   ')).toBe('event');
      expect(slugify('---')).toBe('event');
    });

    it('bounds slug length cleanly without trailing dashes', () => {
      const longTitle = 'A'.repeat(120);
      const slug = slugify(longTitle);
      expect(slug.length).toBeLessThanOrEqual(80);
      expect(slug.endsWith('-')).toBe(false);
    });
  });

  describe('extractIdFromSlug', () => {
    it('extracts trailing ID from standard slug', () => {
      expect(extractIdFromSlug('cbe-fx-directive-101')).toBe(101);
      expect(extractIdFromSlug('event-title-42')).toBe(42);
    });

    it('extracts ID from standalone number', () => {
      expect(extractIdFromSlug('101')).toBe(101);
    });

    it('returns null for invalid or missing numbers', () => {
      expect(extractIdFromSlug('invalid-slug-without-number')).toBeNull();
      expect(extractIdFromSlug('')).toBeNull();
      expect(extractIdFromSlug('-0')).toBeNull();
    });
  });

  describe('getCanonicalEventPath & getCanonicalEventUrl', () => {
    const sampleEvent: NewsEvent = {
      id: 42,
      canonical_title: 'Raw Title',
      ai_headline: 'NBE Issues 25th Foreign Exchange Auction Results',
      slug: 'nbe-issues-25th-foreign-exchange-auction-results',
      ai_summary: 'Central bank published auction results.',
      ai_summary_generated: true,
      category: {
        id: 1,
        name: 'Banking & Finance',
        slug: 'banking-finance',
      },
      source_count: 3,
      first_seen_at: '2026-08-11T12:00:00Z',
      last_updated_at: '2026-08-11T14:00:00Z',
    };

    it('builds relative canonical path with category and immutable slug', () => {
      const path = getCanonicalEventPath(sampleEvent);
      expect(path).toBe(
        '/news/banking-finance/nbe-issues-25th-foreign-exchange-auction-results-42'
      );
    });

    it('falls back to general category when category is missing', () => {
      const eventNoCategory = { ...sampleEvent, category: null };
      const path = getCanonicalEventPath(eventNoCategory);
      expect(path).toBe(
        '/news/general/nbe-issues-25th-foreign-exchange-auction-results-42'
      );
    });

    it('builds absolute canonical URL', () => {
      const url = getCanonicalEventUrl(sampleEvent);
      expect(url).toBe(
        `${DEFAULT_SITE_URL}/news/banking-finance/nbe-issues-25th-foreign-exchange-auction-results-42`
      );
    });
  });

  describe('buildEventMetadata & buildCategoryMetadata', () => {
    const sampleEvent: NewsEvent = {
      id: 55,
      canonical_title: 'Original Title',
      ai_headline: 'Coffee Export Revenue Surges 30 Percent',
      slug: 'coffee-export-revenue-surges-30-percent',
      ai_summary: 'Ethiopian coffee export revenues reached record highs.',
      ai_summary_generated: true,
      category: { id: 3, name: 'Trade & Export', slug: 'trade-export' },
      entities: [{ id: 1, name: 'ECTA', type: 'ORGANIZATION' }],
      source_count: 2,
      first_seen_at: '2026-08-12T08:00:00Z',
      last_updated_at: '2026-08-12T10:00:00Z',
    };

    it('constructs OpenGraph and Twitter card metadata for event', () => {
      const canonicalUrl = 'https://efi.et/news/trade-export/coffee-export-revenue-surges-30-percent-55';
      const metadata = buildEventMetadata(sampleEvent, canonicalUrl);

      expect(metadata.title).toContain('Coffee Export Revenue Surges 30 Percent');
      expect(metadata.alternates?.canonical).toBe(canonicalUrl);
      expect((metadata.openGraph as Record<string, unknown>)?.type).toBe('article');
      expect((metadata.openGraph as Record<string, unknown>)?.url).toBe(canonicalUrl);
      expect((metadata.twitter as Record<string, unknown>)?.card).toBe('summary_large_image');
    });

    it('constructs metadata for category page', () => {
      const cat: Category = { id: 1, name: 'Banking & Finance', slug: 'banking-finance' };
      const canonicalUrl = 'https://efi.et/category/banking-finance';
      const metadata = buildCategoryMetadata(cat, canonicalUrl);

      expect(metadata.title).toContain('Banking & Finance News');
      expect(metadata.alternates?.canonical).toBe(canonicalUrl);
    });
  });
});
