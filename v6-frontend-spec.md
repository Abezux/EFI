# V6 — Frontend (V1)
### Ethiopia News Aggregation Platform

**Scope note:** This document defines V6 only. It builds on V0–V5 (foundation through the public read-only API, all live-verified). It references — but does not repeat — `docs/architecture/platform-architecture.md`. V6's job: a working Next.js frontend consuming V5's API — homepage, category pages, event detail pages, search — with simple polling-based "live" updates. **No SSE yet** (that's V7 — this version deliberately uses the simplest correct approach first, per the architecture doc's roadmap note). No deep SEO work yet (structured data, sitemaps — V8, though basic SSR/semantic HTML is in scope since it's free at this stage).

---

## 1. V6 SPEC

### 1.1 What V6 Adds to the Repository

```
/
├── web/
│   ├── app/
│   │   ├── layout.tsx                    # root layout, nav, footer
│   │   ├── page.tsx                       # homepage: latest events feed
│   │   ├── category/[slug]/page.tsx       # category-filtered feed
│   │   ├── events/[id]/page.tsx           # event detail
│   │   ├── search/page.tsx                # search results
│   │   └── error.tsx / not-found.tsx      # error/404 states
│   ├── components/
│   │   ├── EventCard.tsx                  # feed list item
│   │   ├── EventDetail.tsx                # full event view
│   │   ├── SourceList.tsx                 # attribution list w/ excerpts
│   │   ├── CategoryNav.tsx
│   │   ├── SearchBar.tsx
│   │   ├── LiveFeedUpdater.tsx             # client component: polling logic
│   │   └── LoadingSkeleton.tsx / ErrorState.tsx
│   ├── lib/
│   │   └── api.ts                          # typed fetch wrapper for the V5 API
│   ├── package.json, tsconfig.json, next.config.js
│   └── __tests__/                          # component tests
├── docs/
│   └── adr/
│       └── 0010-frontend-rendering-and-live-update-strategy.md
└── infra/
    └── docker-compose.yml                   # add web service
```

First appearance of `web/` in the repo — no other directory changes.

### 1.2 Functional Scope (architecture doc Section 7, minus SSE/deep SEO)

- **Homepage**: SSR'd list of latest published events (via `GET /api/v1/events`), newest-first, each rendered as an `EventCard` (title, category badge, source count, time). Client-side polling (`LiveFeedUpdater`, 1.3) checks for newer events periodically and prepends them without a full page reload.
- **Category pages** (`/category/[slug]`): same feed pattern, filtered via the API's `category` query param. SSR'd for direct-link/crawlability, same polling behavior.
- **Event detail page** (`/events/[id]`): full `EventDetail` view — `canonical_title`, the AI summary **explicitly labeled** using the API's `ai_summary_generated` flag (e.g., a visible "AI-generated summary" tag — this is the first place the architecture doc's Section 0/4 transparency principle actually reaches a human reader, not just a database field), category, entities, and the full `SourceList` (channel name, timestamp, bounded excerpt, and a link to the original Telegram post where derivable) — this is the attribution requirement made real.
- **Search page** (`/search`): a search box hitting `GET /api/v1/search`, results rendered the same way as the feed.
- **Loading/error states**: skeleton loaders during SSR hydration gaps; a clear, non-crashing error state if the API is unreachable (per V5's own reliability requirement — the frontend must degrade gracefully to match, not assume the API is always up).
- **Pagination**: basic paginated feed (page numbers or "load more" hitting `limit`/`offset`) — the architecture doc's fuller infinite-scroll-with-paginated-fallback pattern is a V8 SEO refinement; a working, simple paginated feed is the correct V6 scope.
- **Mobile responsive**: single-column layout on small screens, matching the architecture doc's mobile-first requirement.
- **Accessibility**: semantic HTML throughout; the polling-updated feed uses an ARIA live region (`aria-live="polite"`) so new events are announced to assistive tech — this is a real accessibility requirement, not a nice-to-have, per the architecture doc's Section 7 note.

### 1.3 Live Updates: Polling, Not SSE (this version's deliberate simplification)

- `LiveFeedUpdater` is a small client component that polls `GET /api/v1/events?since=<timestamp>` on an interval (e.g., every 30–60 seconds — configurable, not hardcoded without a documented reason) and prepends any genuinely new events to the feed with a subtle visual entrance (not a jarring reload), matching the "correction loop" UX described in the original architecture doc's Section 0.
- This deliberately does **not** attempt to also reflect V4's "existing event gained a new source" update case (the architecture doc's `updated` events) — polling for brand-new events only is the correct, bounded V6 scope. Refreshing an already-rendered event's source count live is a nice V7 (SSE) enhancement, not a V6 requirement — note this explicitly rather than half-implementing it.
- No WebSocket, no SSE connection anywhere in V6 — this is intentionally the "simplest correct thing before SSE" step the architecture doc's roadmap calls for.

### 1.4 AGENTS.md Updates

Append a V6-specific section:
- All data fetching goes through `lib/api.ts` — no component calls `fetch()` directly against the API, so the API's base URL and response typing live in exactly one place.
- Two API URL environment variables, not one, and they are not interchangeable: an internal URL for server-side rendering (reachable within the docker network, e.g. `http://api:8080`) and a public URL for client-side polling (reachable from the browser, e.g. `http://localhost:8080` in dev, a real public host later) — using the wrong one in the wrong context is a real bug class here, flag it clearly in code comments.
- `ai_summary_generated` must always be checked before rendering `ai_summary` — no component may render that field without also rendering its generated-content label. This is a direct extension of the transparency requirement already enforced at the API layer in V5; the agent must not treat it as optional polish at the frontend layer.
- Frontend code follows: TypeScript strict mode, ESLint + Prettier, component tests via React Testing Library for anything with real logic (the polling updater, the AI-label rendering) — not for every presentational component.

### 1.5 ADR: Frontend Rendering & Live-Update Strategy (0010)

- **Context**: the architecture doc calls for SSR (SEO, Section 6) and eventually real-time updates (Section 5), but V7 (SSE) isn't built yet.
- **Decision**: Next.js App Router with server components for initial page loads (SSR, good for SEO groundwork even though full SEO work is V8), and a small client component (`LiveFeedUpdater`) for polling-based freshness on top. No SSE/WebSocket connection in this version.
- **Consequences**: users get "close to live" (poll-interval-bounded) rather than instant updates until V7 lands; this is an accepted, documented V6 limitation, not a regression — V7's job is specifically to replace the polling mechanism, not to add a new one alongside it.

### 1.6 Database Changes

None. V6 only consumes the existing V5 API.

### 1.7 Security Baseline (V6-specific)

- No secrets in frontend code — the only environment values needed are the two API base URLs (1.4), and neither is sensitive (the API is already public per V5).
- All rendered content passes through React's default escaping — no `dangerouslySetInnerHTML` anywhere in V6 unless a specific, reviewed reason requires it (there shouldn't be one at this scope — flag it if the agent reaches for it).
- Search query input goes straight to the API as a query param — no client-side construction of raw SQL or HTML from user input (the API layer already owns input validation per V5; the frontend's job is just not to introduce a new injection surface of its own, e.g., reflecting the raw query string back into the page without escaping — which React handles by default as long as it's rendered normally, not injected via `dangerouslySetInnerHTML`).

### 1.8 Reliability Scope

- If the API is unreachable at SSR time, the page renders a clear error state server-side (not a Next.js crash page) — test this explicitly by pointing the frontend at a stopped API.
- If the API becomes unreachable *during* client-side polling, `LiveFeedUpdater` should fail silently (stop polling or back off) rather than surfacing a disruptive error on top of an otherwise-working page the user is currently reading — a difference in user experience between "the page won't load" (loud) and "live updates paused" (quiet) that's worth getting right.

### 1.9 Testing

- Component tests for `LiveFeedUpdater` (mocked API responses: new events appear, no new events → no change, API failure → silent degradation per 1.8).
- Component tests confirming `EventDetail` never renders `ai_summary` without its generated-content label, for both `ai_summary_generated: true` and (hypothetically) `false` inputs.
- A build-time check (`next build` succeeding, plus `tsc --noEmit`) as the primary "does this compile and typecheck correctly" gate — full end-to-end browser testing (Playwright etc.) is reasonable to defer past V6 given the scope, but note this as a explicit, accepted gap rather than silently skipping it.
- Manual review (not automated) is the primary acceptance mechanism for V6, more than any prior version — visual/UX quality isn't something a test suite judges well, and the spec should not pretend otherwise.

### 1.10 CI Updates

Add a Node/Next.js job to `ci.yml`: install deps, `eslint`, `tsc --noEmit`, `next build`, run component tests. No deployment step yet.

### 1.11 Logging

Minimal at this layer — rely on Next.js's own server-side request logging in dev/prod, no custom structured logging needed for a V6-scope frontend (this is a deliberate simplification, unlike every backend service so far — flag it as such rather than over-building observability for a service that doesn't have pipeline decisions to audit).

### 1.12 V6 Acceptance Criteria

- [ ] `web/` exists per 1.1, runs locally via docker-compose, builds successfully (`next build`).
- [ ] Homepage renders real, current events from the live API — manually reviewed by you in a browser, not just confirmed to compile.
- [ ] Category pages correctly filter — manually spot-check at least two categories against real data.
- [ ] Event detail page shows a real event with: the AI summary **visibly labeled** as generated, real sources with real channel names and excerpts, and (if present) real entities — manually reviewed for whether it actually looks right, not just renders without erroring.
- [ ] Search returns real, correct results for a real query against real data.
- [ ] Polling updater demonstrated live: post something new (or wait for real channel activity), observe it appear in the homepage feed without a manual refresh, within the configured poll interval.
- [ ] ARIA live region present and correctly configured on the polling feed — spot-checked (e.g., via browser accessibility inspector, not just present in the JSX).
- [ ] Mobile responsive layout manually checked at a phone-width viewport.
- [ ] API-down error state manually verified (stop the api service, load a page, confirm a clean error rather than a crash).
- [ ] No `ai_summary` rendered anywhere without its generated-content label — verified by test (1.9) and spot-checked visually.
- [ ] ADR-0010 exists.
- [ ] `AGENTS.md` V6 section exists per 1.4.
- [ ] CI passes.
- [ ] No SSE/WebSocket code, no structured-data/sitemap/SEO-specific work beyond basic SSR, and no admin functionality exists anywhere in the repo.

---

## 2. V6 IMPLEMENTATION PLAN

**Step 1 — Scaffold `web/`**
- Creates: Next.js App Router project (TypeScript, ESLint), `lib/api.ts` with typed fetch functions matching V5's exact response shapes, the two-URL env setup (1.4).
- Purpose: establish the project and its one API-access seam before any UI is built.
- Checks: `next build` succeeds on the empty scaffold; `lib/api.ts` functions have basic unit tests against a mocked fetch.
- Expected result: an installable, buildable, empty frontend correctly wired to real API types.

**Step 2 — `EventCard`, homepage (`app/page.tsx`)**
- Creates: the feed list item component and the SSR homepage using it, fetching real data server-side.
- Purpose: the first real, visible page.
- Checks: manual review in a browser against real live data — this is the point where you first actually see your own news platform rendering real content.
- Expected result: a working, real homepage.

**Step 3 — `CategoryNav`, category pages**
- Creates: category navigation component, `app/category/[slug]/page.tsx`.
- Purpose: extend the feed pattern to filtered views.
- Checks: manual spot-check across at least two real categories.
- Expected result: working category filtering.

**Step 4 — `SourceList`, `EventDetail`, event detail page**
- Creates: the attribution component and full detail view, including the mandatory AI-summary label per 1.2/1.4.
- Purpose: the page that makes the architecture doc's transparency principle real for an actual reader — treat this as the most important page in V6.
- Checks: the AI-label test (1.9) plus manual review of a real event page — confirm sources, excerpts, and the generated-content label all look correct and honest.
- Expected result: a working, transparent event detail page.

**Step 5 — `SearchBar`, search page**
- Creates: search input and results page.
- Purpose: complete the core navigation surface.
- Checks: manual test with a few real queries against real content (including at least one Amharic query, matching V5's search capability).
- Expected result: working search.

**Step 6 — `LiveFeedUpdater`: polling**
- Creates: the client-side polling component per 1.3, wired into the homepage (and optionally category pages).
- Purpose: the "live" part of V6, correctly bounded per 1.3's scope note.
- Checks: component tests (1.9) plus a real manual observation — leave a page open, get a new post through the pipeline, watch it appear without refreshing.
- Expected result: proven, correctly-scoped live updates.

**Step 7 — Loading/error states, accessibility, mobile pass**
- Creates: skeleton loaders, `error.tsx`/`not-found.tsx`, ARIA live region wiring, responsive CSS pass across all pages.
- Purpose: the polish that makes V6 actually usable, not just functional.
- Checks: manual review at mobile viewport width; manual API-down test (1.8); accessibility spot-check on the live region.
- Expected result: a frontend that degrades gracefully and works on a phone.

**Step 8 — docker-compose + CI**
- Creates: `web` service added to `infra/docker-compose.yml`; Node job added to `ci.yml`.
- Purpose: make the frontend part of the standard local-dev and CI loop alongside all four backend services.
- Checks: `docker-compose up` brings up all five services; CI green on a clean PR.
- Expected result: the full stack running together via one command.

**Step 9 — ADR-0010 + AGENTS.md V6 section**
- Creates: the ADR and addendum per 1.5/1.4.
- Purpose: document before declaring done.
- Checks: manual review.
- Expected result: documented rendering/live-update decisions for V7 to build on.

**Step 10 — Final review against V6 acceptance criteria**
- Purpose: verify, don't assume — and specifically, spend real time actually browsing your own site before signing off, since this version is judged more by feel than by test output.
- Checks: walk every item in 1.12.
- Expected result: V6 acceptance criteria fully met; ready to hand off to V7.

---

## 3. V6 ANTIGRAVITY PROMPT

```
Read docs/architecture/platform-architecture.md, v0-foundation-spec.md,
v1-ingestion-spec.md, v2-processing-spec.md, v3-semantic-clustering-spec.md,
v4-ai-enrichment-spec.md, v5-public-api-spec.md, and v6-frontend-spec.md
(repo root) before doing anything else. The last of these contains the full V6
spec, implementation plan, and your exact instructions — follow it as written,
in order. Follow AGENTS.md, including all prior version sections and the V6
addendum you will create.

Inspect the current repository state first — V0 through V5 are implemented,
committed, and pushed, including a working public REST API at /api/v1 serving
real enriched news events with real sources, categories, and AI summaries.
Do not recreate or modify V0-V5 code — V6 only adds a new web/ directory that
consumes the existing API.

Implement V6 per sections 1 and 2 of the spec: a Next.js (App Router,
TypeScript) frontend with a homepage, category pages, event detail pages, and
search, all server-rendered and consuming the V5 API through a single typed
client (lib/api.ts) — no component should call fetch() directly.

CRITICAL transparency requirement: no component may render ai_summary without
also rendering a visible "AI-generated summary" label, driven by the API's
ai_summary_generated field. Write this as an explicit component test, not just
an implied behavior.

Live updates in V6 are polling-based only (LiveFeedUpdater, per section 1.3) —
do NOT implement SSE or WebSockets, that is explicitly V7's job. Polling should
only surface brand-new events, not live-refresh an existing event's source
count — note that boundary in a comment rather than half-implementing the
broader case.

Use two separate API URL environment variables — one for server-side
rendering (internal docker-network address) and one for client-side polling
(public-facing address) — and be precise about which is used where; mixing
them up is a real bug class here.

Handle API-down gracefully at both SSR time (clean error page, not a crash)
and during client-side polling (silent degradation, not a disruptive error on
an otherwise-working page) — these are different failure modes needing
different handling, per section 1.8.

Do NOT implement: SSE/WebSockets, structured data/sitemaps/deep SEO work
(that's V8), or any admin/write functionality. Basic SSR and semantic HTML are
in scope since they're free at this stage; deep SEO polish is not.

Work through the implementation plan step by step, running lint/typecheck/build
checks after each step. Manual review is the primary acceptance mechanism for
this version more than any prior one — after each page-building step (2, 3,
4, 5), stop and show me what you built rather than continuing silently, since
visual/UX quality needs my eyes, not just a passing build.

Do not introduce any new dependency or infrastructure beyond what's specified
without first writing an ADR and pausing for confirmation. If anything is
ambiguous, stop and ask rather than guessing.

When finished:
- Run the full test suite, lint, typecheck, and build; confirm docker-compose
  brings up all five services together.
- Walk me through the actual pages — homepage, a category page, an event
  detail page with real sources and a labeled AI summary, and search results
  — so I can review them visually, plus confirm the polling updater and the
  API-down error states work as described.
- Report what was created/changed and confirm no V7+ functionality (SSE,
  deep SEO, admin) was implemented.
- Stop. Do not continue into V7 without explicit instruction.
```

---

## V6 → V7 Handoff

V6 leaves in place: a working, SSR'd, publicly-browsable frontend covering the homepage, category pages, event detail, and search, with a correctly-scoped polling-based freshness mechanism and a properly enforced AI-transparency label on every summary shown.

V7 (real-time delivery, per the architecture doc's roadmap) can build directly on this: add the SSE endpoint to the API (per the architecture doc's Section 5 recommendation, already justified back in the original architecture spec), and **replace** `LiveFeedUpdater`'s polling logic with an SSE subscription — including, this time, the fuller "existing event gained a new source" live-update case that V6 explicitly deferred. V7 should not need to touch the SSR page structure, `EventDetail`, `SourceList`, or the API's existing REST endpoints — those are stable V6 output; V7's job is specifically upgrading the delivery mechanism underneath the already-correct UI.
