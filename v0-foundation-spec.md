# V0 — Project Foundation Specification
### Ethiopia News Aggregation Platform

**Scope note:** This document defines V0 only — the repository, tooling, and conventions that make future AI-assisted development safe and consistent. It references, but does not repeat, the full architecture in `ethiopia-news-platform-architecture-spec.md`. V0 produces no product functionality.

---

## 1. V0 SPEC

### 1.1 Repository Structure

Minimum needed now — later services/dirs are created when their version arrives, not pre-scaffolded empty.

```
/
├── AGENTS.md
├── README.md
├── docs/
│   ├── architecture/
│   │   └── platform-architecture.md      # the existing full spec, copied here
│   ├── adr/
│   │   ├── 0001-postgres-only-queue-at-v1.md
│   │   ├── 0002-sse-over-websockets.md
│   │   ├── 0003-pgvector-over-dedicated-vector-db.md
│   │   └── 0004-language-split-python-listener-go-processor.md
│   └── runbooks/                          # empty at V0, structure only + .gitkeep
├── db/
│   └── migrations/
│       └── 0001_init.sql                  # channels, raw_posts only (per V1 scope)
├── infra/
│   └── docker-compose.yml                 # Postgres only at V0
├── .github/
│   └── workflows/
│       └── ci.yml
├── .env.example
├── .gitignore
├── .editorconfig
└── tests/
    └── fixtures/
        └── sample_telegram_posts.json      # fixture data, no service consumes it yet
```

No `services/`, `web/`, or `api/` code directories are created in V0. Creating empty placeholder service folders invites the agent to fill them prematurely — they get created in the version that actually needs them (V1 creates `services/listener/`, etc.).

### 1.2 `AGENTS.md`

Must state, at minimum:
- V0's job is foundation only; link to the architecture doc and this V0 spec as the source of truth.
- Before any change, read `docs/architecture/platform-architecture.md`, relevant ADRs in `docs/adr/`, and this file.
- No new infrastructure dependency (queue, cache, external service, framework) without first writing an ADR proposing it.
- No service code, ingestion, API, or frontend code in V0 — if asked to add functionality beyond V0 scope, stop and flag rather than proceed.
- All schema changes go through a new numbered migration file in `db/migrations/` — never edit an existing migration once merged.
- Coding standards: Go — `gofmt` + `golangci-lint`; SQL migrations — plain numbered `.sql` files, no ORM at V0. (Python/TypeScript standards get added in the ADR/AGENTS update that introduces those languages' first real code, i.e., V1.)
- Commit messages follow Conventional Commits.
- Every change must pass lint + tests + build in CI before merge; the agent must run these locally before considering a step done.
- If uncertain whether something is in scope, the agent stops and asks rather than guessing.

### 1.3 `docs/` Structure

- `docs/architecture/` — the source-of-truth platform spec. V0 copies the existing document in verbatim; future versions update it in place as decisions evolve.
- `docs/adr/` — one file per architectural decision, numbered sequentially, using a fixed minimal template (Title / Status / Context / Decision / Consequences).
- `docs/runbooks/` — structure created now (empty, `.gitkeep`), populated starting V1 (e.g., "Telegram session expired"). Creating the folder now signals the convention without inventing content prematurely.

### 1.4 ADR Structure + Essential Initial ADRs

Template (`docs/adr/000X-title.md`):
```
# ADR-000X: <Title>
Status: Accepted | Proposed | Superseded
Date: YYYY-MM-DD

## Context
## Decision
## Consequences
```

Only four ADRs are essential at V0 — they justify decisions that already constrain V0's own foundation work (the DB and dev-environment choices) or that a coding agent might otherwise "helpfully" second-guess later:

1. **0001 — Postgres-only queue at V1** (no Kafka/Redis yet)
2. **0002 — SSE over WebSockets** (relevant later, but documented now so no agent introduces WS infra prematurely)
3. **0003 — pgvector over a dedicated vector DB**
4. **0004 — Python listener / Go processor language split**

All four are already justified in the architecture doc — the ADRs here are short, extracted, decision-focused records, not new analysis.

### 1.5 Database / Migration Foundation

- Single Postgres instance, no ORM — plain numbered SQL migration files, applied via a minimal migration runner (e.g., `golang-migrate` CLI — a widely-used, dependency-light tool, not a framework).
- `0001_init.sql` creates exactly two tables, matching V1's declared scope from the architecture doc: `channels` and `raw_posts`, including the `UNIQUE (channel_id, telegram_message_id)` idempotency constraint.
- No other tables (`news_events`, `categories`, etc.) are created yet — they arrive in the migration that accompanies the version that needs them, keeping schema and code in lockstep.
- Migrations run against a local Postgres via docker-compose; a `make migrate` (or equivalent script) target applies them.

### 1.6 Docker / Local Development

- `infra/docker-compose.yml` defines **one service: Postgres**. Nothing else — no app containers yet, since there's no app code in V0.
- `.env.example` documents the DB connection variables needed locally.
- A short `README.md` "Getting started" section: clone, copy `.env.example` to `.env`, `docker-compose up -d`, run migrations, run tests.
- Goal: any future agent or contributor can get a working local Postgres with the V1-scope schema applied in under five minutes, with zero real credentials required.

### 1.7 Test Foundation + Fixtures

- `tests/fixtures/sample_telegram_posts.json` — a small set of realistic fixture posts, including deliberate near-duplicates across different fake "channels," matching the format V1's listener will eventually ingest. No code consumes this yet; it exists so V1 can start writing ingestion tests against real-shaped data on day one.
- A placeholder test-running convention is established (e.g., a `tests/` root and a CI step that runs `go test ./...` where applicable) even though there's no application code to test yet — the CI step should currently pass trivially (e.g., a single smoke test asserting the fixture file parses as valid JSON matching an expected schema). This proves the test pipeline works before there's real logic to protect with it.

### 1.8 Linting / Formatting

- Go: `gofmt` (formatting) + `golangci-lint` (linting), config file `.golangci.yml` with a sane default ruleset — introduced now even though there's minimal Go code yet, so the convention exists before code accumulates.
- SQL: consistent lowercase-keyword, snake_case style enforced by review convention (documented in `AGENTS.md`), no separate SQL linter at V0 — unnecessary tooling for two tables.
- General: `.editorconfig` for consistent whitespace/line-endings across editors.
- Markdown: no linter at V0 — premature for two doc types.

### 1.9 CI

Single GitHub Actions workflow (`.github/workflows/ci.yml`), triggered on PRs and pushes to main:
1. Checkout
2. Spin up Postgres (service container)
3. Run migrations against it
4. Run lint (`golangci-lint` if any Go exists, else skip step gracefully)
5. Run tests (fixture validation test, migration-applies-cleanly test)
6. Fail the build on any of the above failing

No deploy step at V0 — there is nothing to deploy yet.

### 1.10 Environment / Secrets Strategy

- `.env.example` lists all variables needed (at V0: just DB connection details), committed to the repo.
- `.env` is gitignored, never committed.
- No real secrets exist at V0 (no Telegram credentials, no AI provider keys yet) — but the convention is established now: secrets always via environment variables, never hardcoded, never logged. This convention is stated explicitly in `AGENTS.md` so it's already binding before any service that actually holds a sensitive secret (the Telegram listener) is built in V1.
- Document in `AGENTS.md`/README that production secrets will use a secrets manager (not committed `.env` files) — decision on which one is deferred to V9 per the architecture doc, but the *rule* (never commit secrets) is a V0-level, non-negotiable baseline.

### 1.11 Logging Conventions

Even with no running service yet, the convention is documented in `AGENTS.md` so all future code follows it from its first commit:
- Structured JSON logs, one object per log line.
- Required fields: `timestamp`, `level`, `service`, `message`, and a `correlation_id` field reserved for future cross-service tracing (per the architecture doc's observability plan).
- No `print`/`fmt.Println` debugging left in committed code — use a logging library from the first line of real service code (library choice deferred to V1 when the first service is actually written, but the convention is fixed now).

### 1.12 Security Baseline

- Secrets never committed (1.10).
- DB connections use least-privilege roles even in local dev (a non-superuser app role in `0001_init.sql`, not the default `postgres` superuser) — establishing the habit before it matters in production.
- `.gitignore` excludes `.env`, local DB data volumes, and any IDE/agent scratch files.
- Dependency versions pinned (Go `go.mod` with exact versions once Go code exists; migration tool version pinned in CI) to avoid unreviewed drift.
- No public-facing anything exists yet at V0, so most of the architecture doc's security section (Section 8) is not yet applicable — noted here explicitly so no agent tries to build auth/rate-limiting prematurely.

### 1.13 Git / Versioning Conventions

- Trunk-based development: short-lived branches, PR required to merge to `main`, CI must pass.
- Conventional Commits (`feat:`, `fix:`, `chore:`, `docs:`, `ci:`) for clear history and future changelog generation.
- Migrations are numbered sequentially and never edited after merge — a mistake gets a new corrective migration, not a rewrite of history.
- No version tags/releases at V0 — first tag happens at V1 completion.

### 1.14 V0 Acceptance Criteria

V0 is complete when **all** of the following are true:
- [ ] Repository matches the structure in 1.1.
- [ ] `AGENTS.md` exists and covers all points in 1.2.
- [ ] Architecture doc is present under `docs/architecture/`.
- [ ] The four ADRs in 1.4 exist, using the template in the same section.
- [ ] `db/migrations/0001_init.sql` creates `channels` and `raw_posts` exactly as scoped in the architecture doc's V1 schema subset, with the idempotency constraint present.
- [ ] `docker-compose up -d` brings up a working local Postgres with no manual steps beyond copying `.env.example`.
- [ ] Migrations apply cleanly against a fresh database (verified by CI).
- [ ] `tests/fixtures/sample_telegram_posts.json` exists, contains realistic sample data including at least one deliberate near-duplicate pair.
- [ ] A trivial fixture-validation test exists and passes in CI.
- [ ] Lint config exists and CI runs it (even with minimal/no code to lint yet).
- [ ] CI workflow runs on PR and passes end-to-end on a clean clone.
- [ ] `.env.example` documents all needed variables; `.env` is gitignored.
- [ ] Logging and secrets conventions are documented in `AGENTS.md`.
- [ ] No service code, API, frontend, ingestion, AI, or clustering logic exists anywhere in the repo.
- [ ] README's "getting started" instructions work verbatim on a clean clone, in order, with no undocumented steps.

---

## 2. V0 IMPLEMENTATION PLAN

Ordered, small steps for an AI coding agent. Each step should be its own commit/PR.

**Step 1 — Repository skeleton**
- Creates: `README.md`, `.gitignore`, `.editorconfig`, empty `docs/`, `db/migrations/`, `infra/`, `tests/fixtures/`, `.github/workflows/` directories.
- Purpose: establish the structure from Spec 1.1 before anything else lands in it.
- Checks: directory structure matches spec; nothing else.
- Expected result: an empty-but-correctly-shaped repo.

**Step 2 — Copy architecture doc + write AGENTS.md**
- Creates: `docs/architecture/platform-architecture.md` (copy of the agreed spec), `AGENTS.md` per Spec 1.2.
- Purpose: establish the source-of-truth references before any decision-dependent work begins.
- Checks: manual review — AGENTS.md covers every bullet in Spec 1.2.
- Expected result: any future agent session can orient itself correctly from these two files alone.

**Step 3 — Write the four initial ADRs**
- Creates: `docs/adr/0001` through `0004`, per Spec 1.4, using the template.
- Purpose: lock in the decisions that constrain V0's own DB/dev-environment work, and pre-empt future agents from silently reversing them.
- Checks: each ADR references the corresponding section of the architecture doc; template followed exactly.
- Expected result: four short, consistent decision records.

**Step 4 — `.env.example` and secrets convention**
- Creates: `.env.example` with DB connection variables; adds secrets rule to `AGENTS.md` if not already fully covered in Step 2.
- Purpose: fix the environment-variable convention before any code that reads config exists.
- Checks: `.env.example` has no real values, only placeholders; `.gitignore` excludes `.env`.
- Expected result: config convention ready for Step 5+ to use.

**Step 5 — docker-compose (Postgres only)**
- Creates: `infra/docker-compose.yml` with a single Postgres service, reading connection config from `.env`.
- Purpose: give a working local DB target for migrations.
- Checks: `docker-compose up -d` succeeds locally and in CI; Postgres is reachable on the configured port.
- Expected result: a running local Postgres with no application code involved yet.

**Step 6 — Initial migration (`0001_init.sql`)**
- Creates: `db/migrations/0001_init.sql` defining `channels` and `raw_posts`, including the idempotency `UNIQUE` constraint and a least-privilege app DB role.
- Purpose: establish the minimal V1-scope schema now, so V1 has a ready foundation.
- Checks: migration applies cleanly against a fresh Postgres (manually, then in CI in Step 8); columns/constraints match the architecture doc's V1 subset exactly — no extra tables, no `news_events` yet.
- Expected result: `channels` and `raw_posts` tables exist after running the migration, and only those two.

**Step 7 — Fixture data**
- Creates: `tests/fixtures/sample_telegram_posts.json` with realistic sample posts, including at least one deliberate near-duplicate pair across two fake channels (mirroring the fuel-price example in the architecture doc).
- Purpose: give V1's future ingestion work real-shaped test data from day one.
- Checks: valid JSON; matches a documented expected shape (fields a real Telegram message would have: channel identifier, message id, text, timestamp).
- Expected result: a fixture file ready for V1 tests to consume, unused by any code in V0.

**Step 8 — Test foundation + CI**
- Creates: a minimal test (e.g., in Go, or a simple script if no language runtime is justified yet) asserting the fixture file parses and matches its expected schema; `.github/workflows/ci.yml` wiring checkout → Postgres service container → migrate → lint (no-op if nothing to lint yet, but the step must exist) → test.
- Purpose: prove the whole pipeline (DB, migration, test, lint) works end-to-end before any real service code depends on it.
- Checks: CI passes on a clean PR; deliberately break the migration locally and confirm CI fails (sanity-check the pipeline actually catches problems, then revert).
- Expected result: green CI on `main`, and a documented, working local-dev + CI loop that V1 can build directly on top of.

**Step 9 — README + final review pass**
- Creates/updates: `README.md` "getting started" section (clone → env → compose up → migrate → test), and does a final read-through against every V0 acceptance criterion in Spec 1.14.
- Purpose: verify V0 is genuinely complete and reproducible by someone with zero prior context.
- Checks: follow the README literally on a clean clone; confirm every checkbox in 1.14.
- Expected result: V0 acceptance criteria fully met; ready to stop and hand off to V1.

No step in this plan touches Telegram, ingestion logic, AI, clustering, an API, or a frontend — consistent with the "V0 MUST NOT IMPLEMENT" constraint.

---

## 3. V0 ANTIGRAVITY PROMPT

```
You are implementing V0 (project foundation only) for the Ethiopia news aggregation
platform. This is NOT a feature release — it is repository scaffolding, tooling, and
conventions for safe future AI-assisted development.

BEFORE WRITING ANYTHING:
1. Inspect the existing repository structure and contents. Do not assume it's empty —
   check first, and do not duplicate or overwrite anything already correctly in place.
2. Read docs/architecture/platform-architecture.md if present (this is the source-of-truth
   system design). If it is not yet in the repo, ask for it before proceeding — do not
   invent architecture decisions yourself.
3. Read AGENTS.md if present, and follow it exactly for the rest of this task. If it does
   not exist yet, you are creating it as part of this work — base it on the V0 spec below.

YOUR TASK: implement V0 only, per this specification (do not paste this back to me,
just follow it):

- Repository structure: docs/ (architecture, adr, runbooks), db/migrations/, infra/,
  tests/fixtures/, .github/workflows/, plus standard root files
  (README.md, AGENTS.md, .env.example, .gitignore, .editorconfig).
- AGENTS.md covering: source-of-truth docs to read first, no new infra/dependency without
  an ADR, no service/product code in V0, migrations are append-only and numbered,
  coding standards, commit conventions, CI must pass before merge, secrets/logging
  conventions, and "stop and ask if scope is unclear."
- Four ADRs only, using a Title/Status/Context/Decision/Consequences template:
  Postgres-only queue at V1, SSE over WebSockets, pgvector over a dedicated vector DB,
  Python-listener/Go-processor language split. Extract these from the architecture doc —
  do not re-analyze from scratch.
- One migration (db/migrations/0001_init.sql) creating exactly two tables — `channels`
  and `raw_posts` — matching the V1-scope schema in the architecture doc, including the
  (channel_id, telegram_message_id) uniqueness constraint and a least-privilege DB role.
  No other tables.
- docker-compose.yml with Postgres only — no application containers.
- tests/fixtures/sample_telegram_posts.json with realistic sample data including at
  least one deliberate near-duplicate pair across two channels.
- A minimal test proving the fixture parses/validates, and a CI workflow that spins up
  Postgres, applies the migration, runs lint (even if a no-op), and runs tests.
- .env.example documenting required variables; .env gitignored; no real secrets anywhere.
- README with a "getting started" sequence that works verbatim on a clean clone.

STRICT SCOPE BOUNDARIES — DO NOT IMPLEMENT ANY OF:
Telegram/Telethon integration, ingestion logic, deduplication, embeddings, AI calls,
clustering, a public API, a Next.js/frontend app, SSE, SEO tooling, Redis, Kafka,
Kubernetes, or any production/scaling infrastructure. If a future version will need an
interface or boundary, document the intent in an ADR or code comment — do not build the
feature itself.

RULES:
- Do not introduce any new dependency, service, or piece of infrastructure beyond what's
  listed above without first writing an ADR justifying it and pausing for confirmation.
- Work in small, ordered steps. After each step, run relevant tests/lint/build checks
  before moving to the next step.
- If you're unsure whether something is in V0 scope, stop and ask rather than guessing.
- Do not create placeholder/empty service directories (services/, web/, api/) — those
  are created in the version that actually needs them.

WHEN DONE:
- Run the full test suite, lint, and confirm docker-compose + migrations work from a
  clean state.
- Report back: what was created/changed, what checks were run and their results, and
  explicit confirmation that no V1+ functionality was implemented.
- Stop. Do not continue into V1 work without explicit instruction.
```

---

## V0 → V1 Handoff

V0 leaves in place: a documented architecture and ADR trail, `AGENTS.md` governing agent behavior, a working local Postgres with the `channels`/`raw_posts` schema and least-privilege role, a green CI pipeline (migrate → lint → test), fixture data shaped like real Telegram posts, and fixed conventions for secrets, logging, commits, and migrations.

V1 can safely build directly on this: create `services/listener/` (Python/Telethon per ADR-0004), wire it to the existing `channels`/`raw_posts` schema and DB role, write ingestion tests against the existing fixture data (extending it as needed), and add its own CI job — all without touching V0's structure, conventions, or existing migration. V1 should add new ADRs only if it deviates from or extends a V0-era decision; it should not need to revisit V0's foundational choices.
