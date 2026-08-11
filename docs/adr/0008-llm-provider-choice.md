# ADR-0008: LLM Provider Choice for Verification and Enrichment

## Status
Accepted

## Context
In V4 (AI Enrichment), the Ethiopia News Aggregation Platform requires text generation and reasoning capabilities for two separate, narrowly-scoped pipeline tasks:
1. **Ambiguous Candidate Verification (`verify.go`)**: Bounded classification determining whether a post in `needs_review` describes the same real-world news event as a candidate event cluster.
2. **Stable Event Enrichment (`enrich.go`)**: Generating objective 2–4 sentence summaries (`ai_summary`), selecting a finance taxonomy category, and extracting named entities (people, places, organizations) for stable multi-source events.

The platform processes both English and Amharic (Ge'ez script) source texts from Ethiopian financial news channels (`@stockmarket_et`, `@shegamediaet`, `@nbethiopia`, `@addisfortune`, `@Capitalethiopia`, etc.).

## Decision
We propose using **Google Gemini (`gemini-flash-latest`)** via the Google AI REST API (`v1beta/v1` `generateContent` endpoint) with structured JSON output enforcement (`responseMimeType: application/json`).

### Reasoning & Provider Comparison
1. **Multilingual & Amharic Comprehension**:
   - Gemini models natively support Ge'ez script and Ethiopian linguistic context, accurately comprehending Amharic financial reporting (e.g., NBE foreign exchange directives, Commercial Bank of Ethiopia capital updates) and translating them into concise, objective English summaries while preserving Amharic and English named entities.
2. **Latency & Throughput**:
   - `gemini-flash-latest` offers sub-second to 1.5s latency per call, well within the batch processor's asynchronous polling cycle.
3. **Cost & Quota Efficiency**:
   - **Free-Tier Limits**: 15 requests per minute (RPM), 1,500 requests per day (RPD), and 1,000,000 tokens per minute (TPM).
   - **Pay-as-you-go Pricing**: $0.075 per 1M input tokens and $0.30 per 1M output tokens (highly economical for 2–4 sentence summaries).
4. **Shared Credentials & Architecture Consistency**:
   - Reuses the existing `GEMINI_API_KEY` configured in V3 (ADR-0007), avoiding additional API accounts or secret management complexity.
5. **Decoupled Seam**:
   - Access is strictly encapsulated behind the `LLMClient` Go interface in `services/processor/llm.go`, ensuring zero coupling between core processor logic and the provider implementation.

## Consequences
- Generation calls consume daily free-tier quota (1,500 RPD). The processor must enforce stability windows (15–30 min) so events are only enriched once when stable, preventing redundant LLM calls on every near-duplicate.
- All verification and enrichment calls must write matching audit rows to `processing_audit` (stage, model, confidence, raw response) in the same database transaction.
- Automated unit tests must mock the `LLMClient` interface to ensure zero external API dependencies during CI test runs.
