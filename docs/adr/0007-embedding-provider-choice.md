# ADR-0007: Embedding Provider Choice for Semantic Clustering

Status: Accepted
Date: 2026-08-11

## Context

At V3, the processing pipeline introduces embedding-based semantic similarity to cluster news posts that share the same underlying event but differ in literal phrasing (resolving the limitation documented in ADR-0006).

Key requirements for the embedding provider:
1. **Multilingual Coverage**: Strong semantic understanding of both **Amharic (Ethiopic script: አማርኛ)** and **English**, which form the primary corpus of Ethiopian news channels.
2. **Low Latency**: Sub-300ms API response times to fit within the real-time processing window.
3. **Cost Efficiency**: Sustainable operational cost under continuous streaming ingestion (10–100 channels, thousands of posts/day).
4. **Rate Limits & Reliability**: High availability and default rate limits (RPM/TPM) sufficient for bursts of breaking news.
5. **Schema Alignment**: Seamless integration with PostgreSQL `pgvector` HNSW indexing.

---

## Provider Evaluation

| Provider & Model | Multilingual & Amharic Support | Output Dimension | Pricing (per 1M tokens) | Typical Latency | Rate Limit (Standard Tier) | Schema Impact |
|---|---|---|---|---|---|---|
| **Google Gemini `text-embedding-004`** *(Selected)* | **Exceptional** across 100+ languages; state-of-the-art representation for Amharic and African languages | **768** | **Free tier** (1500 RPM) / **$0.08** | 100–250 ms | 1,500 RPM | Migration 0004 alters `raw_posts.embedding` to `vector(768)` |
| **OpenAI `text-embedding-3-small`** | Good multilingual; higher token expansion on Ethiopic script | **1536** | $0.02 | 120–250 ms | 3,000 RPM | Matches legacy placeholder `vector(1536)` |
| **Cohere `embed-multilingual-v3.0`** | High quality multilingual embeddings | **1024** | $0.10 | 150–300 ms | 5 RPM (Trial) / 1,000 RPM (Paid) | Requires altering `raw_posts.embedding` to `vector(1024)` |
| **Self-Hosted `BAAI/bge-m3`** | Open weights, excellent multilingual performance | **1024** | Hardware cost ($50+/mo GPU) | 50–100 ms (local GPU) | Self-managed | Requires dedicated GPU infrastructure |

---

## Decision

We select **Google Gemini `text-embedding-004`** (768 dimensions) as the primary embedding model for V3 semantic clustering:

1. **Superior Multilingual & Amharic Accuracy**: Gemini's `text-embedding-004` provides market-leading semantic clustering and retrieval fidelity on Ethiopic script (Amharic) and mixed English/Amharic Ethiopian news broadcasts.
2. **Generous Rate Limits & Free Tier**: 1,500 Requests Per Minute (RPM) on Google AI Studio provides zero-cost operation during development and early production, scaling at $0.00002 / 1k characters (~$0.08 / 1M tokens).
3. **Optimized Vector Dimension (768-d)**: 768 dimensions offer high semantic density while reducing memory footprint in `pgvector` HNSW index structures by 50% compared to 1536 dimensions.
4. **Schema Update**: Migration `0004_semantic_clustering.sql` explicitly modifies `raw_posts.embedding` from the unused `vector(1536)` placeholder to `vector(768)` and adds `news_events.embedding_centroid vector(768)`.
5. **Provider-Agnostic Seam (`Embedder` interface)**: The Go processor interacts with embeddings strictly through the `Embedder` interface in `embed.go`. All provider interactions are isolated behind this interface and configured via `GEMINI_API_KEY` (or `EMBEDDING_API_KEY`).

---

## Consequences

### Positive
- **High Semantic Accuracy**: Resolves paraphrase and multilingual clustering gaps (e.g., matching English and Amharic reports of the same event).
- **Reduced Index Memory**: 768-dimension vectors consume half the RAM in PostgreSQL HNSW indexes compared to 1536-dimension vectors.
- **Provider Isolation**: Decoupled from clustering and database layers via `Embedder` interface.

### Negative / Trade-offs
- **Schema Migration**: Migration `0004_semantic_clustering.sql` must alter the unused `raw_posts.embedding` column to `vector(768)`.
- **API Key Configuration**: Requires `GEMINI_API_KEY` in deployment environments (mocked in CI/unit tests).
