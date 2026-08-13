'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import {
  getAdminEvents,
  hideAdminEvent,
  restoreAdminEvent,
  AdminEventSummary,
} from '@/lib/api';
import { useAdminAuth } from '@/app/admin/AdminAuthContext';


export default function AdminEventsPage() {
  const { csrfToken } = useAdminAuth();
  const [events, setEvents] = useState<AdminEventSummary[]>([]);
  const [total, setTotal] = useState(0);
  const [limit] = useState(20);
  const [offset, setOffset] = useState(0);
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [hiddenFilter, setHiddenFilter] = useState<boolean | undefined>(undefined);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Moderation action modal state
  const [activeEvent, setActiveEvent] = useState<AdminEventSummary | null>(null);
  const [actionType, setActionType] = useState<'hide' | 'restore' | null>(null);
  const [reasonInput, setReasonInput] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const fetchEvents = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const res = await getAdminEvents({
        limit,
        offset,
        status: statusFilter || undefined,
        hidden: hiddenFilter,
      });
      setEvents(res.events);
      setTotal(res.total);
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to fetch events');
      }
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchEvents();
  }, [offset, statusFilter, hiddenFilter]);

  const handleActionSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!activeEvent || !actionType || !reasonInput.trim()) {
      setError('A valid rationale is required for moderation actions.');
      return;
    }

    setIsSubmitting(true);
    try {
      if (actionType === 'hide') {
        await hideAdminEvent(activeEvent.id, reasonInput.trim(), csrfToken);
      } else {
        await restoreAdminEvent(activeEvent.id, reasonInput.trim(), csrfToken);
      }
      setActiveEvent(null);
      setActionType(null);
      setReasonInput('');
      await fetchEvents();
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to perform moderation action');
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  const setFilterTab = (tab: 'all' | 'published' | 'hidden' | 'review') => {
    setOffset(0);
    switch (tab) {
      case 'all':
        setStatusFilter('');
        setHiddenFilter(undefined);
        break;
      case 'published':
        setStatusFilter('active');
        setHiddenFilter(false);
        break;
      case 'hidden':
        setStatusFilter('');
        setHiddenFilter(true);
        break;
      case 'review':
        setStatusFilter('needs_review');
        setHiddenFilter(undefined);
        break;
    }
  };

  const currentTab =
    hiddenFilter === true
      ? 'hidden'
      : statusFilter === 'needs_review'
      ? 'review'
      : statusFilter === 'active' && hiddenFilter === false
      ? 'published'
      : 'all';

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-title">Event Moderation & Takedowns</h1>
          <p className="admin-page-subtitle">
            Manage clustered stories, review Soft-Takedowns, and inspect attribution
          </p>
        </div>
      </div>

      {/* Filter Tabs */}
      <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1.5rem' }}>
        <button
          onClick={() => setFilterTab('all')}
          className={`btn-admin-secondary ${currentTab === 'all' ? 'active' : ''}`}
          style={{
            borderColor: currentTab === 'all' ? 'var(--accent-primary)' : 'var(--border-subtle)',
            color: currentTab === 'all' ? 'var(--text-primary)' : 'var(--text-secondary)',
          }}
        >
          All Events ({total})
        </button>
        <button
          onClick={() => setFilterTab('published')}
          className={`btn-admin-secondary ${currentTab === 'published' ? 'active' : ''}`}
          style={{
            borderColor: currentTab === 'published' ? 'var(--accent-success)' : 'var(--border-subtle)',
            color: currentTab === 'published' ? 'var(--text-primary)' : 'var(--text-secondary)',
          }}
        >
          🟢 Public Live
        </button>
        <button
          onClick={() => setFilterTab('hidden')}
          className={`btn-admin-secondary ${currentTab === 'hidden' ? 'active' : ''}`}
          style={{
            borderColor: currentTab === 'hidden' ? '#ef4444' : 'var(--border-subtle)',
            color: currentTab === 'hidden' ? 'var(--text-primary)' : 'var(--text-secondary)',
          }}
        >
          🚫 Soft-Taken Down (Hidden)
        </button>
        <button
          onClick={() => setFilterTab('review')}
          className={`btn-admin-secondary ${currentTab === 'review' ? 'active' : ''}`}
          style={{
            borderColor: currentTab === 'review' ? '#f59e0b' : 'var(--border-subtle)',
            color: currentTab === 'review' ? 'var(--text-primary)' : 'var(--text-secondary)',
          }}
        >
          ⚠️ Needs Review
        </button>
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

      <div className="admin-card" style={{ padding: 0, overflow: 'hidden' }}>
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
            <p style={{ color: 'var(--text-secondary)' }}>Loading event list...</p>
          </div>
        ) : (
          <table className="admin-table">
            <thead>
              <tr>
                <th>Event Title & AI Summary</th>
                <th>Category</th>
                <th>Sources</th>
                <th>Clustering</th>
                <th>Public Status</th>
                <th>Last Active</th>
                <th style={{ textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {events.map((ev) => (
                <tr key={ev.id}>
                  <td style={{ maxWidth: '400px' }}>
                    <Link
                      href={`/admin/events/${ev.id}`}
                      style={{ fontWeight: 600, color: 'var(--accent-primary)', display: 'block', marginBottom: '0.25rem' }}
                    >
                      {ev.ai_headline || ev.canonical_title}
                    </Link>
                    <p
                      style={{
                        fontSize: '0.8125rem',
                        color: 'var(--text-secondary)',
                        whiteSpace: 'nowrap',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                      }}
                    >
                      {ev.ai_summary || ev.canonical_title}
                    </p>
                    {ev.is_hidden && ev.last_moderation_reason && (
                      <div style={{ fontSize: '0.75rem', color: '#ef4444', marginTop: '0.25rem' }}>
                        <strong>Moderation Note:</strong> {ev.last_moderation_reason}
                      </div>
                    )}
                  </td>
                  <td>
                    {ev.category ? (
                      <span className="status-badge" style={{ background: 'var(--bg-surface)', border: '1px solid var(--border-subtle)' }}>
                        {ev.category.name}
                      </span>
                    ) : (
                      <span style={{ color: 'var(--text-muted)' }}>—</span>
                    )}
                  </td>
                  <td style={{ fontWeight: 600 }}>{ev.source_count}</td>
                  <td>
                    <span className={`status-badge ${ev.status === 'active' ? 'active' : 'review'}`}>
                      {ev.status}
                    </span>
                  </td>
                  <td>
                    <span className={`status-badge ${ev.is_hidden ? 'hidden' : 'active'}`}>
                      {ev.is_hidden ? 'Hidden (Taken Down)' : 'Published'}
                    </span>
                  </td>
                  <td style={{ color: 'var(--text-secondary)', fontSize: '0.8125rem' }}>
                    {new Date(ev.last_updated_at || ev.first_seen_at).toLocaleDateString()}
                  </td>
                  <td style={{ textAlign: 'right' }}>
                    <div style={{ display: 'inline-flex', gap: '0.5rem' }}>
                      <Link
                        href={`/admin/events/${ev.id}`}
                        className="btn-admin-secondary"
                        style={{ padding: '0.35rem 0.65rem' }}
                      >
                        Inspect
                      </Link>
                      {ev.is_hidden ? (
                        <button
                          onClick={() => {
                            setActiveEvent(ev);
                            setActionType('restore');
                            setReasonInput('');
                          }}
                          className="btn-admin-success"
                        >
                          Restore
                        </button>
                      ) : (
                        <button
                          onClick={() => {
                            setActiveEvent(ev);
                            setActionType('hide');
                            setReasonInput('');
                          }}
                          className="btn-admin-danger"
                        >
                          Take Down
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
              {events.length === 0 && (
                <tr>
                  <td colSpan={7} style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-secondary)' }}>
                    No events match the selected filter.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        )}
      </div>

      {/* Pagination */}
      {total > limit && (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: '1rem' }}>
          <span style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
            Showing {offset + 1}–{Math.min(offset + limit, total)} of {total} events
          </span>
          <div style={{ display: 'flex', gap: '0.5rem' }}>
            <button
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - limit))}
              className="btn-admin-secondary"
            >
              Previous
            </button>
            <button
              disabled={offset + limit >= total}
              onClick={() => setOffset(offset + limit)}
              className="btn-admin-secondary"
            >
              Next
            </button>
          </div>
        </div>
      )}

      {/* Modal: Takedown / Restore */}
      {activeEvent && actionType && (
        <div className="modal-backdrop">
          <div className="modal-card">
            <h2 style={{ fontSize: '1.25rem', fontWeight: 700, marginBottom: '0.5rem' }}>
              {actionType === 'hide' ? 'Take Down Event (Soft-Takedown)' : 'Restore Event to Public Surface'}
            </h2>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem', marginBottom: '1.5rem' }}>
              {actionType === 'hide'
                ? `Taking down "${activeEvent.canonical_title}" will immediately remove it from all public REST APIs, search feeds, and real-time streams. It remains preserved in the admin dashboard for compliance audits.`
                : `Restoring "${activeEvent.canonical_title}" will make it visible to readers again.`}
            </p>

            <form onSubmit={handleActionSubmit}>
              <div className="admin-form-group">
                <label className="admin-label">Rationale / Justification (Required for Audit Trail)</label>
                <textarea
                  required
                  placeholder={
                    actionType === 'hide'
                      ? 'e.g. Legal defamation notice, copyright infringement, or erroneous source fabrication...'
                      : 'e.g. Cleared on verified official clarification...'
                  }
                  className="admin-textarea"
                  value={reasonInput}
                  onChange={(e) => setReasonInput(e.target.value)}
                />
              </div>

              <div style={{ display: 'flex', gap: '0.75rem', justifyContent: 'flex-end', marginTop: '1.5rem' }}>
                <button
                  type="button"
                  onClick={() => {
                    setActiveEvent(null);
                    setActionType(null);
                  }}
                  className="btn-admin-secondary"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isSubmitting || !reasonInput.trim()}
                  className={actionType === 'hide' ? 'btn-admin-danger' : 'btn-admin-success'}
                >
                  {isSubmitting ? 'Recording...' : actionType === 'hide' ? 'Confirm Takedown' : 'Confirm Restoration'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
