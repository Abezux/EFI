package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockStore struct {
	eventsFn     func(ctx context.Context, filter EventFilter) (*EventListResult, error)
	eventByIDFn  func(ctx context.Context, id int64) (*EventDetail, error)
	categoriesFn func(ctx context.Context) ([]CategoryWithCount, error)
	searchFn     func(ctx context.Context, query string, limit, offset int) (*EventListResult, error)
	pingFn       func(ctx context.Context) error
}

func (m *mockStore) GetEvents(ctx context.Context, filter EventFilter) (*EventListResult, error) {
	if m.eventsFn != nil {
		return m.eventsFn(ctx, filter)
	}
	return &EventListResult{Events: []EventSummary{}, Total: 0, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (m *mockStore) GetEventByID(ctx context.Context, id int64) (*EventDetail, error) {
	if m.eventByIDFn != nil {
		return m.eventByIDFn(ctx, id)
	}
	return nil, ErrNotFound
}

func (m *mockStore) GetCategories(ctx context.Context) ([]CategoryWithCount, error) {
	if m.categoriesFn != nil {
		return m.categoriesFn(ctx)
	}
	return []CategoryWithCount{}, nil
}

func (m *mockStore) SearchEvents(ctx context.Context, query string, limit, offset int) (*EventListResult, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, query, limit, offset)
	}
	return &EventListResult{Events: []EventSummary{}, Total: 0, Limit: limit, Offset: offset}, nil
}

func (m *mockStore) Ping(ctx context.Context) error {
	if m.pingFn != nil {
		return m.pingFn(ctx)
	}
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

func TestHealthHandler(t *testing.T) {
	logger := NewLogger("ERROR")

	t.Run("healthy database", func(t *testing.T) {
		mock := &mockStore{
			pingFn: func(ctx context.Context) error { return nil },
		}
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		handler := HealthHandler(mock, logger)
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		var resp HealthResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != "healthy" {
			t.Errorf("expected status 'healthy', got %q", resp.Status)
		}
	})

	t.Run("unhealthy database", func(t *testing.T) {
		mock := &mockStore{
			pingFn: func(ctx context.Context) error { return errors.New("connection refused") },
		}
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		handler := HealthHandler(mock, logger)
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", rec.Code)
		}
		var resp HealthResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != "unhealthy" {
			t.Errorf("expected status 'unhealthy', got %q", resp.Status)
		}
	})
}

func TestListEventsHandler(t *testing.T) {
	logger := NewLogger("ERROR")
	fixedTime := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	mock := &mockStore{
		eventsFn: func(ctx context.Context, filter EventFilter) (*EventListResult, error) {
			return &EventListResult{
				Events: []EventSummary{
					{
						ID:                 101,
						CanonicalTitle:     "NBE Issues New Directive",
						AISummary:          "The central bank introduced updated liquidity requirements.",
						AISummaryGenerated: true,
						Category: &CategoryInfo{
							ID:   1,
							Name: "Banking & Finance",
							Slug: "banking-finance",
						},
						Entities: []EntityInfo{
							{ID: 5, Name: "National Bank of Ethiopia", Type: "organization"},
						},
						SourceCount:   3,
						FirstSeenAt:   fixedTime,
						LastUpdatedAt: fixedTime,
					},
				},
				Total:  1,
				Limit:  filter.Limit,
				Offset: filter.Offset,
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?category=banking-finance&limit=10&offset=0", nil)
	rec := httptest.NewRecorder()

	handler := ListEventsHandler(mock, 50, logger)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res EventListResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Total != 1 || len(res.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(res.Events))
	}
	ev := res.Events[0]
	if ev.ID != 101 {
		t.Errorf("expected ID 101, got %d", ev.ID)
	}
	if !ev.AISummaryGenerated {
		t.Errorf("expected AISummaryGenerated = true")
	}
	if ev.Category == nil || ev.Category.Slug != "banking-finance" {
		t.Errorf("expected category slug 'banking-finance', got %v", ev.Category)
	}
}

func TestGetEventHandler(t *testing.T) {
	logger := NewLogger("ERROR")
	fixedTime := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	mock := &mockStore{
		eventByIDFn: func(ctx context.Context, id int64) (*EventDetail, error) {
			if id == 42 {
				return &EventDetail{
					ID:                 42,
					CanonicalTitle:     "Commercial Bank Expands Digital Services",
					AISummary:          "CBE announced new digital payment integration.",
					AISummaryGenerated: true,
					Category: &CategoryInfo{
						ID:   1,
						Name: "Banking & Finance",
						Slug: "banking-finance",
					},
					Entities: []EntityInfo{
						{ID: 1, Name: "CBE", Type: "organization"},
					},
					Sources: []SourceDetail{
						{
							ChannelName:   "Addis Standard",
							ChannelHandle: "addisstandard",
							PostedAt:      fixedTime,
							Excerpt:       "Commercial Bank of Ethiopia announced the rollout...",
						},
					},
					SourceCount:   1,
					FirstSeenAt:   fixedTime,
					LastUpdatedAt: fixedTime,
				}, nil
			}
			return nil, ErrNotFound
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/events/{id}", GetEventHandler(mock, logger))

	t.Run("found event", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/42", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		var detail EventDetail
		if err := json.NewDecoder(rec.Body).Decode(&detail); err != nil {
			t.Fatalf("failed to decode detail: %v", err)
		}
		if detail.ID != 42 {
			t.Errorf("expected ID 42, got %d", detail.ID)
		}
		if !detail.AISummaryGenerated {
			t.Errorf("expected AISummaryGenerated = true")
		}
		if len(detail.Sources) != 1 || detail.Sources[0].ChannelName != "Addis Standard" {
			t.Errorf("expected source from Addis Standard, got %v", detail.Sources)
		}
	})

	t.Run("not found event", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/999", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/invalid", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})
}

func TestListCategoriesHandler(t *testing.T) {
	logger := NewLogger("ERROR")
	mock := &mockStore{
		categoriesFn: func(ctx context.Context) ([]CategoryWithCount, error) {
			return []CategoryWithCount{
				{ID: 1, Name: "Banking & Finance", Slug: "banking-finance", EventCount: 5},
				{ID: 2, Name: "Forex & Trade", Slug: "forex-trade", EventCount: 3},
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)
	rec := httptest.NewRecorder()

	handler := ListCategoriesHandler(mock, logger)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var cats []CategoryWithCount
	if err := json.NewDecoder(rec.Body).Decode(&cats); err != nil {
		t.Fatalf("failed to decode categories: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}
	if cats[0].EventCount != 5 || cats[1].EventCount != 3 {
		t.Errorf("unexpected event counts: %v", cats)
	}
}

func TestSearchHandler(t *testing.T) {
	logger := NewLogger("ERROR")
	fixedTime := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	mock := &mockStore{
		searchFn: func(ctx context.Context, query string, limit, offset int) (*EventListResult, error) {
			if query == "inflation" {
				return &EventListResult{
					Events: []EventSummary{
						{
							ID:                 55,
							CanonicalTitle:     "Inflation Rates Stable in July",
							AISummary:          "Consumer price index report indicates steady inflation.",
							AISummaryGenerated: true,
							SourceCount:        2,
							FirstSeenAt:        fixedTime,
							LastUpdatedAt:      fixedTime,
						},
					},
					Total:  1,
					Limit:  limit,
					Offset: offset,
				}, nil
			}
			return &EventListResult{Events: []EventSummary{}, Total: 0, Limit: limit, Offset: offset}, nil
		},
	}

	t.Run("matching query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=inflation", nil)
		rec := httptest.NewRecorder()

		handler := SearchHandler(mock, 50, logger)
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		var res EventListResult
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if res.Total != 1 || len(res.Events) != 1 || res.Events[0].ID != 55 {
			t.Errorf("expected 1 result with ID 55, got %v", res)
		}
	})

	t.Run("empty query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=", nil)
		rec := httptest.NewRecorder()

		handler := SearchHandler(mock, 50, logger)
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		var res EventListResult
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if res.Total != 0 || len(res.Events) != 0 {
			t.Errorf("expected 0 results for empty query, got %v", res)
		}
	})
}
