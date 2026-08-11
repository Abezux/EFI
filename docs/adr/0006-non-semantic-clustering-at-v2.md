# ADR-0006: Non-Semantic Clustering at V2

Status: Accepted
Date: 2026-08-11

## Context

The Ethiopia News Aggregation Platform requires post deduplication and event clustering to aggregate multiple source reports of the same real-world news story into unified events.

In V2, the goal is to implement the core processing pipeline and basic deduplication before introducing vector embeddings and AI/LLM components. We evaluated whether to introduce semantic embedding models immediately at V2 or use a non-semantic, text-similarity approach (Simhash) with clear phase boundaries.

## Decision

We will use **64-bit Simhash with Hamming distance comparison** for deduplication and clustering in **V2**:
- Posts with identical normalized text (Hamming distance = 0) or near-identical text (Hamming distance ≤ threshold, default: 10) are clustered into the matching active `news_events` row within the time window (default: 48 hours).
- Posts exceeding the distance threshold instantiate a new `news_events` row.
- **No external AI models, LLMs, or vector embeddings (`pgvector`) are used in V2.**

We explicitly document and accept the known limitation of this approach:
- Simhash effectively catches exact duplicates, forwards, cross-posted channel broadcasts, and lightly edited reposts.
- Simhash **will not** cluster paraphrased reports with distinct vocabulary (e.g., "Government announces new fuel prices" vs. "Fuel prices to increase starting Monday").
- This semantic clustering capability is deferred to **V3 (Embeddings & Semantic Clustering)**, which will introduce multilingual embeddings (`paraphrase-multilingual-mpnet-base-v2` or similar) and `pgvector` cosine similarity.

## Consequences

### Positive
- **Zero Heavy Infrastructure at V2**: Avoids deploying vector embedding generation models, GPU/inference infrastructure, or external LLM API dependencies during pipeline foundation build.
- **Fast, Deterministic Pipeline**: Simhash computation and Hamming distance calculations execute in sub-millisecond time with negligible memory and CPU overhead.
- **Clear Phase Boundary**: Establishes the database schema (`news_events`, `event_sources`, `raw_posts.simhash`), transaction semantics, and pipeline poll loop in Go, providing a clean integration target for V3's semantic search and clustering enhancements.

### Negative / Trade-offs
- **Semantic Misses**: Stories covering the same real-world event but written with substantially different wording will create separate events until V3 semantic clustering is activated.
- **Threshold Tuning**: Simhash threshold requires calibration to balance false positive attachments against false negative splits.
