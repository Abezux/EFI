import React from 'react';
import { render, screen } from '@testing-library/react';
import HomePage from '../page';
import CategoryPage from '../category/[slug]/page';
import EventDetailPage from '../events/[id]/page';
import SearchPage from '../search/page';
import * as api from '@/lib/api';

const mockEventsResponse: api.EventsResponse = {
  events: [
    {
      id: 101,
      canonical_title: 'Commercial Bank of Ethiopia FX Directive',
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
          posted_at: '2026-08-11T12:00:00Z',
          excerpt: 'CBE implemented directive today...',
        },
      ],
      source_count: 2,
      first_seen_at: '2026-08-11T12:00:00Z',
      last_updated_at: '2026-08-11T12:00:00Z',
    },
  ],
  total: 1,
  limit: 25,
  offset: 0,
};

const mockCategories: api.Category[] = [
  { id: 1, name: 'Banking & Finance', slug: 'banking-finance', event_count: 1 },
  { id: 2, name: 'Inflation & Prices', slug: 'inflation-prices', event_count: 0 },
];

describe('App Router Pages Integration', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('HomePage', () => {
    it('renders news feed with events and categories', async () => {
      jest.spyOn(api, 'getEvents').mockResolvedValueOnce(mockEventsResponse);
      jest.spyOn(api, 'getCategories').mockResolvedValueOnce(mockCategories);

      const PageComponent = await HomePage();
      render(PageComponent);

      expect(
        screen.getByText('Commercial Bank of Ethiopia FX Directive')
      ).toBeInTheDocument();
      expect(screen.getAllByText('Banking & Finance').length).toBeGreaterThanOrEqual(1);
      expect(screen.getByTestId('ai-summary-badge')).toBeInTheDocument();
    });

    it('renders clean error state if API fails', async () => {
      jest
        .spyOn(api, 'getEvents')
        .mockRejectedValueOnce(new Error('Connection refused'));
      jest
        .spyOn(api, 'getCategories')
        .mockRejectedValueOnce(new Error('Connection refused'));

      const PageComponent = await HomePage();
      render(PageComponent);

      expect(
        screen.getByText('News Feed Temporarily Unavailable')
      ).toBeInTheDocument();
      expect(screen.getByText('Reload Feed')).toBeInTheDocument();
    });
  });

  describe('CategoryPage', () => {
    it('renders category filtered feed', async () => {
      jest.spyOn(api, 'getCategories').mockResolvedValueOnce(mockCategories);
      jest.spyOn(api, 'getEvents').mockResolvedValueOnce(mockEventsResponse);

      const PageComponent = await CategoryPage({
        params: { slug: 'banking-finance' },
      });
      render(PageComponent);

      expect(
        screen.getAllByText('Banking & Finance').length
      ).toBeGreaterThanOrEqual(1);
      expect(
        screen.getByText('Commercial Bank of Ethiopia FX Directive')
      ).toBeInTheDocument();
    });
  });

  describe('EventDetailPage', () => {
    it('renders full event details, AI executive summary, and source list', async () => {
      jest
        .spyOn(api, 'getEventById')
        .mockResolvedValueOnce(mockEventsResponse.events[0]);

      const PageComponent = await EventDetailPage({
        params: { id: '101' },
      });
      render(PageComponent);

      expect(
        screen.getByText('Commercial Bank of Ethiopia FX Directive')
      ).toBeInTheDocument();
      expect(screen.getByText('Executive Summary')).toBeInTheDocument();
      expect(screen.getByTestId('ai-summary-badge')).toBeInTheDocument();
      expect(
        screen.getByText(
          'Central bank launched new FX policies across major retail banks.'
        )
      ).toBeInTheDocument();
      expect(screen.getByText('Source Reports (1)')).toBeInTheDocument();
      expect(
        screen.getByText(/CBE implemented directive today\.\.\./)
      ).toBeInTheDocument();
    });
  });

  describe('SearchPage', () => {
    it('renders search results for query', async () => {
      jest.spyOn(api, 'searchEvents').mockResolvedValueOnce({
        events: mockEventsResponse.events,
        total: 1,
        limit: 30,
        offset: 0,
      });

      const PageComponent = await SearchPage({
        searchParams: { q: 'CBE' },
      });
      render(PageComponent);

      expect(screen.getByText(/Found/i)).toBeInTheDocument();
      expect(screen.getAllByText(/CBE/i).length).toBeGreaterThanOrEqual(1);
      expect(
        screen.getByText('Commercial Bank of Ethiopia FX Directive')
      ).toBeInTheDocument();
    });
  });
});
