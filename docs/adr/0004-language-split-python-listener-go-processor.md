# ADR-0004: Language Split — Python Listener and Go Processor

Status: Accepted
Date: 2026-08-11

## Context

The platform is split into two major backend operational domains:
1. **Telegram Ingestion Listener**: Connects to Telegram MTProto protocol via user/bot clients, listens to streaming events across public channels, handles flood wait errors, manages session reconnection, and persists raw messages to the database.
2. **Processing Pipeline & API Services**: Performs text normalization, simhashing, embedding management, clustering, AI verification orchestration, database operations, and high-concurrency REST/SSE streaming.

We evaluated whether to standardize the entire backend on a single language (either pure Go or pure Python) or maintain a split architecture.

## Decision

We will use a split language architecture:
- **Ingestion Listener (`services/listener/`)**: Implemented in **Python** using the **Telethon** MTProto client library.
- **Pipeline Processor & API (`services/processor/`, `services/api/`)**: Implemented in **Go**.

Telethon in Python represents the most battle-tested, mature ecosystem for Telegram MTProto client handling, session resilience, reconnection recovery, and flood limit handling. Conversely, Go provides high concurrency performance, predictable low memory footprints, fast startup times, robust static typing, and strong native PostgreSQL drivers for the processing pipeline and real-time streaming API.

The two services communicate strictly through PostgreSQL (the listener writes immutable `raw_posts`; the Go processor consumes them asynchronously).

## Consequences

### Positive
- **Mitigates Critical Integration Risk**: Minimizes Telegram session bans, MTProto handshake issues, and edge-case handling by relying on Telethon's battle-tested Python codebase.
- **Optimal Pipeline & API Performance**: Go delivers low-latency processing, robust concurrency, and minimal resource usage under continuous streaming load.
- **Clean Isolation Boundary**: The listener is completely decoupled from the rest of the application; failure or restart of the listener does not affect the processing pipeline or API delivery, and vice versa.

### Negative / Trade-offs
- **Dual Runtime Tooling**: Development and CI pipelines must manage both Python and Go environments, linting suites, and dependencies.
- **Cross-Service Code Sharing**: Shared data structures (such as `RawPost` definitions) must be kept consistent between Python and Go schemas and database migrations.
