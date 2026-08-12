# ADR-0010: Next.js App Router Server-Side Rendering and Hybrid Frontend Architecture

## Status
Accepted

## Context
In V6 (Web Frontend), the Ethiopia News Aggregation Platform requires a modern, responsive, and SEO-friendly web frontend to present aggregated, categorized, and AI-summarized Ethiopian financial news to public users.

Key requirements and constraints:
1. **SEO & Initial Page Load Speed**: News content, category indexes, and individual event reports must be fast, crawlable, and fully rendered on the server before client-side hydration.
2. **Dual Execution Contexts**: The frontend operates in two distinct network contexts:
   - Server-side (Node.js runtime inside container networks / SSR), which must connect to the internal API address (e.g. `http://api:8080`).
   - Client-side (Browser runtime), which connects via the public API URL (e.g. `http://localhost:8080` or public domain).
3. **AI Transparency & Attribution Invariant**: Every AI-synthesized summary must visibly feature an accessible badge indicating AI generation, and source articles must cite original Telegram channels with bounded excerpts.
4. **Resilient Failure Handling**: Backend outages must not crash the web frontend. SSR should render clean fallback error banners, while live client updates must degrade silently without disruptive alerts.

## Decision
We choose **Next.js 14 with App Router (React Server Components)** using **TypeScript**, **Vanilla CSS design tokens**, and a **dual-context API client (`web/lib/api.ts`)**.

### Rationale
1. **Server-Side Rendering (SSR)**:
   - Dynamic server components (`export const dynamic = 'force-dynamic'`) fetch current news feeds at request time, delivering fully rendered HTML to users and search engines with zero client-side layout shifts.
2. **Dual-Context API Client**:
   - `getApiBaseUrl(isServer)` automatically distinguishes between SSR requests (using `INTERNAL_API_URL`) and browser-side client requests (using `NEXT_PUBLIC_API_URL`).
3. **Accessible AI Transparency Components**:
   - Reusable `AiBadge` and `EventCard` components enforce visual indicators whenever `ai_summary_generated: true`, fulfilling core platform trust invariants.
4. **Graceful Error Handling & Silent Degradation**:
   - Server-rendered pages show helpful retry banners when the API is temporarily unreachable. Background update mechanisms degrade silently with exponential backoff on transient network failures.

## Consequences
- **Build & Packaging**: Next.js standalone output (`output: 'standalone'`) produces optimized, lightweight Docker container images.
- **Testing**: Frontend logic and components are verified using Jest and React Testing Library across unit and integration test suites.
