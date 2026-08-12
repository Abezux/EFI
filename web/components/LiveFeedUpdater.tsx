'use client';

import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { getEventById, getStreamUrl, NewsEvent } from '@/lib/api';
import { RefreshCw, Radio, WifiOff } from 'lucide-react';

interface LiveFeedUpdaterProps {
  initialLatestTimestamp?: string;
  category?: string;
}

type ConnectionStatus = 'connected' | 'connecting' | 'reconnecting' | 'disconnected';

export function LiveFeedUpdater({
  category,
}: LiveFeedUpdaterProps) {
  const router = useRouter();
  const [newEvents, setNewEvents] = useState<NewsEvent[]>([]);
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  const [updatedCount, setUpdatedCount] = useState<number>(0);
  const eventSourceRef = useRef<EventSource | null>(null);

  const handleRefresh = useCallback(() => {
    setNewEvents([]);
    setUpdatedCount(0);
    router.refresh();
  }, [router]);

  useEffect(() => {
    // Graceful SSR check
    if (typeof window === 'undefined' || typeof EventSource === 'undefined') {
      return;
    }

    const streamUrl = getStreamUrl();
    let es: EventSource | null = null;

    try {
      es = new EventSource(streamUrl);
      eventSourceRef.current = es;

      es.onopen = () => {
        setStatus('connected');
      };

      es.onmessage = async (event) => {
        try {
          if (!event.data) return;
          const msg = JSON.parse(event.data);
          if (!msg || !msg.type || !msg.event_id) return;

          if (msg.type === 'new_event') {
            // Fetch the fresh event details
            const freshEvent = await getEventById(msg.event_id, { isServer: false });
            if (freshEvent) {
              // If filtering by category, verify event category match
              if (category && freshEvent.category?.slug !== category) {
                return;
              }
              setNewEvents((prev) => {
                if (prev.some((e) => e.id === freshEvent.id)) {
                  return prev;
                }
                return [freshEvent, ...prev];
              });
            }
          } else if (msg.type === 'event_updated') {
            // An existing event received a new source or updated summary
            setUpdatedCount((prev) => prev + 1);
            // In addition, automatically trigger a soft refresh in background
            router.refresh();
          }
        } catch {
          // Ignore malformed message payloads safely
        }
      };

      es.onerror = () => {
        // EventSource will automatically retry in background
        setStatus('reconnecting');
      };
    } catch {
      setStatus('disconnected');
    }

    return () => {
      if (es) {
        es.close();
      }
      eventSourceRef.current = null;
    };
  }, [category, router]);

  return (
    <aside
      aria-live="polite"
      aria-atomic="true"
      style={{ marginBottom: newEvents.length > 0 ? '1rem' : '0.5rem' }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          marginBottom: '0.75rem',
          fontSize: '0.8125rem',
          color: 'var(--text-muted)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '0.4rem' }}>
          {status === 'connected' ? (
            <>
              <span
                data-testid="sse-status-connected"
                style={{
                  display: 'inline-block',
                  width: '8px',
                  height: '8px',
                  borderRadius: '50%',
                  backgroundColor: '#10b981',
                  boxShadow: '0 0 6px rgba(16, 185, 129, 0.6)',
                }}
              />
              <span style={{ color: '#34d399', fontWeight: 500 }}>Live Feed Active</span>
            </>
          ) : status === 'reconnecting' ? (
            <>
              <span
                data-testid="sse-status-reconnecting"
                style={{
                  display: 'inline-block',
                  width: '8px',
                  height: '8px',
                  borderRadius: '50%',
                  backgroundColor: '#f59e0b',
                }}
              />
              <span style={{ color: '#fbbf24' }}>Reconnecting stream...</span>
            </>
          ) : (
            <>
              <WifiOff size={13} data-testid="sse-status-offline" />
              <span>Offline</span>
            </>
          )}
        </div>

        {updatedCount > 0 && (
          <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
            {updatedCount} story {updatedCount === 1 ? 'update' : 'updates'} synced
          </span>
        )}
      </div>

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
