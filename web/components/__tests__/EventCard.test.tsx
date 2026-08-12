import React from 'react';
import { render, screen } from '@testing-library/react';
import { EventCard } from '../EventCard';
import { NewsEvent } from '@/lib/api';

const mockEvent: NewsEvent = {
  id: 42,
  canonical_title: 'National Bank FX Directive 2026',
  ai_summary: 'Central bank eased import forex restrictions for key commodities.',
  ai_summary_generated: true,
  category: {
    id: 1,
    name: 'Banking & Finance',
    slug: 'banking-finance',
  },
  entities: [
    { id: 1, name: 'National Bank of Ethiopia', type: 'ORGANIZATION' },
    { id: 2, name: 'Addis Ababa', type: 'LOCATION' },
  ],
  sources: [
    {
      channel_name: 'Tikvah Magazine',
      channel_handle: 'tikvahethiopia',
      posted_at: '2026-08-11T12:00:00Z',
      excerpt: 'Directive announced today...',
    },
  ],
  source_count: 3,
  first_seen_at: '2026-08-11T12:00:00Z',
  last_updated_at: '2026-08-11T12:00:00Z',
};

describe('EventCard Component', () => {
  it('renders event title, category, sources, and AI summary badge', () => {
    render(<EventCard event={mockEvent} />);

    expect(
      screen.getByText('National Bank FX Directive 2026')
    ).toBeInTheDocument();
    expect(screen.getByText('Banking & Finance')).toBeInTheDocument();
    expect(screen.getByText('3 sources')).toBeInTheDocument();
    expect(screen.getByTestId('ai-summary-badge')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Central bank eased import forex restrictions for key commodities.'
      )
    ).toBeInTheDocument();
    expect(
      screen.getByText('National Bank of Ethiopia')
    ).toBeInTheDocument();
  });

  it('links title to event detail page', () => {
    render(<EventCard event={mockEvent} />);
    const link = screen.getByRole('link', {
      name: 'National Bank FX Directive 2026',
    });
    expect(link).toHaveAttribute('href', '/events/42');
  });

  it('does not render AI badge if ai_summary_generated is false', () => {
    const unsummarizedEvent: NewsEvent = {
      ...mockEvent,
      ai_summary_generated: false,
    };
    render(<EventCard event={unsummarizedEvent} />);
    expect(screen.queryByTestId('ai-summary-badge')).not.toBeInTheDocument();
  });
});
