package main

import (
	"context"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestVerify_LowConfidencePreservesNeedsReview(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store := NewStore(db)
	logger := NewLogger("INFO")
	cfg := &Config{
		ClusteringWindow:          48 * time.Hour,
		VerifyConfidenceThreshold: 0.75,
		MaxLLMRetries:             2,
	}

	uniqueVecRaw := make([]float32, 768)
	uniqueVecRaw[78] = 1.0
	uniqueVec := FormatPgVector(uniqueVecRaw)

	// 1. Setup candidate event and needs_review post
	var eventID int64
	err := db.QueryRow(`
		INSERT INTO news_events (canonical_title, status, first_seen_at, last_updated_at, source_count, embedding_centroid)
		VALUES ('NBE Banking Reform Directive', 'active', NOW(), NOW(), 1, $1::vector)
		RETURNING id
	`, uniqueVec).Scan(&eventID)
	if err != nil {
		t.Fatalf("setup candidate event: %v", err)
	}
	defer db.Exec(`DELETE FROM news_events WHERE id = $1`, eventID)

	var postID int64
	err = db.QueryRow(`
		INSERT INTO raw_posts (channel_id, telegram_message_id, raw_text, normalized_text, processing_status, posted_at, embedding)
		VALUES (1, 88001, 'Uncertain news post text', 'uncertain news post text', 'needs_review', NOW(), $1::vector)
		RETURNING id
	`, uniqueVec).Scan(&postID)
	if err != nil {
		t.Fatalf("setup post: %v", err)
	}
	defer db.Exec(`DELETE FROM raw_posts WHERE id = $1`, postID)
	defer db.Exec(`DELETE FROM processing_audit WHERE raw_post_id = $1`, postID)

	post := &RawPost{
		ID:       postID,
		RawText:  "Uncertain news post text",
		PostedAt: time.Now(),
	}

	// 2. Mock LLM returns LOW CONFIDENCE (0.50 < 0.75 threshold)
	mockLLM := &MockLLMClient{
		VerifyFunc: func(ctx context.Context, postText, candidateEventText string) (*VerificationResult, error) {
			return &VerificationResult{
				Decision:    "same_event",
				Confidence:  0.50, // Low confidence!
				Reasoning:   "Vague semantic relation; cannot determine with certainty.",
				RawResponse: `{"decision": "same_event", "confidence": 0.50, "reasoning": "Vague semantic relation"}`,
			}, nil
		},
	}

	// 3. Run verification
	outcome, err := ProcessSingleNeedsReviewPost(context.Background(), store, mockLLM, post, cfg, logger, "test-corr-low-conf")
	if err != nil {
		t.Fatalf("ProcessSingleNeedsReviewPost error: %v", err)
	}

	if outcome != OutcomeRemainsInReview {
		t.Errorf("expected outcome %q, got %q", OutcomeRemainsInReview, outcome)
	}

	// 4. Verify in DB: post MUST still be 'needs_review'
	var status string
	err = db.QueryRow(`SELECT processing_status FROM raw_posts WHERE id = $1`, postID).Scan(&status)
	if err != nil {
		t.Fatalf("query post status: %v", err)
	}
	if status != "needs_review" {
		t.Errorf("CRITICAL SAFETY FAILURE: expected status 'needs_review', but post was force-decided to %q", status)
	}

	// 5. Verify in DB: processing_audit row MUST exist with decision 'low_confidence_unresolved'
	var auditDecision string
	var auditConfidence float32
	err = db.QueryRow(`
		SELECT decision, confidence
		FROM processing_audit
		WHERE raw_post_id = $1 AND stage = 'verification'
		ORDER BY id DESC LIMIT 1
	`, postID).Scan(&auditDecision, &auditConfidence)
	if err != nil {
		t.Fatalf("query processing_audit: %v", err)
	}
	if auditDecision != "low_confidence_unresolved" {
		t.Errorf("expected audit decision 'low_confidence_unresolved', got %q", auditDecision)
	}
	if auditConfidence != 0.50 {
		t.Errorf("expected audit confidence 0.50, got %f", auditConfidence)
	}
}

func TestVerify_HighConfidenceSameEventAttaches(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store := NewStore(db)
	logger := NewLogger("INFO")
	cfg := &Config{
		ClusteringWindow:          48 * time.Hour,
		VerifyConfidenceThreshold: 0.75,
		MaxLLMRetries:             2,
	}

	uniqueVecRaw := make([]float32, 768)
	uniqueVecRaw[77] = 1.0
	uniqueVec := FormatPgVector(uniqueVecRaw)

	var eventID int64
	err := db.QueryRow(`
		INSERT INTO news_events (canonical_title, status, first_seen_at, last_updated_at, source_count, embedding_centroid)
		VALUES ('Fuel Price Increase', 'active', NOW(), NOW(), 1, $1::vector)
		RETURNING id
	`, uniqueVec).Scan(&eventID)
	if err != nil {
		t.Fatalf("setup candidate event: %v", err)
	}
	defer db.Exec(`DELETE FROM news_events WHERE id = $1`, eventID)

	var postID int64
	err = db.QueryRow(`
		INSERT INTO raw_posts (channel_id, telegram_message_id, raw_text, normalized_text, processing_status, posted_at, embedding)
		VALUES (1, 88002, 'Ministry announces new benzene tariff', 'ministry announces new benzene tariff', 'needs_review', NOW(), $1::vector)
		RETURNING id
	`, uniqueVec).Scan(&postID)
	if err != nil {
		t.Fatalf("setup post: %v", err)
	}
	defer db.Exec(`DELETE FROM raw_posts WHERE id = $1`, postID)
	defer db.Exec(`DELETE FROM processing_audit WHERE raw_post_id = $1`, postID)
	defer db.Exec(`DELETE FROM event_sources WHERE raw_post_id = $1`, postID)

	post := &RawPost{
		ID:       postID,
		RawText:  "Ministry announces new benzene tariff",
		PostedAt: time.Now(),
	}

	mockLLM := &MockLLMClient{
		VerifyFunc: func(ctx context.Context, postText, candidateEventText string) (*VerificationResult, error) {
			return &VerificationResult{
				Decision:    "same_event",
				Confidence:  0.92,
				Reasoning:   "Both texts report the Ministry fuel price announcement.",
				RawResponse: `{"decision": "same_event", "confidence": 0.92}`,
			}, nil
		},
	}

	outcome, err := ProcessSingleNeedsReviewPost(context.Background(), store, mockLLM, post, cfg, logger, "test-corr-high-same")
	if err != nil {
		t.Fatalf("ProcessSingleNeedsReviewPost error: %v", err)
	}

	if outcome != OutcomeAttachedToEvent {
		t.Errorf("expected outcome %q, got %q", OutcomeAttachedToEvent, outcome)
	}

	var status string
	_ = db.QueryRow(`SELECT processing_status FROM raw_posts WHERE id = $1`, postID).Scan(&status)
	if status != "processed" {
		t.Errorf("expected status 'processed', got %q", status)
	}

	var srcCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM event_sources WHERE event_id = $1 AND raw_post_id = $2`, eventID, postID).Scan(&srcCount)
	if srcCount != 1 {
		t.Errorf("expected post to be attached in event_sources")
	}
}
