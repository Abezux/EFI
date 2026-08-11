# V3 — Semantic Clustering
### Ethiopia News Aggregation Platform

**Scope note:** This document defines V3 only. It builds on V0 (foundation), V1 (Telegram ingestion, live-verified), and V2 (text-similarity clustering, live-verified). It references — but does not repeat — `docs/architecture/platform-architecture.md`. V3's job: add embedding-based semantic similarity alongside V2's simhash layer, upgrade the two-way clustering decision into the architecture doc's three-way decision, and correctly identify (not resolve) the ambiguous band. **No AI verification calls happen in V3** — that's V4. V3 only needs to correctly route posts into "confidently same event," "confidently different event," or "uncertain, needs review."

---

## 1. V3 SPEC

### 1.1 What V3 Adds to the Repository

```
/
├── services/
│   └── processor/
│       ├── embed.go               # embedding provider client (interface + one implementation)
│       ├── embed_test.go
│       ├── cluster.go              # UPDATED: three-way decision, combining simhash + embedding similarity
│       ├── cluster_test.go         # UPDATED: new tests for the "uncertain" band
│       ├── store.go                # UPDATED: read/write embedding, pgvector candidate search
│       └── config.go               # UPDATED: embedding provider config, similarity thresholds
├── docs/
│   └── adr/
│       └── 0007-embedding-provider-choice.md
├── db/
│   └── migrations/
│       └── 0004_semantic_clustering.sql   # pgvector extension, embedding columns, ANN index
└── infra/
    └── docker-compose.yml                  # unchanged structurally; processor now needs an API key env var
```

No new services, no `services/api/`, no frontend. This version only touches the processor.

### 1.2 Functional Scope (architecture doc Section 4, steps 5–6, three-way branch of step 8 minus the AI call itself)

- For each `raw_post` reaching the processor (after V2's normalization/simhash step, unchanged): generate an embedding of `normalized_text` via a hosted embedding API, store on `raw_posts.embedding` (column already exists from the original V0 schema, unused until now).
- **Candidate search**: instead of (or in addition to — see 1.3) V2's simhash-only candidate lookup, use `pgvector` ANN search against `news_events.embedding_centroid` for events within the existing time window (architecture doc's 48-hour window, same as V2).
- **Three-way clustering decision** (this is the actual V3 upgrade):
  - **cosine similarity above high threshold** → auto-attach to the matching event (same as V2's "attach" path, now using semantic rather than just lexical similarity — this is what correctly merges the fuel-price example from the architecture doc's opening scenario).
  - **cosine similarity below low threshold, and no simhash match either** → create a new event (same as V2's "new" path).
  - **similarity in between the two thresholds** → do **not** auto-decide. Mark the post `processing_status = 'needs_review'` (new status value) and leave it unattached to any event. This is the entire scope of "handling" the ambiguous band at V3 — correctly identifying it, not resolving it.
- When an event's centroid gets a new attached source, recompute `embedding_centroid` (simple running average of member embeddings is sufficient at V3 — don't over-engineer a weighted scheme yet).
- V2's simhash check is **not removed** — see 1.3 for why both layers stay.

### 1.3 Why Keep Simhash Alongside Embeddings (don't replace V2's logic)

Simhash and embeddings catch different things: simhash is cheap and excellent at catching literal reposts/forwards instantly (no API call needed), while embeddings are needed for genuine paraphrase detection but cost latency and money per call. The efficient design — and the one to implement — is:

1. Check simhash first (as in V2). If it matches → attach immediately, skip the embedding call entirely for that post's clustering decision (still generate and store the embedding for future centroid comparisons, but don't wait on it to make this decision).
2. If simhash doesn't find a match, only then fall through to the embedding/pgvector path.

This keeps the fast path fast (matches the architecture doc's latency budget from Section 5 — most reposts should still resolve in the sub-second simhash path, not the 200ms–1s embedding path) and only pays the embedding cost when it's actually needed.

### 1.4 AGENTS.md Updates

Append a V3-specific section:
- The embedding provider is accessed through an interface (`embed.go`), not called directly from `cluster.go` — this is the seam the architecture doc's "provider-agnostic" requirement (Section 2) refers to; swapping providers later should mean changing `embed.go`'s implementation, not touching clustering logic.
- `needs_review` posts are explicitly **not** processed further by any V3 code path — no auto-retry, no fallback heuristic to force a decision. They sit and wait for V4's AI verification step. An agent implementing V4 should treat `needs_review` as its primary work queue, not V3 revisiting its own decision logic.
- Embedding API calls must handle provider failures gracefully — per the architecture doc's Section 12 reliability requirement ("the system should not lose news simply because an AI service is temporarily unavailable"), a failed embedding call must **not** block the post from being published — see 1.6.

### 1.5 ADR: Embedding Provider Choice (0007)

- **Context**: the architecture doc calls for a multilingual embedding model (Amharic + English) and explicitly recommends starting with a hosted provider rather than self-hosting (Section 2).
- **Decision**: [to be filled in by you/the agent based on actual provider evaluation at implementation time — e.g., a hosted multilingual embedding endpoint from a major provider]. Document: why it was chosen (multilingual support including Amharic, cost per 1000 calls, latency), what was rejected and why, and that it's accessed only through the `embed.go` interface.
- **Consequences**: cost scales with post volume, not channel count (per the architecture doc's cost section) — worth monitoring from day one of V3, not waiting until it's a surprise. Note the specific rate limits of the chosen provider, since that directly affects the reliability handling in 1.6.

*(This ADR is intentionally left with a placeholder decision — evaluating and picking the actual embedding provider is legitimate implementation-time work, not something to lock in from this planning document. The agent should propose a specific choice with the reasoning above, and you should confirm before it's treated as final.)*

### 1.6 Reliability Scope

- If the embedding API call fails or times out for a given post: log the failure, leave `processing_status = 'ingested'` (i.e., don't advance past where V2 left it) so the poll loop retries it on a later pass, up to a small retry limit — beyond that, mark it `needs_review` as a safe fallback rather than silently dropping it. Either way, **the post is never lost**, and this must be demonstrated by a test that simulates a provider failure.
- No circuit breaker or backoff-and-retry framework needed at V3's scale (a handful of channels) — a simple bounded retry count in the poll loop is enough; note in the ADR that a real backoff strategy is worth revisiting once retry-storm risk becomes real (higher channel count).
- `embedding_centroid` recomputation must be transactionally consistent with the `event_sources` insert that triggered it — no window where a source is attached but the centroid hasn't caught up (matters for the next post's candidate search being accurate).

### 1.7 Database Changes

New migration `0004_semantic_clustering.sql`:
```
-- pgvector extension already enabled in 0001_init.sql per the original schema design;
-- confirm rather than re-run CREATE EXTENSION here.

ALTER TABLE news_events ADD COLUMN embedding_centroid vector(<dimension>);
-- <dimension> matches the chosen embedding model's output size, fixed per ADR-0007.

CREATE INDEX idx_news_events_embedding_centroid
  ON news_events USING hnsw (embedding_centroid vector_cosine_ops);

-- raw_posts.embedding already exists from the V0 schema (vector type, unused until now) —
-- no new column needed there, just confirm its dimension matches the chosen model.

-- New processing_status value: 'needs_review' — no schema change needed, it's just a new
-- string value in the existing text column; document it in AGENTS.md's V3 section (1.4)
-- rather than adding a CHECK constraint that would need another migration to extend later.
```

Deliberately **not** added yet: `processing_audit` table (that's V4's — it's for auditing AI *decisions*, and V3 makes no AI decisions, only deterministic threshold comparisons which are fully explained by the logged similarity score, same pattern as V2).

### 1.8 Testing

- Unit tests for `embed.go` using a mocked provider client (no real API calls in CI — matching V1's precedent of not hitting live Telegram in CI).
- Unit tests for the three-way `cluster.go` decision: high-similarity case → attach; low-similarity case → new event; mid-band case → `needs_review`, unattached. Use synthetic embeddings with known cosine distances for these tests rather than real API calls.
- **The architecture doc's own fuel-price example, finally testable**: construct a fixture-style test with the three differently-worded fuel-price sentences from the architecture doc's opening scenario, run them through real (or realistically mocked) embeddings, and confirm they cluster into one event — this is the single most important test in V3, since it's the exact case V2 was documented as unable to handle (ADR-0006) and V3 exists to fix.
- Integration test simulating an embedding provider failure (per 1.6): confirm the post isn't lost and eventually reaches either a clustering decision or `needs_review`.

### 1.9 CI Updates

Extend the existing Go job to include the new test files. No new CI infrastructure needed — embedding provider calls are mocked in tests, so no API key is required in CI (document this explicitly, since it also means CI can't catch real provider integration issues — that's what the manual live-review step, 1.11, is for).

### 1.10 Logging

Per the established convention: log the embedding call outcome (success/failure/latency) separately from the clustering decision, so provider performance and clustering accuracy can be reasoned about independently. The `needs_review` outcome should log the similarity score that put it in the ambiguous band — useful for tuning the two thresholds once real data volume builds up.

### 1.11 V3 Acceptance Criteria

- [ ] `embed.go` exists, calls a real hosted embedding provider, and is used only through its interface by `cluster.go`.
- [ ] Migration `0004_semantic_clustering.sql` applies cleanly on top of the current live database (V0–V2 data intact).
- [ ] The fuel-price three-sentence test (1.8) passes — this is the single acceptance criterion that matters most; V3 isn't done if this doesn't work.
- [ ] A clearly unrelated post does not get merged into an unrelated event (no false-positive regression from adding semantic matching).
- [ ] A genuinely ambiguous pair (constructed deliberately, or observed in real data) lands in `needs_review`, not force-decided either way.
- [ ] Simulated provider failure does not lose the post (1.6) — verified by test.
- [ ] Running the processor against a fresh batch of real live data produces at least one plausible semantic merge that V2's simhash alone would have missed — manually confirmed by you, since this is the actual proof the upgrade works, not just that tests pass.
- [ ] ADR-0007 exists with an actual (not placeholder) provider decision, confirmed by you.
- [ ] `AGENTS.md` V3 section exists per 1.4.
- [ ] CI passes.
- [ ] No AI verification calls, `ai_summary`, categorization, public API, or frontend code exists anywhere in the repo.
- [ ] `raw_posts.raw_text` still confirmed unmodified.

---

## 2. V3 IMPLEMENTATION PLAN

**Step 1 — ADR-0007: propose an embedding provider**
- Creates: `docs/adr/0007-embedding-provider-choice.md` with a concrete, reasoned recommendation (multilingual support incl. Amharic, cost, latency, rate limits).
- Purpose: this decision needs to be made explicitly and reviewed by you before any code depends on it — don't let the agent silently pick a provider mid-implementation.
- Checks: manual review and explicit confirmation from you before proceeding to Step 2.
- Expected result: a named, justified provider choice you've approved.

**Step 2 — Migration `0004_semantic_clustering.sql`**
- Creates: `embedding_centroid` column + HNSW index on `news_events`, per 1.7, using the dimension from the confirmed provider.
- Purpose: schema ready before embedding code is written.
- Checks: applies cleanly against the live database; confirm `raw_posts.embedding`'s existing dimension matches (or note if the original V0 schema's vector column needs a dimension fix — flag this rather than silently working around a mismatch).
- Expected result: schema ready for real embeddings to be stored.

**Step 3 — `embed.go`: provider client + interface**
- Creates: an interface (e.g., `Embedder.Embed(text string) ([]float32, error)`) and one real implementation calling the chosen provider, with the API key read from config/environment (never hardcoded, per V0's secrets convention).
- Purpose: isolate the one genuinely new external dependency behind a clean seam.
- Checks: unit tests against a mocked implementation of the interface; a separate, manual (not-CI) smoke test script to confirm the real provider integration actually works, run once by you/the agent together with real credentials.
- Expected result: a working, swappable embedding client.

**Step 4 — Update `cluster.go`: three-way decision**
- Creates: the simhash-first, embedding-fallback, three-way branch logic per 1.2–1.3.
- Purpose: the actual V3 upgrade to the decision function, kept as a pure/testable function per V2's established pattern.
- Checks: the fuel-price test (1.8) — this is the priority — plus the false-positive regression check and the ambiguous-band test.
- Expected result: three-way decisions proven correct against known cases, especially the case V2 documented as out of scope.

**Step 5 — Update `store.go`: pgvector candidate search + centroid maintenance**
- Creates: ANN query against `embedding_centroid`, transactionally-consistent centroid recomputation on attach (per 1.6).
- Purpose: the data-layer support for the new decision logic.
- Checks: integration tests against real local Postgres — attach a post, confirm centroid updates correctly and consistently; concurrent-safety isn't a V3 concern (single processor instance, per V2's existing assumption) but transactional correctness within one process still is.
- Expected result: proven centroid maintenance.

**Step 6 — Reliability: provider-failure handling**
- Creates: retry-with-bound logic in the poll loop for embedding failures, fallback to `needs_review` after the retry limit, per 1.6.
- Purpose: satisfy the architecture doc's "don't lose news if AI is unavailable" requirement, now that an AI-adjacent dependency exists for the first time.
- Checks: simulated failure test (mock the embedder to always error) confirms the post isn't lost and reaches a terminal, safe state.
- Expected result: proven graceful degradation.

**Step 7 — Wire it into `main.go`'s poll loop, update logging**
- Creates: the updated end-to-end flow (simhash check → embedding call if needed → three-way decision → write), structured logs per 1.10.
- Purpose: the actual running V3 processor.
- Checks: run locally against a batch of real live data (or freshly ingested posts if you post new test content), manually review the resulting merges — specifically look for any case where a semantic merge happened that simhash alone wouldn't have caught, per acceptance criterion 1.11.
- Expected result: a working end-to-end semantic clustering pass, manually reviewed by you.

**Step 8 — AGENTS.md V3 section + final ADR polish**
- Creates: V3 addendum to `AGENTS.md` per 1.4; finalize ADR-0007 with real observed cost/latency numbers from Step 7's live run if available.
- Purpose: document before declaring done.
- Checks: manual review.
- Expected result: future agents (V4's) understand the `needs_review` queue is theirs to pick up, and understand the embedding-interface seam.

**Step 9 — Final review against V3 acceptance criteria**
- Purpose: verify, don't assume.
- Checks: walk every item in 1.11, with special attention to the fuel-price test and the live-data manual review — these two are the actual proof V3 achieved its purpose, not just that code compiles and unit tests are green.
- Expected result: V3 acceptance criteria fully met; ready to hand off to V4.

---

## 3. V3 ANTIGRAVITY PROMPT

```
Read docs/architecture/platform-architecture.md, v0-foundation-spec.md,
v1-ingestion-spec.md, v2-processing-spec.md, and v3-semantic-clustering-spec.md
(repo root) before doing anything else. The last of these contains the full V3
spec, implementation plan, and your exact instructions — follow it as written,
in order. Follow AGENTS.md, including all prior version sections and the V3
addendum you will create.

Inspect the current repository state first — V0, V1, and V2 are already
implemented, committed, and pushed, including a working processor with
simhash-based text-similarity clustering already running against live data.
Do not recreate or modify V0/V1/V2 code except where v3-semantic-clustering-spec.md
explicitly calls for it (updates to cluster.go, store.go, config.go).

IMPORTANT — Step 1 requires my confirmation before you proceed: propose a
specific embedding provider in ADR-0007 (per section 1.5 of the spec — must
support multilingual text including Amharic), with your reasoning on cost,
latency, and rate limits. Stop and show me this ADR. Do not write any embedding
integration code until I confirm the provider choice.

Once I've confirmed the provider, implement V3 per sections 1 and 2 of the spec.
V3's job: add embedding-based semantic similarity alongside the existing simhash
check (simhash first, embedding fallback — do not remove or replace the simhash
logic), and upgrade the clustering decision from two-way to three-way (attach /
new event / needs_review for the ambiguous band). Do NOT attempt to resolve the
needs_review band with any heuristic — leaving it correctly unattached is the
entire V3 deliverable for that case.

Do NOT implement: any AI verification call, ai_summary or categorization fields,
a public API, a frontend, or a processing_audit table (that's V4's). If a future
version will need an interface or boundary here, note it in a comment or ADR —
do not build the feature itself.

The most important test in this version is the fuel-price example from the
architecture doc's opening scenario (three differently-worded sentences about
the same event) — construct this test and make sure it passes before considering
V3 functionally complete. Also test that adding semantic matching doesn't cause
false-positive merges of genuinely unrelated posts.

Work through the implementation plan step by step, running tests/lint/build
checks after each step. Provider API calls must be mocked in automated tests —
no real API key required in CI. Flag clearly which checks are automated vs.
which require my manual review (especially Step 7's live-data review and any
real API smoke test).

Do not introduce any new dependency or infrastructure beyond what's specified
without first writing an ADR and pausing for confirmation. If anything is
ambiguous, stop and ask rather than guessing.

When finished:
- Run the full test suite, lint, and build.
- Show me the fuel-price test passing explicitly, plus a sample of real
  semantic merges from live data that simhash alone would have missed.
- Report what was created/changed, what's automated vs. needs my review, and
  confirm no V4+ functionality was implemented.
- Stop. Do not continue into V4 without explicit instruction.
```

---

## V3 → V4 Handoff

V3 leaves in place: working semantic clustering (simhash-first, embedding-fallback), a `needs_review` queue of genuinely ambiguous posts sitting untouched, ADR-0007 documenting the embedding provider decision, and a proven three-way decision function.

V4 (AI enrichment, per the architecture doc's roadmap) can build directly on this: introduce an LLM call to resolve `needs_review` posts (the actual "AI verification" step the architecture doc describes), add the `processing_audit` table to log those decisions with model/version/confidence, and add `ai_summary`/categorization generation for stable published events. V4 should not need to touch `embed.go`, the simhash logic, or the auto-attach/new-event paths of `cluster.go` — those are stable V3 output; V4's job is specifically the band V3 deliberately left unresolved.
