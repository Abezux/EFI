'use client';

import React from 'react';
import { AlertCircle } from 'lucide-react';

export default function ErrorBoundary({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <div
      style={{
        textAlign: 'center',
        padding: '5rem 1rem',
        maxWidth: '540px',
        margin: '0 auto',
      }}
    >
      <AlertCircle size={48} color="#ef4444" style={{ margin: '0 auto 1.5rem' }} />
      <h1
        style={{
          fontSize: '1.875rem',
          fontWeight: 800,
          marginBottom: '0.75rem',
          color: 'var(--text-primary)',
        }}
      >
        Something went wrong
      </h1>
      <p
        style={{
          color: 'var(--text-secondary)',
          fontSize: '0.9375rem',
          marginBottom: '2rem',
          lineHeight: 1.6,
        }}
      >
        {error.message ||
          'An unexpected error occurred while loading this page.'}
      </p>
      <button type="button" onClick={() => reset()} className="btn-retry">
        Try again
      </button>
    </div>
  );
}
