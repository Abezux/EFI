'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import {
  getAdminChannels,
  getAdminEvents,
  getReviewQueue,
  AdminChannel,
  AdminEventSummary,
  NeedsReviewPost,
} from '@/lib/api';
import { useAdminAuth } from '@/app/admin/AdminAuthContext';


export default function AdminDashboardPage() {
  const { user } = useAdminAuth();
  const [channels, setChannels] = useState<AdminChannel[]>([]);
  const [eventsTotal, setEventsTotal] = useState<number>(0);
  const [hiddenTotal, setHiddenTotal] = useState<number>(0);
  const [reviewPosts, setReviewPosts] = useState<NeedsReviewPost[]>([]);
  const [reviewTotal, setReviewTotal] = useState<number>(0);
  const [isLoading, setIsLoading] = useState<boolean>(true);

  useEffect(() => {
    async function loadStats() {
      try {
        const [chRes, evRes, hiddenRes, revRes] = await Promise.all([
          getAdminChannels(),
          getAdminEvents({ limit: 1 }),
          getAdminEvents({ limit: 1, hidden: true }),
          getReviewQueue({ limit: 5 }),
        ]);
        setChannels(chRes.channels);
        setEventsTotal(evRes.total);
        setHiddenTotal(hiddenRes.total);
        setReviewPosts(revRes.posts);
        setReviewTotal(revRes.total);
      } catch (err) {
        console.error('Failed to load dashboard data:', err);
      } finally {
        setIsLoading(false);
      }
    }
    loadStats();
  }, []);

  const activeChannelsCount = channels.filter((c) => c.is_active).length;

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-title">Dashboard Overview</h1>
          <p className="admin-page-subtitle">
            Welcome back, {user?.email} ({user?.role})
          </p>
        </div>
      </div>

      {isLoading ? (
        <div style={{ textAlign: 'center', padding: '3rem 0' }}>
          <div
            style={{
              width: '32px',
              height: '32px',
              border: '3px solid var(--border-subtle)',
              borderTopColor: 'var(--accent-primary)',
              borderRadius: '50%',
              animation: 'spin 1s linear infinite',
              margin: '0 auto 1rem',
            }}
          />
          <p style={{ color: 'var(--text-secondary)' }}>Loading platform analytics...</p>
        </div>
      ) : (
        <>
          {/* Analytics Stat Cards */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
              gap: '1.25rem',
              marginBottom: '2rem',
            }}
          >
            <div className="admin-card" style={{ marginBottom: 0 }}>
              <span style={{ fontSize: '0.8125rem', color: 'var(--text-muted)', fontWeight: 600 }}>
                MONITORED CHANNELS
              </span>
              <div style={{ fontSize: '2rem', fontWeight: 700, margin: '0.5rem 0' }}>
                {activeChannelsCount}{' '}
                <span style={{ fontSize: '1rem', color: 'var(--text-secondary)', fontWeight: 400 }}>
                  / {channels.length} active
                </span>
              </div>
              <Link href="/admin/channels" style={{ fontSize: '0.8125rem', color: 'var(--accent-primary)', fontWeight: 500 }}>
                Manage Channels →
              </Link>
            </div>

            <div className="admin-card" style={{ marginBottom: 0 }}>
              <span style={{ fontSize: '0.8125rem', color: 'var(--text-muted)', fontWeight: 600 }}>
                TOTAL NEWS EVENTS
              </span>
              <div style={{ fontSize: '2rem', fontWeight: 700, margin: '0.5rem 0' }}>
                {eventsTotal}
              </div>
              <Link href="/admin/events" style={{ fontSize: '0.8125rem', color: 'var(--accent-primary)', fontWeight: 500 }}>
                Browse Events →
              </Link>
            </div>

            <div className="admin-card" style={{ marginBottom: 0 }}>
              <span style={{ fontSize: '0.8125rem', color: 'var(--text-muted)', fontWeight: 600 }}>
                TAKEN DOWN (HIDDEN)
              </span>
              <div
                style={{
                  fontSize: '2rem',
                  fontWeight: 700,
                  margin: '0.5rem 0',
                  color: hiddenTotal > 0 ? '#ef4444' : 'var(--text-primary)',
                }}
              >
                {hiddenTotal}
              </div>
              <Link href="/admin/events?hidden=true" style={{ fontSize: '0.8125rem', color: 'var(--accent-primary)', fontWeight: 500 }}>
                View Moderated Events →
              </Link>
            </div>

            <div className="admin-card" style={{ marginBottom: 0 }}>
              <span style={{ fontSize: '0.8125rem', color: 'var(--text-muted)', fontWeight: 600 }}>
                REVIEW QUEUE
              </span>
              <div
                style={{
                  fontSize: '2rem',
                  fontWeight: 700,
                  margin: '0.5rem 0',
                  color: reviewTotal > 0 ? '#f59e0b' : 'var(--text-primary)',
                }}
              >
                {reviewTotal}
              </div>
              <Link href="/admin/review-queue" style={{ fontSize: '0.8125rem', color: 'var(--accent-primary)', fontWeight: 500 }}>
                Resolve Unmatched Posts →
              </Link>
            </div>
          </div>

          {/* Review Queue Preview */}
          <div className="admin-card">
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1rem' }}>
              <h2 style={{ fontSize: '1.125rem', fontWeight: 700 }}>Ambiguous Review Queue ({reviewTotal})</h2>
              <Link href="/admin/review-queue" className="btn-admin-secondary">
                View Full Queue →
              </Link>
            </div>

            {reviewPosts.length === 0 ? (
              <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
                🎉 The review queue is clear! All ingested posts have been automatically clustered or verified.
              </p>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
                {reviewPosts.map((post) => (
                  <div
                    key={post.raw_post_id}
                    style={{
                      background: 'var(--bg-secondary)',
                      padding: '1rem',
                      borderRadius: 'var(--radius-md)',
                      border: '1px solid var(--border-subtle)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      gap: '1rem',
                    }}
                  >
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', marginBottom: '0.25rem' }}>
                        <span style={{ fontWeight: 600, fontSize: '0.8125rem' }}>{post.channel_name}</span>
                        {post.channel_handle && (
                          <span style={{ color: 'var(--text-muted)', fontSize: '0.75rem' }}>
                            @{post.channel_handle}
                          </span>
                        )}
                        <span className="status-badge review">Needs Review</span>
                      </div>
                      <p
                        style={{
                          fontSize: '0.875rem',
                          color: 'var(--text-secondary)',
                          whiteSpace: 'nowrap',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                        }}
                      >
                        {post.raw_text}
                      </p>
                    </div>
                    <Link href={`/admin/review-queue`} className="btn-admin-primary" style={{ flexShrink: 0, fontSize: '0.8125rem' }}>
                      Review Post
                    </Link>
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}
