import React from 'react';
import { EventSource } from '@/lib/api';
import { Radio, ExternalLink } from 'lucide-react';
import { formatTimeAgo } from './EventCard';

interface SourceListProps {
  sources?: EventSource[];
}

export function SourceList({ sources }: SourceListProps) {
  if (!sources || sources.length === 0) {
    return (
      <section className="source-list-section">
        <h2 className="section-title">
          <Radio size={20} color="#3b82f6" />
          <span>Source Reports</span>
        </h2>
        <p style={{ color: 'var(--text-muted)', fontSize: '0.9375rem' }}>
          No source channel records attached.
        </p>
      </section>
    );
  }

  return (
    <section className="source-list-section" aria-labelledby="sources-heading">
      <h2 id="sources-heading" className="section-title">
        <Radio size={20} color="#3b82f6" />
        <span>Source Reports ({sources.length})</span>
      </h2>

      <div className="source-list">
        {sources.map((source, index) => {
          const telegramLink = source.channel_handle
            ? `https://t.me/${source.channel_handle.replace(/^@/, '')}`
            : null;

          return (
            <div
              key={`${source.channel_handle}-${source.posted_at}-${index}`}
              className="source-item"
              data-testid="source-item"
            >
              <div className="source-header">
                <div className="source-channel-info">
                  <span className="source-channel-name">{source.channel_name}</span>
                  {source.channel_handle && (
                    <span className="source-channel-handle">
                      @{source.channel_handle.replace(/^@/, '')}
                    </span>
                  )}
                </div>

                <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                  <time
                    dateTime={source.posted_at}
                    style={{ fontSize: '0.8125rem', color: 'var(--text-muted)' }}
                  >
                    {formatTimeAgo(source.posted_at)}
                  </time>
                  {telegramLink && (
                    <a
                      href={telegramLink}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="source-channel-handle"
                      title="View channel on Telegram"
                      style={{ display: 'flex', alignItems: 'center', gap: '0.2rem' }}
                    >
                      <ExternalLink size={14} />
                    </a>
                  )}
                </div>
              </div>

              <blockquote className="source-excerpt">
                &ldquo;{source.excerpt}&rdquo;
              </blockquote>
            </div>
          );
        })}
      </div>
    </section>
  );
}
