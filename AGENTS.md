# Agent Guidelines — Ethiopia News Aggregation Platform

This file governs the behavior of AI coding agents working in this repository. All rules are binding.

---

## 1. Sources of Truth & Mandatory Reading

Before planning or executing any task:
1. **System Architecture**: Read [`docs/architecture/platform-architecture.md`](docs/architecture/platform-architecture.md). This is the source-of-truth system design.
2. **Current Phase Specification**: Read [`v0-foundation-spec.md`](v0-foundation-spec.md) (or the corresponding version specification).
3. **Architectural Decision Records (ADRs)**: Review all records in [`docs/adr/`](docs/adr/).
4. **This File**: Review [`AGENTS.md`](AGENTS.md).

Never invent architectural decisions independently. If requirements are ambiguous or not covered in the documentation, **stop and ask** rather than guessing.

---

## 2. Scope Boundaries & Phasing

- **V0 Scope**: Repository foundation, tooling, documentation, ADRs, database migrations for the V1 subset (`channels`, `raw_posts`), test fixtures, and CI workflows.
- **Strict Prohibition**: In V0, do NOT create or implement any runtime services, Telegram/Telethon ingestion logic, deduplication, embeddings, AI enrichment, clustering algorithms, public APIs, or frontend code.
- **Directory Scaffolding**: Do NOT create empty placeholder service directories (`services/`, `web/`, `api/`) in advance. Directories must only be created in the version where their implementation begins.
- If asked to implement functionality beyond the current phase scope, stop and explicitly flag the boundary to the user.

---

## 3. Architecture Decisions & Infrastructure Dependencies

- Do NOT introduce any new infrastructure dependency (e.g., database, queue, cache, vector store, broker, external service, or heavy framework) without first drafting an ADR in `docs/adr/` and obtaining explicit approval.
- The existing architectural decisions are binding:
  - **ADR-0001**: Postgres-only queue at V1 (`FOR UPDATE SKIP LOCKED` / `river`). No Kafka or Redis for queueing at V1.
  - **ADR-0002**: Server-Sent Events (SSE) over WebSockets for real-time delivery.
  - **ADR-0003**: `pgvector` inside PostgreSQL over a dedicated vector database.
  - **ADR-0004**: Python + Telethon for Telegram ingestion listener; Go for pipeline processing and API services.
  - **ADR-0005**: Telegram session storage in environment variable.
  - **ADR-0006**: Non-semantic text-similarity clustering at V2.
  - **ADR-0007**: Google Gemini `text-embedding-004` (768-d) for semantic clustering.

---

## 4. Database & Migration Rules

- All database schema modifications must be made via sequential, numbered SQL migration files in `db/migrations/` (e.g., `0001_init.sql`, `0002_*.sql`).
- **Never edit an existing migration once merged.** Schema corrections require a new forward migration.
- **Data Immutability**: `raw_posts.raw_text` is immutable. No migration, service code, or pipeline stage may modify raw source text after insertion.
- **Least Privilege**: Always connect using least-privilege database roles (e.g., `app_user` for standard application access), not the Postgres superuser.
- **SQL Conventions**: Use plain SQL migrations with consistent lowercase keywords and `snake_case` identifiers. No ORMs at V0/V1.

---

## 5. Coding & Quality Standards

- **Go**:
  - Code must be formatted with `gofmt`.
  - Code must pass `golangci-lint` using `.golangci.yml`.
  - Unit tests must accompany all core business and pipeline logic.
- **SQL**: Plain SQL migration files, syntax-checked and validated against PostgreSQL.
- **Environment & Formatting**: Adhere to `.editorconfig` (UTF-8, LF line endings, trailing whitespace trimmed).
- **Python / TypeScript**: Coding standards will be formalized in the ADR and `AGENTS.md` update that introduces the first runtime code for those stacks (V1/V6).

---

## 6. Logging & Observability Conventions

- All runtime logs must be structured JSON, outputting one JSON object per line.
- Required fields for every log entry:
  - `timestamp` (ISO 8601 / RFC 3339 UTC)
  - `level` (`DEBUG`, `INFO`, `WARN`, `ERROR`)
  - `service` (name of the service emitting the log)
  - `message` (human-readable log message)
  - `correlation_id` (UUID / tracing identifier for cross-service tracking)
- **No Debug Prints**: Never leave `print()`, `fmt.Println()`, or `console.log()` statements in committed service code. Use a structured logging library from the first line of real code.

---

## 7. Secrets & Security Baseline

- **Zero Committed Secrets**: Never hardcode, commit, or log credentials, API keys, session strings, or passwords.
- **Environment Variables**: All configuration and secrets must be injected via environment variables. Document required variables in `.env.example`.
- **`.env` Ignored**: `.env` is gitignored and must never be tracked in source control.
- **Production Secrets**: Production deployments will retrieve secrets via a dedicated secrets manager (policy finalized at V9).
- **Dependency Pinning**: All dependencies must be pinned to exact or locked versions (`go.mod`, container base images, GitHub Actions).

---

## 8. Version Control & CI/CD Workflow

- **Branching Strategy**: Trunk-based development using short-lived feature branches and pull requests to `main`.
- **Commit Messages**: Follow [Conventional Commits](https://www.conventionalcommits.org/):
  - `feat:` for new features
  - `fix:` for bug fixes
  - `chore:` for tooling, dependencies, and scaffolding
  - `docs:` for documentation updates
  - `ci:` for CI/CD workflow changes
  - `test:` for test additions and modifications
- **CI Gate**: Every commit and pull request must pass the full CI pipeline (lint + test + migration checks) before merging. Agents must run checks locally before concluding any step.

---

## 9. Decision Escalation Protocol

If you encounter:
- An unspecified requirement or technical ambiguity
- A situation suggesting a deviation from `docs/architecture/platform-architecture.md`
- A request that conflicts with the phase boundaries (e.g., implementing V1+ features during V0)

**Stop immediately, describe the situation, and request clarification from the user.**

---

## 10. V1 Addendum — Telegram Ingestion Listener

- **Python Standards**: Python code in `services/listener/` must use `black` for code formatting, `ruff` for linting, and `pytest` for test suites. All dependencies must be pinned in `requirements.txt`.
- **Telegram Session Secrets (ADR-0005)**: The Telegram session string is a high-risk credential. It must never be committed to source control, written into SQLite `.session` files inside the repository/container, or output in logs. It is loaded strictly from the `TELEGRAM_SESSION_STRING` environment variable.
- **Fixture-Driven Testing**: `services/listener/` must be fully runnable and unit-tested against `tests/fixtures/sample_telegram_posts.json` without requiring live Telegram credentials. Real credentials are restricted to live MTProto listener runs.
- **Database Write Boundary**: Services in `services/listener/` are strictly limited to writing `raw_posts` (with `processing_status = 'ingested'`) and reading/upserting `channels`. No listener code may write to `news_events`, `event_sources`, `categories`, or any downstream tables.

---

## 11. V2 Addendum — Pipeline Processor & Deduplication

- **Go Standards**: Go code in `services/processor/` must be formatted with `gofmt`, pass `golangci-lint` without warnings, and include comprehensive unit tests for normalization, Simhash, and clustering decisions. Dependencies must be managed and pinned via `go.mod` / `go.sum`.
- **Pipeline Boundaries (ADR-0006)**: V2 is strictly limited to text-similarity deduplication using 64-bit Simhash and Hamming distance. No vector embeddings (`pgvector`), AI enrichment models, or external LLM API calls may be introduced in V2.
- **Database Write & Immutability Rules**:
  - `raw_posts.raw_text` is strictly immutable.
  - The processor reads posts with `processing_status = 'ingested'`, writes `raw_posts.normalized_text` and `raw_posts.simhash`, and transitions status to `'processed'`.
  - The processor writes and updates `news_events` and `event_sources` in atomic database transactions.
- **Structured Logging & Decision Auditing**: All batch runs and clustering decisions must emit structured JSON logs containing `timestamp`, `level`, `service: "processor"`, `correlation_id`, `raw_post_id`, `event_id`, and `decision` (`created_new_event` or `attached_to_event`).

---

## 12. V3 Addendum — Semantic Clustering & Vector Embeddings

- **Embedding Provider Seam (ADR-0007)**: All embedding generation is decoupled behind the `Embedder` Go interface in `embed.go`. Direct embedding HTTP requests from outside this interface are prohibited.
- **Three-Way Decision State Machine**:
  - **Fast Path (Simhash)**: If Hamming distance <= `SIMHASH_THRESHOLD` (10), post attaches immediately.
  - **Fallback Path (Embeddings)**:
    - Cosine similarity >= `EMBEDDING_HIGH_THRESHOLD` (0.82) -> `attached_to_event`
    - Cosine similarity < `EMBEDDING_LOW_THRESHOLD` (0.65) -> `created_new_event`
    - Ambiguous band (`0.65 <= similarity < 0.82`) -> `needs_review` (no event created, not attached)
- **Centroid Maintenance**: `news_events.embedding_centroid` must be updated atomically on event creation and every subsequent post attachment using PostgreSQL vector aggregation.
- **Provider Resilience**: External embedding API failures must be retried with exponential backoff up to `MAX_EMBEDDING_RETRIES`. If retries are exhausted, the post transitions to `needs_review` to prevent pipeline stall and guarantee zero data loss.
- **V4 Boundary**: V3 is strictly limited to vector embedding generation, centroid maintenance, and semantic clustering. No AI enrichment models (LLM summary generation, entity extraction, category classification, or sentiment analysis) may be added until V4.

---

## 13. V4 Addendum — AI Enrichment & LLM Verification

- **LLM Provider Seam (ADR-0008)**: All LLM generation (verification and enrichment) is decoupled behind the `LLMClient` Go interface in `llm.go`. Direct LLM API calls outside this interface are prohibited.
- **Verification Rule & Confidence Safeguard**: Ambiguous posts (`needs_review`) are verified against candidate events with structured JSON schema. If `confidence < VERIFY_CONFIDENCE_THRESHOLD` (0.75), the post MUST remain in `needs_review` and log a `processing_audit` record (`low_confidence_unresolved`). The system must NEVER force-decide on low confidence.
- **Enrichment Lifecycle & Stability**: Events are enriched only after becoming stable (`last_updated_at <= NOW() - STABILITY_WINDOW_MINUTES`). Summarization, classification into one of the 9 canonical categories, and entity extraction are written atomically in `news_events`, `entities`, `event_entities`, and `processing_audit`.
- **Provider Resilience & Fail-Safe Operation**: Upstream LLM failures retry with exponential backoff up to `MAX_LLM_RETRIES`. If retries are exhausted, events remain active without summaries, and posts remain in `needs_review`. The pipeline never crashes or loses data due to upstream AI errors.
- **V5 Boundary**: V4 is strictly limited to LLM verification and event enrichment. No public API endpoints, SSE streams, or frontend web client code may be implemented until V5/V6.
