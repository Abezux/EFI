import './globals.css';
import React from 'react';
import type { Metadata } from 'next';
import Link from 'next/link';
import { Newspaper, Sparkles } from 'lucide-react';

export const metadata: Metadata = {
  title: 'Ethiopia Financial News Platform',
  description:
    'Real-time automated aggregation, verification, and AI-summarization of Ethiopian financial and economic news.',
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
