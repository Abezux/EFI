# ADR-0001: Postgres-Only Queue at V1

Status: Accepted
Date: 2026-08-11

## Context

The platform processes incoming Telegram posts through multiple asynchronous stages (normalization, deduplication, embedding generation, clustering, and AI enrichment). At V1, the system ingests from 5 to 10 Telegram channels, resulting in a volume of a few hundred messages per day.

Introducing a dedicated message broker such as Apache Kafka or Redis Streams at this early stage introduces significant operational and deployment overhead without delivering practical performance benefits for the expected workload. Furthermore, keeping a separate message broker transactionally synchronized with database state introduces dual-write complexities.

## Decision

We will use PostgreSQL as the sole asynchronous message queue mechanism for V1. Workers will consume pending posts from `raw_posts` using standard transactional patterns (`FOR UPDATE SKIP LOCKED`) or a lightweight Go queue library backed by Postgres (e.g., `river`).

Redis Streams or Kafka will be deferred until post-V1 milestones when throughput (e.g., hundreds of channels), multiple independent consumer groups, or stream replay requirements genuinely stress Postgres.

## Consequences

### Positive
- **Zero Additional Infrastructure**: Operates entirely within the existing PostgreSQL instance, avoiding new containers, brokers, and network dependencies.
- **Transactional Consistency**: Ingestion writes and queue states remain atomically consistent without distributed transaction overhead or dual-write failure modes.
- **Operational Simplicity**: Backups, monitoring, and debugging leverage standard PostgreSQL tooling.

### Negative / Trade-offs
- **Throughput Ceiling**: Postgres is not optimized for million-message-per-second streaming; polling or advisory locks incur slight database I/O overhead.
- **Migration Path Needed**: When scaling to 100+ channels (V10), the worker interface must be transitioned to Redis Streams or Kafka behind clean interface boundaries.
