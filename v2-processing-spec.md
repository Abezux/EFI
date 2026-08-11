# V2 — Processing & Basic Deduplication
### Ethiopia News Aggregation Platform

**Scope note:** This document defines V2 only. It builds on the completed V0 (foundation) and V1 (Telegram ingestion, now verified against live data) milestones. It references — but does not repeat — `docs/architecture/platform-architecture.md`. V2's job: take `raw_posts` rows sitting in `processing_status = 'ingested'`, normalize them, detect exact and near-duplicates, and group matching posts into a `news_events` row — using **text similarity only**. No embeddings, no vector search, no AI calls. Those are V3 (semantic clustering) and V4 (AI enrichment).

---

## 1. V2 SPEC

### 1.1 What V2 Adds to the Repository

```
/
├── services/
│   └── processor/
│       ├── main.go               # entrypoint: poll loop, process one raw_post at a time
│       ├── normalize.go          # text normalization (case, punctuation, whitespace)
│       ├── simhash.go            # simhash computation + Hamming distance comparison
│       ├── cluster.go            # matching decision: attach to existing event or create new
│       ├── store.go              # DB access: read raw_posts, write news_events/event_sources
│       ├── config.go             # env loading (DB conn, poll interval, similarity thresholds)
│       ├── go.mod / go.sum
│       └── processor_test.go     # unit tests per module above
├── docs/
│   └── adr/
│       └── 0006-non-semantic-clustering-at-v2.md
├── db/
│   └── migrations/
│       └── 0003_events_and_clustering.sql   # news_events, event_sources, raw_posts.simhash
├── infra/
│   └── docker-compose.yml                    # add processor service
└── tests/
    └── fixtures/
        └── sample_telegram_posts.json         # (existing from V0/V1 — reused, not duplicated)
```

No `services/api/` or `web/` directories yet — those are V5/V6.

### 1.2 Functional Scope (architecture doc Section 4, steps 3–4 and 6–7 partially — text-similarity clustering only, no embeddings)

- Poll `raw_posts` for rows with `processing_status = 'ingested'` (simple interval poll at V2 — this is consistent with ADR-0001's Postgres-only-queue decision; `LISTEN/NOTIFY` is a valid V2 upgrade *if* polling proves too slow in practice, but start with polling and only add NOTIFY if there's a measured reason to).
- **Normalize**: lowercase, strip Telegram markdown/entities down to plain text, collapse whitespace, remove URLs (store separately if needed later, but don't let a shared URL cause a false-positive match) — write result to `raw_posts.normalized_text` (column already exists from V0/V1 schema).
- **Exact-duplicate detection**: hash of normalized text (e.g., SHA-256) — if two posts normalize to the identical string, they're certainly the same content (a straight repost/forward).
- **Near-duplicate / text-similarity detection**: compute a simhash of the normalized text, store on `raw_posts.simhash` (new column, this migration). Compare against the simhashes of posts already attached to open/recent `news_events` (time-windowed — e.g., last 48 hours, matching the architecture doc's clustering window) using Hamming distance. Below a configured distance threshold → same event.
- **Clustering decision** (two-way at V2, not three-way — the "ambiguous band requiring AI verification" from the architecture doc's Section 4 step 8 is explicitly V4, not V2):
  - distance below threshold → attach to the matching existing event, append to `event_sources`, increment `source_count`, update `last_updated_at`.
  - distance above threshold (or no candidate in the time window) → create a new `news_events` row, with this post as its first (and so far only) source.
- Update `raw_posts.processing_status` to `'processed'` once handled (a new value on the existing state machine — no new column needed).
- No AI is called anywhere in this version. No `ai_summary` field exists yet — `news_events.canonical_title` at V2 is simply the normalized text of the *first* post attached to that event, truncated to a reasonable length. This is a known, acceptable rough placeholder — it's explicitly not meant to read as a polished headline yet, and that's fine, because nothing downstream depends on it looking good until V4.

### 1.3 Why Text-Similarity Clustering Will Sometimes Be Wrong (documented, not hidden)

Simhash/text-similarity catches near-identical phrasing well (reposts, lightly-edited forwards) but will genuinely miss the architecture doc's headline example — "Government announces new fuel prices" vs. "Fuel prices to increase starting Monday" — since those share little literal text overlap despite being the same event. **This is expected and acceptable at V2.** Catching that case is exactly what V3's embedding-based semantic clustering is for. V2's job is the cheaper, faster first pass; it should be judged against "does it correctly merge near-identical text" not "does it correctly merge all paraphrases of the same event" — the latter is out of scope until V3.

### 1.4 AGENTS.md Updates

Append a V2-specific section (don't rewrite V0/V1 sections):
- Go code in `services/processor/` follows the same standards as any future Go service: `gofmt` + `golangci-lint`, per V0's `AGENTS.md` baseline (this is the first real Go service — get the pattern right here since V5 API will follow it).
- Clustering logic (`cluster.go`) is the highest-risk-of-silent-regression code in the system per V0's `AGENTS.md` note — any change to it requires tests demonstrating both a correct-merge case and a correct-non-merge case, not just one or the other.
- `services/processor/` reads from `raw_posts` and writes to `raw_posts` (status/simhash columns only), `news_events`, and `event_sources`. It must never modify `raw_posts.raw_text` — that field remains immutable per the architecture doc's provenance requirement, enforced now in code review, not just convention.
- Similarity thresholds (Hamming distance cutoff, time window) are configuration values, not hardcoded magic numbers — documented in `config.go` with the reasoning for the chosen defaults, so they're easy to tune once real clustering behavior is observed against live data.

### 1.5 ADR: Non-Semantic Clustering at V2 (0006)

- **Context**: the architecture doc's full pipeline includes semantic embedding similarity (Section 4, step 5) for correctly matching paraphrased posts about the same event. Implementing that now would require introducing an embedding provider dependency before V2's actual goal — proving the ingestion→processing→event pipeline works end-to-end — is achieved.
- **Decision**: V2 clusters using only exact-hash and simhash-based text similarity. This deliberately accepts under-clustering (same event, different wording, published as separate events) as a known V2 limitation, resolved in V3.
- **Consequences**: expect `news_events` at V2 to contain some duplicate events that a human (or V3's semantic layer) would recognize as the same story. This is fine — it doesn't corrupt data, it just means V2's output is intentionally rougher than the eventual product. No migration is needed to "fix" this later — V3 will simply reduce the rate of new events created going forward, and could optionally include a one-time backfill re-clustering pass, which is itself a V3-scope decision, not V2's.

### 1.6 Database Changes

New migration `0003_events_and_clustering.sql`:
```
ALTER TABLE raw_posts ADD COLUMN simhash BIGINT;

CREATE TABLE news_events (
  id BIGSERIAL PRIMARY KEY,
  canonical_title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',      -- 'active' only at V2; more values added in later versions
  first_seen_at TIMESTAMPTZ NOT NULL,
  last_updated_at TIMESTAMPTZ NOT NULL,
  source_count INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE event_sources (
  event_id BIGINT NOT NULL REFERENCES news_events(id) ON DELETE CASCADE,
  raw_post_id BIGINT NOT NULL REFERENCES raw_posts(id) ON DELETE RESTRICT,
  similarity_score INTEGER,                    -- Hamming distance at match time; NULL for the founding post
  added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (event_id, raw_post_id)
);

CREATE INDEX idx_raw_posts_processing_status ON raw_posts(processing_status);
CREATE INDEX idx_news_events_last_updated_at ON news_events(last_updated_at);
```
Deliberately **not** included yet (per the architecture doc's fuller schema, but out of V2 scope): `category_id`, `ai_summary`, `is_breaking`, `slug`, `embedding_centroid`, `entities`/`event_entities`, `media`. Adding nullable columns now for features that don't exist until V3/V4/V8 is exactly the kind of premature complexity the architecture doc warns against — they arrive in the migration that accompanies the version that actually uses them.

### 1.7 Reliability Scope

- If the processor crashes mid-decision (after reading a `raw_post`, before writing the event update), the `raw_post` remains `processing_status = 'ingested'` and gets picked up again on the next poll — safe by construction, since clustering a post twice against the same time-windowed candidates should produce the same decision (idempotent in effect, even without a unique constraint enforcing it directly — note this as an assumption in `cluster.go`'s tests: verify re-processing an already-processed-equivalent post doesn't create a second event).
- No distributed-lock/multi-instance coordination needed at V2 — a single processor instance is the explicit V2 assumption (matches V1's single-listener-instance assumption); multi-instance coordination is deferred to whichever later version actually needs horizontal scaling (per the architecture doc's scaling roadmap).

### 1.8 Testing

- Unit tests for `normalize.go`: whitespace/case/markdown-stripping produces expected output on fixture data.
- Unit tests for `simhash.go`: near-identical strings produce low Hamming distance; clearly different strings produce high distance.
- Unit tests for `cluster.go` using the **existing V0 fixture data's deliberate near-duplicate pair** — this is exactly what that fixture was created for back in V0 (see V0 spec 1.7) and should now finally be exercised by real logic: confirm the near-duplicate pair clusters into one event, and confirm a clearly unrelated fixture post creates a separate event.
- Integration test: process a batch of fixture posts against a real local Postgres (via existing docker-compose), confirm resulting `news_events`/`event_sources` row counts and relationships match expectations.

### 1.9 CI Updates

Extend `ci.yml` with a Go job for `services/processor/` (build, `golangci-lint`, `go test ./...`), alongside the existing Postgres/migration job and V1's Python job. All three must pass.

### 1.10 Logging

Per the established convention: one log line per raw_post processed, including the decision made (`attached_to_event_id: X` or `created_new_event_id: Y`) and the similarity score that drove it — this is the audit trail for clustering decisions, valuable for tuning thresholds once real data is flowing (and a lightweight precursor to the full `processing_audit` table the architecture doc describes for V4's AI-verification stage — V2 doesn't need that table yet since there's no AI decision to audit, just log the deterministic decision for now).

### 1.11 V2 Acceptance Criteria

- [x] `services/processor/` exists per 1.1, builds and runs locally via docker-compose.
- [x] Migration `0003_events_and_clustering.sql` applies cleanly on top of V0/V1's schema; `news_events`/`event_sources` match 1.6 exactly — no extra columns.
- [x] Processor picks up `ingested` raw_posts, normalizes them, and flips them to `processed`.
- [x] The V0 fixture's deliberate near-duplicate pair correctly clusters into a single `news_events` row — verified by automated test.
- [x] A batch of clearly distinct fixture posts correctly creates separate `news_events` rows — verified by automated test.
- [x] Running the processor against real data now sitting in `raw_posts` (from V1's live verification) produces plausible `news_events` rows — verified and presented for user review.
- [x] ADR-0006 exists and accurately describes the text-similarity-only limitation.
- [x] `AGENTS.md` V2 section exists per 1.4.
- [x] CI passes, including the new Go job.
- [x] No embeddings, vector search, AI calls, API, or frontend code exists anywhere in the repo.
- [x] `raw_posts.raw_text` is confirmed unmodified by any V2 code path (spot-check).


---

## 2. V2 IMPLEMENTATION PLAN

**Step 1 — Migration `0003_events_and_clustering.sql`**
- Creates: `news_events`, `event_sources`, `raw_posts.simhash` column, indexes, per 1.6.
- Purpose: schema foundation before any processor code is written.
- Checks: applies cleanly against the current V1 database state (real data included) without data loss; rollback path documented (a corresponding "down" migration or manual rollback note, per whatever convention V0's migration tool supports).
- Expected result: new tables exist, empty; existing `raw_posts` data untouched except the new nullable `simhash` column.

**Step 2 — Scaffold `services/processor/`**
- Creates: Go module, `config.go` (DB conn, poll interval, Hamming distance threshold, time window — with documented defaults), empty stubs for the other files.
- Purpose: establish the service skeleton and configuration surface before logic.
- Checks: `go build ./...` succeeds; lint runs clean on the skeleton.
- Expected result: an installable, lintable, empty service.

**Step 3 — `normalize.go`**
- Creates: normalization function per 1.2.
- Purpose: isolated, independently testable text-cleaning step.
- Checks: unit tests against V0's fixture data, asserting expected normalized output per fixture entry.
- Expected result: normalization behavior locked in by tests before simhash/clustering depend on it.

**Step 4 — `simhash.go`**
- Creates: simhash computation + Hamming distance function.
- Purpose: isolated similarity primitive, independently testable.
- Checks: unit tests — the fixture's near-duplicate pair produces low distance; an unrelated pair produces high distance; identical text produces distance 0.
- Expected result: a proven, tunable similarity measure before it's wired into clustering decisions.

**Step 5 — `store.go`**
- Creates: DB access functions — read next batch of `ingested` raw_posts, read recent `news_events` + their member simhashes within the time window, insert new event, attach post to existing event, update `processing_status`.
- Purpose: isolate all V2 SQL in one place, matching V0/V1's no-ORM convention.
- Checks: integration tests against real local Postgres — insert/attach operations produce exactly the expected rows, no duplicates on repeated calls with the same input.
- Expected result: proven data-layer correctness independent of the clustering decision logic itself.

**Step 6 — `cluster.go`**
- Creates: the two-way clustering decision function per 1.2, combining exact-hash and simhash comparison against `store.go`'s candidate-fetching.
- Purpose: the core V2 logic, kept as a pure decision function (input: normalized post + simhash + candidates; output: attach-to-X or create-new) so it's testable without hitting the DB for every test case.
- Checks: the fixture near-duplicate pair test from 1.8 (this is the one that matters most — don't skip it), plus edge cases (empty candidate list → always creates new; candidate outside the time window → not considered even if textually similar).
- Expected result: clustering behavior proven correct against known fixture cases before running against live data.

**Step 7 — `main.go`: poll loop wiring**
- Creates: the poll loop tying `store.go` (fetch) → `normalize.go` → `simhash.go` → `cluster.go` (decide) → `store.go` (write) together, with structured logging per 1.10.
- Purpose: the actual running service.
- Checks: run locally against the real V1 database (containing live-ingested posts), observe the resulting `news_events`/`event_sources` rows manually — this is the point where you personally look at the output and judge whether it's reasonable, since full correctness here isn't fully unit-testable.
- Expected result: a working end-to-end processing pass over real data, manually reviewed.

**Step 8 — ADR-0006 + AGENTS.md V2 section**
- Creates: `docs/adr/0006-non-semantic-clustering-at-v2.md`, V2 addendum to `AGENTS.md`.
- Purpose: document the deliberate limitation (1.3/1.5) before considering V2 done — this is the ADR most likely to be misread by a future agent as "a bug to fix" rather than "a documented, intentional V2 boundary," so get the wording right.
- Checks: manual review.
- Expected result: future agents (V3's) understand why V2 under-clusters and that fixing it is exactly V3's job, not a V2 regression.

**Step 9 — docker-compose + CI updates**
- Creates: adds `processor` service to `infra/docker-compose.yml`; adds the Go job to `ci.yml`.
- Purpose: make the processor part of the standard local-dev and CI loop, alongside the existing Postgres and listener services.
- Checks: `docker-compose up` brings up all three services (postgres, listener, processor) without errors; CI passes on a clean PR.
- Expected result: full V0+V1+V2 stack runs together via one command.

**Step 10 — Final review against V2 acceptance criteria**
- Purpose: verify, don't assume — same discipline as V0/V1.
- Checks: walk every item in 1.11, including the manual review of clustering output against real live data from V1.
- Expected result: V2 acceptance criteria fully met; ready to stop and hand off to V3.

---

## 3. V2 ANTIGRAVITY PROMPT

```
Read docs/architecture/platform-architecture.md, v0-foundation-spec.md,
v1-ingestion-spec.md, and v2-processing-spec.md (repo root) before doing anything
else. The last of these contains the full V2 spec, implementation plan, and your
exact instructions — follow it as written, in order. Follow AGENTS.md, including
the V0 and V1 sections already there and the V2 addendum you will create.

Inspect the current repository state first — V0 and V1 are already implemented
and committed, including a working Telegram listener with real live data already
sitting in raw_posts. Do not recreate or modify anything from V0/V1 except where
v2-processing-spec.md explicitly calls for it (the new simhash column, new
processing_status value).

Implement V2 only, exactly as specified in v2-processing-spec.md sections 1 and 2.
V2's sole job: normalize raw_posts, detect exact and near-duplicates via simhash
(text similarity only — no embeddings), and cluster matching posts into
news_events/event_sources rows.

Do NOT implement: embeddings, vector/semantic similarity, pgvector usage, AI
calls of any kind, ai_summary or categorization fields, a public API, a frontend,
SSE, or multi-instance/distributed processing coordination. If a future version
will need an interface or boundary here, note it in a comment or ADR — do not
build the feature itself. It is expected and acceptable that V2's clustering will
sometimes fail to merge paraphrased posts about the same event (e.g., the
"government announces fuel prices" vs "fuel prices to increase" example in the
architecture doc) — do not attempt to work around this with heuristics beyond
what the spec describes; that gap is intentionally closed in V3, not V2.

Work through the implementation plan (section 2) step by step. After each step,
run the relevant tests/lint/build checks before moving to the next. Step 7
(running the processor against real live data) requires manual review by me —
flag it clearly rather than declaring it verified on your own.

Do not introduce any new dependency, service, or infrastructure beyond what's
listed in the spec without first writing an ADR and pausing for confirmation.
If anything is ambiguous or not covered by the spec documents, stop and ask
rather than guessing.

When finished:
- Run the full test suite, lint, and confirm docker-compose brings up all three
  services (postgres, listener, processor) together.
- Show me a sample of the resulting news_events/event_sources rows from real
  live data so I can judge clustering quality myself.
- Report what was created/changed, what checks were run (automated vs. what
  needs my manual review), and explicit confirmation that no V3+ functionality
  was implemented.
- Stop. Do not continue into V3 without explicit instruction.
```

---

## V2 → V3 Handoff

V2 leaves in place: a working processor turning raw ingested posts into clustered `news_events`, ADR-0006 documenting the deliberate text-similarity-only limitation, and a proven pipeline (poll → normalize → similarity-check → cluster-decide → write) that V3 extends rather than replaces.

V3 (semantic clustering, per the architecture doc's roadmap) can build directly on this: add an embedding-generation step alongside (not instead of) the existing simhash check, add `pgvector` and `news_events.embedding_centroid` via a new migration, and upgrade `cluster.go`'s two-way decision into the architecture doc's three-way decision (auto-attach / new-event / needs-AI-verification — though the AI-verification *call* itself is still V4, V3 just needs to correctly identify and queue the ambiguous band). V3 should not need to touch `normalize.go`, `store.go`'s basic read/write functions, or the processor's poll-loop structure — those are stable V2 output for V3 to extend.
