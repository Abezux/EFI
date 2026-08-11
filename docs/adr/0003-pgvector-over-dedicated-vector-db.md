# ADR-0003: pgvector over Dedicated Vector Database

Status: Accepted
Date: 2026-08-11

## Context

The news processing pipeline performs approximate nearest neighbor (ANN) vector searches to cluster related incoming posts into cohesive news events based on text embeddings.

Dedicated vector databases (e.g., Pinecone, Weaviate, Qdrant, Milvus) offer high specialized scale and advanced indexing. However, operating an external vector database introduces an additional stateful system to provision, secure, monitor, back up, and synchronize with primary relational entities (`raw_posts`, `news_events`, `event_sources`).

At 10 to 100 channels, the platform generates thousands to tens of thousands of vectors per day—a workload well within the operational sweet spot of PostgreSQL extensions.

## Decision

We will use the `pgvector` PostgreSQL extension for storing text embeddings and performing ANN similarity searches (via HNSW indexing with cosine/inner product distance, time-windowed to recent events).

We reject deploying or subscribing to a dedicated vector database at V0/V1. A dedicated vector database will only be reconsidered if the dataset exceeds 10M+ active vectors or sub-10ms ANN latency is required at high concurrent query volumes (e.g., at the 1,000+ channel scale).

## Consequences

### Positive
- **Single Datastore**: All relational metadata, raw posts, audit logs, and embedding vectors reside in the same PostgreSQL instance.
- **Transactional Integrity**: Inserting raw posts and their embeddings occurs atomically in single transactions without synchronization delays or drift.
- **Relational Filtering**: Efficiently combines vector similarity search with relational filters (e.g., `WHERE posted_at >= NOW() - INTERVAL '48 hours'`) in standard SQL queries.
- **Reduced Infrastructure Costs**: No external SaaS subscription or additional container maintenance.

### Negative / Trade-offs
- **Shared Resource Contention**: Vector indexing and memory consumption share PostgreSQL memory and CPU resources with standard relational queries.
- **Scaling Limits**: If vector volumes grow to tens of millions, vector search memory requirements may necessitate dedicated DB instance sizing or a later migration.
