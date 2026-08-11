# V4 — AI Enrichment
### Ethiopia News Aggregation Platform

**Scope note:** This document defines V4 only. It builds on V0–V3 (foundation, ingestion, text-similarity clustering, semantic clustering — all live-verified). It references — but does not repeat — `docs/architecture/platform-architecture.md`. V4's job: resolve the `needs_review` queue V3 deliberately left unresolved, generate `ai_summary` and basic categorization/entities for stable events, and build the `processing_audit` trail the architecture doc requires so every AI-generated field is traceable and never blindly trusted. No public API, no frontend.

---

## 1. V4 SPEC

### 1.1 What V4 Adds to the Repository

```
/
├── services/
│   └── processor/
│       ├── llm.go                  # LLM provider client (interface + implementation)
│       ├── llm_test.go
│       ├── verify.go                # resolves needs_review posts via LLM same/different-event call
│       ├── verify_test.go
│       ├── enrich.go                # generates ai_summary, category, entities for stable events
│       ├── enrich_test.go
│       ├── cluster.go               # UPDATED: needs_review posts now get picked up by verify.go
│       └── store.go                 # UPDATED: processing_audit writes, category/entity writes
├── docs/
│   └── adr/
│       └── 0008-llm-provider-choice.md
├── db/
│   └── migrations/
│       └── 0005_ai_enrichment.sql   # processing_audit, categories, entities, event_entities, ai_summary
└── infra/
    └── docker-compose.yml           # unchanged structurally; processor now also needs LLM API key
```

Still no `services/api/` or `web/`. This version only touches the processor.

### 1.2 Functional Scope (architecture doc Section 4, steps 8–9)

**A. Resolving `needs_review` (architecture doc step 8 — AI verification for uncertain cases)**

- The processor now also polls `raw_posts` where `processing_status = 'needs_review'`.
- For each, construct a prompt asking the LLM a narrow, specific question: "do these two texts describe the same real-world news event?" — comparing the post's `normalized_text` against the candidate event's `canonical_title` (or a representative source's text). This is a **targeted, bounded classification call**, not an open-ended request — keep the prompt narrow per the architecture doc's anti-hallucination principle (Section 0/4).
- Store the LLM's decision, confidence, model name/version, and raw reasoning in `processing_audit` (new table, 1.6) — this is the audit trail requirement, non-negotiable per the architecture doc.
- If the LLM says "same event" above a confidence threshold → attach to the candidate event (same as V3's auto-attach path, just AI-confirmed instead of purely score-based). If "different event" → create a new event. If the LLM itself is uncertain (low confidence, or a malformed/ambiguous response) → **do not force a decision** — leave it `needs_review` and log why, rather than let a low-confidence AI guess silently become ground truth. This preserves the architecture doc's "don't blindly trust AI" principle at the one point where it matters most.

**B. Enrichment for stable events (architecture doc step 9)**

- An event is "stable" for enrichment purposes when it hasn't gained a new source in a configurable window (e.g., 15–30 minutes) since its last update — matching the architecture doc's Section 4 note about not re-summarizing on every near-duplicate. Re-enrichment triggers again only if a materially later new source attaches.
- For stable events without an `ai_summary` yet (or with one that predates their most recent source), call the LLM once to generate: a short `ai_summary` (2–4 sentences, in the agent's own words — not a copy of any single source, consistent with the copyright-safety posture noted in the architecture doc's Section 12), a single `category` (from a small fixed list — see 1.6), and a short list of named `entities` (people/places/organizations mentioned).
- Every enrichment call is also logged to `processing_audit` — model, version, and what was generated — so any summary can be traced back to which call produced it.
- **`ai_summary` is always clearly a generated field.** The spec doesn't build a frontend yet, but the data model itself must keep this field distinguishable from source facts — it lives only on `news_events`, never overwrites `raw_posts.raw_text`, and its presence in `processing_audit` means a future API/frontend can label it "AI-generated summary" honestly rather than presenting it as a direct quote.

### 1.3 Why the Verification Prompt Must Stay Narrow

The architecture doc's Section 0/4 anti-hallucination requirement isn't a footnote here — it's the actual design constraint for V4. A same/different-event classification call is low-risk: it's a bounded question with a checkable answer, easy to log and audit, and wrong answers just mean a post sits in `needs_review` a bit longer or gets its own event — no data is corrupted either way. An open-ended "tell me about this event" call would be higher-risk (more room for the model to add unsupported detail). Keep verification and enrichment as **separate, narrowly-scoped calls** (already true per 1.2's A/B split) rather than merging them into one do-everything prompt — this is a deliberate architectural choice, not just a style preference, and it directly serves the audit-trail requirement (each call's purpose and output are independently checkable).

### 1.4 AGENTS.md Updates

Append a V4-specific section:
- `llm.go` is accessed only through an interface, same pattern as V3's `embed.go` — provider-agnostic per the architecture doc's Section 2 requirement.
- Every LLM call (verification or enrichment) must write a `processing_audit` row before or atomically with whatever decision it drives — no LLM output is used to change data without a corresponding audit entry. This is enforced in code review, not just convention: a PR adding a new LLM call site without a matching audit write should be rejected.
- `ai_summary`, category, and entities are never presented, logged, or treated as verified fact — code and comments referring to them should consistently describe them as "generated" or "AI-classified," not "determined" or "confirmed."
- Low-confidence verification results must not force a clustering decision (1.2.A) — this is a specific rule the agent must not "optimize away" even if it seems like it would clear the `needs_review` queue faster.

### 1.5 ADR: LLM Provider Choice (0008)

- **Context**: needed for both the narrow verification classification (1.2.A) and short-form enrichment generation (1.2.B). Given V3's choice, sticking with Gemini for text generation (not just embeddings) is a reasonable default — same account, same billing, same free-tier considerations — but should be confirmed, not assumed.
- **Decision**: *(placeholder — same pattern as ADR-0007: the agent proposes a specific model, e.g., a Gemini text-generation model, with reasoning on cost, latency, and multilingual/Amharic generation quality, and you confirm before implementation.)*
- **Consequences**: note actual free-tier limits for text generation specifically (these are typically different from the embedding endpoint's limits), and note that generation calls are more expensive than embedding calls — worth watching quota more closely here than in V3.

### 1.6 Database Changes

New migration `0005_ai_enrichment.sql`:
```
CREATE TABLE processing_audit (
  id BIGSERIAL PRIMARY KEY,
  raw_post_id BIGINT REFERENCES raw_posts(id) ON DELETE SET NULL,
  news_event_id BIGINT REFERENCES news_events(id) ON DELETE SET NULL,
  stage TEXT NOT NULL,              -- 'verification' | 'enrichment'
  decision TEXT,                    -- e.g. 'same_event' | 'different_event' | 'summary_generated'
  confidence REAL,
  model_used TEXT NOT NULL,         -- e.g. 'gemini-<version>'
  raw_response TEXT,                -- the model's actual output, for auditability
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE categories (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  slug TEXT NOT NULL UNIQUE
);
-- Seed a small fixed list matching realistic categories for your actual channel
-- content (finance-focused, per your channel list) — e.g., 'Markets', 'Policy',
-- 'Banking', 'Currency', 'Companies', 'General' — confirm the actual list with
-- the user rather than inventing a generic news taxonomy that doesn't fit.

ALTER TABLE news_events
  ADD COLUMN ai_summary TEXT,
  ADD COLUMN category_id INTEGER REFERENCES categories(id);

CREATE TABLE entities (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL,               -- 'person' | 'place' | 'organization'
  UNIQUE (name, type)
);

CREATE TABLE event_entities (
  event_id BIGINT NOT NULL REFERENCES news_events(id) ON DELETE CASCADE,
  entity_id BIGINT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  PRIMARY KEY (event_id, entity_id)
);

CREATE INDEX idx_processing_audit_raw_post_id ON processing_audit(raw_post_id);
CREATE INDEX idx_processing_audit_news_event_id ON processing_audit(news_event_id);
```
Deliberately **not** added yet: `is_breaking`, `slug` (both are frontend/SEO concerns — V6/V8), `media` table (no image handling yet — that's implied by later versions once the frontend needs it).

### 1.7 Reliability Scope

- If a verification call fails (provider error/timeout): same bounded-retry-then-leave-as-`needs_review` pattern established in V3 for embedding failures (1.6 of the V3 spec) — the post is never lost, never force-decided on a failure.
- If an enrichment call fails: the event is still published/stable without an `ai_summary` — per the architecture doc's explicit requirement ("the system should not lose news simply because an AI service is temporarily unavailable"). Retry enrichment on a later poll pass; don't block anything else on it.
- `processing_audit` writes themselves should never be the reason a decision is lost — if the audit write fails, the whole operation (decision + audit) should roll back together (same transaction), not partially apply. Better to retry the whole thing than have an undocumented AI-driven decision in the data.

### 1.8 Testing

- Unit tests for `llm.go` using a mocked provider (no real API calls in CI, same pattern as `embed.go`).
- Unit tests for `verify.go`: high-confidence "same" → attach; high-confidence "different" → new event; low-confidence → stays `needs_review`, with a specific test confirming a genuinely low-confidence mocked response does **not** get force-decided (this is the test that proves 1.3/1.4's core safety property).
- Unit tests for `enrich.go`: stable event with mocked LLM response → `ai_summary`/category/entities populated correctly; a "not yet stable" event (recent source) → enrichment skipped, verified by a test that changes the last-updated timestamp and confirms the trigger condition.
- Test that every successful verification/enrichment call produces a corresponding `processing_audit` row (integration test against real local Postgres) — this is the single most important structural test in V4, since the whole audit-trail requirement rests on it always being true, not just usually true.
- Test simulating provider failure for both verify and enrich paths, confirming graceful degradation per 1.7.

### 1.9 CI Updates

Extend the existing Go job. LLM calls mocked in tests, no real API key required in CI (same rationale as V3).

### 1.10 Logging

Per the established convention: log every verification/enrichment call's outcome (decision, confidence, model, latency) — this is largely a mirror of what's already written to `processing_audit`, but in the structured log stream too, so operational monitoring doesn't require querying the DB directly for basic visibility.

### 1.11 V4 Acceptance Criteria

- [ ] `llm.go` exists, calls a real confirmed LLM provider, used only through its interface.
- [ ] Migration `0005_ai_enrichment.sql` applies cleanly on top of the live V0–V3 database.
- [ ] A real post currently sitting in `needs_review` (if any exist from V3's live testing) gets correctly resolved by a real LLM call — manually reviewed by you, not just trusted from a log line.
- [ ] A test confirms a low-confidence verification response does **not** force a decision — this is checked explicitly, not just implied by other passing tests.
- [ ] A stable real event gets a real `ai_summary`, category, and at least one entity generated — manually reviewed by you for whether the summary is actually reasonable (accuracy, not just "a summary exists").
- [ ] Every verification/enrichment call in the live run has a matching `processing_audit` row — spot-checked directly in the database.
- [ ] Simulated provider failure for both verify and enrich paths leaves data intact and retryable — verified by test.
- [ ] `raw_posts.raw_text` still confirmed unmodified; `ai_summary` never overwrites or is confused with source text anywhere in the code.
- [ ] ADR-0008 exists with a real (not placeholder) confirmed provider decision.
- [ ] `AGENTS.md` V4 section exists per 1.4.
- [ ] CI passes.
- [ ] No public API or frontend code exists anywhere in the repo.

---

## 2. V4 IMPLEMENTATION PLAN

**Step 1 — ADR-0008: propose an LLM provider for generation**
- Creates: `docs/adr/0008-llm-provider-choice.md` with a concrete recommendation (likely Gemini text-generation, given V3, but confirmed rather than assumed) — cost, latency, Amharic generation quality, free-tier limits for generation specifically.
- Purpose: same discipline as ADR-0007 — confirm before code depends on it.
- Checks: manual review and explicit confirmation from you.
- Expected result: a named, justified, approved provider+model choice.

**Step 2 — Migration `0005_ai_enrichment.sql`**
- Creates: `processing_audit`, `categories` (seeded with a category list you confirm fits your actual finance-focused channels — don't let the agent invent a generic taxonomy), `entities`, `event_entities`, `news_events.ai_summary`/`category_id`, per 1.6.
- Purpose: schema ready before LLM code is written.
- Checks: applies cleanly against the live database; category seed list reviewed and confirmed by you before proceeding.
- Expected result: schema ready, with a category list that actually matches your content.

**Step 3 — `llm.go`: provider client + interface**
- Creates: an interface (e.g., `LLMClient.Classify(...)`, `LLMClient.Generate(...)`) and the real Gemini (or confirmed alternative) implementation.
- Purpose: isolate the new dependency behind a clean seam, same pattern as `embed.go`.
- Checks: unit tests against a mocked implementation; a manual smoke test with real credentials run once by you/the agent together.
- Expected result: a working, swappable LLM client supporting both the narrow classification call and the short generation call.

**Step 4 — `verify.go`: resolve `needs_review`**
- Creates: the narrow same/different-event classification flow per 1.2.A, writing to `processing_audit` and driving the attach/new-event/stay-unresolved outcome.
- Purpose: finally close the loop V3 deliberately left open.
- Checks: the low-confidence-doesn't-force-a-decision test (1.8) — priority — plus the straightforward high-confidence same/different cases.
- Expected result: proven, safe resolution logic for the ambiguous band.

**Step 5 — `enrich.go`: summaries, categorization, entities**
- Creates: the stability check, summary/category/entity generation flow per 1.2.B, writing to `news_events` and `processing_audit`.
- Purpose: the actual enrichment product feature.
- Checks: unit tests per 1.8; a real run against a real stable event from your live data, manually reviewed by you for summary quality/accuracy (not just presence).
- Expected result: proven enrichment logic, with output you've personally judged as reasonable.

**Step 6 — Reliability: provider-failure handling for both paths**
- Creates: bounded retry + graceful degradation for both `verify.go` and `enrich.go`, per 1.7.
- Purpose: satisfy the architecture doc's "don't lose news if AI is unavailable" requirement for this version's two new AI-dependent paths.
- Checks: simulated failure tests for both paths.
- Expected result: proven graceful degradation, matching V3's established pattern.

**Step 7 — Wire into `main.go`'s poll loop, update logging**
- Creates: the updated poll loop now also handling `needs_review` posts and stable-event enrichment passes, structured logs per 1.10.
- Purpose: the actual running V4 processor.
- Checks: run locally against real live data — resolve any real `needs_review` posts, enrich any real stable events, manually review both outcomes.
- Expected result: a working end-to-end AI enrichment pass, manually reviewed by you.

**Step 8 — AGENTS.md V4 section + audit-trail spot-check**
- Creates: V4 addendum to `AGENTS.md` per 1.4.
- Purpose: document before declaring done, and specifically verify the audit-trail property holds.
- Checks: query `processing_audit` directly and confirm every verification/enrichment decision in the live run has a matching row — this is the check that proves the architecture doc's "don't blindly trust AI" requirement is actually implemented, not just described.
- Expected result: a verified, complete audit trail.

**Step 9 — Final review against V4 acceptance criteria**
- Purpose: verify, don't assume.
- Checks: walk every item in 1.11.
- Expected result: V4 acceptance criteria fully met; ready to hand off to V5.

---

## 3. V4 ANTIGRAVITY PROMPT

```
Read docs/architecture/platform-architecture.md, v0-foundation-spec.md,
v1-ingestion-spec.md, v2-processing-spec.md, v3-semantic-clustering-spec.md,
and v4-ai-enrichment-spec.md (repo root) before doing anything else. The last
of these contains the full V4 spec, implementation plan, and your exact
instructions — follow it as written, in order. Follow AGENTS.md, including all
prior version sections and the V4 addendum you will create.

Inspect the current repository state first — V0 through V3 are implemented,
committed, and pushed, including working semantic clustering verified against
real Gemini embeddings and real live data, with some real posts currently
sitting in needs_review status. Do not recreate or modify V0-V3 code except
where v4-ai-enrichment-spec.md explicitly calls for it.

IMPORTANT — Step 1 requires my confirmation before you proceed: propose a
specific LLM provider/model for text generation in ADR-0008 (likely Gemini,
given V3's choice, but confirm rather than assume), with reasoning on cost,
latency, and Amharic generation quality. Also propose a category list (Step 2)
that actually fits my finance-focused channel content — not a generic news
taxonomy. Stop and show me both before writing any LLM integration code.

Once I've confirmed the provider and category list, implement V4 per sections
1 and 2 of the spec. V4 has two distinct jobs, kept as separate narrow calls
per section 1.3 — do not merge them into one prompt:

1. Resolve needs_review posts via a narrow same/different-event classification
   call (verify.go). A low-confidence result must NOT force a decision — leave
   it needs_review and log why. This rule is not optional or something to
   "optimize away."
2. Generate ai_summary, category, and entities for stable events only
   (enrich.go) — an event is stable when it hasn't gained a new source in a
   configurable window.

Every single verification and enrichment call must write a matching row to
processing_audit (model, decision, confidence, raw response) — no exceptions,
enforced as a structural test, not just a convention.

Do NOT implement: a public API, a frontend, is_breaking/slug fields, or media
handling — those are later versions. ai_summary must never overwrite or be
confused with raw_posts.raw_text anywhere in the code.

Work through the implementation plan step by step, running tests/lint/build
checks after each step. Provider calls must be mocked in automated tests — no
real API key required in CI. Flag clearly which checks are automated vs. which
require my manual review, especially the real-data verification/enrichment
runs in Steps 4, 5, and 7.

Do not introduce any new dependency or infrastructure beyond what's specified
without first writing an ADR and pausing for confirmation. If anything is
ambiguous, stop and ask rather than guessing.

When finished:
- Run the full test suite, lint, and build.
- Show me: any real needs_review post that got resolved, the reasoning/confidence
  behind it; a real ai_summary generated for a real stable event, for my
  accuracy review; and confirmation (via a direct query) that processing_audit
  has a matching row for every decision made in this run.
- Report what was created/changed, what's automated vs. needs my review, and
  confirm no V5+ functionality was implemented.
- Stop. Do not continue into V5 without explicit instruction.
```

---

## V4 → V5 Handoff

V4 leaves in place: a resolved (or safely still-pending) `needs_review` queue, real `ai_summary`/category/entity data on stable events, a complete `processing_audit` trail for every AI decision, and ADR-0008 documenting the generation provider choice.

V5 (public API, per the architecture doc's roadmap) can build directly on this: expose `news_events` (with their now-real summaries, categories, and sources) via REST endpoints, without needing to touch any processor code — the processor's job (ingest → cluster → enrich) is functionally complete as of V4. V5 should only revisit V4's code if the API layer surfaces a data-shape problem (e.g., a field that's awkward to serialize), in which case that's a migration + ADR, following the same discipline every prior version handoff has followed.
