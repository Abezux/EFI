# Real-Time Ethiopia News Aggregation Platform
## Technical Architecture Specification & Development Roadmap (V0)

**Status:** Architecture/specification only — no implementation code included.
**Audience:** Human reviewer now, AI coding agent later.

---

## 0. Framing & Challenges to the Original Idea

Before the architecture, three assumptions in the brief need to be challenged directly, because they change downstream decisions:

1. **"Very fast" and "high accuracy" are in tension.** Semantic clustering and AI verification take time. If you optimize purely for latency, you will publish premature/wrong clusters (e.g., merging two unrelated fuel-price stories, or publishing three unmerged posts before clustering catches up). The realistic target is "fast, with a self-correcting event model" — publish immediately as individual posts, then *upgrade* into a merged event within seconds as clustering confirms it. This means your UI needs to support events whose source-count and content quietly grow after first render — a real design constraint, not a footnote.

2. **Telegram scraping without owning the channels is a legal and platform-policy risk, not just an engineering detail.** This needs a real answer, not a best-effort mention (see Section 12). It affects architecture: you need an audit trail proving each event's provenance, per-channel opt-out/removal capability, and a takedown pipeline from day one — these are not "V9 polish," they're V1 requirements once you're aggregating other people's content publicly.

3. **"Scale from 10 to 1,000 channels without a rewrite" and "avoid premature complexity" pull in opposite directions.** The resolution isn't picking one — it's designing clean seams (interfaces) between ingestion, processing, and storage now, while deploying the simplest implementation behind those seams at V1 (e.g., a single Postgres LISTEN/NOTIFY queue instead of Kafka). The seam is architecture; the implementation behind it is allowed to be humble.

---

## 1. Executive Architecture

```
[Telegram Channels (public, read-only)]
        │  MTProto client (Telethon/GramJS), one persistent session
        ▼
[Ingestion Service]  — normalizes raw message → RawPost, writes to DB, emits event
        │
        ▼
[Processing Pipeline] (async worker, pulls from queue)
   1. Idempotency check (channel_id + message_id)
   2. Text normalization (strip formatting, resolve entities, language detect)
   3. Exact/near-duplicate check (simhash / minhash over normalized text)
   4. Embedding generation (multilingual model — Amharic + English)
   5. Candidate event search (pgvector ANN search, time-windowed)
   6. Clustering decision:
        - high similarity → attach to existing event
        - low similarity → new event
        - borderline → queue for AI verification
   7. AI enrichment (summary, category, entities) — only after event is stable
   8. Publish/update event row, mark ready
        ▼
[PostgreSQL]  (events, source_posts, channels, categories, entities, media)
        ▼
[API layer] (REST + SSE stream)
        ▼
[Frontend] (Next.js, SSR for SEO, live updates via SSE)
```

Two loops matter architecturally:
- **Publish loop**: ingestion → processing → DB → API → frontend (the "fast" path).
- **Correction loop**: later posts can retroactively attach to an existing event, updating its source list and possibly its summary. The frontend must treat every event as "live" until some staleness threshold (e.g., no new sources in 30 minutes → considered settled).

---

## 2. Recommended Technology Stack (with reasoning)

The brief listed Go, PostgreSQL, pgvector, Redis, Kafka, Next.js, React, TypeScript, Docker, Cloudflare, object storage. Evaluated below — accepted, deferred, or replaced.

| Layer | Recommendation | Why | When it becomes necessary |
|---|---|---|---|
| Telegram listener | **Python + Telethon** (not Go) | Telegram's unofficial client ecosystem (Telethon, GramJS) is far more mature in Python/JS than in Go. This is the single highest-risk integration point (session bans, flood limits, reconnect edge cases) — use the ecosystem with the most battle-tested handling of those, not your preferred general-purpose language. | V1 |
| Ingestion/API service | **Go** | Good fit once you're past the Telegram-specific quirks: strong concurrency, low memory footprint, easy to containerize, good Postgres drivers. Accept the brief's instinct here. | V1 |
| Database | **PostgreSQL** | Correct choice. Relational integrity for events/sources/channels, mature tooling, and pgvector removes the need for a separate vector DB early on. | V1 |
| Vector search | **pgvector**, IVFFlat or HNSW index | Avoids operating a second database (Pinecone/Weaviate/Qdrant) for what is, at 10–100 channels, a small vector workload (thousands to tens of thousands of vectors/day). | V1. Reconsider a dedicated vector DB only if you cross ~10M+ vectors or need sub-10ms ANN at high QPS — i.e., "1,000 channels" territory, not V1. |
| Queue (V1) | **Postgres-based queue** (`FOR UPDATE SKIP LOCKED` pattern, or a lightweight lib like `river` in Go) | At 10 channels you will process maybe a few hundred messages/day. A dedicated broker is unjustified operational overhead. Using Postgres for the queue also means one fewer moving part to keep consistent with the DB. | V1 |
| Queue (later) | **Redis Streams**, then **Kafka** if needed | Redis Streams is the natural next step when you need higher throughput or multiple consumer groups without full Kafka operational cost. Kafka only when you have multiple independent processing pipelines/teams consuming the same stream, replay requirements, or throughput that genuinely stresses Postgres. | Redis: ~100 channels. Kafka: ~1,000 channels or multi-consumer fan-out needs. |
| Cache | **Redis** | Justified early for session/rate-limit state and hot-path caching (homepage, trending), not for queueing at V1. | V2–V3, when read traffic grows |
| AI / embeddings | Provider-agnostic interface; start with a hosted multilingual embedding model + a hosted LLM for enrichment (e.g., via API) | Don't self-host models at V1 — operational burden without payoff at this scale. Design the interface so providers are swappable. | V1 (interface), reassess self-hosting only at real scale/cost pressure |
| Frontend | **Next.js + React + TypeScript** | Correct for SEO (SSR/ISR), and React Server Components let you keep the "live-updating event" logic client-side while keeping first paint server-rendered for crawlers. | V1 (basic), full real-time V6+ |
| Real-time transport | **SSE**, not WebSockets | See Section 5 — one-directional server→client push is all this product needs. | V1 |
| Containerization | **Docker** + docker-compose for local/dev; a single orchestrator (start with a managed platform like Fly.io/Render/Railway, or plain Docker on a VM) before Kubernetes | Kubernetes at 10 channels is pure overhead. | Docker: V1. K8s: only when you have multiple independently-scaling services and an ops team to run it — likely post-V9. |
| CDN / edge | **Cloudflare** | Correct — free tier covers CDN, DDoS protection, and can later host edge caching rules for article pages. | V1 (basic proxy), full page-rule caching V8–V9 |
| Object storage | **S3-compatible** (Cloudflare R2 preferred — no egress fees, pairs with Cloudflare CDN) | For media (images/thumbnails pulled from Telegram posts). | V1, once media handling begins (likely V2–V3) |
| Observability | **OpenTelemetry** for tracing, **Prometheus + Grafana** or a hosted equivalent (Grafana Cloud) for metrics, structured JSON logs shipped to a log aggregator (Loki, or hosted) | Standard, vendor-neutral, avoids lock-in. | Basic logging/metrics: V1. Full tracing: V4+ once the pipeline has enough hops to need it. |

**Explicitly rejected/deferred:**
- **Kubernetes at V1** — operational cost far exceeds benefit at this scale.
- **Kafka at V1** — see above; Postgres queue is enough until throughput or fan-out demands it.
- **Microservices-per-pipeline-stage** — run the pipeline as a modular monolith (one Go service, internally organized into clear packages per stage) until a specific stage actually needs independent scaling (most likely: AI enrichment, which is the slowest and most rate-limited stage).

---

## 3. Database Architecture

Core entities and relationships:

```
channels
  id, telegram_channel_id, name, handle, category_default, is_active,
  trust_tier, added_at, last_seen_at

raw_posts                      -- immutable, one row per Telegram message
  id, channel_id (FK), telegram_message_id, raw_text, raw_entities (jsonb),
  media_refs (jsonb), posted_at (Telegram's timestamp), ingested_at,
  normalized_text, language, embedding (vector), processing_status,
  UNIQUE (channel_id, telegram_message_id)   -- idempotency

news_events
  id, canonical_title, ai_summary, category_id (FK), status
    (pending | clustering | verified | published | updated | archived),
  first_seen_at, last_updated_at, source_count, embedding_centroid (vector),
  is_breaking, slug (for SEO URLs)

event_sources                  -- join table: which raw_posts belong to which event
  event_id (FK), raw_post_id (FK), similarity_score, added_at
  UNIQUE (event_id, raw_post_id)

categories
  id, name, slug

entities                       -- named entities (people, places, orgs) for tagging/SEO
  id, name, type, slug

event_entities
  event_id (FK), entity_id (FK)

media
  id, raw_post_id (FK), storage_url, type, width, height, alt_text

users                          -- admin/moderator accounts only at V1; no public accounts needed yet
  id, email, role, created_at

processing_audit               -- append-only log of pipeline decisions per raw_post
  id, raw_post_id (FK), stage, decision, confidence, model_used, created_at
```

**Key design decisions:**
- `raw_posts` is **immutable** and never edited after ingestion — this is your legal/audit backbone (Section 12). Corrections happen by adding new rows/relations, never by mutating source text.
- `processing_status` on `raw_posts` and `status` on `news_events` are separate state machines — a post can be "clustered" while its parent event is still "pending" AI verification.
- Uniqueness constraint on `(channel_id, telegram_message_id)` is your idempotency guarantee against reconnect-driven re-ingestion.
- `embedding` and `embedding_centroid` use `pgvector`'s `vector` type; index with HNSW (better recall/speed tradeoff than IVFFlat for this scale) once you're past a few thousand rows — before that, sequential scan is fine and an index is premature.
- `processing_audit` exists specifically so an AI-generated classification can always be traced back to which model/version produced it — required for the "don't blindly trust AI" principle and for debugging clustering mistakes.
- Foreign keys with `ON DELETE RESTRICT` on `raw_posts → channels` (don't allow silent data loss); `ON DELETE CASCADE` on `event_sources` when an event is archived.

---

## 4. News Processing Pipeline (detailed)

1. **Raw ingestion** — Telegram listener writes a row to `raw_posts` with `processing_status = 'ingested'`. This write must succeed even if every downstream service is down — ingestion's only job is "don't lose the message."
2. **Idempotency** — enforced at the DB constraint level, not just application logic (reconnects *will* redeliver messages).
3. **Normalization** — strip Telegram markdown/entities, resolve forwarded-message headers, detect language (Amharic vs English vs mixed), store `normalized_text`.
4. **Exact/near-duplicate detection** — cheap first pass using simhash on normalized text; catches near-identical reposts/forwards cheaply before spending embedding-model calls.
5. **Embedding generation** — multilingual embedding model call; store vector.
6. **Candidate event search** — ANN search against `news_events.embedding_centroid`, time-windowed (e.g., only events from the last 48 hours — an old event about fuel prices from 3 months ago shouldn't match a new one).
7. **Clustering decision** — three-way branch:
   - similarity above high threshold → auto-attach, recompute centroid
   - similarity below low threshold → new event
   - **in between → queue for AI verification** rather than guessing. This is the honest way to handle the "same event, different wording" ambiguity the brief describes.
8. **AI verification** (only for the ambiguous band) — a cheap, targeted LLM call: "are these two texts describing the same real-world event?" Store the model's reasoning/confidence in `processing_audit`.
9. **AI enrichment** — once an event is stable (has ≥1 source, decision made), generate `ai_summary` and category/entity tags. This is explicitly separated from step 7/8's *matching* decision — enrichment failures should never block publication of the raw sources.
10. **Source attribution** — every event always carries its full `event_sources` list; the frontend must always be able to show "as reported by Channel A, Channel B, Channel C."
11. **Publication** — event flips to `published`; API layer notified (Postgres NOTIFY → ingestion of the SSE stream, or a lightweight pub/sub if load requires it later).
12. **Continuous updates** — new matching posts re-open a `published` event to `updated`, append to `event_sources`, and re-trigger enrichment if the new source adds materially new information (simple heuristic at V1: new source triggers re-summarization only if it's >X hours after the last one, to avoid re-summarizing on every near-duplicate).

**Distinguishing fact layers (explicit, not implicit):**
- `raw_posts.raw_text` — original source fact, never modified.
- `event_sources`, `event_entities` — derived metadata (algorithmic, not generative).
- `news_events.ai_summary` — AI-generated, always labeled as such in the UI, always shown alongside links to original sources.
- `processing_audit` — AI classification decisions, versioned by model, queryable for review.

This separation is what lets you say, credibly, "we don't blindly trust AI output" — every generated field is traceable to inputs and a specific model version, and originals are never overwritten.

---

## 5. Real-Time Architecture: SSE vs WebSockets

**Recommendation: Server-Sent Events (SSE).**

| | SSE | WebSockets |
|---|---|---|
| Direction needed | Server → client only (new events, event updates) | Bidirectional |
| Your actual need | Push-only: users don't send real-time data back | — |
| Infra complexity | Works over plain HTTP/1.1 or HTTP/2, reconnects automatically, plays well with CDNs/load balancers | Needs sticky sessions or a pub/sub backplane across instances; more moving parts |
| Browser support | Universal for this use case | Universal but heavier client code |
| Scaling pattern | Each instance can independently stream from Postgres LISTEN/NOTIFY or Redis pub/sub | Same, but connection state management is more complex |

WebSockets would only become worth the complexity if you later add features like live chat, collaborative moderation dashboards, or client-initiated real-time queries — none of which are in scope. SSE is the right tool for "push new/updated articles to the homepage."

**Latency budget (Telegram post → browser):**

| Stage | Target | Notes |
|---|---|---|
| Telegram → listener detects | <1–2s | Telegram push delivery is near-instant once connected |
| Ingestion write to DB | <200ms | Simple insert |
| Queue pickup by worker | <1s (V1, polling) → <100ms (V2+, LISTEN/NOTIFY or Redis) | Polling interval is the main lever at V1 |
| Normalization + dup check | <300ms | |
| Embedding generation | 200ms–1s | Depends on provider; this is your biggest controllable latency cost |
| Clustering decision | <100ms (DB ANN search) | |
| AI verification (only if ambiguous) | 1–3s | Only hits the ambiguous band, not every post |
| Publication write + notify | <100ms | |
| API → SSE → browser | <500ms | |
| **Total (clear-cut case)** | **~3–5 seconds** | |
| **Total (ambiguous, needs AI verification)** | **~5–8 seconds** | |

This is a realistic, defensible target — not instant, but genuinely "near-real-time," and it's honest about where the AI-verification path costs extra time.

---

## 6. SEO Architecture

- **Rendering**: SSR for article and category pages (Next.js `generateMetadata` + server components), ISR (incremental static regeneration) for pages that don't need per-second freshness (about, static pages); dynamic SSR for the homepage and breaking-news feed.
- **URLs**: `/news/{category}/{slug}-{short-id}` — human-readable, stable even if the title changes.
- **Canonical URLs**: every event page sets a canonical tag to itself; if an event's summary is later revised, the URL doesn't change (revisions update `last_updated_at`, not the slug).
- **Metadata**: dynamic `<title>`, `<meta description>` generated from `ai_summary` (truncated, clearly the AI-assisted summary — still fine for SEO, this is standard practice), Open Graph tags with a representative image from `media`.
- **Structured data**: `NewsArticle` schema.org JSON-LD per event page, including `datePublished`, `dateModified`, `author` (attributed to the platform, not fabricated bylines), and `citation`/`isBasedOn`-style linking to note it aggregates multiple sources — important for transparency and for avoiding misrepresentation to search engines.
- **Sitemap**: dynamically generated `sitemap.xml`, split by category and date if volume grows; a `news-sitemap.xml` following Google News sitemap conventions if you pursue Google News inclusion (see Section 12 — this has its own eligibility/policy requirements).
- **robots.txt**: standard, explicitly allow crawling of article/category pages, disallow admin routes.
- **Internal linking**: category pages link to events, event pages link to related events (via shared `entities`), breadcrumb navigation for crawlability.
- **Core Web Vitals**: SSR reduces LCP risk; keep the SSE connection and any client JS for "live updating" off the critical rendering path (hydrate after first paint); serve images via Cloudflare/R2 with responsive `srcset`.

---

## 7. Frontend Architecture

**Structure** (Bloomberg/Reuters-inspired density, own visual identity):
- Homepage: breaking-news banner (top, only when `is_breaking = true` events exist), latest-events feed (chronological, live-updating via SSE), trending sidebar (by view count or source-count velocity).
- Category pages: filtered event feed, same live-update behavior.
- Event/article page: canonical summary, full source list with channel names/links/timestamps (transparency requirement), related events, entity tags.
- Search: full-text search (Postgres `tsvector` at V1 — don't reach for Elasticsearch until you actually need faceted/fuzzy search at scale).
- Live-update UX: new events fade in at the top of the feed; existing events that gain a new source show a subtle "updated" badge — this directly reflects the "correction loop" from Section 0.
- Loading/error states: skeleton loaders for SSR hydration gaps; graceful fallback if SSE connection drops (auto-reconnect with backoff, and a visible "reconnecting" indicator rather than silently going stale).
- Accessibility: semantic HTML, ARIA live regions for the auto-updating feed (important — live-updating content without ARIA live regions is a real accessibility failure, not a nice-to-have).
- Mobile: mobile-first layout; the dense multi-column Bloomberg-style layout collapses to a single-column feed.
- Pagination: infinite scroll for the main feed, standard pagination for category archives (better for SEO crawlability than infinite scroll alone — pair infinite scroll with a paginated fallback route).

---

## 8. Security Architecture

- **Public site**: read-only, no user auth needed at V1 (no comments/accounts in scope).
- **Admin/moderation panel**: separate authenticated area (email+password or SSO), role-based (admin vs moderator), used for channel management, event corrections, and takedown requests.
- **API security**: rate limiting (per-IP, via Cloudflare or an app-level limiter) on public endpoints and the SSE endpoint; no write endpoints exposed publicly.
- **Telegram session security**: the listener's session credentials are the single most sensitive secret in the system (a compromised session can be used to impersonate your bot/account) — store via a secrets manager (not env files in plaintext in the repo), rotate access, run the listener in an isolated container with minimal permissions.
- **Input validation**: treat all Telegram message content as untrusted input — sanitize before storage/display (strip/escape any embedded HTML, validate media URLs before fetching, scan for injection in downstream AI prompts — i.e., don't naively interpolate raw post text into prompts without delimiting it, since a malicious channel could attempt prompt injection against your enrichment step).
- **Database security**: least-privilege DB roles (ingestion service can INSERT to `raw_posts` only; API service is read-mostly; admin panel has broader access), connections over TLS, regular backups.
- **Media security**: media pulled from Telegram is fetched server-side and re-hosted on your own object storage (never hotlinked) — this avoids exposing your infrastructure to arbitrary external content at request time and gives you control to remove media on takedown.
- **Logging/auditing**: admin actions (channel add/remove, manual event edits, takedowns) logged with actor + timestamp, immutable.

---

## 9. Observability Architecture

- **Structured logging**: JSON logs, one line per significant event (ingested, clustered, published, error), correlation ID threaded from raw_post through to published event so you can trace a single message's full journey.
- **Metrics** (Prometheus-style):
  - ingestion success/failure rate, per channel
  - end-to-end latency (post → published), as a histogram
  - duplicate rate, clustering-decision distribution (auto-attach / new-event / needs-AI-verification)
  - AI call success/failure/latency, per provider
  - queue depth / processing backlog
  - API latency (p50/p95/p99), SSE connection count
  - DB query performance (slow query log)
- **Tracing**: OpenTelemetry spans across ingestion → processing stages → publication, useful once the pipeline has enough hops that "why was this slow" isn't obvious from logs alone (introduce at V4+, not V1 — premature tracing infra adds cost before you have anything worth tracing at depth).
- **Health checks**: `/healthz` per service, checking DB connectivity and (for the listener) active Telegram session status.
- **Alerting**: Telegram listener disconnected >2 min, processing backlog above threshold, AI provider error rate spike, DB connection pool exhaustion.

---

## 10. Development Roadmap

| Version | Objective | Explicitly NOT built yet |
|---|---|---|
| **V0** | This document. Architecture agreed, repo scaffolded, ADRs started. | Any runtime code. |
| **V1** | Telegram ingestion: listener connects to ~5–10 channels, writes raw_posts reliably, survives reconnects, passes idempotency tests. | Clustering, AI, frontend, real-time. |
| **V2** | Processing + deduplication: normalization, exact/near-dup detection, basic non-semantic clustering (e.g., simple text similarity threshold only). Manual review tooling for the team to sanity-check clustering. | Embeddings/semantic clustering, AI enrichment. |
| **V3** | Semantic clustering: embeddings, pgvector ANN search, three-way clustering decision, ambiguous-band queue (verification stubbed/manual at first). | AI-generated summaries. |
| **V4** | AI enrichment: summaries, categorization, entity extraction, `processing_audit` fully wired, AI verification for ambiguous clustering goes live. Tracing introduced. | Public frontend. |
| **V5** | API layer: REST endpoints for events/categories/search, auth for admin routes, rate limiting. | Real-time push. |
| **V6** | Frontend V1: SSR homepage, category pages, event pages, search — polling-based "live" updates (simplest correct thing before SSE). | SSE. |
| **V7** | Real-time delivery: SSE endpoint, Postgres LISTEN/NOTIFY wiring, frontend live feed with ARIA live regions and reconnect handling. | CDN tuning, sitemap. |
| **V8** | SEO: structured data, sitemaps, OG tags, canonical URLs, Core Web Vitals pass. | Horizontal scaling infra. |
| **V9** | Production infrastructure: CI/CD, staging environment, backups, secrets management hardening, admin/moderation panel with takedown workflow, legal/attribution review (Section 12) actually implemented, not just planned. | Kafka/Redis Streams migration. |
| **V10** | Scaling: introduce Redis for caching/queue if load justifies it, revisit vector index strategy, load testing, horizontal scaling of the processing tier. | Kubernetes (only if genuinely justified by then). |

**Acceptance criteria pattern for every version** (apply this template per version): objective met and demoable; tests passing (unit for pipeline logic, integration for DB/queue interactions, at least one end-to-end test per version); no regression in prior versions' acceptance criteria; security review for anything touching credentials or public input; performance within the latency budget for whatever the version's scope covers; explicit sign-off that "not built yet" items were genuinely not touched (prevents scope creep by the coding agent).

---

## 11. Repository & AI-Coding-Agent Setup

```
/
├── AGENTS.md                 # instructions for the AI coding agent (see below)
├── docs/
│   ├── architecture/         # this spec, kept up to date
│   ├── adr/                  # Architecture Decision Records, one file per decision
│   └── runbooks/             # "Telegram session expired," "clustering looks wrong," etc.
├── services/
│   ├── listener/              # Python/Telethon, isolated — talks to Telegram + DB only
│   ├── processor/              # Go, the pipeline (stages as internal packages)
│   ├── api/                  # Go, REST + SSE
│   └── admin/                 # minimal internal tool, can be same binary as api at V1
├── web/                       # Next.js frontend
├── db/
│   └── migrations/            # versioned, one direction of change per migration
├── infra/                     # docker-compose (V1), IaC later
├── .github/workflows/         # CI: lint, test, build, on every PR
└── tests/
    ├── unit/
    ├── integration/
    └── e2e/
```

**AGENTS.md should specify:**
- The agent must read `docs/architecture/` and relevant ADRs before making any change that touches more than one service.
- The agent must not introduce a new infrastructure dependency (new DB, new queue, new external service) without creating an ADR proposing it first, even if asked to "just make it work."
- Every schema change goes through a migration file, never a manual `ALTER TABLE`.
- `raw_posts.raw_text` is immutable — no migration or code path may modify it after insert.
- Tests are required for any change to the processing pipeline's clustering logic (this is the highest-risk-of-silent-regression code in the system).
- Coding standards: Go — `gofmt` + `golangci-lint`; TypeScript — ESLint + Prettier; commit messages follow Conventional Commits for changelog generation.
- Branching: trunk-based with short-lived feature branches, PR required, CI must pass before merge.
- Local dev: `docker-compose up` should bring up Postgres + all services against a seeded fixture dataset (fake Telegram posts) so no agent needs real Telegram credentials to develop against most of the stack.

**ADRs** should be written for at minimum: choice of Postgres-queue-over-Kafka at V1, SSE-over-WebSockets, pgvector-over-dedicated-vector-DB, Python-listener/Go-processor language split, and the three-way clustering decision thresholds (and how those thresholds get tuned over time).

---

## 12. Risks, Legal/Policy Issues, and Failure Modes

**This needs to be a real answer, not a footnote — you're publicly republishing other people's content at scale.**

- **Telegram Terms of Service**: Telegram's ToS and API terms restrict automated bulk collection and republishing in some contexts, and using unofficial client libraries (Telethon/GramJS) via a user account rather than the official Bot API carries account-ban risk if usage looks abusive. This is a genuine legal/policy question, not just an engineering risk — get this reviewed (by yourself or ideally counsel familiar with Ethiopian and Telegram's applicable terms) before scaling past a handful of channels, not after.
- **Copyright**: republishing full post text from channels you don't own can constitute copyright infringement depending on jurisdiction and how much you reproduce. Architecturally, this pushes toward: store `raw_text` for internal processing/audit, but consider whether the *public-facing* article should show an AI summary + short excerpt + prominent link/attribution to the original channel, rather than the full original text verbatim. This is a product decision with legal weight, and it should be made explicitly, not by default.
- **Attribution**: always link back to and name the original channel — this is both an ethical baseline and a mitigating factor legally (transparency, not concealment, of sourcing).
- **Right to be forgotten / takedown**: a channel owner or a subject of a news event may request removal. You need an operational process (not just a technical one) for this — the admin panel's takedown workflow in V9 is not optional polish, it's a requirement once you're live.
- **Defamation risk**: AI-generated summaries could misrepresent a source post in a way that's defamatory, especially for sensitive political content in the Ethiopian context. The audit trail (Section 4) exists specifically so you can show what the source said versus what was generated, and correct/retract quickly.
- **Platform-policy risk of losing access**: if your listener account gets banned or a channel blocks you, that channel's ingestion silently stops — the observability plan's per-channel ingestion metrics (Section 9) need alerting on this specifically, or you'll have a stale-but-quiet failure.
- **Failure modes to design for explicitly**: Telegram disconnects (reconnect with backoff, resume from last known message ID per channel); worker crash mid-processing (idempotent re-processing, no partial-state corruption — this is why `raw_posts` and `news_events` have independent status fields); AI provider outage (pipeline should still publish raw sources without enrichment rather than blocking — "no enrichment yet" beats "no news at all"); DB failure (standard backups + point-in-time recovery; this is not optional for a public information product).

---

## 13. Cost Considerations (rough, V1 scale)

- Compute: a single small VM or managed-platform instance (e.g., Fly.io/Railway) easily handles listener + processor + API at 10 channels — likely under $50/month.
- Database: managed Postgres (e.g., Neon, Supabase, or a small RDS instance) — free tier to ~$25/month at this scale.
- AI/embeddings: the dominant variable cost, scales with post volume, not channel count directly. Budget per-1000-posts cost from your chosen provider before committing; this is worth an explicit ADR since it's your biggest ongoing line item.
- Object storage: Cloudflare R2's no-egress-fee model is cost-predictable for media.
- CDN: Cloudflare free tier covers V1 needs.
- **Net**: V1 is realistically a sub-$150/month operation; the AI enrichment cost is the line item to watch as channel/post volume grows, and it's the first thing worth optimizing (e.g., only re-summarizing on materially new sources, as already designed in Section 4).

---

## 14. Recommended V1 Scope (concrete, buildable now)

- 5–10 Telegram channels, Python/Telethon listener, writes to `raw_posts` with idempotency.
- Postgres schema for `channels` and `raw_posts` only (rest of schema exists as migrations but unused until V2+).
- Basic health check + structured logging.
- Docker-compose local dev environment with a fixture-data mode (no real Telegram credentials required to develop).
- CI running lint + unit tests on every PR.
- **No** clustering, AI, frontend, or public API yet — V1's only job is proving reliable, idempotent ingestion, which is the foundation everything else depends on.

---

## 15. Final Implementation Specification (handoff summary for the coding agent)

When you're ready to start implementation, give the coding agent:
1. This document.
2. The `AGENTS.md` (drafted per Section 11).
3. The initial migration files for `channels` and `raw_posts` (V1 scope only).
4. Explicit instruction: **implement V1 only**, per Section 10/14, and stop for review before starting V2 — do not let the agent "helpfully" continue into clustering or frontend work in the same pass.
5. A fixture dataset of realistic sample Telegram posts (including some genuine near-duplicates across "channels") so the agent can build and test the idempotency/dedup logic path even before real Telegram access is wired up.

---

*This is a living document — update it (and file an ADR) whenever a V1 decision above is revisited during implementation.*
