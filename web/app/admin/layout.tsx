'use client';

import React from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { AdminAuthProvider, useAdminAuth } from '@/app/admin/AdminAuthContext';


function AdminNav() {
  const pathname = usePathname();
  const { user, logout } = useAdminAuth();

  if (pathname === '/admin/login') {
    return null;
  }

  const navItems = [
    { label: 'Overview', href: '/admin' },
    { label: 'Channels', href: '/admin/channels' },
    { label: 'Events & Takedowns', href: '/admin/events' },
    { label: 'Review Queue', href: '/admin/review-queue' },
  ];

  return (
    <header className="admin-nav">
      <div className="container admin-nav-inner">
        <div style={{ display: 'flex', alignItems: 'center', gap: '2rem' }}>
          <Link href="/admin" className="admin-brand">
            <span>EFI Platform</span>
            <span className="admin-badge">Admin</span>
          </Link>

          <nav className="admin-links">
            {navItems.map((item) => {
              const isActive =
                item.href === '/admin'
                  ? pathname === '/admin'
                  : pathname.startsWith(item.href);
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={`admin-link ${isActive ? 'active' : ''}`}
                >
                  {item.label}
                </Link>
              );
            })}
          </nav>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
          {user && (
            <div className="admin-user-pill">
              <span style={{ color: 'var(--text-secondary)' }}>{user.email}</span>
              <span
                style={{
                  background: user.role === 'admin' ? 'rgba(59, 130, 246, 0.2)' : 'rgba(16, 185, 129, 0.2)',
                  color: user.role === 'admin' ? 'var(--accent-primary)' : 'var(--accent-success)',
                  padding: '0.1rem 0.4rem',
                  borderRadius: 'var(--radius-sm)',
                  fontSize: '0.7rem',
                  fontWeight: 600,
                  textTransform: 'uppercase',
                }}
              >
                {user.role}
              </span>
              <button
                onClick={() => logout()}
                className="admin-logout-btn"
                title="Logout"
              >
                Sign Out
              </button>
            </div>
          )}
          <Link
            href="/"
            target="_blank"
            className="btn-admin-secondary"
            style={{ display: 'inline-flex', alignItems: 'center', gap: '0.25rem' }}
          >
            Public Site ↗
          </Link>
        </div>
      </div>
    </header>
  );
}

function AdminLayoutContent({ children }: { children: React.ReactNode }) {
  const { isLoading, user } = useAdminAuth();
  const pathname = usePathname();

  if (isLoading && pathname !== '/admin/login') {
    return (
      <div
        style={{
          minHeight: '100vh',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: 'var(--bg-primary)',
        }}
      >
        <div style={{ textAlign: 'center' }}>
          <div
            style={{
              width: '40px',
              height: '40px',
              border: '3px solid var(--border-subtle)',
              borderTopColor: 'var(--accent-primary)',
              borderRadius: '50%',
              animation: 'spin 1s linear infinite',
              margin: '0 auto 1rem',
            }}
          />
          <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
            Verifying administrative session...
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="admin-layout">
      <AdminNav />
      <main className="admin-main">
        <div className="container">{children}</div>
      </main>
    </div>
  );
}

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <AdminAuthProvider>
      <AdminLayoutContent>{children}</AdminLayoutContent>
    </AdminAuthProvider>
  );
}
