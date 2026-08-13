# ADR-0014: Administrative Authentication, Soft-Takedown Workflow, and Audit Controls

## Status
Accepted

## Context
In V9.1 (Admin Panel & Moderation Workflow), the Ethiopia News Aggregation Platform requires administrative controls to manage source channels, resolve ambiguous clustering classifications, detach erroneously linked sources, and execute soft-takedowns of sensitive, defamatory, or copyrighted content.

Prior to V9.1, the public API was entirely read-only (enforced via least-privilege role `efi_api`), and the pipeline operated autonomously without manual intervention mechanisms. Introducing administrative mutability requires rigorous security isolation, session protection, CSRF defense, and audit trail guarantees to prevent data leakage and unauthorized modifications.

## Decision

1. **Authentication Architecture & Session Management**:
   - **Zero Public Registration**: There is no public registration or self-signup endpoint. Initial administrator accounts are provisioned strictly via a CLI seed utility (`admin_seed.go`) directly accessing the database.
   - **Password Security**: Passwords are authenticated using `bcrypt` (cost factor 12) and never stored or logged in plain text.
   - **Session Storage**: Stateful sessions are managed in PostgreSQL (`admin_sessions`) with 256-bit cryptographically random tokens (`crypto/rand`).
   - **Cookie Isolation**: Session tokens are transmitted strictly via `httpOnly`, `SameSite=Strict`, and `Secure` cookies (`efi_session`) with a 24-hour time-to-live.
   - **Anti-CSRF Defense**: All mutating administrative REST endpoints (`POST`, `DELETE`) require an `X-CSRF-Token` header validated against the session's cryptographically derived CSRF token, protecting against cross-site request forgery.
   - **Login Rate Limiting**: The login endpoint enforces a per-IP rate limit of 5 attempts per 15 minutes to prevent brute-force attacks.

2. **Least-Privilege Database Role Separation**:
   - The administrative API connects to PostgreSQL using a dedicated `efi_admin` role (`ADMIN_DATABASE_URL`).
   - The public read-only API retains its strictly sandboxed `efi_api` role with zero access to `admin_users`, `admin_sessions`, or `admin_audit_log`.

3. **Soft-Takedown Separation (`is_hidden` vs `status`)**:
   - We introduce a dedicated boolean column `news_events.is_hidden` (default `false`) with a partial index `WHERE is_hidden = false`.
   - **Clustering Orthogonality**: The existing `status` column (`active`, `needs_review`) governs pipeline lifecycle and clustering eligibility. `is_hidden` governs public surface visibility. Takedowns never mutate pipeline clustering state.
   - **Public Zero-Leak Guarantee**: All public-facing SQL queries (`GetEvents`, `GetEventByID`, `GetEventBySlug`, `SearchEvents`) strictly enforce `WHERE is_hidden = false AND status = 'active'`.
   - **Preservation & Reversibility**: Hidden events remain intact with all source attributions and vector embeddings for administrative audit and legal compliance, allowing instant restoration if cleared.

4. **Source Detachment & Ambiguity Resolution**:
   - Administrators can detach a misclustered source post from an event cluster via `DELETE /api/v1/admin/events/{id}/sources/{raw_post_id}` without deleting the raw post.
   - The Ambiguous Review Queue (`/admin/review-queue`) enables human verification of posts falling into the ambiguity band (`0.65 <= similarity < 0.82`), supporting attachment, new event creation, or spam discarding with an audit trail recorded in `processing_audit`.

5. **Mandatory Audit Trail**:
   - Every administrative mutation (channel toggle, event soft-takedown, event restoration, source detachment, and review queue decision) requires a non-empty human justification string.
   - All mutations are logged atomically to `admin_audit_log` with the administrator's user ID, IP address, user agent, action type, target ID, and reason.

6. **SEO & Crawler Isolation**:
   - `robots.txt` explicitly disallows all crawlers from indexing `/admin/` and administrative subpaths.

## Consequences

### Positive
- **Complete Editorial Governance**: Operators can immediately pause compromised channels, hide problematic stories, correct misclustering, and resolve edge cases.
- **Strong Security Baseline**: Multi-layered defense (httpOnly cookies, SameSite=Strict, CSRF tokens, bcrypt, rate limiting, least-privilege DB roles) protects administrative operations.
- **Zero Public Data Leakage**: Public APIs guarantee complete exclusion of hidden events and audit tables.
- **Accountability & Compliance**: Permanent, immutable audit log of every moderation action.

### Trade-offs & Limitations
- **Stateful Sessions**: Active sessions are validated against PostgreSQL on each admin request. At the platform's current scale (small editorial team), database session lookups add sub-millisecond overhead. Redis session caching can be evaluated in future phases if administrative concurrency expands.
