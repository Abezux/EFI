package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// TestNeedsReviewDataLeakGuarantee strictly verifies that records in 'needs_review' status
// NEVER leak into any public API endpoint responses under any query condition.
func TestNeedsReviewDataLeakGuarantee(t *testing.T) {
	appDBURL := os.Getenv("APP_DATABASE_URL")
	if appDBURL == "" {
		appDBURL = os.Getenv("DATABASE_URL")
	}
	if appDBURL == "" {
		appDBURL = "postgres://postgres:postgres@localhost:5432/efi_dev?sslmode=disable"
	}

	// 1. Connect as app/admin to insert a controlled temporary 'needs_review' event
	adminDB, err := sql.Open("postgres", appDBURL)
	if err != nil {
		t.Skipf("skipping leak test: cannot connect as admin: %v", err)
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		t.Skipf("skipping leak test: database ping failed: %v", err)
	}

	// Insert test channel
	var channelID int64
	err = adminDB.QueryRow(`
		INSERT INTO channels (telegram_channel_id, name, handle, is_active)
		VALUES (88880001, 'Confidential Review Channel', 'confidential_review', true)
		ON CONFLICT (telegram_channel_id) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`).Scan(&channelID)
	if err != nil {
		t.Fatalf("failed to insert test channel: %v", err)
	}

	// Insert test raw post in needs_review status
	var rawPostID int64
	err = adminDB.QueryRow(`
		INSERT INTO raw_posts (channel_id, telegram_message_id, raw_text, posted_at, processing_status)
		VALUES ($1, 770001, 'SECRET_UNCONFIRMED_LEAK_TEST_KEYWORD: Internal draft memo on bank merger', now(), 'needs_review')
		ON CONFLICT (channel_id, telegram_message_id) DO UPDATE SET raw_text = EXCLUDED.raw_text, processing_status = 'needs_review'
		RETURNING id
	`, channelID).Scan(&rawPostID)
	if err != nil {
		t.Fatalf("failed to insert test raw post: %v", err)
	}

	// Insert test news_event in needs_review status
	var reviewEventID int64
	err = adminDB.QueryRow(`
		INSERT INTO news_events (canonical_title, ai_summary, status, source_count, first_seen_at, last_updated_at)
		VALUES ('SECRET_UNCONFIRMED_LEAK_TEST_TITLE', 'SECRET_UNCONFIRMED_LEAK_TEST_SUMMARY', 'needs_review', 1, now(), now())
		RETURNING id
	`).Scan(&reviewEventID)
	if err != nil {
		t.Fatalf("failed to insert test review event: %v", err)
	}

	// Cleanup test rows after test
	defer func() {
		_, _ = adminDB.Exec("DELETE FROM event_sources WHERE event_id = $1", reviewEventID)
		_, _ = adminDB.Exec("DELETE FROM news_events WHERE id = $1", reviewEventID)
		_, _ = adminDB.Exec("DELETE FROM raw_posts WHERE id = $1", rawPostID)
		_, _ = adminDB.Exec("DELETE FROM channels WHERE id = $1", channelID)
	}()

	// Attach raw post to review event
	_, err = adminDB.Exec(`
		INSERT INTO event_sources (event_id, raw_post_id, similarity_score)
		VALUES ($1, $2, 0.70)
		ON CONFLICT DO NOTHING
	`, reviewEventID, rawPostID)
	if err != nil {
		t.Fatalf("failed to link event_sources: %v", err)
	}

	// 2. Connect API store as efi_api (read-only least privilege)
	apiDBURL := os.Getenv("API_DATABASE_URL")
	if apiDBURL == "" {
		apiDBURL = "postgres://efi_api:efi_api_pass@localhost:5432/efi_dev?sslmode=disable"
	}
	apiStore, err := NewSQLStore(apiDBURL)
	if err != nil {
		t.Fatalf("failed to connect apiStore: %v", err)
	}
	defer apiStore.Close()

	logger := NewLogger("ERROR")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/events", ListEventsHandler(apiStore, 50, logger))
	mux.HandleFunc("GET /api/v1/events/{id}", GetEventHandler(apiStore, logger))
	mux.HandleFunc("GET /api/v1/search", SearchHandler(apiStore, 50, logger))

	// Check 1: GET /api/v1/events MUST NOT contain reviewEventID
	t.Run("GET /api/v1/events excludes needs_review", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=50", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var res EventListResult
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("decode err: %v", err)
		}
		for _, ev := range res.Events {
			if ev.ID == reviewEventID {
				t.Fatalf("CRITICAL SECURITY LEAK: needs_review event %d appeared in /api/v1/events!", reviewEventID)
			}
		}
	})

	// Check 2: GET /api/v1/events/{id} MUST return 404 for reviewEventID
	t.Run("GET /api/v1/events/{id} returns 404 for needs_review", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+string(rune('0'+reviewEventID)), nil)
		req = httptest.NewRequest(http.MethodGet, "/api/v1/events/"+sqlIntString(reviewEventID), nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("CRITICAL SECURITY LEAK: GET /api/v1/events/%d returned HTTP %d instead of 404!", reviewEventID, rec.Code)
		}
	})

	// Check 3: GET /api/v1/search?q=LEAK_TEST MUST return 0 results
	t.Run("GET /api/v1/search excludes needs_review", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=LEAK_TEST", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var res EventListResult
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("decode err: %v", err)
		}
		if res.Total > 0 || len(res.Events) > 0 {
			t.Fatalf("CRITICAL SECURITY LEAK: search for 'LEAK_TEST' returned %d results containing needs_review data!", res.Total)
		}
	})
}

func sqlIntString(n int64) string {
	return jsonNumberString(n)
}

func jsonNumberString(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
