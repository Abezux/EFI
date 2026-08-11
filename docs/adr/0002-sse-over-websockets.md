# ADR-0002: Server-Sent Events (SSE) over WebSockets

Status: Accepted
Date: 2026-08-11

## Context

The platform requires real-time delivery of newly published news events and live updates (e.g., source count increases, summary revisions) to web browsers.

There are two primary protocols for browser real-time streaming: WebSockets and Server-Sent Events (SSE). The system needs an efficient, resilient, and operationally simple mechanism to stream updates from the backend to client frontends.

## Decision

We will use Server-Sent Events (SSE) over standard HTTP (HTTP/1.1 or HTTP/2) for delivering real-time news updates to clients, and explicitly reject WebSockets.

Clients only receive updates (server-to-client push). There are no requirements for client-to-server real-time bi-directional messaging (such as live chat or client-initiated socket queries) in the product scope.

## Consequences

### Positive
- **Simpler Protocol & Infrastructure**: Works over standard HTTP ports without requiring sticky sessions, protocol upgrades, or complex socket state management across load balancers.
- **Built-in Browser Resilience**: The native browser `EventSource` API handles automatic reconnections, backoff, and event ID resumption out of the box.
- **CDN & Proxy Compatibility**: Works naturally with edge CDNs (like Cloudflare), HTTP proxies, and standard security inspection tools.
- **Independent Scaling**: Backend instances can stream updates directly from PostgreSQL `LISTEN`/`NOTIFY` or an internal pub/sub mechanism.

### Negative / Trade-offs
- **Unidirectional Only**: If bidirectional communication is ever required in future versions (e.g., collaborative moderation dashboards), a separate mechanism or protocol migration would be required.
