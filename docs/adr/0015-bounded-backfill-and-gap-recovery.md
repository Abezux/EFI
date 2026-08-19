# ADR-0015: Bounded Backfill and Gap Recovery Strategy

## Status
Accepted

## Context
In the Ethiopia News Aggregation Platform, the Telegram ingestion listener (`services/listener/`) subscribes to real-time Telegram updates via MTProto and Telethon. However, when the listener is stopped for maintenance, crashes due to network interruption, or encounters transient infrastructure outages, new messages posted to monitored Telegram channels during the downtime window are lost from the live event stream.

Without a catch-up mechanism, gaps form in the raw post ingestion pipeline, causing missed breaking stories, incomplete cluster attributions, and skewed event timelines.

At the same time, an unbounded backfill (such as downloading complete channel history) would overwhelm the pipeline, exhaust LLM/embedding API quotas, process stale out-of-date news, and trigger aggressive Telegram `FloodWaitError` rate limits.

## Decision

1. **Persistent Channel Checkpointing (`last_synced_message_id`)**:
   - We introduce `channels.last_synced_message_id` (BIGINT, default NULL) to store the highest Telegram message ID successfully synced for each monitored channel.
   - The checkpoint is updated greedily and atomically within the exact same database transaction as `raw_posts` insertion (`GREATEST(COALESCE(last_synced_message_id, 0), %(telegram_message_id)s)`), guaranteeing that the checkpoint never lags behind committed posts.

2. **Bounded Backfill Parameters (Smaller of 100 Messages or 48 Hours)**:
   - Upon startup and reconnection, the listener runs a bounded backfill for each active channel using Telethon's `iter_messages(min_id=last_synced_id, limit=BACKFILL_MAX_MESSAGES)`.
   - The backfill enforces two hard bounds:
     - **Count Bound**: Maximum 100 messages (`BACKFILL_MAX_MESSAGES`).
     - **Recency Bound**: Maximum 48 hours (`BACKFILL_MAX_HOURS`), discarding messages older than `NOW() - 48h`.

3. **Fresh-Start Baseline for New Channels**:
   - When a channel is registered for the first time (`last_synced_message_id IS NULL`), the listener establishes a baseline by retrieving only the single latest message ID (`limit=1`) and setting `last_synced_message_id = latest_msg.id`.
   - Zero historical messages are dumped into `raw_posts` on fresh channel setup, preventing backfill storms when onboarding high-volume channels.

4. **Reuse of V1 Idempotent Insert Path**:
   - All backfilled messages are normalized via `normalize_telethon_message` and inserted using the existing `Database.insert_raw_post` path.
   - The PostgreSQL `UNIQUE(channel_id, telegram_message_id)` constraint ensures complete idempotency and zero duplicates even if live updates overlap with backfill execution.

5. **Telegram Flood-Wait Resilience**:
   - Telethon `FloodWaitError` exceptions during backfill execution are caught explicitly. The backfiller extracts `flood_err.seconds`, logs a warning, sleeps asynchronously for the specified duration, and resumes iterating without crashing the service.

6. **Failure Isolation Across Channels**:
   - Backfill is executed per-channel within isolated try/catch boundaries. A transient failure or rate-limit on one channel does not interrupt or abort backfilling on remaining channels.

## Consequences

### Positive
- **Automatic Gap Recovery**: Downtime, container restarts, and network disconnects are seamlessly recovered up to 48 hours or 100 messages per channel without manual operator intervention.
- **Zero Duplicate Risk**: Idempotent database insertion prevents duplicated posts even under overlapping live and backfill events.
- **Rate Limit & Cost Protection**: Hard bounds prevent overwhelming downstream embedding models, clustering workers, and Telegram API limits.
- **Graceful Baseline Establishment**: New channels start clean from the current timestamp without ingesting years of legacy archives.

### Trade-offs & Limitations
- **Downtime Greater Than 48 Hours / 100 Messages**: Gaps spanning longer than 48 hours or more than 100 messages per channel will only recover the most recent 100 messages within the 48-hour window. Any older posts are intentionally omitted to preserve pipeline focus on active news.
