# V1 — Telegram Ingestion
### Ethiopia News Aggregation Platform

**Scope note:** This document defines V1 only. It builds directly on the completed V0 foundation (repo structure, `AGENTS.md`, schema, CI, conventions) and references — but does not repeat — `docs/architecture/platform-architecture.md`. V1's only job: reliably get real Telegram posts into `raw_posts`, idempotently, surviving disconnects. Nothing downstream of that.

---

## 1. V1 SPEC

### 1.1 What V1 Adds to the Repository

```
/
├── services/
│   └── listener/
│       ├── main.py               # entrypoint: connect, listen, dispatch to ingest()
│       ├── ingest.py             # writes normalized raw event -> raw_posts (idempotent)
│       ├── config.py             # loads env vars (DB conn, Telegram creds, channel list)
│       ├── db.py                 # thin DB access layer, no ORM
│       ├── requirements.txt      # pinned versions (Telethon, psycopg or similar, pytest)
│       └── tests/
│           ├── test_ingest.py    # idempotency, reconnection-replay handling
│           └── test_fixtures.py  # consumes tests/fixtures/sample_telegram_posts.json
├── docs/
│   ├── adr/
│   │   └── 0005-telegram-session-storage.md
│   └── runbooks/
│       └── telegram-listener-disconnected.md
├── db/
│   └── migrations/
│       └── 0002_seed_channels.sql   # optional: seed 5-10 real channel rows (no secrets)
├── infra/
│   └── docker-compose.yml           # updated: add listener service, still no processor/API
└── .env.example                     # updated: Telegram credential variable names (no values)
```

No changes to `news_events`, `categories`, or any V2+ table — `raw_posts`/`channels` from V0 are sufficient and should not be altered unless the architecture doc's V1 subset was wrong (if so, that's a migration + ADR, not a silent schema change).

### 1.2 Functional Scope (from the architecture doc, Section 4, steps 1–2 only)

- Connect to Telegram via Telethon using a single persistent session, for 5–10 configured public channels (per architecture doc's V1 scope, Section 14).
- On new message: normalize into a raw event record (channel id, message id, raw text, raw entities, posted timestamp, ingested timestamp).
- Write to `raw_posts` with the existing `(channel_id, telegram_message_id)` uniqueness constraint doing the idempotency work — a duplicate delivery (e.g., after reconnect) must be a no-op, not an error and not a duplicate row.
- Handle reconnects: on disconnect, retry with backoff; on reconnect, do not assume message loss — Telethon's catch-up behavior plus the idempotency constraint together are the safety net, not a custom "resume from last ID" mechanism at V1 (avoid building that complexity until it's proven necessary — see 1.6).
- No processing beyond normalization — no dedup logic, no embeddings, no clustering, no AI. `processing_status` on `raw_posts` should exist (schema already allows it per V0) but at V1 it only ever gets set to `'ingested'`; no other value is written yet, and no code reads/acts on it downstream.

### 1.3 AGENTS.md Updates

Add to the existing V0 `AGENTS.md` (don't rewrite it — append a V1-specific section):
- Python code in `services/listener/` follows: `black` for formatting, `ruff` for linting, `pytest` for tests, dependencies pinned in `requirements.txt`.
- The Telegram session file/string is a secret — never committed, never logged, loaded only from an environment variable or mounted secret file (see 1.7).
- `services/listener/` must be runnable and testable against `tests/fixtures/sample_telegram_posts.json` without any real Telegram credentials — real credentials are only required for the actual live-connection path, which should be behind a clearly separated code path from the ingest/idempotency logic being tested.
- No code in `services/listener/` may write to any table other than `raw_posts` (and read `channels`). If a future need arises to write elsewhere, that's a V2+ conversation, not a V1 shortcut.

### 1.4 ADR: Telegram Session Storage (0005)

This is the one genuinely new architectural decision V1 introduces (V0's four ADRs don't cover it). Content to capture:
- **Context**: the Telegram session is the most sensitive credential in the system (architecture doc, Section 8) — compromise allows impersonation.
- **Decision**: session string stored in an environment variable at V1 (single-operator, single-environment scale), loaded via `.env` locally (gitignored) and via the hosting platform's secret storage in any deployed environment — never written to a file inside the repo or image. Explicitly note this is a V1-appropriate simplification; a dedicated secrets manager is deferred (per the architecture doc's V9 scope) until there's more than one operator/environment to justify it.
- **Consequences**: rotating the session means updating one env var; if the hosting platform's env-var storage is ever not encrypted-at-rest, this decision needs revisiting before going further — flag this explicitly as a revisit trigger, not just accept it silently.

### 1.5 Database Changes

- No schema changes to `raw_posts`/`channels` structure.
- One optional new migration, `0002_seed_channels.sql`, inserting the 5–10 real channel rows (channel name/handle/telegram_channel_id — public information, not secret) so the listener has something configured to connect to out of the box. If you'd rather manage channel list via `.env`/config instead of a seed migration, that's an equally valid V1 choice — pick one and note it in the runbook, don't do both.

### 1.6 Reliability Scope (explicitly bounded)

Per the architecture doc's Section 12 failure modes — V1 must handle:
- Disconnect → reconnect with backoff (Telethon has this built in; configure sane retry/backoff bounds, don't hand-roll a new retry loop).
- Redelivery after reconnect → absorbed by the DB uniqueness constraint (no application-level "have I seen this ID" cache needed at V1 — the DB already guarantees it, and adding a second idempotency mechanism at this scale is unnecessary complexity).
- Listener process crash → on restart, Telethon re-establishes the session and continues receiving new messages; V1 does **not** need to implement gap-detection/backfill for messages missed during downtime — that's a reasonable V1 limitation to accept explicitly, not silently.

V1 must **not** attempt: multi-instance listener redundancy, automatic backfill of historical channel messages, or any queue/broker in front of the DB write (per ADR-0001, still Postgres-only — a single INSERT is a queue of one).

### 1.7 Security Baseline (V1 additions to V0's baseline)

- Telegram session credential handling per ADR-0005.
- Treat all incoming message text/entities as untrusted input even at the storage layer — no execution of anything from message content, no interpolation into shell commands or queries outside of parameterized DB calls (SQL injection surface: none, if using parameterized queries throughout — verify this explicitly in review, not just by convention).
- Listener runs as its own container/process with only the DB credentials it needs (the least-privilege app role from V0's `0001_init.sql` should already be sufficient — confirm it has INSERT on `raw_posts` and SELECT on `channels` only, nothing broader).

### 1.8 Logging

Per V0's established convention (structured JSON, `correlation_id` reserved field): the listener logs one line per ingested message (success), one line per ingestion failure (with reason — e.g., constraint violation being informative, not just swallowed), and one line per connect/disconnect/reconnect event. This is the first real code that makes V0's logging convention concrete — get the shape right here since V2+ services will follow the same pattern.

### 1.9 Testing

- Unit tests for `ingest.py`'s idempotency behavior: feeding the same fixture message twice results in one row, not two/an error.
- Unit tests using `tests/fixtures/sample_telegram_posts.json` (from V0) to verify normalization produces the expected `raw_posts` shape.
- No live-Telegram integration test in CI (can't authenticate in CI without real credentials, and shouldn't try) — the connect/listen code path is exercised manually/locally, only the ingest/normalize/idempotency logic is unit-tested in CI. Document this boundary explicitly so no one mistakes "unit tests pass" for "verified against live Telegram."

### 1.10 CI Updates

Extend V0's `ci.yml`: add a Python job (install pinned deps, run `ruff`, run `pytest` for `services/listener/tests/`), running alongside the existing Postgres/migration job. Both must pass.

### 1.11 V1 Acceptance Criteria

- [ ] `services/listener/` exists per 1.1, runnable locally with a real Telegram session (manually verified by you, not just the agent) and connects to at least one real public channel.
- [ ] A real message posted in a monitored channel appears as a row in `raw_posts` within the architecture doc's V1 latency expectations (informal check — no automated latency test needed yet).
- [ ] Feeding a duplicate message (simulated reconnect) does not create a duplicate row — verified by an automated test, not just manual inspection.
- [ ] Listener recovers from a forced disconnect (kill the connection) without manual restart, per Telethon's reconnect behavior.
- [ ] ADR-0005 exists and is followed by the actual credential-loading code (no session string in any committed file, ever — grep the repo history if unsure).
- [ ] `AGENTS.md` V1 section exists per 1.3.
- [ ] CI passes, including the new Python job.
- [ ] No code exists for dedup-beyond-idempotency, embeddings, clustering, AI, API, or frontend.
- [ ] `processing_status` is written but not read/branched-on anywhere in V1 code.

---

## 2. V1 IMPLEMENTATION PLAN

**Step 1 — Scaffold `services/listener/`**
- Creates: directory structure per 1.1, `requirements.txt` with pinned Telethon/psycopg/pytest/ruff/black versions, empty `main.py`/`ingest.py`/`config.py`/`db.py`.
- Purpose: establish the service skeleton before any real logic.
- Checks: `pip install -r requirements.txt` succeeds; lint runs (even against near-empty files).
- Expected result: an installable, lintable, empty service.

**Step 2 — `config.py`: environment loading**
- Creates: config loader reading DB connection vars and Telegram credential vars from environment, with clear errors if required vars are missing.
- Purpose: fix the config/secrets convention (per ADR-0005) before anything depends on it.
- Checks: unit test that missing required vars raises a clear error, not a silent None.
- Expected result: config loading is explicit and fails loudly when misconfigured.

**Step 3 — `db.py`: minimal data access layer**
- Creates: a small module with one function to insert a raw post (parameterized query, relying on the existing DB constraint for idempotency — no pre-check query, let the constraint do the work and handle the resulting conflict gracefully as a no-op).
- Purpose: isolate all SQL in one place, no ORM, matching V0's convention.
- Checks: unit test against a real local Postgres (via V0's docker-compose) — insert, then insert the same (channel_id, message_id) again, assert exactly one row exists afterward and no unhandled exception propagates.
- Expected result: proven idempotent insert behavior at the data layer, independent of Telegram.

**Step 4 — `ingest.py`: normalization**
- Creates: function(s) taking a raw Telegram-message-shaped input and producing the normalized record `db.py` expects (strip formatting/entities into stored fields, capture posted/ingested timestamps).
- Purpose: separate "shape a message" from "connect to Telegram" and from "write to DB" — three independently testable concerns.
- Checks: unit tests using `tests/fixtures/sample_telegram_posts.json`, asserting normalized output matches expected shape for each fixture entry.
- Expected result: normalization logic fully tested without needing Telethon or a live connection.

**Step 5 — `main.py`: Telethon wiring**
- Creates: connect-to-Telegram, subscribe to configured channels, on new message call `ingest.py` then `db.py`; reconnect/backoff relies on Telethon defaults, tuned via config if needed.
- Purpose: the only place real Telegram I/O happens — kept thin, since it's the one part that can't be fully unit-tested in CI.
- Checks: manual local run against real credentials and a real (or test) channel — confirm a posted message lands in `raw_posts`. Kill the connection mid-run, confirm auto-reconnect and continued ingestion.
- Expected result: working end-to-end ingestion, verified manually since it requires live credentials.

**Step 6 — ADR-0005 + AGENTS.md V1 section + runbook**
- Creates: `docs/adr/0005-telegram-session-storage.md`, V1 addendum to `AGENTS.md`, `docs/runbooks/telegram-listener-disconnected.md` (what to check/do if the listener stops ingesting).
- Purpose: document the one new architectural decision and operational knowledge before considering V1 done.
- Checks: manual review against 1.3/1.4.
- Expected result: future agents/operators have what they need without re-deriving it.

**Step 7 — docker-compose + .env.example updates**
- Creates: adds a `listener` service to `infra/docker-compose.yml` (build from `services/listener/`, reads env vars, depends on Postgres); updates `.env.example` with the new (nameless/placeholder) Telegram credential variables.
- Purpose: make local dev able to run the full listener via compose, not just via a manually-activated virtualenv.
- Checks: `docker-compose up` builds and starts the listener container successfully (it will fail to authenticate without real creds locally, which is expected and fine — confirm it fails gracefully with a clear config error, not a crash/stack trace).
- Expected result: one-command local environment, real credentials still required only for actual ingestion.

**Step 8 — CI updates**
- Creates/changes: `.github/workflows/ci.yml` gains a Python job — install deps, `ruff check`, `pytest services/listener/tests/`.
- Purpose: make V1's tests (Steps 3–4) part of the enforced pipeline.
- Checks: CI green on a clean PR; deliberately break an idempotency test locally, confirm CI catches it, then revert.
- Expected result: V1's core guarantees (idempotency, normalization correctness) are protected by CI going forward.

**Step 9 — Optional: seed migration for channel list**
- Creates: `db/migrations/0002_seed_channels.sql` (or documents the config-based alternative — pick one per 1.5).
- Purpose: give the listener a real, working channel list without manual DB inserts.
- Checks: migration applies cleanly; listener config picks up the seeded channels correctly.
- Expected result: fresh environment setup includes a usable channel list out of the box.

**Step 10 — Final review against V1 acceptance criteria**
- Purpose: verify, don't assume.
- Checks: walk every item in 1.11, including the manual real-Telegram checks that can't be automated.
- Expected result: V1 acceptance criteria fully met; ready to stop and hand off to V2.

---

## 3. V1 ANTIGRAVITY PROMPT

```
Read docs/architecture/platform-architecture.md, v0-foundation-spec.md, and
v1-ingestion-spec.md (repo root) before doing anything else. The last of these
contains the full V1 spec, implementation plan, and your exact instructions —
follow it as written, in order. Follow AGENTS.md, including its V1 addendum
you will create as part of this work.

Implement V1 only, exactly as specified in v1-ingestion-spec.md sections 1 and 2.
V1's sole job: a Telegram listener service that ingests messages from configured
public channels into the existing raw_posts table, idempotently, surviving
reconnects. Nothing downstream of that.

Do NOT implement: deduplication beyond DB-level idempotency, text normalization
similarity/clustering, embeddings, AI calls of any kind, a public API, a frontend,
SSE, or any queue/broker beyond the existing Postgres writes. If a future version
will need an interface or boundary here, note it in a comment or ADR — do not
build the feature itself.

Work through the implementation plan (section 2) step by step. After each step,
run the relevant tests/lint/build checks before moving to the next. Steps that
require real Telegram credentials (connecting live, verifying reconnect behavior)
should be clearly flagged as needing manual verification by me — do not claim
these are done based on unit tests alone, and do not fabricate a "verified" claim
for anything you cannot actually test without live credentials.

Do not introduce any new dependency, service, or infrastructure beyond what's
listed in the spec without first writing an ADR and pausing for confirmation.
If anything is ambiguous or not covered by the spec documents, stop and ask
rather than guessing.

When finished:
- Run the full test suite, lint, and confirm docker-compose builds the listener
  service correctly (even though live ingestion itself needs my real credentials
  to verify).
- Report what was created/changed, what checks were run (automated vs. what
  still needs my manual verification with real credentials), and explicit
  confirmation that no V2+ functionality was implemented.
- Stop. Do not continue into V2 without explicit instruction.
```

---

## V1 → V2 Handoff

V1 leaves in place: a working, tested Telegram listener writing idempotently to `raw_posts`, ADR-0005 governing session credential handling, a runbook for listener disconnects, and CI covering both the DB/migration path (from V0) and the ingestion logic path (from V1).

V2 (processing + basic deduplication, per the architecture doc's roadmap) can build directly on this: read from `raw_posts` where `processing_status = 'ingested'`, add normalization/near-duplicate logic and a non-semantic clustering pass, and introduce the `news_events`/`event_sources` tables via a new migration — all without needing to touch the listener itself. V2 should only revisit V1's code if the shape of `raw_posts` proves insufficient once real data is flowing, in which case that's a migration + ADR, following the same discipline V1 followed with V0.
