# ADR-0011: Real-Time Event Delivery via Postgres LISTEN/NOTIFY and In-Process Server-Sent Events (SSE) Hub

## Status
Accepted

## Context
In V7 (Real-Time Delivery), the Ethiopia News Aggregation Platform requires real-time push updates to client browsers whenever:
1. A new news event is created (`new_event`).
2. An existing event gains a new attached source or receives updated enrichment summaries (`event_updated`).

The platform previously decided in ADR-0002 to use Server-Sent Events (SSE) over WebSockets for client communication, and in ADR-0001 to maintain a Postgres-only queueing and messaging footprint at V1–V7 without introducing external message brokers (such as Redis or Kafka).

This record details the concrete mechanism used to bridge pipeline writes in the `processor` service to streaming SSE clients connected to the `api` service.

## Decision
We choose **application-level PostgreSQL `NOTIFY` calls** in `services/processor/store.go` coupled within write transactions, paired with a persistent **`pq.Listener` subscriber (`notify_listener.go`)** in `services/api/` that fans out notifications to active browser clients via an **in-process `SSEHub` (`sse.go`)** serving `GET /api/v1/stream`.

### Key Architectural Choices & Rationale
1. **Explicit Application-Level NOTIFY vs. Database Triggers**:
   - The notification calls (`pg_notify('news_events_channel', ...)`) are written directly in Go application code (`store.go`) inside the respective database transactions (`CreateEvent`, `AttachToEvent`, `SaveEventEnrichment`).
   - *Rationale*: Traceability and visibility. Keeping notification logic explicit in Go alongside the domain logic ensures developers and automated agents can inspect and maintain event triggers directly in the codebase without hidden database trigger side effects.
2. **Minimal Notification Payloads**:
   - Notifications carry only `{ "type": "new_event" | "event_updated", "event_id": <id> }`.
   - *Rationale*: PostgreSQL NOTIFY payloads are limited to 8000 bytes. Carrying only the event ID and event type avoids payload truncation, prevents data serialization divergence, and allows client components to fetch full, consistent representations via existing REST endpoints.
3. **In-Process SSE Hub & Connection Limiting**:
   - The API service maintains an in-memory `SSEHub` with non-blocking channel fan-out, automatic slow-client eviction, and per-IP concurrent connection bounding (`SSEConnectionLimiter`, defaulting to `MAX_SSE_PER_IP = 5`).
   - *Rationale*: Protects server resources from connection exhaustion attacks and prevents slow network consumers from blocking active clients.
4. **Self-Healing LISTEN Subscriber with Reconnection Backoff**:
   - `NotifyListener` utilizes `pq.NewListener` with automatic heartbeat pinging and exponential reconnection backoff.
   - *Rationale*: The LISTEN subscriber is the single point of event capture between PostgreSQL and the API hub. Clear structured logging and automatic reconnection ensure seamless recovery during database restarts or network interruptions.
5. **Single-Instance API Scope & Future Scalability Seam**:
   - For V7, single-instance deployment is standard. When the API service scales horizontally across multiple container instances in future versions (V9/V10), the pub/sub tier can transition to Redis pub/sub or NATS without modifying the client-facing SSE contract.

## Consequences
### Positive
- **Zero New Infrastructure**: Operates entirely on the existing PostgreSQL instance and Go runtime with zero additional third-party daemons or cloud services.
- **Event-Driven UX**: Browser clients update live in real time without continuous polling intervals.
- **Atomic Notification Consistency**: Notifications fire if and only if the underlying database transaction commits successfully.
- **Graceful Degradation**: If SSE streaming is unavailable or blocked by network proxies, frontend clients degrade silently to initial SSR and user-initiated refreshes.

### Trade-offs & Limitations
- **Best-Effort Delivery**: PostgreSQL `NOTIFY` is ephemeral; notifications are not queued for disconnected listeners. If the API's listener is reconnecting during a notification, that specific push is missed. The frontend's initial SSR fetch and manual reload capabilities mitigate this limitation.
