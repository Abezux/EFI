# V8 — SEO
### Ethiopia News Aggregation Platform

**Scope note:** This document defines V8 only. It builds on V0–V7 (foundation through real-time delivery, plus the V4.1 enrichment revision and V3.1 clustering fix — all live-verified). It references — but does not repeat — `docs/architecture/platform-architecture.md` (Section 6) and `v6-frontend-spec.md`. V8's job: make the existing SSR'd pages actually discoverable and correctly represented to search engines and social platforms — structured data, sitemaps, canonical URLs, Open Graph, and a Core Web Vitals pass. **No changes to clustering, enrichment, real-time delivery, or page functionality** — this version only adds metadata and URL structure on top of what already works.

---

## 1. V8 SPEC

### 1.1 What V8 Adds to the Repository

```
/
├── web/
│   ├── app/
│   │   ├── news/[category]/[slug]/page.tsx   # NEW canonical event URL structure
│   │   ├── events/[id]/page.tsx               # UPDATED: redirects to canonical /news/ URL
│   │   ├── sitemap.xml/route.ts                # dynamic sitemap
│   │   ├── robots.txt/route.ts                 # dynamic robots.txt
│   │   └── layout.tsx                          # UPDATED: default OG/meta tags
│   ├── components/
│   │   └── StructuredData.tsx                  # NewsArticle JSON-LD renderer
│   └── lib/
│       └── seo.ts                               # slug helpers, meta-tag builders
├── services/
│   └── api/
│       └── store.go                             # UPDATED: expose slug in event responses
├── docs/
│   └── adr/
│       └── 0013-url-structure-and-sitemap-strategy.md
└── db/
    └── migrations/
        └── 0009_event_slugs.sql
```

### 1.2 Functional Scope (architecture doc Section 6, made real)

- **Canonical URL structure**: `/news/{category-slug}/{event-slug}-{event-id}`, matching the architecture doc's original pattern exactly. `event-slug` is generated once, at first enrichment, from `ai_headline` (falling back to `canonical_title` if headline generation ever failed) — human-readable, stable.
- **Slug immutability**: once set, a slug **never changes**, even if the event is later re-enriched and `ai_headline` changes (per V4/V7's re-enrichment triggers). The URL stays stable for anyone who's already linked to it; only the *rendered* headline on the page updates. This is the architecture doc's canonical-URL requirement made concrete.
- **Old `/events/{id}` URLs redirect** (HTTP 301) to the new canonical `/news/...` form — V6/V7 already-shared links keep working, search engines get a clear signal about which URL is authoritative.
- **`NewsArticle` structured data** (JSON-LD) on every event page: `headline` (`ai_headline`), `datePublished` (`first_seen_at`), `dateModified` (`last_updated_at`), `mainEntityOfPage` (the canonical URL), `publisher` (your brand/organization, not a fabricated person), and a `citation`/`isBasedOn`-style reference to the source channels — making the multi-source aggregation nature machine-readable, not just visible in the UI, consistent with the architecture doc's transparency requirement carried all the way through to how search engines see the content.
- **Open Graph + Twitter Card tags** on every page: `og:title`, `og:description` (a bounded excerpt of `ai_summary` — this is your own generated text, not raw source text, so the copyright caution that shaped V5's source-excerpt limit doesn't apply here the same way, though still keep it within normal meta-description length, ~150–160 characters), `og:type=article`, `og:url` (canonical), `twitter:card` (summary card; `summary_large_image` only if/when the platform actually has representative images — it doesn't yet, per the architecture doc's later `media` table being out of scope so far).
- **`sitemap.xml`**: dynamically generated, listing all published (`status = 'active'`, not `needs_review`) event URLs plus category pages, with `lastmod` from `last_updated_at`. Single sitemap file at V8's expected volume — splitting into multiple sitemap files by date/category is explicitly deferred until real volume requires it (per the architecture doc's "avoid premature complexity" guidance, reaffirmed here).
- **`robots.txt`**: allow crawling of all public pages; disallow nothing at V8 since no admin routes exist yet — but structured so a future `/admin` disallow rule (V9) is a trivial addition, not a rework.
- **Category pages** get their own meta description and canonical tag (already SSR'd since V6; this version adds the metadata layer on top).
- **Core Web Vitals pass**: confirm the LCP element on each page type is server-rendered text (not client-fetched), no layout-shift-causing unstyled content flashes, and that `LiveFeedUpdater`'s SSE connection doesn't block or delay initial render (it shouldn't, given V7's design, but this is the version that actually verifies it with real measurement).

### 1.3 Why Slug Generation Happens at Enrichment Time, Not at Serve Time

Generating the slug once, when `ai_headline` is first produced, and storing it — rather than deriving it fresh from the current headline every time a page is requested — is what makes the canonical URL actually canonical. If the slug were derived live from the current headline, a re-enrichment (which V4/V7 can trigger) would silently change every event's URL, breaking every link anyone had already shared or that search engines had already indexed. Storing it once and never touching it again is the only way to honor the architecture doc's canonical-URL requirement given that headlines *can* legitimately change after publication.

### 1.4 AGENTS.md Updates

Append a V8-specific section:
- Slugs are generated once and stored — no code path may regenerate or overwrite an existing `news_events.slug` value, even during re-enrichment. A PR that touches slug generation outside the initial-creation path should be treated as a bug, not a feature.
- All meta tags (title, description, OG, JSON-LD) must be rendered server-side, in the initial HTML response — never injected client-side only via `useEffect` or similar, since that's invisible to most crawlers. This is a hard SEO requirement, not a style preference, and worth a specific test (1.9) rather than trusting it by convention alone.
- `NewsArticle` structured data must only ever reflect real data already in the API response — no inventing values (e.g., don't fabricate an `author` person, don't guess at an `image` that doesn't exist) to make the structured data look more complete than it is.

### 1.5 ADR: URL Structure & Sitemap Strategy (0013)

- **Context**: the architecture doc specified the `/news/{category}/{slug}-{id}` pattern back in the original spec (Section 6) without it being implemented until now; a decision on sitemap splitting and Google News eligibility needs recording.
- **Decision**: single `sitemap.xml` at V8's scale (splitting deferred, per 1.2); slugs generated once at enrichment time and immutable thereafter (per 1.3); Google News-specific sitemap format (`news-sitemap.xml`) and formal Google News Publisher Center submission are **noted as a real future option but explicitly not pursued in V8** — that program has its own eligibility/editorial-standards review process external to this codebase, worth a deliberate decision later rather than folding into this version's scope.
- **Consequences**: the 301-redirect from old `/events/{id}` URLs to canonical `/news/...` URLs needs to remain in place indefinitely (or at least for a long transition period) — removing it later would break any links already indexed or shared under the old pattern.

### 1.6 Database Changes

New migration `0009_event_slugs.sql`:
```sql
ALTER TABLE news_events ADD COLUMN slug TEXT;
CREATE UNIQUE INDEX idx_news_events_slug ON news_events(slug) WHERE slug IS NOT NULL;
-- Nullable + partial unique index: existing events get backfilled (one-time script,
-- generating slugs from their current ai_headline/canonical_title, same rule as
-- new events going forward); any future event without ai_headline yet falls back
-- to a slug derived from canonical_title rather than staying null indefinitely.
```

### 1.7 Reliability Scope

- If slug generation ever produces a collision (two events with very similar headlines), append a short disambiguating suffix (the existing `event-id` at the end of the URL already guarantees uniqueness in practice, but the human-readable slug portion itself should still avoid being identical across events where reasonably avoidable — note this as a minor polish item, not a blocking requirement, since the trailing id already prevents any actual routing ambiguity).
- Sitemap generation must not fail or time out as event volume grows — since it's dynamically generated per request at V8 (no caching yet), note in the ADR that this is fine at current scale and worth revisiting (e.g., periodic regeneration + caching) once volume grows enough that live generation becomes slow — a real, deferred scaling concern, not solved now.

### 1.8 Testing

- Test confirming a slug, once generated, does not change even after a simulated re-enrichment that changes `ai_headline`.
- Test confirming the old `/events/{id}` route correctly 301-redirects to the canonical `/news/...` URL.
- Test confirming meta tags and JSON-LD are present in the raw server-rendered HTML response (not just present after client hydration) — fetch the page server-side in the test and assert on the raw HTML, not the hydrated DOM.
- Test confirming `sitemap.xml` is well-formed XML and includes only `active` events, never `needs_review` ones (same data-leak discipline established back in V5).
- Manual Lighthouse/Core Web Vitals check on real live pages — this is not meaningfully automatable at this scope and should be treated as a real manual acceptance step, not skipped for lack of a CI equivalent.

### 1.9 CI Updates

Extend the existing Node web job with the new test files (redirect behavior, SSR meta-tag presence, sitemap validity).

### 1.10 Logging

Minimal addition — log sitemap generation requests at a coarse level (useful to notice if a crawler is hammering it) but no new structured pipeline-decision logging needed here, consistent with V6's precedent that this layer doesn't carry pipeline-audit weight the backend services do.

### 1.11 V8 Acceptance Criteria

- [ ] Every event page reachable at its canonical `/news/{category}/{slug}-{id}` URL, with the old `/events/{id}` URL correctly redirecting.
- [ ] A real event's slug confirmed stable across a real or simulated re-enrichment — manually verified, not just tested in isolation.
- [ ] `NewsArticle` JSON-LD present and valid on a real event page — verified with Google's Rich Results Test or the schema.org validator against a real live URL, not just "renders without erroring."
- [ ] Open Graph tags verified by actually testing a real URL against a social preview debugger (e.g., Facebook's Sharing Debugger, or Twitter/X's card validator) — confirming it actually looks right when shared, not just that the tags exist in the HTML.
- [ ] `sitemap.xml` accessible, well-formed, contains only published events — manually opened and spot-checked.
- [ ] `robots.txt` accessible and correctly permissive.
- [ ] Meta tags/JSON-LD confirmed present in raw server-rendered HTML (view-source, not devtools-inspected-after-hydration) on at least homepage, a category page, and an event page.
- [ ] A real Lighthouse run on a real live page reviewed by you — Core Web Vitals scores noted, not necessarily perfect, but no glaring regression from V6/V7 and no SSE-connection-blocking-render issue found.
- [ ] ADR-0013 exists.
- [ ] `AGENTS.md` V8 section exists per 1.4.
- [ ] CI passes.
- [ ] No changes to clustering, enrichment, real-time, or API business logic beyond exposing `slug` in responses.

---

## 2. V8 IMPLEMENTATION PLAN

**Step 1 — Migration `0009_event_slugs.sql` + backfill**
- Creates: nullable `slug` column, partial unique index, one-time backfill script generating slugs for all existing events from their current `ai_headline`/`canonical_title`.
- Purpose: schema and existing-data foundation before any URL/routing changes.
- Checks: migration applies cleanly; backfill confirmed to have populated every existing event; no duplicate slugs (partial unique index would catch this, but confirm the backfill script itself handles collisions gracefully rather than crashing).
- Expected result: every event, old and new, has a stable slug.

**Step 2 — Slug generation on event creation (`services/processor/`)**
- Creates: slug-generation logic wired into the existing enrichment write path — generated once, when `ai_headline` is first set, never touched again afterward.
- Purpose: ensure the immutability property (1.3) holds for all future events, not just the backfilled ones.
- Checks: test confirming re-enrichment does not alter an already-set slug.
- Expected result: proven immutable slug generation going forward.

**Step 3 — API: expose slug**
- Creates: `slug` added to the existing event response shape in `services/api/store.go` and handlers.
- Purpose: make the frontend able to build canonical URLs.
- Checks: existing API tests updated/extended to cover the new field; confirm no unrelated response shape changes.
- Expected result: `slug` available via the existing, unchanged endpoints.

**Step 4 — Canonical route + redirect (`web/app/news/[category]/[slug]/page.tsx`)**
- Creates: the new canonical route (parsing the trailing event-id from the URL segment, fetching via the existing REST endpoint, verifying/redirecting if the slug portion doesn't match current canonical form); update `app/events/[id]/page.tsx` to issue a 301 to the canonical URL.
- Purpose: the actual URL-structure upgrade.
- Checks: redirect test per 1.8; manual click-through confirming old links still resolve correctly.
- Expected result: working canonical URLs with proper legacy redirect behavior.

**Step 5 — `StructuredData.tsx`, JSON-LD wiring**
- Creates: the `NewsArticle` JSON-LD component, rendered server-side on event pages using only real available data (1.4).
- Purpose: machine-readable structured data.
- Checks: SSR-presence test (1.8); real validation against Google's Rich Results Test on a live deployed/tunneled URL (or local, if the validator tooling can reach it) — this needs a real external check, not just internal test passage.
- Expected result: valid, verified structured data.

**Step 6 — Meta tags: OG, Twitter Card, canonical, description**
- Creates: `lib/seo.ts` helpers, wired into `layout.tsx` and each page type (homepage, category, event, search).
- Purpose: correct social-preview and search-snippet behavior.
- Checks: SSR-presence test; real social-debugger checks (1.11) on at least one real event URL.
- Expected result: pages that render correctly when shared on real platforms, verified by hand.

**Step 7 — `sitemap.xml` and `robots.txt`**
- Creates: the dynamic route handlers per 1.2.
- Purpose: crawlability and discovery.
- Checks: XML validity test; `needs_review` exclusion test (same pattern as V5's leak test); manual fetch and read-through.
- Expected result: correct, complete, safe sitemap and robots files.

**Step 8 — Core Web Vitals pass**
- Creates: any fixes needed based on real Lighthouse findings (e.g., render-blocking resources, layout shift sources) — scope of actual changes here depends on what the real measurement turns up, not predetermined.
- Purpose: confirm V6/V7's SSR foundation is actually performing well now that it's under real SEO scrutiny.
- Checks: real Lighthouse run, reviewed by you.
- Expected result: a documented baseline Core Web Vitals score, with any clear/easy wins addressed.

**Step 9 — ADR-0013 + AGENTS.md V8 section**
- Creates: the ADR and addendum per 1.5/1.4.
- Purpose: document before declaring done.
- Checks: manual review.
- Expected result: documented URL/sitemap decisions for V9+ to build on.

**Step 10 — Final review against V8 acceptance criteria**
- Purpose: verify, don't assume — and specifically, run the real external validators (Rich Results Test, social debuggers, Lighthouse) yourself, since these are the actual proof this version works, not internal test output.
- Checks: walk every item in 1.11.
- Expected result: V8 acceptance criteria fully met; ready to hand off to V9.

---

## 3. V8 ANTIGRAVITY PROMPT

```
Read docs/architecture/platform-architecture.md, v6-frontend-spec.md, and
v8-seo-spec.md (repo root) before doing anything else. The last of these
contains the full V8 spec, implementation plan, and your exact instructions —
follow it as written, in order. Follow AGENTS.md, including all prior version
sections and the V8 addendum you will create.

Inspect the current repository state first — V0 through V7, plus the V4.1
enrichment revision and V3.1 clustering fix, are implemented, committed, and
pushed. Do not touch clustering, enrichment, verification, or real-time
delivery logic — V8 only adds SEO metadata and URL structure on top of
already-working pages.

Implement V8 per sections 1 and 2 of the spec: canonical /news/{category}/
{slug}-{id} URLs with slugs generated ONCE at enrichment time and NEVER
regenerated (this immutability is the single most important property in this
version — a slug must survive re-enrichment unchanged, verify this with an
explicit test), a 301 redirect from the old /events/{id} URLs, NewsArticle
JSON-LD structured data, Open Graph/Twitter Card tags, a dynamic sitemap.xml
(active events only, same needs_review exclusion discipline as V5's API), and
robots.txt.

CRITICAL: all meta tags and structured data must be present in the raw
server-rendered HTML, not injected client-side only — write an explicit test
that checks the raw SSR HTML response, not the post-hydration DOM.

Do NOT implement: Google News-specific sitemap format or Publisher Center
submission (explicitly deferred per ADR-0013), sitemap splitting/pagination
(single file is correct at this scale), or any change to business logic in
the processor or API beyond exposing the new slug field.

Work through the implementation plan step by step, running tests/lint/build
checks after each step. Several steps require real external verification I
must do myself, not something you can fully verify alone — flag these clearly:
Step 5 (Google Rich Results Test against a real URL), Step 6 (real social
preview debuggers), and Step 8 (real Lighthouse run). Show me what to check
and how, rather than declaring these done from internal tests alone.

Do not introduce any new dependency or infrastructure beyond what's specified
without first writing an ADR and pausing for confirmation. If anything is
ambiguous, stop and ask rather than guessing.

When finished:
- Run the full test suite, lint, and build.
- Show me: a real event's canonical URL and confirm the old /events/{id}
  link redirects correctly; the raw view-source HTML of a real event page
  showing the JSON-LD and meta tags present server-side; the real
  sitemap.xml content; and clear instructions for me to run the Rich Results
  Test, a social debugger, and Lighthouse myself against real URLs.
- Report what was created/changed and confirm no clustering/enrichment/
  real-time logic was touched.
- Stop. Do not continue into V9 without explicit instruction.
```

---

## V8 → V9 Handoff

V8 leaves in place: stable, canonical, SEO-correct URLs with verified structured data and social preview behavior, a working sitemap/robots setup, and a documented Core Web Vitals baseline.

V9 (production infrastructure, per the architecture doc's roadmap) can build directly on this: CI/CD deployment pipelines, staging environment, secrets management hardening (moving past V1's ADR-0005 single-environment-variable approach, per its own noted revisit trigger), backups, and — critically, per the architecture doc's Section 12 — the admin/moderation panel with a real takedown workflow, which has been a known, explicitly-flagged requirement since the very first architecture document and hasn't been built yet. V9 should not need to touch V8's URL structure or SEO metadata, though the takedown workflow will need to interact with the same `status` field and slug system already in place.
