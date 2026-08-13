'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import {
  getAdminEventById,
  hideAdminEvent,
  restoreAdminEvent,
  detachAdminSource,
  AdminEventDetail,
  AdminEventSource,
} from '@/lib/api';
import { useAdminAuth } from '@/app/admin/AdminAuthContext';


export default function AdminEventDetailPage() {
  const { id } = useParams();
  const router = useRouter();
  const { csrfToken } = useAdminAuth();

  const [event, setEvent] = useState<AdminEventDetail | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Moderation state
  const [modAction, setModAction] = useState<'hide' | 'restore' | null>(null);
  const [modReason, setModReason] = useState('');
  const [isModSubmitting, setIsModSubmitting] = useState(false);

  // Detach state
  const [detachSource, setDetachSource] = useState<AdminEventSource | null>(null);
  const [detachReason, setDetachReason] = useState('');
  const [isDetachSubmitting, setIsDetachSubmitting] = useState(false);

  const fetchDetail = async () => {
    if (!id) return;
    try {
      setIsLoading(true);
      setError(null);
      const res = await getAdminEventById(id as string);
      if (!res) {
        setError('Event not found.');
      } else {
        setEvent(res);
      }
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to fetch event detail');
      }
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchDetail();
  }, [id]);

  const handleModSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!event || !modAction || !modReason.trim()) return;

    setIsModSubmitting(true);
    try {
      if (modAction === 'hide') {
        await hideAdminEvent(event.id, modReason.trim(), csrfToken);
      } else {
        await restoreAdminEvent(event.id, modReason.trim(), csrfToken);
      }
      setModAction(null);
      setModReason('');
      await fetchDetail();
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to update event moderation status');
      }
    } finally {
      setIsModSubmitting(false);
    }
  };

  const handleDetachSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!event || !detachSource || !detachReason.trim()) return;

    setIsDetachSubmitting(true);
    try {
      await detachAdminSource(event.id, detachSource.raw_post_id, detachReason.trim(), csrfToken);
      setDetachSource(null);
      setDetachReason('');
      await fetchDetail();
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to detach source');
      }
    } finally {
      setIsDetachSubmitting(false);
    }
  };

  if (isLoading) {
    return (
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
        <p style={{ color: 'var(--text-secondary)' }}>Loading event inspection data...</p>
      </div>
    );
  }

  if (error || !event) {
    return (
      <div>
        <Link href="/admin/events" className="btn-admin-secondary" style={{ marginBottom: '1.5rem', display: 'inline-flex' }}>
          ← Back to Events
        </Link>
        <div className="error-banner">
          <div className="error-title">Error Loading Event</div>
          <div className="error-desc">{error || 'Event not found'}</div>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div style={{ marginBottom: '1.5rem', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Link href="/admin/events" className="btn-admin-secondary" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.25rem' }}>
          ← Back to Events List
        </Link>
        <div style={{ display: 'flex', gap: '0.75rem' }}>
          {event.is_hidden ? (
            <button
              onClick={() => {
                setModAction('restore');
                setModReason('');
              }}
              className="btn-admin-success"
            >
              Restore Event
            </button>
          ) : (
            <button
              onClick={() => {
                setModAction('hide');
                setModReason('');
              }}
              className="btn-admin-danger"
            >
              Take Down Event
            </button>
          )}
        </div>
      </div>

      {/* Main Event Overview Card */}
      <div className="admin-card">
        <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '0.75rem' }}>
          <span className={`status-badge ${event.is_hidden ? 'hidden' : 'active'}`}>
            {event.is_hidden ? '🚫 Hidden / Soft-Taken Down' : '🟢 Public Published'}
          </span>
          <span className={`status-badge ${event.status === 'active' ? 'active' : 'review'}`}>
            {event.status}
          </span>
          {event.category && (
            <span className="status-badge" style={{ background: 'var(--bg-surface)', border: '1px solid var(--border-subtle)' }}>
              {event.category.name}
            </span>
          )}
          <span style={{ color: 'var(--text-muted)', fontSize: '0.8125rem', marginLeft: 'auto' }}>
            Event ID #{event.id}
          </span>
        </div>

        <h1 style={{ fontSize: '1.5rem', fontWeight: 700, marginBottom: '0.5rem' }}>
          {event.ai_headline || event.canonical_title}
        </h1>

        {event.ai_headline && (
          <p style={{ color: 'var(--text-muted)', fontSize: '0.875rem', marginBottom: '1rem', fontStyle: 'italic' }}>
            Canonical Title: {event.canonical_title}
          </p>
        )}

        <div
          style={{
            background: 'var(--bg-secondary)',
            padding: '1.25rem',
            borderRadius: 'var(--radius-md)',
            border: '1px solid var(--border-subtle)',
            fontSize: '0.9375rem',
            lineHeight: 1.6,
            color: 'var(--text-primary)',
            marginBottom: '1.5rem',
          }}
        >
          {event.ai_summary || '(No AI summary synthesized)'}
        </div>

        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))',
            gap: '1rem',
            borderTop: '1px solid var(--border-subtle)',
            paddingTop: '1rem',
            fontSize: '0.8125rem',
            color: 'var(--text-secondary)',
          }}
        >
          <div>
            <strong>First Seen:</strong> {new Date(event.first_seen_at).toLocaleString()}
          </div>
          <div>
            <strong>Last Updated:</strong> {new Date(event.last_updated_at).toLocaleString()}
          </div>
          <div>
            <strong>Attached Sources:</strong> {event.source_count}
          </div>
          <div>
            <strong>Canonical Slug:</strong> {event.slug || '—'}
          </div>
        </div>
      </div>

      {/* Attached Sources */}
      <div className="admin-card">
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1rem' }}>
          <h2 style={{ fontSize: '1.125rem', fontWeight: 700 }}>
            Attached Channel Sources ({(event.sources || []).length})
          </h2>
          <span style={{ fontSize: '0.8125rem', color: 'var(--text-muted)' }}>
            Primary verification feeds
          </span>
        </div>

        {(event.sources || []).length === 0 ? (
          <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
            No sources currently attached to this event.
          </p>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
            {(event.sources || []).map((src) => (
              <div
                key={src.raw_post_id}
                style={{
                  background: 'var(--bg-secondary)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: 'var(--radius-md)',
                  padding: '1.25rem',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.75rem' }}>
                  <div>
                    <span style={{ fontWeight: 600, color: 'var(--text-primary)' }}>{src.channel_name}</span>
                    {src.channel_handle && (
                      <span style={{ color: 'var(--text-muted)', fontSize: '0.8125rem', marginLeft: '0.5rem' }}>
                        @{src.channel_handle}
                      </span>
                    )}
                    <span style={{ color: 'var(--text-muted)', fontSize: '0.75rem', marginLeft: '1rem' }}>
                      Msg ID: {src.telegram_message_id} • Posted: {new Date(src.posted_at).toLocaleString()}
                    </span>
                  </div>
                  <button
                    onClick={() => {
                      setDetachSource(src);
                      setDetachReason('');
                    }}
                    className="btn-admin-danger"
                    style={{ fontSize: '0.75rem', padding: '0.25rem 0.5rem' }}
                  >
                    Detach Source
                  </button>
                </div>
                <div
                  style={{
                    background: 'var(--bg-primary)',
                    padding: '0.75rem 1rem',
                    borderRadius: 'var(--radius-sm)',
                    fontSize: '0.875rem',
                    color: 'var(--text-primary)',
                    whiteSpace: 'pre-wrap',
                    maxHeight: '150px',
                    overflowY: 'auto',
                  }}
                >
                  {src.raw_text}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Moderation Audit History */}
      <div className="admin-card">
        <h2 style={{ fontSize: '1.125rem', fontWeight: 700, marginBottom: '1rem' }}>
          Moderation & Compliance Audit Trail
        </h2>

        {(event.moderation_history || []).length === 0 ? (
          <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
            No administrative moderation actions recorded for this event.
          </p>
        ) : (
          <table className="admin-table">
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>Actor</th>
                <th>Action</th>
                <th>Justification / Reason</th>
              </tr>
            </thead>
            <tbody>
              {(event.moderation_history || []).map((record) => (
                <tr key={record.id}>
                  <td style={{ color: 'var(--text-secondary)', fontSize: '0.8125rem', whiteSpace: 'nowrap' }}>
                    {new Date(record.created_at).toLocaleString()}
                  </td>
                  <td style={{ fontWeight: 600 }}>{record.actor_email}</td>
                  <td>
                    <span className="status-badge review">{record.action_type}</span>
                  </td>
                  <td style={{ color: 'var(--text-primary)' }}>
                    {record.reason || '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Modal: Takedown / Restore */}
      {modAction && (
        <div className="modal-backdrop">
          <div className="modal-card">
            <h2 style={{ fontSize: '1.25rem', fontWeight: 700, marginBottom: '0.5rem' }}>
              {modAction === 'hide' ? 'Take Down Event (Soft-Takedown)' : 'Restore Event to Public Surface'}
            </h2>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem', marginBottom: '1.5rem' }}>
              {modAction === 'hide'
                ? `Taking down this event will immediately remove it from all public APIs and client pages.`
                : `Restoring this event will make it visible to readers again.`}
            </p>

            <form onSubmit={handleModSubmit}>
              <div className="admin-form-group">
                <label className="admin-label">Action Rationale (Required)</label>
                <textarea
                  required
                  placeholder="Explain why this action is being taken..."
                  className="admin-textarea"
                  value={modReason}
                  onChange={(e) => setModReason(e.target.value)}
                />
              </div>

              <div style={{ display: 'flex', gap: '0.75rem', justifyContent: 'flex-end', marginTop: '1.5rem' }}>
                <button
                  type="button"
                  onClick={() => setModAction(null)}
                  className="btn-admin-secondary"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isModSubmitting || !modReason.trim()}
                  className={modAction === 'hide' ? 'btn-admin-danger' : 'btn-admin-success'}
                >
                  {isModSubmitting ? 'Recording...' : modAction === 'hide' ? 'Confirm Takedown' : 'Confirm Restore'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Modal: Detach Source */}
      {detachSource && (
        <div className="modal-backdrop">
          <div className="modal-card">
            <h2 style={{ fontSize: '1.25rem', fontWeight: 700, marginBottom: '0.5rem' }}>
              Detach Source Post
            </h2>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem', marginBottom: '1.5rem' }}>
              Detaching post #{detachSource.telegram_message_id} from {detachSource.channel_name} will remove it from this event cluster without deleting the raw post or taking down the event.
            </p>

            <form onSubmit={handleDetachSubmit}>
              <div className="admin-form-group">
                <label className="admin-label">Detachment Rationale (Required)</label>
                <textarea
                  required
                  placeholder="e.g. Unrelated topic accidentally clustered with this story..."
                  className="admin-textarea"
                  value={detachReason}
                  onChange={(e) => setDetachReason(e.target.value)}
                />
              </div>

              <div style={{ display: 'flex', gap: '0.75rem', justifyContent: 'flex-end', marginTop: '1.5rem' }}>
                <button
                  type="button"
                  onClick={() => setDetachSource(null)}
                  className="btn-admin-secondary"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isDetachSubmitting || !detachReason.trim()}
                  className="btn-admin-danger"
                >
                  {isDetachSubmitting ? 'Detaching...' : 'Confirm Detachment'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
