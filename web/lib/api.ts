/**
 * Typed API Client for Ethiopia News Platform (V5 REST API)
 *
 * CRITICAL ARCHITECTURAL CONVENTION (AGENTS.md Section 14/15):
 * Two separate API URL environment variables are used and are NOT interchangeable:
 * 1. INTERNAL_API_URL: Used for Server-Side Rendering (SSR) executed within the server
 *    or Docker network (e.g. http://api:8080 in production Docker, or http://localhost:8080 in dev).
 * 2. NEXT_PUBLIC_API_URL: Used for Client-Side components (e.g. LiveFeedUpdater polling)
 *    reachable from the user's web browser (e.g. http://localhost:8080 or public domain).
 *
 * No component should call fetch() directly against the API; all calls route through this module.
 */

export interface Category {
  id: number;
  name: string;
  slug: string;
  event_count?: number;
}

export interface Entity {
  id: number;
  name: string;
  type: string;
}

export interface EventSource {
  channel_name: string;
  channel_handle: string;
  posted_at: string;
  excerpt: string;
  telegram_message_id?: number;
}

export interface NewsEvent {
  id: number;
  canonical_title: string;
  slug?: string;
  ai_headline?: string | null;
  ai_summary: string;
  ai_summary_generated: boolean;
  category?: Category | null;
  entities?: Entity[];
  sources?: EventSource[];
  source_count: number;
  first_seen_at: string;
  last_updated_at: string;
}

export interface EventsResponse {
  events: NewsEvent[];
  total: number;
  limit: number;
  offset: number;
}

export interface SearchResponse {
  events: NewsEvent[];
  total: number;
  limit: number;
  offset: number;
}

export interface HealthResponse {
  status: string;
}

export class ApiError extends Error {
  public status: number;
  public statusText: string;

  constructor(status: number, statusText: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.statusText = statusText;
  }
}

/**
 * Resolves the appropriate base API URL depending on whether the execution context
 * is server-side (Node.js runtime) or client-side (Browser runtime).
 */
export function getApiBaseUrl(isServer?: boolean): string {
  const isServerContext =
    isServer !== undefined ? isServer : typeof window === 'undefined';

  if (isServerContext) {
    // Server-side context (SSR / Server Component)
    // Prefers INTERNAL_API_URL for Docker container network access (e.g., http://api:8080)
    return (
      process.env.INTERNAL_API_URL ||
      process.env.NEXT_PUBLIC_API_URL ||
      'http://localhost:8080'
    );
  }
  // Client-side context (Browser runtime)
  // MUST use NEXT_PUBLIC_API_URL as browser cannot resolve internal Docker hostnames
  return process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
}

interface FetchOptions extends RequestInit {
  timeoutMs?: number;
  isServer?: boolean;
}

async function fetchJson<T>(path: string, options: FetchOptions = {}): Promise<T> {
  const baseUrl = getApiBaseUrl(options.isServer).replace(/\/$/, '');
  const url = `${baseUrl}${path.startsWith('/') ? path : `/${path}`}`;
  const { timeoutMs = 8000, isServer, ...fetchInit } = options;

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const res = await fetch(url, {
      ...fetchInit,
      signal: controller.signal,
      headers: {
        Accept: 'application/json',
        ...fetchInit.headers,
      },
    });

    if (!res.ok) {
      let errorMsg = `API request failed with status ${res.status}: ${res.statusText}`;
      try {
        const errJson = await res.json();
        if (errJson && errJson.error) {
          errorMsg = errJson.error;
        }
      } catch {
        // Fallback to default message
      }
      throw new ApiError(res.status, res.statusText, errorMsg);
    }

    return (await res.json()) as T;
  } catch (err: unknown) {
    if (err instanceof ApiError) {
      throw err;
    }
    const isAbort = err instanceof Error && err.name === 'AbortError';
    const message = isAbort
      ? `API request to ${url} timed out after ${timeoutMs}ms`
      : err instanceof Error
      ? err.message
      : 'Unknown network error';
    throw new ApiError(0, 'NetworkError', message);
  } finally {
    clearTimeout(timeoutId);
  }
}

/**
 * Fetch paginated active news events with optional category or since filter.
 */
export async function getEvents(params?: {
  limit?: number;
  offset?: number;
  category?: string;
  since?: string;
  isServer?: boolean;
}): Promise<EventsResponse> {
  const query = new URLSearchParams();
  if (params?.limit !== undefined) query.set('limit', String(params.limit));
  if (params?.offset !== undefined) query.set('offset', String(params.offset));
  if (params?.category) query.set('category', params.category);
  if (params?.since) query.set('since', params.since);

  const queryString = query.toString();
  const path = `/api/v1/events${queryString ? `?${queryString}` : ''}`;
  return fetchJson<EventsResponse>(path, {
    cache: 'no-store',
    isServer: params?.isServer,
  });
}

/**
 * Fetch a single active news event by ID.
 * Returns null if the event is not found (HTTP 404).
 */
export async function getEventById(
  id: number | string,
  options?: { isServer?: boolean }
): Promise<NewsEvent | null> {
  try {
    return await fetchJson<NewsEvent>(`/api/v1/events/${id}`, {
      cache: 'no-store',
      isServer: options?.isServer,
    });
  } catch (err: unknown) {
    if (err instanceof ApiError && err.status === 404) {
      return null;
    }
    throw err;
  }
}

/**
 * Fetch all categories with event counts.
 */
export async function getCategories(options?: {
  isServer?: boolean;
}): Promise<Category[]> {
  return fetchJson<Category[]>('/api/v1/categories', {
    cache: 'no-store',
    isServer: options?.isServer,
  });
}

/**
 * Search active events by text query (supports English and Amharic).
 */
export async function searchEvents(
  query: string,
  params?: { limit?: number; offset?: number; isServer?: boolean }
): Promise<SearchResponse> {
  const searchParams = new URLSearchParams();
  searchParams.set('q', query);
  if (params?.limit !== undefined) searchParams.set('limit', String(params.limit));
  if (params?.offset !== undefined) searchParams.set('offset', String(params.offset));

  return fetchJson<SearchResponse>(`/api/v1/search?${searchParams.toString()}`, {
    cache: 'no-store',
    isServer: params?.isServer,
  });
}

/**
 * Health check probe.
 */
export async function checkHealth(options?: {
  isServer?: boolean;
}): Promise<HealthResponse> {
  return fetchJson<HealthResponse>('/healthz', {
    cache: 'no-store',
    isServer: options?.isServer,
  });
}

/**
 * Returns the browser-accessible URL for the SSE stream endpoint.
 */
export function getStreamUrl(): string {
  const baseUrl = getApiBaseUrl(false).replace(/\/$/, '');
  return `${baseUrl}/api/v1/stream`;
}

/* =========================================================================
 * V9.1 Admin Panel Types and Client Functions
 * ========================================================================= */

export interface AdminUser {
  id: number;
  email: string;
  role: string;
  created_at?: string;
}

export interface AdminChannel {
  id: number;
  telegram_channel_id: number;
  name: string;
  handle?: string;
  is_active: boolean;
  added_at: string;
  last_seen_at?: string;
  post_count: number;
}

export interface AdminEventSummary {
  id: number;
  canonical_title: string;
  slug: string;
  ai_headline?: string | null;
  ai_summary: string;
  status: string;
  is_hidden: boolean;
  source_count: number;
  first_seen_at: string;
  last_updated_at: string;
  category?: Category | null;
  last_moderation_reason?: string;
  last_moderated_at?: string;
}

export interface AdminEventSource {
  event_id: number;
  raw_post_id: number;
  channel_id: number;
  channel_name: string;
  channel_handle?: string;
  telegram_message_id: number;
  raw_text: string;
  posted_at: string;
  attached_at: string;
}

export interface ModerationActionRecord {
  id: number;
  actor_user_id: number;
  actor_email: string;
  action_type: string;
  target_type: string;
  target_id: number;
  reason?: string;
  created_at: string;
}

export interface AdminEventDetail {
  id: number;
  canonical_title: string;
  slug: string;
  ai_headline?: string | null;
  ai_summary: string;
  status: string;
  is_hidden: boolean;
  source_count: number;
  first_seen_at: string;
  last_updated_at: string;
  category?: Category | null;
  sources: AdminEventSource[];
  entities: Entity[];
  moderation_history: ModerationActionRecord[];
}

export interface NeedsReviewPost {
  raw_post_id: number;
  channel_id: number;
  channel_name: string;
  channel_handle?: string;
  telegram_message_id: number;
  raw_text: string;
  posted_at: string;
  ingested_at: string;
  candidate_event_id?: number;
  candidate_event_title?: string;
  ai_run_reason?: string;
  audit_created_at?: string;
}

export interface ReviewQueueResponse {
  posts: NeedsReviewPost[];
  total: number;
  limit: number;
  offset: number;
}

export interface AdminEventsResponse {
  events: AdminEventSummary[];
  total: number;
  limit: number;
  offset: number;
}

export async function adminLogin(email: string, password: string): Promise<{ user: AdminUser; csrf_token: string }> {
  return fetchJson<{ user: AdminUser; csrf_token: string }>('/api/v1/admin/login', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });
}

export async function adminLogout(): Promise<{ message: string }> {
  return fetchJson<{ message: string }>('/api/v1/admin/logout', {
    method: 'POST',
    credentials: 'include',
  });
}

export async function getAdminMe(): Promise<{ user: AdminUser }> {
  return fetchJson<{ user: AdminUser }>('/api/v1/admin/me', {
    credentials: 'include',
    cache: 'no-store',
  });
}

export async function getAdminCSRF(): Promise<{ csrf_token: string }> {
  return fetchJson<{ csrf_token: string }>('/api/v1/admin/csrf', {
    credentials: 'include',
    cache: 'no-store',
  });
}

export async function getAdminChannels(): Promise<{ channels: AdminChannel[] }> {
  return fetchJson<{ channels: AdminChannel[] }>('/api/v1/admin/channels', {
    credentials: 'include',
    cache: 'no-store',
  });
}

export async function addAdminChannel(data: {
  telegram_channel_id: number;
  name: string;
  handle: string;
  reason?: string;
  csrfToken: string;
}): Promise<AdminChannel> {
  return fetchJson<AdminChannel>('/api/v1/admin/channels', {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': data.csrfToken,
    },
    body: JSON.stringify({
      telegram_channel_id: data.telegram_channel_id,
      name: data.name,
      handle: data.handle,
      reason: data.reason || '',
    }),
  });
}

export async function toggleAdminChannel(channelId: number, reason: string, csrfToken: string): Promise<AdminChannel> {
  return fetchJson<AdminChannel>(`/api/v1/admin/channels/${channelId}/toggle`, {
    method: 'PATCH',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify({ reason }),
  });
}

export async function getAdminEvents(params?: {
  limit?: number;
  offset?: number;
  status?: string;
  hidden?: boolean;
}): Promise<AdminEventsResponse> {
  const query = new URLSearchParams();
  if (params?.limit !== undefined) query.set('limit', String(params.limit));
  if (params?.offset !== undefined) query.set('offset', String(params.offset));
  if (params?.status) query.set('status', params.status);
  if (params?.hidden !== undefined) query.set('hidden', String(params.hidden));

  const queryString = query.toString();
  return fetchJson<AdminEventsResponse>(`/api/v1/admin/events${queryString ? `?${queryString}` : ''}`, {
    credentials: 'include',
    cache: 'no-store',
  });
}

export async function getAdminEventById(id: number | string): Promise<AdminEventDetail | null> {
  try {
    return await fetchJson<AdminEventDetail>(`/api/v1/admin/events/${id}`, {
      credentials: 'include',
      cache: 'no-store',
    });
  } catch (err: unknown) {
    if (err instanceof ApiError && err.status === 404) {
      return null;
    }
    throw err;
  }
}

export async function hideAdminEvent(eventId: number, reason: string, csrfToken: string): Promise<{ message: string }> {
  return fetchJson<{ message: string }>(`/api/v1/admin/events/${eventId}/hide`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify({ reason }),
  });
}

export async function restoreAdminEvent(eventId: number, reason: string, csrfToken: string): Promise<{ message: string }> {
  return fetchJson<{ message: string }>(`/api/v1/admin/events/${eventId}/restore`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify({ reason }),
  });
}

export async function detachAdminSource(eventId: number, rawPostId: number, reason: string, csrfToken: string): Promise<{ message: string }> {
  return fetchJson<{ message: string }>(`/api/v1/admin/events/${eventId}/detach-source`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify({ raw_post_id: rawPostId, reason }),
  });
}

export async function getReviewQueue(params?: {
  limit?: number;
  offset?: number;
}): Promise<ReviewQueueResponse> {
  const query = new URLSearchParams();
  if (params?.limit !== undefined) query.set('limit', String(params.limit));
  if (params?.offset !== undefined) query.set('offset', String(params.offset));

  const queryString = query.toString();
  return fetchJson<ReviewQueueResponse>(`/api/v1/admin/review-queue${queryString ? `?${queryString}` : ''}`, {
    credentials: 'include',
    cache: 'no-store',
  });
}

export async function resolveReviewQueue(
  rawPostId: number,
  data: {
    decision: 'attach_to_event' | 'create_new_event' | 'discard';
    target_event_id?: number;
    reason: string;
    csrfToken: string;
  }
): Promise<{ message: string }> {
  return fetchJson<{ message: string }>(`/api/v1/admin/review-queue/${rawPostId}/resolve`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': data.csrfToken,
    },
    body: JSON.stringify({
      decision: data.decision,
      target_event_id: data.target_event_id,
      reason: data.reason,
    }),
  });
}

