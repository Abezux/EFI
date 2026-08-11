# V5 — Public API
### Ethiopia News Aggregation Platform

**Scope note:** This document defines V5 only. It builds on V0–V4 (foundation through AI enrichment, all live-verified). It references — but does not repeat — `docs/architecture/platform-architecture.md`. V5's job: expose the now-enriched `news_events` data through a read-only REST API. No real-time push (V7), no frontend (V6), no admin/write endpoints (V9). This is also the **first version where the system becomes publicly network-reachable** — treat that as the central fact of this version, not an afterthought.

---

## 1. V5 SPEC

### 1.1 What V5 Adds to the Repository

```
/
├── services/
│   └── api/
│       ├── main.go                # entrypoint: HTTP server, route registration
│       ├── routes.go              # route definitions
│       ├── handlers_events.go     # GET /api/v1/events, GET /api/v1/events/{id}
│       ├── handlers_categories.go # GET /api/v1/categories
│       ├── handlers_search.go     # GET /api/v1/search
│       ├── handlers_health.go     # GET /healthz
│       ├── middleware.go          # rate limiting, request logging, CORS, recovery
│       ├── store.go               # read-only DB access layer (separate from processor's store.go)
│       ├── config.go              # env loading (DB conn, port, rate limit settings)
│       ├── go.mod / go.sum
│       └── *_test.go              # per-handler tests
├── docs/
│   └── adr/
│       └── 0009-api-routing-and-versioning.md
├── db/
│   └── migrations/
│       └── 0006_api_read_role.sql  # least-privilege read-only DB role for the API
└── infra/
    └── docker-compose.yml           # add api service, expose its port
```

Still no `web/` frontend directory. This version only adds the API service.

### 1.2 Functional Scope (architecture doc Section 2, V5 row; Section 7's data needs, read-only)

**Endpoints** (all under `/api/v1`, per ADR-0009's versioning decision):

- `GET /events` — paginated list of published events (`status = 'active'`, i.e. not `needs_review`), newest-first by default. Query params: `category` (slug filter), `limit`/`offset` (bounded — e.g., max `limit=50`), `since` (ISO timestamp, for "what's new since I last checked" — useful groundwork for V6/V7 even though nothing consumes it live yet).
- `GET /events/{id}` — single event detail: `canonical_title`, `ai_summary` (explicitly flagged — see 1.3), `category`, `entities`, `source_count`, `first_seen_at`, `last_updated_at`, and the **full `event_sources` list** with each source's originating channel name/handle and a link back to the original Telegram post where derivable — this is the transparency/attribution requirement from the architecture doc's Section 12, made concrete for the first time.
- `GET /categories` — the fixed category list from V4's migration, each with a count of published events (useful for a future category nav, cheap to compute now).
- `GET /search` — basic full-text search over `news_events.canonical_title`/`ai_summary` using Postgres's built-in `tsvector`/`tsquery` (per the architecture doc's explicit guidance not to reach for Elasticsearch until real need is proven — see architecture doc Section 7). Query param: `q`. Paginated same as `/events`.
- `GET /healthz` — DB connectivity check, matching the pattern from `listener` and `processor`.

**Explicitly excluded from V5**: any endpoint that writes, any endpoint exposing `needs_review` or unpublished events, any admin/moderation functionality (channel management, takedowns — that's V9), and any real-time/streaming endpoint (SSE — that's V7).

### 1.3 Transparency in the Response Shape (this is a product requirement, not styling)

Every event response must make the fact/generated-content distinction from the architecture doc's Section 0/4 structurally visible in the JSON, not just documented in a README. Concretely:
```json
{
  "id": 42,
  "canonical_title": "...",
  "ai_summary": "...",
  "ai_summary_generated": true,
  "category": { "name": "Banking & Finance", "slug": "banking-finance" },
  "entities": [{ "name": "National Bank of Ethiopia", "type": "organization" }],
  "sources": [
    {
      "channel_name": "NBEthiopia",
      "channel_handle": "nbethiopia",
      "posted_at": "2026-08-11T12:15:20Z",
      "excerpt": "..."
    }
  ],
  "source_count": 3,
  "first_seen_at": "...",
  "last_updated_at": "..."
}
```
The `ai_summary_generated: true` field is deliberately explicit and always present (not just implied by the field existing) — a future frontend should never have to guess whether it's showing generated or original content. `excerpt` on each source is a short, bounded slice of `raw_text` (not the full post) — bounded length here is a reasonable first step toward the copyright-safety posture the architecture doc raised in Section 12, though the real policy decision on how much source text is safe to expose publicly is still one worth confirming with you explicitly before this ships beyond your own testing (flag this in the ADR or a comment — don't treat the bounded-excerpt choice as a fully resolved legal question just because it's implemented).

### 1.4 AGENTS.md Updates

Append a V5-specific section:
- `services/api/` reads from the database through its own read-only role (`efi_api`, 1.7) — never the `efi_app` role the listener/processor use, and never able to write anything.
- No endpoint in V5 may accept or process a write — if a future version needs a write endpoint (e.g., admin takedowns in V9), that's a new, clearly-separated authenticated route group, not an addition to these public handlers.
- Every public endpoint must be rate-limited (1.8) — a new endpoint added without rate-limiting middleware applied should be treated as incomplete, not just missing a nice-to-have.
- Query parameters (`limit`, `offset`, `category`, `q`) must be validated and bounded server-side — never trust client-supplied pagination values without clamping them (this is both a performance and a basic security concern, first time it's applicable since this is the first public-facing code).

### 1.5 ADR: API Routing & Versioning (0009)

- **Context**: need a routing approach and a decision on whether/how to version the API from the start.
- **Decision**: use Go's standard `net/http` with a minimal router (Go 1.22+'s built-in enhanced `ServeMux` pattern matching is likely sufficient and avoids adding a third-party router dependency at this scale — note if the agent finds a specific limitation requiring a library like `chi`, that's fine, but justify it rather than defaulting to a framework out of habit). Version the API from day one via a `/api/v1` prefix — costs nothing now, avoids a painful migration later once a real frontend depends on stable paths.
- **Consequences**: `/api/v1` implies a versioning discipline going forward — breaking changes get a `/api/v2`, not a silent change to `/api/v1`'s existing shape, once anything depends on it.

### 1.6 Database Changes

New migration `0006_api_read_role.sql`:
```
CREATE ROLE efi_api LOGIN PASSWORD '<set via migration variable or documented manual step, never hardcoded>';
GRANT CONNECT ON DATABASE efi_dev TO efi_api;
GRANT USAGE ON SCHEMA public TO efi_api;
GRANT SELECT ON news_events, event_sources, raw_posts, channels, categories, entities, event_entities TO efi_api;
-- Explicitly no SELECT needed on processing_audit for the public API — that's
-- an internal/operational table, not public-facing data.
```
No table structure changes — V5 only reads what V0–V4 already built. If the role-creation-with-password pattern doesn't fit cleanly into a plain SQL migration file (password management in a committed file is exactly the kind of secret-handling mistake V0's conventions exist to prevent), document the actual chosen approach (e.g., role created via a separate, gitignored bootstrap script, or via environment-variable substitution at migration time) in the ADR or a runbook rather than silently committing a real password.

### 1.7 Security Baseline (V5-specific, building on V0's baseline)

- **This is the first version where the system accepts connections from anyone, not just you.** Treat every input as hostile by default: validate all query params, reject malformed requests with clear 400s rather than 500s, never leak internal error details (DB errors, stack traces) in API responses — log them server-side, return generic error messages to the client.
- Least-privilege `efi_api` role (1.6) — read-only, no access to `processing_audit` or any table not needed for public responses.
- Rate limiting (1.8) is a security control here, not just a performance one — the API is now a public target for scraping/abuse.
- CORS: since this is public read-only data intended to eventually serve a frontend, permissive CORS (`Access-Control-Allow-Origin: *`) is a reasonable V5 default — document this explicitly as a decision, not an oversight, and note it should be revisited if/when the API ever needs to serve non-public data.
- No secrets are exposed in any response — spot-check that error messages, headers, and response bodies never leak DB connection details or internal paths.

### 1.8 Rate Limiting

- A simple in-memory per-IP token bucket (e.g., N requests per minute) is sufficient at V5's scale — this is explicitly a single-instance limitation (resets on restart, doesn't coordinate across instances) and that's fine, since V5 assumes a single API instance, consistent with every prior version's single-instance assumption. Note in the ADR or a comment that this needs to move to Redis-backed rate limiting (per the architecture doc's scaling roadmap) once/if the API runs multi-instance.
- Return `429 Too Many Requests` with a `Retry-After` header when the limit is hit — don't just silently drop or hang requests.

### 1.9 Reliability Scope

- If the database is unreachable, `/healthz` reports unhealthy and other endpoints return `503` with a generic message — the API should never hang indefinitely or return a confusing partial response.
- No dependency on the listener or processor being currently running — the API only reads what's already in the database; it should function correctly (just showing whatever data currently exists) even if ingestion/processing is temporarily down. This is worth a specific test, since it's an easy assumption to accidentally violate.

### 1.10 Testing

- Handler tests using Go's `httptest` package against a real local Postgres (via docker-compose) seeded with known fixture-like data — assert response shape, status codes, pagination behavior, and the `ai_summary_generated` field's presence.
- A specific test asserting `needs_review` events never appear in any public response, under any filter combination — this is a real data-leak risk worth its own explicit test, not just an implied behavior.
- Rate-limit test: exceed the configured limit in a test, confirm `429` + `Retry-After`.
- A test confirming malformed/out-of-range query params (negative offset, oversized limit, nonexistent category slug) are handled gracefully (400 or empty result, not 500).

### 1.11 CI Updates

Extend CI with a Go job for `services/api/` (build, lint, test), alongside the existing listener/processor jobs.

### 1.12 Logging

Per the established convention: structured request logs (method, path, status, latency, client IP for rate-limit debugging) — the first service where request-level logging (not just pipeline-decision logging) matters, since it's now handling arbitrary public traffic.

### 1.13 V5 Acceptance Criteria

- [ ] `services/api/` exists per 1.1, runs locally via docker-compose, connects only via the `efi_api` read-only role.
- [ ] All endpoints in 1.2 implemented and manually exercised by you against real live data (curl or browser) — confirm real enriched events, categories, and search results look correct.
- [ ] `GET /events/{id}` response matches the shape in 1.3 exactly, including `ai_summary_generated: true` and a populated `sources` array with real channel attribution.
- [ ] Automated test confirms `needs_review` events never leak through any endpoint.
- [ ] Rate limiting works — verified by test and by you manually hitting the limit once.
- [ ] Malformed query params handled gracefully — verified by test.
- [ ] `efi_api` role has no write access and no access to `processing_audit` — spot-checked directly against the DB.
- [ ] API remains functional with the listener/processor stopped (1.9) — verified manually.
- [ ] ADR-0009 exists; the excerpt-length/copyright question from 1.3 is explicitly flagged for your review, not silently resolved.
- [ ] `AGENTS.md` V5 section exists per 1.4.
- [ ] CI passes.
- [ ] No frontend, SSE, or admin/write functionality exists anywhere in the repo.

---

## 2. V5 IMPLEMENTATION PLAN

**Step 1 — Migration `0006_api_read_role.sql`**
- Creates: `efi_api` read-only role per 1.6, handling the password-in-migration problem explicitly (flag and propose an approach rather than silently committing a secret).
- Purpose: the API's DB access should be scoped correctly before any handler code is written.
- Checks: role exists, has SELECT only on the correct table list, confirmed by direct query; confirm no password is committed anywhere in the repo.
- Expected result: a correctly-scoped, non-secret-leaking DB role ready for the API to use.

**Step 2 — Scaffold `services/api/`**
- Creates: Go module, `config.go`, empty handler/route stubs, `main.go` with a basic HTTP server and `/healthz`.
- Purpose: establish the service skeleton before real endpoints.
- Checks: `go build`, server starts, `/healthz` responds.
- Expected result: a running, empty API service.

**Step 3 — `store.go`: read-only data access**
- Creates: functions to fetch paginated events, a single event with its sources/entities/category, categories with counts, and search results — all read-only SQL.
- Purpose: isolate all V5 SQL in one place, matching the established no-ORM convention.
- Checks: integration tests against real local Postgres, confirming correct joins (event → sources → channels, event → entities, event → category) and that `needs_review` events are excluded by construction (e.g., the base query always filters `status != 'needs_review'`, not filtered ad-hoc per handler).
- Expected result: proven, correctly-scoped data access.

**Step 4 — `handlers_events.go`, `handlers_categories.go`, `handlers_search.go`**
- Creates: the actual endpoint handlers per 1.2, using `store.go`, shaping responses per 1.3.
- Purpose: the core API surface.
- Checks: `httptest`-based tests per 1.10, including the `needs_review` leak test and malformed-param handling.
- Expected result: proven-correct handlers.

**Step 5 — `middleware.go`: rate limiting, CORS, recovery, request logging**
- Creates: the cross-cutting middleware per 1.7/1.8/1.12, applied globally to all routes.
- Purpose: the security/observability baseline for a now-public service.
- Checks: rate-limit test (1.10), manual verification that a panic in a handler doesn't crash the whole server (recovery middleware working), CORS headers present on responses.
- Expected result: a hardened request pipeline, not just working happy-path handlers.

**Step 6 — Wire into `main.go`, docker-compose**
- Creates: full route registration, server startup/shutdown handling (graceful shutdown on SIGTERM, matching the listener's established pattern); add `api` service to `infra/docker-compose.yml`, exposing its port to the host.
- Purpose: a fully running, locally-accessible API.
- Checks: `docker-compose up` brings up all four services (postgres, listener, processor, api); manually hit every endpoint with curl against real live data and read the actual responses.
- Expected result: a working, manually-verified public API over real enriched data.

**Step 7 — ADR-0009 + AGENTS.md V5 section**
- Creates: `docs/adr/0009-api-routing-and-versioning.md`, V5 addendum to `AGENTS.md`, explicit flag of the excerpt-length/copyright open question from 1.3.
- Purpose: document decisions and open questions before declaring done — especially the one that's a real product/legal decision, not just an engineering one.
- Checks: manual review; confirm the copyright flag is visible to you, not buried.
- Expected result: documented decisions, with the one genuinely open question surfaced for your judgment, not the agent's.

**Step 8 — CI updates**
- Creates: Go job for `services/api/` in `ci.yml`.
- Purpose: enforce V5's tests going forward.
- Checks: CI green on a clean PR.
- Expected result: V5 protected by CI alongside V1–V4.

**Step 9 — Final review against V5 acceptance criteria**
- Purpose: verify, don't assume — and specifically, actually read real API responses yourself rather than trusting "tests pass."
- Checks: walk every item in 1.13.
- Expected result: V5 acceptance criteria fully met; ready to hand off to V6.

---

## 3. V5 ANTIGRAVITY PROMPT

```
Read docs/architecture/platform-architecture.md, v0-foundation-spec.md,
v1-ingestion-spec.md, v2-processing-spec.md, v3-semantic-clustering-spec.md,
v4-ai-enrichment-spec.md, and v5-public-api-spec.md (repo root) before doing
anything else. The last of these contains the full V5 spec, implementation
plan, and your exact instructions — follow it as written, in order. Follow
AGENTS.md, including all prior version sections and the V5 addendum you will
create.

Inspect the current repository state first — V0 through V4 are implemented,
committed, and pushed, including real enriched news_events with ai_summary,
categories, and entities from live Gemini calls. Do not recreate or modify
V0-V4 code except where v5-public-api-spec.md explicitly calls for it (the new
efi_api database role).

This is the first version that makes the system reachable over the network —
treat every design choice with that in mind. Implement V5 per sections 1 and 2
of the spec: a read-only REST API (GET /events, GET /events/{id},
GET /categories, GET /search, GET /healthz) under /api/v1, using a new
least-privilege efi_api role with SELECT-only access (explicitly excluding
processing_audit).

CRITICAL data-leak requirement: needs_review events must never appear in any
public response, under any filter or search combination. Write this as an
explicit automated test, not just an implied behavior from a WHERE clause.

Every event response must include an explicit ai_summary_generated: true field
alongside ai_summary — never let generated content be ambiguous with source
fact in the response shape. Include a bounded excerpt (not full raw_text) from
each source in the sources array.

Apply rate limiting, CORS, panic recovery, and request logging as middleware to
every route — an endpoint without these applied is incomplete, not just
missing a nice-to-have, per section 1.4.

Do NOT implement: any write/admin endpoint, SSE/real-time push, a frontend, or
Elasticsearch/external search infrastructure (Postgres tsvector is sufficient
at this scale per the spec). If migration 0006 requires setting a role
password, do not hardcode or commit it — flag the approach you're taking and
let me confirm it.

Work through the implementation plan step by step, running tests/lint/build
checks after each step. Step 6 requires me to manually hit every endpoint
against real live data and read actual responses — flag this clearly rather
than declaring the API verified based on tests alone.

Also flag explicitly, for my review and not your own resolution: the excerpt-
length/copyright question noted in spec section 1.3 — how much of raw_text is
safe to expose per-source in public API responses. This is a product/legal
judgment call, not something to decide silently.

Do not introduce any new dependency or infrastructure beyond what's specified
without first writing an ADR and pausing for confirmation. If anything is
ambiguous, stop and ask rather than guessing.

When finished:
- Run the full test suite, lint, and build; confirm docker-compose brings up
  all four services together.
- Show me real curl output (or equivalent) from every endpoint against real
  live data, plus confirmation of the needs_review leak test passing, plus
  the efi_api role's actual granted permissions queried directly from Postgres.
- Report what was created/changed, what's automated vs. needs my review, and
  confirm no V6+ functionality was implemented.
- Stop. Do not continue into V6 without explicit instruction.
```

---

## V5 → V6 Handoff

V5 leaves in place: a working, rate-limited, publicly-reachable read-only REST API under `/api/v1`, a least-privilege `efi_api` role, an explicit fact/generated-content distinction in every response, and a flagged-but-not-silently-resolved question about source excerpt length worth your judgment before wider exposure.

V6 (frontend, per the architecture doc's roadmap) can build directly on this: a Next.js app consuming `/api/v1/events`, `/events/{id}`, `/categories`, and `/search` for SSR pages, using the `ai_summary_generated` field to render generated content honestly (e.g., a visible "AI-generated summary" label) and the `sources` array to render the attribution/transparency UI the architecture doc's Section 7 calls for. V6 should not need any new backend endpoints beyond what V5 already exposes — if it turns out to, that's a sign an endpoint was missed here, worth a small V5.1-style addition rather than scope creep into V6's own version boundary.
