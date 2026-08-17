import {
  getEvents,
  getEventById,
  getCategories,
  searchEvents,
  checkHealth,
  getApiBaseUrl,
  ApiError,
} from '../api';

describe('lib/api typed client', () => {
  const originalFetch = global.fetch;
  const originalEnv = process.env;

  beforeEach(() => {
    jest.resetModules();
    process.env = { ...originalEnv };
  });

  afterEach(() => {
    global.fetch = originalFetch;
    process.env = originalEnv;
  });

  describe('getApiBaseUrl', () => {
    it('uses INTERNAL_API_URL in server environment if present', () => {
      process.env.INTERNAL_API_URL = 'http://api:8080';
      process.env.NEXT_PUBLIC_API_URL = 'http://localhost:8080';
      expect(getApiBaseUrl(true)).toBe('http://api:8080');
    });

    it('falls back to NEXT_PUBLIC_API_URL if INTERNAL_API_URL is unset', () => {
      delete process.env.INTERNAL_API_URL;
      process.env.NEXT_PUBLIC_API_URL = 'http://example.com/api';
      expect(getApiBaseUrl(true)).toBe('http://example.com/api');
    });

    it('falls back to default localhost:8080 if neither is set', () => {
      delete process.env.INTERNAL_API_URL;
      delete process.env.NEXT_PUBLIC_API_URL;
      expect(getApiBaseUrl(true)).toBe('http://localhost:8080');
    });

    it('uses NEXT_PUBLIC_API_URL in client context', () => {
      process.env.INTERNAL_API_URL = 'http://api:8080';
      process.env.NEXT_PUBLIC_API_URL = 'https://news.et/api';
      expect(getApiBaseUrl(false)).toBe('https://news.et/api');
    });
  });

  describe('getEvents', () => {
    it('fetches events with query parameters correctly', async () => {
      const mockResponse = {
        events: [
          {
            id: 1,
            canonical_title: 'Test Event',
            ai_summary: 'Summary',
            ai_summary_generated: true,
            source_count: 3,
            first_seen_at: '2026-08-11T12:00:00Z',
            last_updated_at: '2026-08-11T12:00:00Z',
          },
        ],
        total: 1,
        limit: 10,
        offset: 0,
      };

      global.fetch = jest.fn().mockResolvedValue({
        ok: true,
        json: async () => mockResponse,
      } as Response);

      const res = await getEvents({ limit: 10, category: 'banking-finance' });
      expect(res).toEqual(mockResponse);
      expect(global.fetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/v1/events?limit=10&category=banking-finance'),
        expect.any(Object)
      );
    });
  });

  describe('getEventById', () => {
    it('returns event on 200 OK', async () => {
      const mockEvent = {
        id: 20,
        canonical_title: 'Budget Directive Reform',
        ai_summary: 'New procurement rules',
        ai_summary_generated: true,
        source_count: 1,
        first_seen_at: '2026-08-07T09:00:00Z',
        last_updated_at: '2026-08-07T09:00:00Z',
      };

      global.fetch = jest.fn().mockResolvedValue({
        ok: true,
        json: async () => mockEvent,
      } as Response);

      const event = await getEventById(20);
      expect(event).toEqual(mockEvent);
    });

    it('returns null on 404 Not Found without throwing', async () => {
      global.fetch = jest.fn().mockResolvedValue({
        ok: false,
        status: 404,
        statusText: 'Not Found',
        json: async () => ({ error: 'Event not found' }),
      } as Response);

      const event = await getEventById(999);
      expect(event).toBeNull();
    });

    it('throws ApiError on 500 server error', async () => {
      global.fetch = jest.fn().mockResolvedValue({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
        json: async () => ({ error: 'Database error' }),
      } as Response);

      await expect(getEventById(1)).rejects.toThrow(ApiError);
    });
  });

  describe('getCategories', () => {
    it('returns categories list', async () => {
      const mockCategories = [
        { id: 1, name: 'Banking & Finance', slug: 'banking-finance', event_count: 4 },
      ];
      global.fetch = jest.fn().mockResolvedValue({
        ok: true,
        json: async () => mockCategories,
      } as Response);

      const categories = await getCategories();
      expect(categories).toEqual(mockCategories);
    });
  });

  describe('searchEvents', () => {
    it('searches events with encoded queries including Amharic', async () => {
      const mockSearch = {
        events: [],
        total: 0,
        limit: 20,
        offset: 0,
      };
      global.fetch = jest.fn().mockResolvedValue({
        ok: true,
        json: async () => mockSearch,
      } as Response);

      const res = await searchEvents('ባንክ');
      expect(res).toEqual(mockSearch);
      expect(global.fetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/v1/search?q=%E1%89%A3%E1%8A%95%E1%8A%AD'),
        expect.any(Object)
      );
    });
  });

  describe('checkHealth', () => {
    it('returns health status', async () => {
      global.fetch = jest.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ status: 'healthy' }),
      } as Response);

      const res = await checkHealth();
      expect(res).toEqual({ status: 'healthy' });
    });
  });
});
