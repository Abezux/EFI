'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import {
  getReviewQueue,
  resolveReviewQueue,
  NeedsReviewPost,
} from '@/lib/api';
import { useAdminAuth } from '@/app/admin/AdminAuthContext';


export default function AdminReviewQueuePage() {
  const { csrfToken } = useAdminAuth();
  const [posts, setPosts] = useState<NeedsReviewPost[]>([]);
  const [total, setTotal] = useState(0);
  const [limit] = useState(20);
  const [offset, setOffset] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Resolution modal state
  const [activePost, setActivePost] = useState<NeedsReviewPost | null>(null);
  const [decision, setDecision] = useState<'attach_to_event' | 'create_new_event' | 'discard'>('attach_to_event');
  const [customEventId, setCustomEventId] = useState<string>('');
  const [reasonInput, setReasonInput] = useState<string>('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const fetchQueue = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const res = await getReviewQueue({ limit, offset });
      setPosts(res.posts);
      setTotal(res.total);
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to fetch review queue');
      }
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchQueue();
  }, [offset]);

  const openResolutionModal = (
    post: NeedsReviewPost,
    initDecision: 'attach_to_event' | 'create_new_event' | 'discard'
  ) => {
    setActivePost(post);
    setDecision(initDecision);
    setCustomEventId(post.candidate_event_id ? String(post.candidate_event_id) : '');
    setReasonInput('');
  };

  const handleResolveSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!activePost) return;

    let targetId: number | undefined = undefined;
    if (decision === 'attach_to_event') {
      targetId = parseInt(customEventId, 10);
      if (isNaN(targetId) || targetId <= 0) {
        setError('Please enter a valid numeric target Event ID');
        return;
      }
    }

    setIsSubmitting(true);
    try {
      await resolveReviewQueue(activePost.raw_post_id, {
        decision,
        target_event_id: targetId,
        reason: reasonInput.trim(),
        csrfToken,
      });
      setActivePost(null);
      await fetchQueue();
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to resolve review post');
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-title">Ambiguous Review Queue</h1>
          <p className="admin-page-subtitle">
            Manually verify posts that fell into the low-confidence ambiguity band
          </p>
        </div>
      </div>

      {error && (
        <div
          style={{
            background: 'rgba(239, 68, 68, 0.12)',
            border: '1px solid rgba(239, 68, 68, 0.3)',
            borderRadius: 'var(--radius-md)',
            padding: '0.75rem 1rem',
            color: '#fca5a5',
            fontSize: '0.875rem',
            marginBottom: '1.5rem',
          }}
        >
          {error}
        </div>
      )}

      {isLoading ? (
        <div style={{ textAlign: 'center', padding: '4rem 0' }}>
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
          <p style={{ color: 'var(--text-secondary)' }}>Loading pending review posts...</p>
        </div>
      ) : posts.length === 0 ? (
        <div className="admin-card" style={{ textAlign: 'center', padding: '3rem' }}>
          <div style={{ fontSize: '2.5rem', marginBottom: '0.5rem' }}>✨</div>
          <h2 style={{ fontSize: '1.25rem', fontWeight: 700, marginBottom: '0.5rem' }}>
            Queue Completely Cleared
          </h2>
          <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem', maxWidth: '400px', margin: '0 auto' }}>
            There are no ambiguous posts pending manual decision. All incoming posts are clustered or verified.
          </p>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
          {posts.map((post) => (
            <div key={post.raw_post_id} className="admin-card">
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1rem' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <span style={{ fontWeight: 600 }}>{post.channel_name}</span>
                  {post.channel_handle && (
                    <span style={{ color: 'var(--text-secondary)', fontSize: '0.8125rem' }}>
                      @{post.channel_handle}
                    </span>
                  )}
                  <span style={{ color: 'var(--text-muted)', fontSize: '0.75rem', marginLeft: '0.5rem' }}>
                    Post ID #{post.raw_post_id} • Msg #{post.telegram_message_id} • Ingested {new Date(post.ingested_at).toLocaleString()}
                  </span>
                </div>
                <span className="status-badge review">Needs Human Review</span>
              </div>

              {/* Raw Post Text */}
              <div
                style={{
                  background: 'var(--bg-secondary)',
                  padding: '1rem',
                  borderRadius: 'var(--radius-md)',
                  border: '1px solid var(--border-subtle)',
                  fontSize: '0.9375rem',
                  lineHeight: 1.6,
                  whiteSpace: 'pre-wrap',
                  marginBottom: '1rem',
                }}
              >
                {post.raw_text}
              </div>

              {/* AI Verification Context */}
              {post.candidate_event_id && (
                <div
                  style={{
                    background: 'var(--accent-ai-bg)',
                    border: '1px solid var(--accent-ai-border)',
                    borderRadius: 'var(--radius-md)',
                    padding: '0.85rem 1rem',
                    fontSize: '0.8125rem',
                    marginBottom: '1rem',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', marginBottom: '0.25rem' }}>
                    <span style={{ color: 'var(--accent-ai)', fontWeight: 600 }}>
                      🤖 AI Candidate Evaluation:
                    </span>
                    <Link
                      href={`/admin/events/${post.candidate_event_id}`}
                      target="_blank"
                      style={{ color: 'var(--accent-primary)', textDecoration: 'underline', fontWeight: 500 }}
                    >
                      Event #{post.candidate_event_id}: {post.candidate_event_title} ↗
                    </Link>
                  </div>
                  {post.ai_run_reason && (
                    <p style={{ color: 'var(--text-secondary)', fontStyle: 'italic' }}>
                      {post.ai_run_reason}
                    </p>
                  )}
                </div>
              )}

              {/* Human Decision Action Buttons */}
              <div style={{ display: 'flex', gap: '0.75rem', justifyContent: 'flex-end', borderTop: '1px solid var(--border-subtle)', paddingTop: '1rem' }}>
                <button
                  onClick={() => openResolutionModal(post, 'discard')}
                  className="btn-admin-secondary"
                  style={{ color: '#ef4444' }}
                >
                  Discard / Spam
                </button>
                <button
                  onClick={() => openResolutionModal(post, 'create_new_event')}
                  className="btn-admin-secondary"
                >
                  Create New Story
                </button>
                <button
                  onClick={() => openResolutionModal(post, 'attach_to_event')}
                  className="btn-admin-primary"
                >
                  Attach to Event
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Modal: Resolve Review Post */}
      {activePost && (
        <div className="modal-backdrop">
          <div className="modal-card">
            <h2 style={{ fontSize: '1.25rem', fontWeight: 700, marginBottom: '0.5rem' }}>
              Confirm Review Resolution
            </h2>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem', marginBottom: '1.5rem' }}>
              Resolution will be recorded in <code>processing_audit</code> under your admin credentials.
            </p>

            <form onSubmit={handleResolveSubmit}>
              <div className="admin-form-group">
                <label className="admin-label">Decision Strategy</label>
                <select
                  className="admin-input"
                  value={decision}
                  onChange={(e) => setDecision(e.target.value as any)}
                >
                  <option value="attach_to_event">Attach to Existing Event</option>
                  <option value="create_new_event">Create New News Event</option>
                  <option value="discard">Discard Post (No Event)</option>
                </select>
              </div>

              {decision === 'attach_to_event' && (
                <div className="admin-form-group">
                  <label className="admin-label">Target News Event ID</label>
                  <input
                    type="number"
                    required
                    placeholder="e.g. 101"
                    className="admin-input"
                    value={customEventId}
                    onChange={(e) => setCustomEventId(e.target.value)}
                  />
                </div>
              )}

              <div className="admin-form-group">
                <label className="admin-label">Human Review Reasoning (Audit Note)</label>
                <textarea
                  required
                  placeholder="Explain why this post is attached, separated, or discarded..."
                  className="admin-textarea"
                  value={reasonInput}
                  onChange={(e) => setReasonInput(e.target.value)}
                />
              </div>

              <div style={{ display: 'flex', gap: '0.75rem', justifyContent: 'flex-end', marginTop: '1.5rem' }}>
                <button
                  type="button"
                  onClick={() => setActivePost(null)}
                  className="btn-admin-secondary"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isSubmitting}
                  className="btn-admin-primary"
                >
                  {isSubmitting ? 'Resolving...' : 'Confirm Resolution'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
