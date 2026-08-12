package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
)

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbURL := os.Getenv("APP_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://efi_app:efi_app_pass@localhost:5432/efi_dev?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Skipf("skipping store integration test: unable to connect to %s: %v", dbURL, err)
	}

	if err := db.Ping(); err != nil {
		t.Skipf("skipping store integration test: postgres ping failed: %v", err)
	}

	return db
}

func TestStore_EventCentroidLifecycle(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewStore(db)

	// Clean up any test artifacts on exit
	var testChannelID int64
	err := db.QueryRowContext(ctx, "SELECT id FROM channels LIMIT 1").Scan(&testChannelID)
	if err != nil {
		t.Skipf("no channels present in database: %v", err)
	}

	// Insert test raw_post 1
	var postID1, postID2 int64
	now := time.Now().UTC()

	insertPostQuery := `
		INSERT INTO raw_posts (channel_id, telegram_message_id, raw_text, posted_at, processing_status)
		VALUES ($1, $2, $3, $4, 'ingested')
		RETURNING id
	`
	msgID1 := time.Now().UnixNano()
	msgID2 := msgID1 + 1

	if err := db.QueryRowContext(ctx, insertPostQuery, testChannelID, msgID1, "Test raw text 1", now).Scan(&postID1); err != nil {
		t.Fatalf("failed to insert test post 1: %v", err)
	}
	if err := db.QueryRowContext(ctx, insertPostQuery, testChannelID, msgID2, "Test raw text 2", now.Add(time.Minute)).Scan(&postID2); err != nil {
		t.Fatalf("failed to insert test post 2: %v", err)
	}

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM event_sources WHERE raw_post_id IN ($1, $2)", postID1, postID2)
		_, _ = db.ExecContext(ctx, "DELETE FROM raw_posts WHERE id IN ($1, $2)", postID1, postID2)
	}()

	// 1. Create founding event with vector [1.0, 0.0, ...]
	vec1 := make([]float32, 768)
	vec1[0] = 1.0
	post1 := &RawPost{ID: postID1, ChannelID: testChannelID, TelegramMessageID: msgID1, RawText: "Test raw text 1", PostedAt: now}

	eventID, err := store.CreateEvent(ctx, post1, 1111, "test normalized text 1", vec1, "Test Event Title")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM news_events WHERE id = $1", eventID)
	}()

	// Verify event centroid is vec1
	candidates, err := store.FetchRecentEventCandidates(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("FetchRecentEventCandidates failed: %v", err)
	}

	var foundCandidate *EventCandidate
	for _, c := range candidates {
		if c.EventID == eventID {
			foundCandidate = c
			break
		}
	}
	if foundCandidate == nil {
		t.Fatalf("created event %d not found in candidates", eventID)
	}
	if len(foundCandidate.EmbeddingCentroid) != 768 || math.Abs(float64(foundCandidate.EmbeddingCentroid[0]-1.0)) > 1e-4 {
		t.Fatalf("initial centroid mismatch: %v", foundCandidate.EmbeddingCentroid[:5])
	}

	// 2. Attach post 2 with vector [0.0, 1.0, ...]
	vec2 := make([]float32, 768)
	vec2[1] = 1.0
	post2 := &RawPost{ID: postID2, ChannelID: testChannelID, TelegramMessageID: msgID2, RawText: "Test raw text 2", PostedAt: now.Add(time.Minute)}

	if err := store.AttachToEvent(ctx, eventID, post2, 2222, "test normalized text 2", vec2, 95); err != nil {
		t.Fatalf("AttachToEvent failed: %v", err)
	}

	// Verify centroid recomputed: average of [1,0,...] and [0,1,...] = [0.5, 0.5, ...]
	candidates, err = store.FetchRecentEventCandidates(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("FetchRecentEventCandidates after attach failed: %v", err)
	}

	for _, c := range candidates {
		if c.EventID == eventID {
			foundCandidate = c
			break
		}
	}
	if len(foundCandidate.EmbeddingCentroid) != 768 {
		t.Fatalf("updated centroid length mismatch, got %d", len(foundCandidate.EmbeddingCentroid))
	}
	if math.Abs(float64(foundCandidate.EmbeddingCentroid[0]-0.5)) > 1e-3 || math.Abs(float64(foundCandidate.EmbeddingCentroid[1]-0.5)) > 1e-3 {
		t.Fatalf("recomputed centroid mismatch: got [%f, %f], expected [0.5, 0.5]", foundCandidate.EmbeddingCentroid[0], foundCandidate.EmbeddingCentroid[1])
	}

	t.Logf("Centroid accurately recomputed to average [%f, %f] across 2 event sources", foundCandidate.EmbeddingCentroid[0], foundCandidate.EmbeddingCentroid[1])
}

func TestStore_NotifyOnWrite(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewStore(db)

	dbURL := os.Getenv("APP_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://efi_app:efi_app_pass@localhost:5432/efi_dev?sslmode=disable"
	}

	listener := pq.NewListener(dbURL, 500*time.Millisecond, 2*time.Second, func(event pq.ListenerEventType, err error) {})
	defer listener.Close()

	if err := listener.Listen("news_events_channel"); err != nil {
		t.Fatalf("failed to listen on news_events_channel: %v", err)
	}

	// Drain any leftover notifications
	drainTimeout := time.After(200 * time.Millisecond)
drainLoop:
	for {
		select {
		case <-listener.NotificationChannel():
		case <-drainTimeout:
			break drainLoop
		}
	}

	var testChannelID int64
	err := db.QueryRowContext(ctx, "SELECT id FROM channels LIMIT 1").Scan(&testChannelID)
	if err != nil {
		t.Skipf("no channels present in database: %v", err)
	}

	var postID1, postID2 int64
	now := time.Now().UTC()
	msgID1 := time.Now().UnixNano()
	msgID2 := msgID1 + 10

	insertPostQuery := `
		INSERT INTO raw_posts (channel_id, telegram_message_id, raw_text, posted_at, processing_status)
		VALUES ($1, $2, $3, $4, 'ingested')
		RETURNING id
	`
	if err := db.QueryRowContext(ctx, insertPostQuery, testChannelID, msgID1, "Test raw text notify 1", now).Scan(&postID1); err != nil {
		t.Fatalf("failed to insert test post 1: %v", err)
	}
	if err := db.QueryRowContext(ctx, insertPostQuery, testChannelID, msgID2, "Test raw text notify 2", now.Add(time.Minute)).Scan(&postID2); err != nil {
		t.Fatalf("failed to insert test post 2: %v", err)
	}

	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM event_sources WHERE raw_post_id IN ($1, $2)", postID1, postID2)
		_, _ = db.ExecContext(ctx, "DELETE FROM raw_posts WHERE id IN ($1, $2)", postID1, postID2)
	}()

	// 1. Test CreateEvent NOTIFY
	post1 := &RawPost{ID: postID1, ChannelID: testChannelID, TelegramMessageID: msgID1, RawText: "Test raw text notify 1", PostedAt: now}
	eventID, err := store.CreateEvent(ctx, post1, 3333, "normalized 1", nil, "Notify Test Event")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM news_events WHERE id = $1", eventID)
	}()

	select {
	case n := <-listener.NotificationChannel():
		if n == nil {
			t.Fatalf("received nil notification on CreateEvent")
		}
		expectedPayload := fmt.Sprintf(`{"type":"new_event","event_id":%d}`, eventID)
		if n.Extra != expectedPayload {
			t.Fatalf("unexpected notification payload on CreateEvent: got %q, want %q", n.Extra, expectedPayload)
		}
		t.Logf("Successfully captured CreateEvent NOTIFY: %s", n.Extra)
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for CreateEvent NOTIFY on news_events_channel")
	}

	// 2. Test AttachToEvent NOTIFY
	post2 := &RawPost{ID: postID2, ChannelID: testChannelID, TelegramMessageID: msgID2, RawText: "Test raw text notify 2", PostedAt: now.Add(time.Minute)}
	if err := store.AttachToEvent(ctx, eventID, post2, 4444, "normalized 2", nil, 90); err != nil {
		t.Fatalf("AttachToEvent failed: %v", err)
	}

	select {
	case n := <-listener.NotificationChannel():
		if n == nil {
			t.Fatalf("received nil notification on AttachToEvent")
		}
		expectedPayload := fmt.Sprintf(`{"type":"event_updated","event_id":%d}`, eventID)
		if n.Extra != expectedPayload {
			t.Fatalf("unexpected notification payload on AttachToEvent: got %q, want %q", n.Extra, expectedPayload)
		}
		t.Logf("Successfully captured AttachToEvent NOTIFY: %s", n.Extra)
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for AttachToEvent NOTIFY on news_events_channel")
	}
}
