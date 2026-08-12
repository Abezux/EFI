import React from 'react';
import Link from 'next/link';
import { HelpCircle } from 'lucide-react';

export default function NotFound() {
  return (
    <div
      style={{
        textAlign: 'center',
        padding: '5rem 1rem',
        maxWidth: '500px',
        margin: '0 auto',
      }}
    >
      <HelpCircle size={48} color="#3b82f6" style={{ margin: '0 auto 1.5rem' }} />
      <h1
        style={{
          fontSize: '2rem',
          fontWeight: 800,
          marginBottom: '0.75rem',
          color: 'var(--text-primary)',
        }}
      >
        404 — Not Found
      </h1>
      <p
        style={{
          color: 'var(--text-secondary)',
          fontSize: '1rem',
          marginBottom: '2rem',
          lineHeight: 1.6,
        }}
      >
        The requested news event or topic could not be found or has not been
        published yet.
      </p>
      <Link href="/" className="btn-retry">
        Back to Feed
      </Link>
    </div>
  );
}
