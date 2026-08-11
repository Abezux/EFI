# ADR-0009: API Framework, Routing, and Versioning Strategy

## Status
Accepted

## Context
In V5 (Public API), the Ethiopia News Aggregation Platform requires a public-facing, read-only REST API to serve processed news events, categorization taxonomies, and search capabilities to frontend clients and external consumers. 

Key constraints and requirements:
1. **Network Exposure**: As the first service exposed to the public internet, security, minimal attack surface, rate limiting, and zero-leak guarantees for internal data (`needs_review` content, `processing_audit` logs) are mandatory.
2. **Minimal Dependencies**: The platform prioritizes lightweight, standard-library-driven architectures without heavy, opinionated web frameworks (e.g., Gin, Fiber, Echo) that introduce external maintenance overhead.
3. **URL Versioning**: The API must support version evolution (`/api/v1/`) without breaking backwards compatibility.
4. **Transparency & Attribution**: Every aggregated event response must explicitly indicate that its summary was AI-generated (`ai_summary_generated: true`) and provide bounded source attribution excerpts (<= 160 characters) rather than full-text republication.

## Decision
We choose the **Go standard library `net/http.ServeMux` (Go 1.22+ enhanced routing)** with **URL path prefix versioning (`/api/v1/`)**, an **in-memory token bucket rate limiter**, and a dedicated **least-privilege PostgreSQL role (`efi_api`)**.

### Rationale
1. **Enhanced `net/http` Routing in Go 1.22+**:
   - Standard library `http.ServeMux` natively supports HTTP method matching and path parameters (`GET /api/v1/events/{id}`).
   - Avoids third-party routing dependencies, reducing container image size and eliminating third-party security vulnerabilities.
2. **URL Path Prefix Versioning (`/api/v1/`)**:
   - Explicit, human-readable, and easily cacheable at CDN/proxy layers.
   - Allows seamless coexistence of future API versions (e.g., `/api/v2/`) if schemas evolve.
3. **Least-Privilege Isolation (`efi_api`)**:
   - The API service authenticates with a restricted database role that has `SELECT` privileges only on `channels`, `raw_posts`, `news_events`, `event_sources`, `categories`, `entities`, and `event_entities`.
   - The role is explicitly denied access to `processing_audit` and possesses zero write permissions (`INSERT`, `UPDATE`, `DELETE`, `TRUNCATE`), providing defense-in-depth against data alteration or internal audit leaks.
4. **Programmatic Data Privacy**:
   - All store queries hard-code `WHERE ne.status = 'active'`, guaranteeing that unverified or ambiguous events (`needs_review`) are never exposed publicly.
5. **Bounded Excerpt Attribution**:
   - Source post previews are strictly truncated to <= 160 runes (`FormatExcerpt`), preserving copyright compliance while giving transparent source channel attribution and timestamp metadata.
   - *Note on Excerpt Length*: The 160-rune limit is adopted as a deliberate V5 baseline default, not a finalized permanent ceiling. It is explicitly flagged for revisiting and potential tuning in subsequent phases based on real user experience and frontend reading ergonomics.

## Consequences
- **Routing**: Handlers utilize `r.PathValue("id")` and standard query parameter parsing.
- **Middleware**: Onion middleware architecture (`PanicRecovery` -> `RequestLogger` -> `CORS` -> `RateLimiter`) provides structured logging, per-IP rate protection, and clean error responses.
- **Testing**: All API endpoints and security invariants (such as leak prevention and rate limits) are covered by automated unit and integration test suites using `httptest`.
