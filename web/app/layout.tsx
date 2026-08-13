import './globals.css';
import React from 'react';
import type { Metadata } from 'next';
import Link from 'next/link';
import { Newspaper, Sparkles } from 'lucide-react';

import { DEFAULT_SITE_URL, SITE_NAME, DEFAULT_DESCRIPTION } from '@/lib/seo';

export const metadata: Metadata = {
  metadataBase: new URL(DEFAULT_SITE_URL),
  title: {
    default: `${SITE_NAME} — Verified Real-Time Economic Intelligence`,
    template: `%s | ${SITE_NAME}`,
  },
  description: DEFAULT_DESCRIPTION,
  openGraph: {
    type: 'website',
    siteName: SITE_NAME,
    title: `${SITE_NAME} — Verified Real-Time Economic Intelligence`,
    description: DEFAULT_DESCRIPTION,
    url: DEFAULT_SITE_URL,
  },
  twitter: {
    card: 'summary_large_image',
    title: `${SITE_NAME} — Verified Real-Time Economic Intelligence`,
    description: DEFAULT_DESCRIPTION,
  },
  robots: {
    index: true,
    follow: true,
  },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>
        <header className="header-wrapper">
          <div className="container header-content">
            <Link href="/" className="logo-link">
              <Newspaper size={24} color="#3b82f6" />
              <span>Ethiopia News</span>
              <span className="logo-badge">EFI</span>
            </Link>

            <nav className="nav-links">
              <Link href="/" className="nav-link">
                Feed
              </Link>
              <Link href="/search" className="nav-link">
                Search
              </Link>
            </nav>
          </div>
        </header>

        <main className="main-wrapper">
          <div className="container">{children}</div>
        </main>

        <footer className="footer-wrapper">
          <div className="container footer-content">
            <p>© 2026 Ethiopia Financial News Aggregation Platform. All rights reserved.</p>
            <div className="footer-badge">
              <Sparkles size={14} color="#8b5cf6" />
              <span>AI-Verified News Intelligence</span>
            </div>
          </div>
        </footer>
      </body>
    </html>
  );
}
