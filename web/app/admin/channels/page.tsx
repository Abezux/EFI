'use client';

import React, { useEffect, useState } from 'react';
import {
  getAdminChannels,
  addAdminChannel,
  toggleAdminChannel,
  AdminChannel,
} from '@/lib/api';
import { useAdminAuth } from '@/app/admin/AdminAuthContext';


export default function AdminChannelsPage() {
  const { user, csrfToken } = useAdminAuth();
  const [channels, setChannels] = useState<AdminChannel[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Add channel modal state
  const [showAddModal, setShowAddModal] = useState(false);
  const [channelIdInput, setChannelIdInput] = useState('');
  const [channelNameInput, setChannelNameInput] = useState('');
  const [channelHandleInput, setChannelHandleInput] = useState('');
  const [addReasonInput, setAddReasonInput] = useState('');
  const [isAdding, setIsAdding] = useState(false);

  // Toggle channel modal state
  const [toggleTarget, setToggleTarget] = useState<AdminChannel | null>(null);
  const [toggleReasonInput, setToggleReasonInput] = useState('');
  const [isToggling, setIsToggling] = useState(false);

  const fetchChannels = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const res = await getAdminChannels();
      setChannels(res.channels);
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to fetch channels');
      }
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchChannels();
  }, []);

  const handleAddSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const tgID = parseInt(channelIdInput, 10);
    if (isNaN(tgID) || !channelNameInput.trim()) {
      setError('Please provide a valid numeric Telegram ID and Name.');
      return;
    }

    setIsAdding(true);
    try {
      await addAdminChannel({
        telegram_channel_id: tgID,
        name: channelNameInput.trim(),
        handle: channelHandleInput.trim(),
        reason: addReasonInput.trim(),
        csrfToken,
      });
      setShowAddModal(false);
      setChannelIdInput('');
      setChannelNameInput('');
      setChannelHandleInput('');
      setAddReasonInput('');
      await fetchChannels();
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to add channel');
      }
    } finally {
      setIsAdding(false);
    }
  };

  const handleToggleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!toggleTarget) return;

    setIsToggling(true);
    try {
      await toggleAdminChannel(toggleTarget.id, toggleReasonInput.trim(), csrfToken);
      setToggleTarget(null);
      setToggleReasonInput('');
      await fetchChannels();
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to toggle channel status');
      }
    } finally {
      setIsToggling(false);
    }
  };

  const isAdmin = user?.role === 'admin';

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-title">Channel Management</h1>
          <p className="admin-page-subtitle">
            Configure Telegram source feeds monitored by the ingestion listener
          </p>
        </div>
        {isAdmin && (
          <button
            onClick={() => setShowAddModal(true)}
            className="btn-admin-primary"
          >
            + Add New Channel
          </button>
        )}
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
            <p style={{ color: 'var(--text-secondary)' }}>Loading channel inventory...</p>
          </div>
        ) : (
          <table className="admin-table">
            <thead>
              <tr>
                <th>Channel Name</th>
                <th>Handle</th>
                <th>Telegram ID</th>
                <th>Status</th>
                <th>Posts Ingested</th>
                <th>Last Active</th>
                {isAdmin && <th style={{ textAlign: 'right' }}>Actions</th>}
              </tr>
            </thead>
            <tbody>
              {channels.map((ch) => (
                <tr key={ch.id}>
                  <td>
                    <span style={{ fontWeight: 600 }}>{ch.name}</span>
                  </td>
                  <td>
                    {ch.handle ? (
                      <span style={{ color: 'var(--text-secondary)' }}>@{ch.handle}</span>
                    ) : (
                      <span style={{ color: 'var(--text-muted)' }}>—</span>
                    )}
                  </td>
                  <td style={{ fontFamily: 'monospace', color: 'var(--text-secondary)' }}>
                    {ch.telegram_channel_id}
                  </td>
                  <td>
                    <span className={`status-badge ${ch.is_active ? 'active' : 'inactive'}`}>
                      {ch.is_active ? 'Active' : 'Paused'}
                    </span>
                  </td>
                  <td style={{ fontWeight: 600 }}>{ch.post_count}</td>
                  <td style={{ color: 'var(--text-secondary)', fontSize: '0.8125rem' }}>
                    {ch.last_seen_at
                      ? new Date(ch.last_seen_at).toLocaleString()
                      : 'Never'}
                  </td>
                  {isAdmin && (
                    <td style={{ textAlign: 'right' }}>
                      <button
                        onClick={() => {
                          setToggleTarget(ch);
                          setToggleReasonInput('');
                        }}
                        className={ch.is_active ? 'btn-admin-danger' : 'btn-admin-success'}
                      >
                        {ch.is_active ? 'Pause Feed' : 'Resume Feed'}
                      </button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Modal: Add Channel */}
      {showAddModal && (
        <div className="modal-backdrop">
          <div className="modal-card">
            <h2 style={{ fontSize: '1.25rem', fontWeight: 700, marginBottom: '0.5rem' }}>
              Register Telegram Channel
            </h2>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem', marginBottom: '1.5rem' }}>
              Add a public Telegram channel for continuous ingestion.
            </p>

            <form onSubmit={handleAddSubmit}>
              <div className="admin-form-group">
                <label className="admin-label">Telegram Channel ID</label>
                <input
                  type="number"
                  required
                  placeholder="-1001234567890"
                  className="admin-input"
                  value={channelIdInput}
                  onChange={(e) => setChannelIdInput(e.target.value)}
                />
              </div>

              <div className="admin-form-group">
                <label className="admin-label">Channel Display Name</label>
                <input
                  type="text"
                  required
                  placeholder="Fana Broadcasting Corporate"
                  className="admin-input"
                  value={channelNameInput}
                  onChange={(e) => setChannelNameInput(e.target.value)}
                />
              </div>

              <div className="admin-form-group">
                <label className="admin-label">Handle (without @)</label>
                <input
                  type="text"
                  placeholder="fanatelevision"
                  className="admin-input"
                  value={channelHandleInput}
                  onChange={(e) => setChannelHandleInput(e.target.value)}
                />
              </div>

              <div className="admin-form-group">
                <label className="admin-label">Addition Rationale (Audit Log)</label>
                <input
                  type="text"
                  placeholder="Primary national news broadcaster"
                  className="admin-input"
                  value={addReasonInput}
                  onChange={(e) => setAddReasonInput(e.target.value)}
                />
              </div>

              <div style={{ display: 'flex', gap: '0.75rem', justifyContent: 'flex-end', marginTop: '1.5rem' }}>
                <button
                  type="button"
                  onClick={() => setShowAddModal(false)}
                  className="btn-admin-secondary"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isAdding}
                  className="btn-admin-primary"
                >
                  {isAdding ? 'Registering...' : 'Save Channel'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Modal: Toggle Channel */}
      {toggleTarget && (
        <div className="modal-backdrop">
          <div className="modal-card">
            <h2 style={{ fontSize: '1.25rem', fontWeight: 700, marginBottom: '0.5rem' }}>
              {toggleTarget.is_active ? 'Pause Channel Ingestion' : 'Resume Channel Ingestion'}
            </h2>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem', marginBottom: '1.5rem' }}>
              {toggleTarget.is_active
                ? `Pausing ${toggleTarget.name} will immediately stop ingestion of new posts from this channel.`
                : `Resuming ${toggleTarget.name} will re-enable message ingestion for this channel.`}
            </p>

            <form onSubmit={handleToggleSubmit}>
              <div className="admin-form-group">
                <label className="admin-label">Rationale for Status Change (Required for Audit)</label>
                <textarea
                  required
                  placeholder="Explain reason for pause/resume..."
                  className="admin-textarea"
                  value={toggleReasonInput}
                  onChange={(e) => setToggleReasonInput(e.target.value)}
                />
              </div>

              <div style={{ display: 'flex', gap: '0.75rem', justifyContent: 'flex-end', marginTop: '1.5rem' }}>
                <button
                  type="button"
                  onClick={() => setToggleTarget(null)}
                  className="btn-admin-secondary"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isToggling || !toggleReasonInput.trim()}
                  className={toggleTarget.is_active ? 'btn-admin-danger' : 'btn-admin-success'}
                >
                  {isToggling ? 'Updating...' : toggleTarget.is_active ? 'Confirm Pause' : 'Confirm Resume'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
