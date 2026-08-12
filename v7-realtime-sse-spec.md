# V7 — Real-Time Delivery (SSE)
### Ethiopia News Aggregation Platform

**Scope note:** This document defines V7 only. It builds on V0–V6 (foundation through frontend, plus the V4.1 enrichment revision — all live-verified). It references — but does not repeat — `docs/architecture/platform-architecture.md`. V7's job: replace V6's polling-based `LiveFeedUpdater` with real SSE push, covering both cases V6 deliberately deferred — a brand-new event, and an existing event gaining a new source. No new pages, no design changes, no SEO work (V8).

---

## 1. V7 SPEC

### 1.1 What V7 Adds to the Repository

```
/
├── services/
│   ├── api/
│   │   ├── sse.go                  # SSE endpoint: hub, client registration, broadcast
│   │   ├── sse_test.go
│   │   ├── notify_listener.go      # Postgres LISTEN subscriber, feeds the hub
│   │   └── routes.go               # UPDATED: add GET /api/v1/stream
│   └── processor/
│       └── store.go                 # UPDATED: explicit NOTIFY after event create/update
├── docs/
│   └── adr/
│       └── 0011-realtime-delivery-mechanism.md
└── web/
    └── components/
        └── LiveFeedUpdater.tsx       # REWRITTEN: EventSource-based, replaces polling
```

No new database tables — this version wires existing writes to Postgres's built-in `LISTEN`/`NOTIFY`, already the justified mechanism per the architecture doc's Section 5 (confirmed again here now that it's actually being implemented) and consistent with ADR-0001's Postgres-only stance.

### 1.2 Functional Scope (architecture doc Section 5, made real)

- **Processor side**: after `store.go` successfully writes a new `news_events` row, or successfully attaches a new source to an existing event (incrementing `source_count`), it issues an explicit `NOTIFY news_events_channel, '<payload>'` as part of the same transaction. Payload is small and structured: `{"type": "new_event" | "event_updated", "event_id": <id>}` — deliberately minimal (Postgres NOTIFY payloads are capped at 8000 bytes, and there's no reason to duplicate full event data in the notification when the API/frontend can just fetch the current state).
- **Explicit `NOTIFY` call in application code, not a database trigger.** This is a deliberate choice (1.5) — keeping the notify logic visible in `store.go` alongside the write it accompanies, rather than hidden in DB trigger SQL a future agent might not think to check.
- **API side**: a new `notify_listener.go` holds one persistent Postgres connection running `LISTEN news_events_channel`, receiving notifications and forwarding them to an in-process hub (`sse.go`) that fans them out to every currently-connected SSE client.
- **`GET /api/v1/stream`**: an SSE endpoint. On connect, a client starts receiving `new_event`/`event_updated` messages as they happen. No historical replay on connect at V7 — a client that connects gets only what happens *after* connecting (matches the architecture doc's real-time framing; a "catch up on what I missed" concern is already handled by the existing paginated `/events` endpoint a client can call on initial load, not something SSE itself needs to solve).
- **Frontend**: `LiveFeedUpdater` is rewritten to open an `EventSource` connection to `/api/v1/stream` instead of polling. On `new_event`, fetch that event via the existing REST endpoint and prepend it to the feed (same UX as V6's polling version, just event-driven instead of interval-driven). On `event_updated` — the case V6 explicitly deferred — re-fetch and update that specific event in place if it's currently rendered (e.g., its source count badge updates live), which is the genuinely new capability this version adds.
- The browser's native `EventSource` handles reconnection automatically on connection drop — no custom reconnect logic needed in the frontend for the base case, though a visible "reconnecting..." indicator (per V6's original reliability-scope intent) is still worth surfacing to the user rather than updates silently pausing with no signal.

### 1.3 Why Application-Level NOTIFY, Not a DB Trigger

A database trigger would work and would guarantee the notification never gets missed even if application code has a bug. But it also means the "when does a notification fire" logic lives in SQL, invisible to anyone reading `store.go`, and invisible to `AGENTS.md`'s established pattern of keeping decision logic traceable in Go code (the same reasoning that's applied to every prior version's clustering/enrichment logic). Given this project's small scale and the value already placed on traceability throughout, an explicit call in the same function that performs the write is the more consistent choice — and it's revisitable later (noted in the ADR) if a real case of a missed notification ever surfaces in practice.

### 1.4 AGENTS.md Updates

Append a V7-specific section:
- Every code path in `store.go` that creates a `news_events` row or attaches a new source must call the corresponding `NOTIFY` in the same transaction as the write — a PR adding a new write path without the matching notify call should be treated as incomplete, mirroring the same discipline already applied to V4's `processing_audit` requirement.
- The SSE hub (`sse.go`) must clean up disconnected clients properly — no goroutine or channel leak per dropped connection. This is the first place in the codebase managing long-lived concurrent connections; treat resource cleanup here as a first-class correctness concern, not an afterthought.
- `notify_listener.go`'s Postgres LISTEN connection must itself reconnect on failure (separate from any individual SSE client's connection) — if this one connection silently dies, every connected client stops receiving updates with no visible symptom until someone notices staleness. This needs its own health/reconnect logic and its own log lines.

### 1.5 ADR: Real-Time Delivery Mechanism (0011)

- **Context**: SSE-over-WebSockets was already decided back in the original architecture doc (Section 5) and reaffirmed in ADR-0009 when the API was built (V5) without yet implementing the streaming endpoint itself. This ADR is where the actual mechanism gets decided and recorded.
- **Decision**: Postgres `LISTEN`/`NOTIFY`, triggered by explicit application-code calls in `store.go` (not a DB trigger, per 1.3), fanned out to SSE clients via an in-process hub in the API service. Single API instance assumed (consistent with every prior version's single-instance assumption) — this hub pattern does not coordinate across multiple API instances; that's a real limitation to note for future scaling (Redis pub/sub is the natural upgrade path per the architecture doc's scaling roadmap, once/if the API needs to run multi-instance).
- **Consequences**: notification delivery is best-effort, not guaranteed — if the API's Postgres LISTEN connection is down for a moment, notifications during that window are lost (not queued for later delivery). This is an accepted V7 limitation: the frontend's initial page load and its own periodic REST fetches (still present as fallback) mean a client is never permanently stale, just occasionally slightly behind — a real, honest tradeoff, not a hidden risk.

### 1.6 Database Changes

None — no new tables, no schema changes. This version only adds `NOTIFY` calls to existing write paths.

### 1.7 Reliability Scope

- If the API's LISTEN connection drops, `notify_listener.go` reconnects with backoff and logs the outage clearly — per 1.4, this is the single point of failure for the entire real-time feature and deserves visible, specific handling, not generic error logging indistinguishable from anything else.
- If a client's SSE connection drops, the browser's native reconnect handles it; the hub must correctly remove the stale client and not attempt to write to a dead connection (which would otherwise leak or error repeatedly).
- The frontend must still function correctly with **zero** real-time updates (e.g., SSE fails entirely, blocked by a corporate proxy, etc.) — a user should still be able to load the site and see current data via the normal SSR path; SSE is an enhancement, not a dependency for basic functionality. Worth a specific test: disable SSE entirely and confirm the site still works, just without live updates.

### 1.8 Testing

- Unit tests for the hub (`sse.go`): a message sent to the hub reaches all currently-registered clients; a disconnected client is cleaned up and doesn't receive further messages or cause errors.
- Integration test: write a new `news_events` row via `store.go` in a test, confirm a real NOTIFY fires and a connected test SSE client receives the expected `new_event` message with the correct `event_id`.
- Same for the `event_updated` case (attach a new source, confirm the notification).
- Frontend test: `LiveFeedUpdater` correctly handles a mocked `new_event` message (fetches and prepends) and a mocked `event_updated` message (re-fetches and updates in place) — and correctly degrades if the `EventSource` connection fails entirely (per 1.7).
- Load-adjacent sanity check (not a full load test — that's V10): open several concurrent SSE connections in a test/manual check, confirm the hub handles them all correctly and cleans up properly when they all disconnect at once.

### 1.9 CI Updates

Extend the existing Go API job and Node web job with the new test files. No new CI infrastructure needed.

### 1.10 Logging

Per the established convention: log every SSE client connect/disconnect (with a count of currently-connected clients, useful for later capacity planning), every NOTIFY sent (from the processor side) and every message the hub broadcasts (from the API side) — this pairing lets you trace a single event's notification from the processor's write all the way to client delivery, matching the correlation-ID discipline used throughout the pipeline so far.

### 1.11 Security Baseline (V7-specific)

- SSE connections are long-lived and hold a server resource for their duration — apply a reasonable per-IP concurrent-connection limit (e.g., a handful per IP) alongside the existing rate limiting from V5, so the endpoint can't be trivially exhausted by one client opening many connections.
- No new data exposure risk beyond what `/api/v1/events` already exposes (the stream only carries `event_id`/`type` — the client still fetches full data through the existing, already-scoped REST endpoints) — worth confirming explicitly rather than assuming, since it's an easy place to accidentally leak more than intended if someone later "helpfully" adds more data to the notify payload.

### 1.12 V7 Acceptance Criteria

- [x] `GET /api/v1/stream` exists, accepts SSE connections, and correctly delivers `new_event` messages for real events created by the live pipeline.
- [x] `event_updated` messages correctly fire and are delivered when a real event gains a new source — manually verified against real live data (post something that clusters into an existing event, confirm the frontend updates live).
- [x] `LiveFeedUpdater` no longer polls — verified by checking network activity (no repeated interval requests, only the persistent SSE connection).
- [x] Site remains fully functional with SSE disabled/blocked (1.7) — manually verified.
- [x] `notify_listener.go` reconnects correctly after a simulated Postgres connection drop — verified by test and/or manual disconnection.
- [x] No goroutine/connection leak after multiple clients connect and disconnect — spot-checked (e.g., connection count returns to baseline after clients close).
- [x] Per-IP SSE connection limit enforced — verified by test.
- [x] ADR-0011 exists.
- [x] `AGENTS.md` V7 section exists per 1.4.
- [x] CI passes.
- [x] No SEO/structured-data work, no admin functionality, no multi-instance coordination exists anywhere in the repo.

---

## 2. V7 IMPLEMENTATION PLAN

**Step 1 — `store.go`: explicit NOTIFY on write**
- Creates: `NOTIFY` calls added to the existing event-create and source-attach write paths in the processor's `store.go`, within the same transaction as the write.
- Purpose: the trigger for everything downstream; get this right and isolated before touching the API side.
- Checks: a test confirming a real `LISTEN` connection in the test itself receives the notification when the write happens — proves the mechanism works before building the hub around it.
- Expected result: proven, transactionally-consistent NOTIFY firing on real writes.

**Step 2 — `notify_listener.go`: Postgres LISTEN subscriber**
- Creates: the persistent LISTEN connection in the API service, with reconnect-on-failure logic per 1.7.
- Purpose: the API-side receiver for what Step 1 now sends.
- Checks: unit/integration test simulating a dropped connection, confirming reconnect; manual test confirming a real notification from a running processor is received here.
- Expected result: a reliable, self-healing subscriber.

**Step 3 — `sse.go`: hub and endpoint**
- Creates: the in-process hub (register/unregister clients, broadcast), and the `GET /api/v1/stream` handler wiring HTTP response writers into the hub as SSE clients.
- Purpose: fan out what Step 2 receives to any number of connected browsers.
- Checks: hub unit tests per 1.8 (message reaches all clients, disconnected clients cleaned up); manual test with `curl -N http://localhost:8080/api/v1/stream` while triggering a real event, confirming raw SSE output.
- Expected result: a working, leak-free SSE endpoint, verified both automatically and by hand.

**Step 4 — Security: rate/connection limiting**
- Creates: per-IP concurrent SSE connection limit, applied as middleware alongside V5's existing rate limiter.
- Purpose: close the resource-exhaustion gap specific to long-lived connections, per 1.11.
- Checks: test opening more than the allowed number of connections from one simulated IP, confirming the limit is enforced.
- Expected result: a hardened streaming endpoint.

**Step 5 — Rewrite `LiveFeedUpdater.tsx`**
- Creates: replace the V6 polling logic with `EventSource`-based subscription, handling both `new_event` (fetch + prepend) and `event_updated` (fetch + update in place) per 1.2, plus a visible reconnecting-state indicator.
- Purpose: the actual real-time UX upgrade users will see.
- Checks: component tests per 1.8; manual review in a browser with real live data — this is the point where you actually watch an update appear without any polling delay.
- Expected result: a working, visibly real-time feed.

**Step 6 — Graceful degradation**
- Creates: fallback handling so the site works correctly with SSE entirely unavailable (per 1.7) — confirm this doesn't require special-casing much, since SSR + the existing REST endpoints should already cover the "no live updates" case naturally if `LiveFeedUpdater` simply fails to connect without crashing anything else.
- Purpose: prove real-time is genuinely additive, not load-bearing.
- Checks: manual test — block/disable the stream endpoint, confirm the site still loads and functions, just without live updates.
- Expected result: proven graceful degradation.

**Step 7 — ADR-0011 + AGENTS.md V7 section**
- Creates: the ADR and addendum per 1.5/1.4.
- Purpose: document the mechanism and its accepted limitations (best-effort delivery, single-instance hub) before declaring done.
- Checks: manual review.
- Expected result: documented decisions for V8+ to build on, with real scaling limitations flagged honestly rather than glossed over.

**Step 8 — CI updates**
- Creates: new test files wired into the existing Go API and Node web CI jobs.
- Purpose: enforce V7's guarantees going forward.
- Checks: CI green on a clean PR.
- Expected result: V7 protected by CI alongside every prior version.

**Step 9 — Final review against V7 acceptance criteria**
- Purpose: verify, don't assume — and specifically, watch a real update happen live in a browser with your own eyes, not just confirm tests pass.
- Checks: walk every item in 1.12.
- Expected result: V7 acceptance criteria fully met; ready to hand off to V8.

---

## 3. V7 ANTIGRAVITY PROMPT

```
Read docs/architecture/platform-architecture.md, v5-public-api-spec.md,
v6-frontend-spec.md, and v7-realtime-sse-spec.md (repo root) before doing
anything else. The last of these contains the full V7 spec, implementation
plan, and your exact instructions — follow it as written, in order. Follow
AGENTS.md, including all prior version sections and the V7 addendum you will
create.

Inspect the current repository state first — V0 through V6 (plus the V4.1
enrichment revision) are implemented, committed, and pushed, including a
working polling-based LiveFeedUpdater that this version replaces. Do not
recreate or modify unrelated code.

Implement V7 per sections 1 and 2 of the spec: add a GET /api/v1/stream SSE
endpoint to the API, backed by Postgres LISTEN/NOTIFY, with the processor
issuing explicit NOTIFY calls (in application code in store.go, NOT a
database trigger — this is a deliberate architectural choice per spec section
1.3, do not implement it as a trigger) whenever a new event is created or an
existing event gains a new source. Replace LiveFeedUpdater's polling with a
real EventSource subscription handling both cases — including the
event-gained-a-new-source case, which the previous version explicitly
deferred.

The notify_listener.go Postgres LISTEN connection is a single point of
failure for the entire feature — give it explicit reconnect-with-backoff
logic and clear, distinguishable logging, per section 1.4/1.7.

Apply a per-IP concurrent SSE connection limit alongside the existing V5 rate
limiter — long-lived connections are a different resource-exhaustion risk
than regular request rate limiting.

CRITICAL reliability requirement: the site must remain fully functional with
SSE completely disabled or blocked — this is an enhancement layer, not a
dependency. Write this as an explicit test and a manual verification step,
not just an assumption.

Do NOT implement: multi-instance/Redis-backed pub/sub (single-instance hub is
correct at this scale, per ADR-0011's documented limitation), historical
message replay on SSE connect, or any SEO/structured-data work (that's V8).

Work through the implementation plan step by step, running tests/lint/build
checks after each step. Step 5 (frontend rewrite) requires me to manually
watch a real live update happen in a browser — flag this clearly rather than
declaring it verified from tests alone.

Do not introduce any new dependency or infrastructure beyond what's specified
without first writing an ADR and pausing for confirmation. If anything is
ambiguous, stop and ask rather than guessing.

When finished:
- Run the full test suite, lint, and build across all affected services.
- Show me: a real curl -N against the stream endpoint while a real event is
  created, confirming raw SSE output; confirmation that source-count updates
  appear live on an already-rendered event without a page refresh; and proof
  the site still works with the stream endpoint blocked/disabled.
- Report what was created/changed, what's automated vs. needs my review, and
  confirm no V8+ functionality was implemented.
- Stop. Do not continue into V8 without explicit instruction.
```

---

## V7 → V8 Handoff

V7 leaves in place: real SSE-based live delivery replacing V6's polling, covering both new-event and existing-event-updated cases, with an honestly-documented single-instance/best-effort-delivery limitation (ADR-0011) and proven graceful degradation when real-time is unavailable.

V8 (SEO, per the architecture doc's roadmap) can build directly on this without touching any real-time code: structured data (`NewsArticle` schema.org JSON-LD, now finally using real `ai_headline` values instead of truncated placeholders — this is where V4.1's fix actually pays off), sitemaps, canonical URLs, Open Graph tags, and Core Web Vitals work, all layered onto the existing SSR pages. V8 should not need to touch `sse.go`, `notify_listener.go`, or `LiveFeedUpdater` — those are stable V7 output.
