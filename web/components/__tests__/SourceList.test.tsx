import React from 'react';
import { render, screen } from '@testing-library/react';
import { SourceList } from '../SourceList';
import { EventSource } from '@/lib/api';

const mockSources: EventSource[] = [
  {
    channel_name: 'Tikvah Ethiopia',
    channel_handle: 'tikvahethiopia',
    posted_at: '2026-08-11T10:00:00Z',
    excerpt: 'First channel report on fuel price update.',
  },
  {
    channel_name: 'Fana Broadcasting',
    channel_handle: 'fanabroadcasting',
    posted_at: '2026-08-11T10:15:00Z',
    excerpt: 'Second channel report confirming the tariff.',
  },
];

describe('SourceList Component', () => {
  it('renders all source records with channel names and bounded excerpts', () => {
    render(<SourceList sources={mockSources} />);

    expect(screen.getByText('Source Reports (2)')).toBeInTheDocument();
    expect(screen.getByText('Tikvah Ethiopia')).toBeInTheDocument();
    expect(screen.getByText('@tikvahethiopia')).toBeInTheDocument();
    expect(
      screen.getByText(/First channel report on fuel price update\./)
    ).toBeInTheDocument();

    expect(screen.getByText('Fana Broadcasting')).toBeInTheDocument();
    expect(screen.getByText('@fanabroadcasting')).toBeInTheDocument();
  });

  it('renders empty fallback when sources is empty or undefined', () => {
    render(<SourceList sources={[]} />);
    expect(
      screen.getByText('No source channel records attached.')
    ).toBeInTheDocument();
  });
});
