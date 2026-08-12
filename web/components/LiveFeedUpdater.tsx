'use client';

import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { getEvents, NewsEvent } from '@/lib/api';
import { RefreshCw, Radio } from 'lucide-react';

interface LiveFeedUpdaterProps {
  initialLatestTimestamp?: string;
  category?: string;
}

const BASE_POLL_INTERVAL_MS = 30000; // 30 seconds
const MAX_POLL_INTERVAL_MS = 300000; // 5 minutes

export function LiveFeedUpdater({
  initialLatestTimestamp,
  category,
}: LiveFeedUpdaterProps) {
  const router = useRouter();
  const [newEvents, setNewEvents] = useState<NewsEvent[]>([]);
  const [isPolling, setIsPolling] = useState(false);
  const backoffRef = useRef(BASE_POLL_INTERVAL_MS);
  const timeoutIdRef = useRef<NodeJS.Timeout | null>(null);
  const latestTimestampRef = useRef<string | undefined>(initialLatestTimestamp);

  const pollForUpdates = useCallback(async () => {
    if (!latestTimestampRef.current) return;

    try {
      setIsPolling(true);
      const res = await getEvents({
        category,
        since: latestTimestampRef.current,
        limit: 10,
        isServer: false, // Force client-side URL resolution
      });

      if (res && res.events && res.events.length > 0) {
        setNewEvents((prev) => {
          const existingIds = new Set(prev.map((e) => e.id));
          const fresh = res.events.filter((e) => !existingIds.has(e.id));
          return [...fresh, ...prev];
        });
      }

      // Reset backoff interval on successful probe
      backoffRef.current = BASE_POLL_INTERVAL_MS;
    } catch {
      // Silent degradation: do not show scary error modals to users
      // Exponential backoff
      backoffRef.current = Math.min(
        backoffRef.current * 1.5,
        MAX_POLL_INTERVAL_MS
      );
    } finally {
      setIsPolling(false);
      // Schedule next polling tick
      timeoutIdRef.current = setTimeout(pollForUpdates, backoffRef.current);
    }
  }, [category]);

  useEffect(() => {
    latestTimestampRef.current = initialLatestTimestamp;
    // Schedule initial polling tick
    timeoutIdRef.current = setTimeout(pollForUpdates, BASE_POLL_INTERVAL_MS);

    return () => {
      if (timeoutIdRef.current) {
        clearTimeout(timeoutIdRef.current);
      }
    };
  }, [initialLatestTimestamp, pollForUpdates]);

  const handleRefresh = () => {
    setNewEvents([]);
    router.refresh();
  };

  return (
    <aside
      aria-live="polite"
      aria-atomic="true"
      style={{ marginBottom: newEvents.length > 0 ? '1rem' : '0' }}
    >
      {newEvents.length > 0 && (
        <div
          className="live-feed-banner"
          data-testid="live-feed-banner"
          role="status"
          onClick={handleRefresh}
          style={{ cursor: 'pointer' }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.6rem' }}>
            <Radio size={16} className="animate-pulse" />
            <span>
              {newEvents.length} new {newEvents.length === 1 ? 'event' : 'events'}{' '}
              available
            </span>
          </div>

          <button
            type="button"
            className="btn-retry"
            style={{
              padding: '0.35rem 0.85rem',
              fontSize: '0.8125rem',
              backgroundColor: 'rgba(16, 185, 129, 0.2)',
              border: '1px solid rgba(16, 185, 129, 0.4)',
              color: '#34d399',
            }}
            onClick={(e) => {
              e.stopPropagation();
              handleRefresh();
            }}
          >
            <RefreshCw size={13} />
            <span>Load updates</span>
          </button>
        </div>
      )}
    </aside>
  );
}
