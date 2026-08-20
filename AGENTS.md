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
  - **ADR-0014**: Admin authentication and moderation workflow.
  - **ADR-0015**: Bounded backfill and gap recovery strategy.

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

---

## 14. V5 Addendum — Public API Service

- **Go Standards**: Go code in `services/api/` must be formatted with `gofmt`, pass `golangci-lint` without warnings, and include comprehensive unit, middleware, and data-leak prevention tests.
- **Least-Privilege Database Role (ADR-0009)**: The API service connects strictly using `API_DATABASE_URL` with the `efi_api` role. This role has `SELECT` privileges only on public-facing tables and has NO access to `processing_audit` and zero write permissions (`INSERT`, `UPDATE`, `DELETE`).
- **Zero-Leak Guarantee**: All database queries must enforce `WHERE ne.status = 'active'` to guarantee that records in `needs_review` are never exposed over the public API.
- **Attribution & Transparency**: All event responses must include `ai_summary_generated: true`. Source post previews must be bounded (`FormatExcerpt` with <= 160 runes) to uphold copyright compliance and transparent attribution.
- **Middleware & Security Baseline**: The public API service must enforce per-IP token bucket rate limiting (returning HTTP 429 with `Retry-After`), CORS headers, panic recovery (returning clean HTTP 500 JSON without service interruption), and structured JSON request logging.
- **V6 Boundary**: V5 is strictly limited to the read-only REST API. Real-time Server-Sent Events (SSE) streaming and web frontend clients are deferred to V6.

---

## 15. V6 Addendum — Web Frontend (Next.js & App Router)

- **TypeScript & React Standards**: Code in `web/` must be written in TypeScript, formatted cleanly, pass ESLint (`next/core-web-vitals`) with zero errors, and include comprehensive unit and component tests using Jest and React Testing Library.
- **AI Transparency Invariant**: Every page or component rendering an AI-synthesized summary must display a clear, accessible visual indicator (`AiBadge`, "AI-Generated Summary") and disclaimer whenever `ai_summary_generated: true`.
- **Source Attribution & Bounded Excerpts**: All source channel reports must clearly attribute channel name and handle, and show bounded excerpts (<= 160 runes) linking to primary Telegram channels.
- **Dual-Context API Client**: The API client in `lib/api.ts` must correctly resolve `INTERNAL_API_URL` (for server-side rendering within container networks) and `NEXT_PUBLIC_API_URL` (for client-side polling in browsers).
- **Graceful Error Handling & Silent Polling Degradation**: Server-rendered pages must display clean fallback error banners if the API is temporarily unreachable. Background polling (`LiveFeedUpdater`) must fail silently without user disruption and back off exponentially on network errors.
- **V7 Boundary**: V6 is strictly limited to the web frontend client. Real-time Server-Sent Events (SSE) streaming, push notifications, and administrative moderation tools are deferred to V7+.

---

## 16. V7 Addendum — Real-Time Delivery (SSE & Postgres LISTEN/NOTIFY)

- **Application-Level NOTIFY Invariant (ADR-0011)**: Every code path in `services/processor/store.go` that creates a `news_events` row (`CreateEvent`), attaches a new source to an existing event (`AttachToEvent`), or writes an enriched summary (`SaveEventEnrichment`) MUST execute an explicit transaction-coupled `pg_notify('news_events_channel', ...)` call carrying minimal payload (`{"type":"new_event"|"event_updated","event_id":<id>}`). Database triggers for NOTIFY are explicitly prohibited.
- **Resource Management & Goroutine Leak Prevention**: The API SSE hub (`services/api/sse.go`) must manage client lifecycles defensively. Dropped or closed connections must be promptly unregistered, channels closed without panicking, and per-IP concurrent connection limits enforced (`MAX_SSE_PER_IP`).
- **Self-Healing LISTEN Subscriber**: The Postgres `LISTEN` subscriber (`services/api/notify_listener.go`) is the single pipeline bridge for real-time delivery. It must handle connection drops with exponential reconnection backoff, emit structured JSON connection status logs, and safely recover without crashing the API service.
- **Graceful Client Degradation**: The web client (`LiveFeedUpdater.tsx`) must gracefully handle SSE disconnects and errors, displaying visible reconnection status when disconnected while allowing the application to function seamlessly via standard SSR and page navigation.
- **V8 Boundary**: V7 is strictly limited to real-time SSE streaming. Search engine optimization (SEO), OpenGraph metadata generation, RSS/Atom feeds, and sitemaps are deferred to V8.

---

## 17. V8 Addendum — Search Engine Optimization & Canonical URL Strategy

- **Slug Immutability Invariant (ADR-0013)**: Event slugs are generated exactly once upon initial event enrichment and written to `news_events.slug`. No pipeline code path, background task, or re-enrichment routine may ever overwrite or regenerate an existing non-null slug. A slug must remain permanent throughout the lifecycle of an event to protect index integrity and external backlinks.
- **Server-Side Rendering (SSR) Metadata Invariant**: All SEO metadata—including title tags, descriptions, Open Graph (`og:*`), Twitter Cards (`twitter:*`), canonical links (`rel="canonical"`), and schema.org `NewsArticle` JSON-LD—must be rendered server-side in the initial HTML payload. Client-only metadata injection via `useEffect` is strictly prohibited.
- **Canonical Routing & 301 Redirects**:
  - The authoritative URL pattern for news events is `/news/{category}/{slug}-{id}`.
  - Legacy routes (`/events/{id}`) and mismatched category/slug requests must return HTTP 301 permanent redirects (`permanentRedirect`) to the canonical URL.
- **Truthful Structured Data**: `NewsArticle` JSON-LD schema must reflect real, verified data from the database and API. Fabricating missing metadata (such as invented author persons or nonexistent article image URLs) is strictly prohibited.
- **Sitemap & Robots Guardrails**: `sitemap.xml` dynamically lists only published events (`status = 'active'`) and category routes. Ambiguous posts in `needs_review` must never be leaked or included in public sitemaps.
- **V9.1 Boundary**: V8 is strictly limited to SEO, canonical URLs, structured data, and sitemaps. Administrative moderation interfaces, analytics dashboards, and multi-tenant authentication are deferred to V9.1+.

---

## 18. V9.1 Addendum — Admin Panel & Moderation Workflow

- **Authentication & Session Security (ADR-0014)**: Admin authentication uses bcrypt password hashing and stateful PostgreSQL sessions (`admin_sessions`). Session tokens are transmitted strictly via `httpOnly`, `Secure`, and `SameSite=Strict` cookies (`efi_session`) with a 24-hour lifetime. All mutating admin requests (`POST`, `DELETE`) require an `X-CSRF-Token` header. Login is rate-limited to 5 attempts per 15 minutes per IP.
- **Zero-Registration & Seed Provisioning**: No public registration or user creation endpoints exist. Initial administrator credentials must be provisioned strictly via the CLI tool `services/api/cmd/seed/admin_seed.go`.
- **Soft-Takedown Isolation (`is_hidden`)**: Editorial moderation operates strictly via `news_events.is_hidden` (boolean). The pipeline clustering `status` (`active`, `needs_review`) is orthogonal and must never be mutated for editorial takedowns. All public API queries and search functions must enforce `WHERE ne.is_hidden = false` to guarantee zero data leakage.
- **Mandatory Audit Trail**: Every administrative action (channel status change, event soft-takedown, event restoration, source detachment, and ambiguity queue resolution) requires a mandatory, non-empty human justification string recorded atomically in `admin_audit_log` and `processing_audit`.
- **Ingestion Listener Gating**: The Python ingestion listener (`services/listener/`) must check `channels.is_active` and skip processing incoming messages from paused channels.
- **Crawler Isolation**: The `/admin/` path and all administrative subroutes must remain disallowed in `robots.txt`.

---

## 19. V9.2 Addendum — Backfill & Gap Detection

- **Checkpoint Invariant (ADR-0015)**: `channels.last_synced_message_id` tracks the highest synced Telegram message ID per channel. It must be updated atomically and greedily within the same database transaction as `insert_raw_post` using `GREATEST(COALESCE(last_synced_message_id, 0), %(telegram_message_id)s)`.
- **Bounded Catch-Up Rule**: Backfills triggered at startup and upon reconnection must strictly enforce double bounding: `min(BACKFILL_MAX_MESSAGES, 100)` messages and messages newer than `NOW() - BACKFILL_MAX_HOURS` (48 hours).
- **Fresh-Start Invariant**: Brand-new channels (`last_synced_message_id IS NULL`) must establish their baseline checkpoint from the current latest message without dumping historical archives.
- **Idempotent Ingestion Reuse**: Backfilled messages must route exclusively through the existing V1 `normalize_telethon_message` and `insert_raw_post` functions, relying on `UNIQUE(channel_id, telegram_message_id)` to ensure zero duplicates.
- **Rate-Limit Resilience**: Telethon `FloodWaitError` must be caught during backfill iterations, sleeping for the requested duration before safely resuming.
- **Channel Error Isolation**: Backfill across channels must isolate per-channel exceptions to ensure a single channel error never aborts the listener run or prevents other channels from catching up.

---

## 20. V9.3 Addendum — Secrets Hardening & Database Backups

- **Zero Plaintext Secrets In Non-Dev `.env` (ADR-0016)**: Secrets (`TELEGRAM_SESSION_STRING`, `TELEGRAM_API_HASH`, `GEMINI_API_KEY`, database passwords) must never be committed to git or stored in plain `.env` files beyond local dev. Containerized staging/production environments load secrets strictly via Docker Secrets (`/run/secrets/`).
- **Seamless Application Secret Seams**: Microservices consume standard environment variables at runtime. Configuration modules (`services/listener/config.py`, `services/processor/config.go`, `services/api/config.go`) resolve secrets from `/run/secrets/` as transparent fallbacks, requiring zero application logic changes.
- **Superseded ADR-0005**: ADR-0005's reliance on unencrypted `.env` session storage is superseded by container secret mounts (`/run/secrets/telegram_session_string`).
- **Automated Dump & 7-Day Retention**: Database backups are taken daily using `pg_dump -Fc` custom binary format, stored in `infra/backup/storage/` off host DB data directories, and automatically pruned after 7 days (`find /backups -name "*.dump" -mtime +7 -delete`).
- **Executable & Verified Restore Drill**: Backup and restore scripts live in `infra/backup/` (`backup.sh`, `restore.sh`, `run_restore_drill.sh`). Restore capability must be verified periodically by executing a real restore into a test database (`efi_restore_test`) and confirming 100% table row count matching against live database.


