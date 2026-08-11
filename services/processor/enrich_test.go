package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestEnrich_StableEventSuccess(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store := NewStore(db)
	logger := NewLogger("INFO")
	cfg := &Config{
		StabilityWindowMinutes: 15,
		MaxLLMRetries:          2,
	}

	// 1. Create a stable event whose last_updated_at was 30 minutes ago
	var eventID int64
	err := db.QueryRow(`
		INSERT INTO news_events (canonical_title, status, first_seen_at, last_updated_at, source_count)
		VALUES ('NBE Banking Directive 2026', 'active', NOW() - interval '40 minutes', NOW() - interval '30 minutes', 2)
		RETURNING id
	`).Scan(&eventID)
	if err != nil {
		t.Fatalf("setup event: %v", err)
	}
	defer db.Exec(`DELETE FROM event_entities WHERE event_id = $1`, eventID)
	defer db.Exec(`DELETE FROM news_events WHERE id = $1`, eventID)
	defer db.Exec(`DELETE FROM processing_audit WHERE news_event_id = $1`, eventID)

	// Attach 2 source posts
	var p1, p2 int64
	_ = db.QueryRow(`INSERT INTO raw_posts (channel_id, telegram_message_id, raw_text, normalized_text, processing_status, posted_at) VALUES (1, 89001, 'NBE increases capital requirement for banks.', 'nbe increases capital requirement for banks', 'processed', NOW() - interval '35 minutes') RETURNING id`).Scan(&p1)
	_ = db.QueryRow(`INSERT INTO raw_posts (channel_id, telegram_message_id, raw_text, normalized_text, processing_status, posted_at) VALUES (1, 89002, 'Commercial banks granted time to reach 5 billion birr.', 'commercial banks granted time to reach 5 billion birr', 'processed', NOW() - interval '30 minutes') RETURNING id`).Scan(&p2)
	defer db.Exec(`DELETE FROM raw_posts WHERE id IN ($1, $2)`, p1, p2)
	defer db.Exec(`DELETE FROM event_sources WHERE event_id = $1`, eventID)

	_, _ = db.Exec(`INSERT INTO event_sources (event_id, raw_post_id, added_at) VALUES ($1, $2, NOW() - interval '35 minutes'), ($1, $3, NOW() - interval '30 minutes')`, eventID, p1, p2)

	// Fetch categories map
	catMap, catNames, err := store.FetchCategories(context.Background())
	if err != nil {
		t.Fatalf("FetchCategories error: %v", err)
	}

	event := &StableEvent{
		ID:             eventID,
		CanonicalTitle: "NBE Banking Directive 2026",
		LastUpdatedAt:  time.Now().Add(-30 * time.Minute),
	}

	mockLLM := &MockLLMClient{
		EnrichFunc: func(ctx context.Context, eventTexts []string, validCategories []string) (*EnrichmentResult, error) {
			return &EnrichmentResult{
				AISummary: "The National Bank of Ethiopia decreed that commercial banks must raise their paid-up capital to 5 billion birr. Financial institutions have been given a compliance transition timeline.",
				Category:  "Banking & Finance",
				Entities: []ExtractedEntity{
					{Name: "National Bank of Ethiopia", Type: "organization"},
					{Name: "Commercial Bank of Ethiopia", Type: "organization"},
				},
				RawResponse: `{"ai_summary": "...", "category": "Banking & Finance"}`,
			}, nil
		},
	}

	// 2. Process enrichment
	err = ProcessSingleStableEvent(context.Background(), store, mockLLM, event, catMap, catNames, cfg, logger, "test-corr-enrich")
	if err != nil {
		t.Fatalf("ProcessSingleStableEvent returned error: %v", err)
	}

	// 3. Verify in DB: news_events has ai_summary and category_id
	var summary sql.NullString
	var catID sql.NullInt32
	var enrichedAt sql.NullTime
	err = db.QueryRow(`SELECT ai_summary, category_id, last_enriched_at FROM news_events WHERE id = $1`, eventID).Scan(&summary, &catID, &enrichedAt)
	if err != nil {
		t.Fatalf("query news_events: %v", err)
	}

	if !summary.Valid || summary.String == "" {
		t.Errorf("expected non-empty ai_summary")
	}
	if !catID.Valid || int(catID.Int32) != catMap["Banking & Finance"] {
		t.Errorf("expected category_id %d, got %v", catMap["Banking & Finance"], catID)
	}
	if !enrichedAt.Valid {
		t.Errorf("expected last_enriched_at to be set")
	}

	// 4. Verify in DB: processing_audit row written
	var auditCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM processing_audit WHERE news_event_id = $1 AND stage = 'enrichment'`, eventID).Scan(&auditCount)
	if err != nil {
		t.Fatalf("query processing_audit: %v", err)
	}
	if auditCount < 1 {
		t.Errorf("expected at least 1 processing_audit row for enrichment, got %d", auditCount)
	}

	// 5. Verify in DB: event_entities written
	var entityCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM event_entities WHERE event_id = $1`, eventID).Scan(&entityCount)
	if err != nil {
		t.Fatalf("query event_entities: %v", err)
	}
	if entityCount != 2 {
		t.Errorf("expected 2 linked entities, got %d", entityCount)
	}
}

func TestEnrich_ProviderFailurePreservesEvent(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store := NewStore(db)
	logger := NewLogger("INFO")
	cfg := &Config{
		StabilityWindowMinutes: 15,
		MaxLLMRetries:          2,
	}

	var eventID int64
	_ = db.QueryRow(`
		INSERT INTO news_events (canonical_title, status, first_seen_at, last_updated_at, source_count)
		VALUES ('Foreign Exchange Floatation', 'active', NOW() - interval '40 minutes', NOW() - interval '30 minutes', 1)
		RETURNING id
	`).Scan(&eventID)
	defer db.Exec(`DELETE FROM news_events WHERE id = $1`, eventID)

	var p1 int64
	_ = db.QueryRow(`INSERT INTO raw_posts (channel_id, telegram_message_id, raw_text, normalized_text, processing_status, posted_at) VALUES (1, 89003, 'Foreign exchange market liberalized.', 'foreign exchange market liberalized', 'processed', NOW() - interval '35 minutes') RETURNING id`).Scan(&p1)
	defer db.Exec(`DELETE FROM raw_posts WHERE id = $1`, p1)
	defer db.Exec(`DELETE FROM event_sources WHERE event_id = $1`, eventID)
	_, _ = db.Exec(`INSERT INTO event_sources (event_id, raw_post_id, added_at) VALUES ($1, $2, NOW() - interval '35 minutes')`, eventID, p1)

	catMap, catNames, _ := store.FetchCategories(context.Background())
	event := &StableEvent{
		ID:             eventID,
		CanonicalTitle: "Foreign Exchange Floatation",
		LastUpdatedAt:  time.Now().Add(-30 * time.Minute),
	}

	mockLLM := &MockLLMClient{
		EnrichFunc: func(ctx context.Context, eventTexts []string, validCategories []string) (*EnrichmentResult, error) {
			return nil, errors.New("upstream 503 service unavailable")
		},
	}

	// Should not fail pipeline or crash
	err := ProcessSingleStableEvent(context.Background(), store, mockLLM, event, catMap, catNames, cfg, logger, "test-corr-fail")
	if err != nil {
		t.Fatalf("ProcessSingleStableEvent should gracefully handle provider failure, got error: %v", err)
	}

	// Event must still be in active state and unmodified
	var status string
	_ = db.QueryRow(`SELECT status FROM news_events WHERE id = $1`, eventID).Scan(&status)
	if status != "active" {
		t.Errorf("expected status 'active', got %q", status)
	}
}
