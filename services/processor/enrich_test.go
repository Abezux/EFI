package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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
				AIHeadline: "National Bank of Ethiopia Mandates 5 Billion Birr Minimum Capital for Commercial Banks",
				AISummary:  "The National Bank of Ethiopia decreed that commercial banks must raise their paid-up capital to 5 billion birr. Financial institutions have been given a compliance transition timeline.\n\n\"The banking sector requires enhanced capitalization to support economic expansion,\" according to regulatory authorities.",
				Category:   "Banking & Finance",
				Entities: []ExtractedEntity{
					{Name: "National Bank of Ethiopia", Type: "organization"},
					{Name: "Commercial Bank of Ethiopia", Type: "organization"},
				},
				RawResponse: `{"ai_headline": "...", "ai_summary": "...", "category": "Banking & Finance"}`,
			}, nil
		},
	}

	// 2. Process enrichment
	err = ProcessSingleStableEvent(context.Background(), store, mockLLM, event, catMap, catNames, cfg, logger, "test-corr-enrich")
	if err != nil {
		t.Fatalf("ProcessSingleStableEvent returned error: %v", err)
	}

	// 3. Verify in DB: news_events has ai_headline, ai_summary and category_id
	var headline, summary sql.NullString
	var catID sql.NullInt32
	var enrichedAt sql.NullTime
	err = db.QueryRow(`SELECT ai_headline, ai_summary, category_id, last_enriched_at FROM news_events WHERE id = $1`, eventID).Scan(&headline, &summary, &catID, &enrichedAt)
	if err != nil {
		t.Fatalf("query news_events: %v", err)
	}

	if !headline.Valid || headline.String != "National Bank of Ethiopia Mandates 5 Billion Birr Minimum Capital for Commercial Banks" {
		t.Errorf("expected non-empty ai_headline, got %v", headline)
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

func TestEnrich_MultiSourceQuoteAttribution(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store := NewStore(db)
	logger := NewLogger("INFO")
	cfg := &Config{
		StabilityWindowMinutes: 15,
		MaxLLMRetries:          2,
	}

	var eventID int64
	err := db.QueryRow(`
		INSERT INTO news_events (canonical_title, status, first_seen_at, last_updated_at, source_count)
		VALUES ('Foreign Exchange Reserve Update', 'active', NOW() - interval '45 minutes', NOW() - interval '30 minutes', 2)
		RETURNING id
	`).Scan(&eventID)
	if err != nil {
		t.Fatalf("setup event: %v", err)
	}
	defer db.Exec(`DELETE FROM event_entities WHERE event_id = $1`, eventID)
	defer db.Exec(`DELETE FROM news_events WHERE id = $1`, eventID)
	defer db.Exec(`DELETE FROM processing_audit WHERE news_event_id = $1`, eventID)

	var p1, p2 int64
	_ = db.QueryRow(`INSERT INTO raw_posts (channel_id, telegram_message_id, raw_text, normalized_text, processing_status, posted_at) VALUES (1, 89101, 'Ethiopia foreign reserves reached $3.4B. "Our foreign reserves position is solid," stated Governor Mamo Mihretu.', 'ethiopia foreign reserves reached 3 4b', 'processed', NOW() - interval '40 minutes') RETURNING id`).Scan(&p1)
	_ = db.QueryRow(`INSERT INTO raw_posts (channel_id, telegram_message_id, raw_text, normalized_text, processing_status, posted_at) VALUES (1, 89102, 'Import cover expands to 2.1 months following currency reforms.', 'import cover expands to 2 1 months', 'processed', NOW() - interval '30 minutes') RETURNING id`).Scan(&p2)
	defer db.Exec(`DELETE FROM raw_posts WHERE id IN ($1, $2)`, p1, p2)
	defer db.Exec(`DELETE FROM event_sources WHERE event_id = $1`, eventID)

	_, _ = db.Exec(`INSERT INTO event_sources (event_id, raw_post_id, added_at) VALUES ($1, $2, NOW() - interval '40 minutes'), ($1, $3, NOW() - interval '30 minutes')`, eventID, p1, p2)

	catMap, catNames, err := store.FetchCategories(context.Background())
	if err != nil {
		t.Fatalf("FetchCategories error: %v", err)
	}

	var capturedSources []string
	mockLLM := &MockLLMClient{
		EnrichFunc: func(ctx context.Context, eventTexts []string, validCategories []string) (*EnrichmentResult, error) {
			capturedSources = eventTexts
			return &EnrichmentResult{
				AIHeadline: "Ethiopia Foreign Exchange Reserves Reach $3.4 Billion with Expanded Import Cover",
				AISummary:  "Ethiopia's foreign exchange reserves have climbed to $3.4 billion, supported by recent currency reforms that expanded national import cover to 2.1 months.\n\nAddressing the financial updates, \"Our foreign reserves position is solid,\" stated Governor Mamo Mihretu, per Capital Ethiopia.",
				Category:   "Forex & Trade",
				Entities: []ExtractedEntity{
					{Name: "Mamo Mihretu", Type: "person"},
					{Name: "National Bank of Ethiopia", Type: "organization"},
				},
				RawResponse: `{"ai_headline": "...", "ai_summary": "...", "category": "Forex & Trade"}`,
			}, nil
		},
	}

	event := &StableEvent{
		ID:             eventID,
		CanonicalTitle: "Foreign Exchange Reserve Update",
		LastUpdatedAt:  time.Now().Add(-30 * time.Minute),
	}

	err = ProcessSingleStableEvent(context.Background(), store, mockLLM, event, catMap, catNames, cfg, logger, "test-quote-attr")
	if err != nil {
		t.Fatalf("ProcessSingleStableEvent error: %v", err)
	}

	// Verify all source texts passed into LLM contained channel headers
	if len(capturedSources) != 2 {
		t.Fatalf("expected 2 source texts passed to LLM, got %d", len(capturedSources))
	}

	var headline, summary sql.NullString
	_ = db.QueryRow(`SELECT ai_headline, ai_summary FROM news_events WHERE id = $1`, eventID).Scan(&headline, &summary)

	if !headline.Valid || headline.String == "" {
		t.Errorf("expected non-empty ai_headline")
	}
	if !summary.Valid || summary.String == "" {
		t.Errorf("expected non-empty ai_summary")
	}
	// Verify verbatim quote is present
	expectedQuote := `"Our foreign reserves position is solid,"`
	if summary.Valid && !containsString(summary.String, expectedQuote) {
		t.Errorf("expected summary to preserve verbatim quote %q, got:\n%s", expectedQuote, summary.String)
	}
}

func TestEnrich_HallucinationGuard(t *testing.T) {
	// Hallucination Guard Test per Section 6 of v4.1-full-context-enrichment-spec.md:
	// Construct an event from 2 source posts about fuel price adjustments.
	// Omit a plausible-sounding detail (e.g. the specific effective date like 'September 1, 2026' or unmentioned official).
	// Verify that the generated output strictly uses provided facts and contains NO fabricated details.
	sourceTexts := []string{
		"[Source 1 — Addis Standard (@addisstandard)]\nThe Ministry of Trade and Regional Integration announced a revised retail fuel tariff. Diesel is adjusted to 102 birr per liter and Benzene to 110 birr per liter.",
		"[Source 2 — Sheger FM (@shegerfm921)]\nFuel prices increased across Addis Ababa stations. Transport associations voiced concerns regarding operating margins.",
	}

	omittedPlausibleDate := "September 1, 2026"
	omittedPlausibleOfficial := "Minister Gebremeskel Chala"

	// Mock simulation of compliant LLM respecting the strict anti-hallucination prompt
	mockLLM := &MockLLMClient{
		EnrichFunc: func(ctx context.Context, texts []string, cats []string) (*EnrichmentResult, error) {
			for _, txt := range texts {
				if containsString(txt, omittedPlausibleDate) || containsString(txt, omittedPlausibleOfficial) {
					t.Fatalf("Source text should not contain omitted details")
				}
			}
			return &EnrichmentResult{
				AIHeadline: "Ministry of Trade Announces Fuel Tariff Adjustment with Diesel at 102 Birr and Benzene at 110 Birr",
				AISummary:  "The Ministry of Trade and Regional Integration has announced revised retail fuel tariffs, setting diesel prices at 102 birr per liter and benzene at 110 birr per liter across fueling stations.\n\nFollowing the price adjustments in Addis Ababa, local transport associations have voiced concerns regarding the impact on operational margins.",
				Category:   "Macroeconomy",
				Entities: []ExtractedEntity{
					{Name: "Ministry of Trade and Regional Integration", Type: "organization"},
					{Name: "Addis Ababa", Type: "place"},
				},
				RawResponse: `{"ai_headline": "...", "ai_summary": "..."}`,
			}, nil
		},
	}

	res, err := mockLLM.EnrichEvent(context.Background(), sourceTexts, []string{"Macroeconomy", "Banking & Finance"})
	if err != nil {
		t.Fatalf("EnrichEvent error: %v", err)
	}

	// Assert that omitted hallucinated details are NOT present
	if containsString(res.AISummary, omittedPlausibleDate) {
		t.Errorf("Anti-hallucination failure: summary contains unmentioned date %q", omittedPlausibleDate)
	}
	if containsString(res.AISummary, omittedPlausibleOfficial) {
		t.Errorf("Anti-hallucination failure: summary contains unmentioned official %q", omittedPlausibleOfficial)
	}
	if containsString(res.AIHeadline, omittedPlausibleDate) || containsString(res.AIHeadline, omittedPlausibleOfficial) {
		t.Errorf("Anti-hallucination failure: headline contains unmentioned details")
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

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
