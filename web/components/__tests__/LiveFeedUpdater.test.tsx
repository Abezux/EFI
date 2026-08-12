import React from 'react';
import { render, screen, act, fireEvent } from '@testing-library/react';
import { LiveFeedUpdater } from '../LiveFeedUpdater';
import * as api from '@/lib/api';
import { useRouter } from 'next/navigation';

const mockRefresh = jest.fn();

class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  closed = false;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  close() {
    this.closed = true;
  }
}

describe('LiveFeedUpdater Component (SSE Streaming)', () => {
  const originalEventSource = global.EventSource;

  beforeEach(() => {
    jest.clearAllMocks();
    MockEventSource.instances = [];
    (global as any).EventSource = MockEventSource;
    (useRouter as jest.Mock).mockReturnValue({
      push: jest.fn(),
      replace: jest.fn(),
      prefetch: jest.fn(),
      back: jest.fn(),
      refresh: mockRefresh,
      forward: jest.fn(),
    });
  });

  afterEach(() => {
    (global as any).EventSource = originalEventSource;
  });

  it('subscribes to SSE stream on mount and displays Live Feed Active status', () => {
    render(<LiveFeedUpdater />);

    expect(MockEventSource.instances.length).toBe(1);
    expect(MockEventSource.instances[0].url).toContain('/api/v1/stream');

    // Trigger onopen
    act(() => {
      if (MockEventSource.instances[0].onopen) {
        MockEventSource.instances[0].onopen();
      }
    });

    expect(screen.getByText('Live Feed Active')).toBeInTheDocument();
    expect(screen.getByTestId('sse-status-connected')).toBeInTheDocument();
  });

  it('handles new_event message and renders banner', async () => {
    const mockNewEvent: api.NewsEvent = {
      id: 88,
      canonical_title: 'New Forex Regulation Announced',
      ai_summary: 'Details on new forex rules.',
      ai_summary_generated: true,
      category: { id: 2, name: 'Forex & Trade', slug: 'forex-trade' },
      source_count: 1,
      first_seen_at: '2026-08-12T10:00:00Z',
      last_updated_at: '2026-08-12T10:00:00Z',
    };

    jest.spyOn(api, 'getEventById').mockResolvedValue(mockNewEvent);

    render(<LiveFeedUpdater />);

    act(() => {
      if (MockEventSource.instances[0].onopen) {
        MockEventSource.instances[0].onopen();
      }
    });

    await act(async () => {
      if (MockEventSource.instances[0].onmessage) {
        MockEventSource.instances[0].onmessage({
          data: JSON.stringify({ type: 'new_event', event_id: 88 }),
        });
      }
    });

    expect(api.getEventById).toHaveBeenCalledWith(88, { isServer: false });
    expect(screen.getByTestId('live-feed-banner')).toBeInTheDocument();
    expect(screen.getByText('1 new event available')).toBeInTheDocument();

    // Clicking Load updates triggers router.refresh() and dismisses banner
    fireEvent.click(screen.getByRole('button', { name: /load updates/i }));
    expect(mockRefresh).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId('live-feed-banner')).not.toBeInTheDocument();
  });

  it('handles event_updated message and triggers refresh and update indicator', async () => {
    render(<LiveFeedUpdater />);

    act(() => {
      if (MockEventSource.instances[0].onopen) {
        MockEventSource.instances[0].onopen();
      }
    });

    await act(async () => {
      if (MockEventSource.instances[0].onmessage) {
        MockEventSource.instances[0].onmessage({
          data: JSON.stringify({ type: 'event_updated', event_id: 88 }),
        });
      }
    });

    expect(mockRefresh).toHaveBeenCalled();
    expect(screen.getByText('1 story update synced')).toBeInTheDocument();
  });

  it('shows reconnecting indicator on stream error', () => {
    render(<LiveFeedUpdater />);

    act(() => {
      if (MockEventSource.instances[0].onerror) {
        MockEventSource.instances[0].onerror();
      }
    });

    expect(screen.getByText('Reconnecting stream...')).toBeInTheDocument();
    expect(screen.getByTestId('sse-status-reconnecting')).toBeInTheDocument();
  });

  it('closes EventSource cleanly on unmount', () => {
    const { unmount } = render(<LiveFeedUpdater />);
    const instance = MockEventSource.instances[0];
    expect(instance.closed).toBe(false);

    unmount();
    expect(instance.closed).toBe(true);
  });
});
