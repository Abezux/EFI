package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
)

// StructuredLog represents the standard JSON log format defined in AGENTS.md.
type StructuredLog struct {
	Timestamp     string         `json:"timestamp"`
	Level         string         `json:"level"`
	Service       string         `json:"service"`
	Message       string         `json:"message"`
	CorrelationID string         `json:"correlation_id"`
	Extra         map[string]any `json:"extra,omitempty"`
}

func generateCorrelationID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("corr-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func logJSON(level, message, correlationID string, extra map[string]any) {
	if correlationID == "" {
		correlationID = generateCorrelationID()
	}
	entry := StructuredLog{
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Level:         level,
		Service:       "processor",
		Message:       message,
		CorrelationID: correlationID,
		Extra:         extra,
	}
	data, err := json.Marshal(entry)
	if err == nil {
		fmt.Println(string(data))
	}
}

func processBatch(ctx context.Context, store *Store, config *Config) (int, error) {
	posts, err := store.FetchUnprocessedPosts(ctx, config.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("fetch unprocessed posts: %w", err)
	}

	if len(posts) == 0 {
		return 0, nil
	}

	processedCount := 0

	for _, post := range posts {
		corrID := generateCorrelationID()

		// 1. Normalize raw text
		normalized := NormalizeText(post.RawText)

		// 2. Compute 64-bit Simhash
		simhash := ComputeSimhash(normalized)

		// 3. Fetch active candidate events within time window
		candidates, err := store.FetchRecentEventCandidates(ctx, config.ClusteringWindow)
		if err != nil {
			logJSON("ERROR", fmt.Sprintf("Failed to fetch event candidates for post %d: %v", post.ID, err), corrID, map[string]any{"raw_post_id": post.ID})
			continue
		}

		// 4. Determine clustering decision (attach or create new)
		decision := DecideClustering(post.ID, normalized, simhash, candidates, config.SimhashThreshold)

		// 5. Execute DB write transaction
		switch decision.Type {
		case DecisionCreateNew:
			eventID, err := store.CreateEvent(ctx, post, simhash, normalized, decision.CanonicalTitle)
			if err != nil {
				logJSON("ERROR", fmt.Sprintf("Failed to create event for post %d: %v", post.ID, err), corrID, map[string]any{"raw_post_id": post.ID})
				continue
			}
			logJSON("INFO", "Clustered post into new news_event", corrID, map[string]any{
				"raw_post_id":     post.ID,
				"event_id":        eventID,
				"decision":        "created_new_event",
				"canonical_title": decision.CanonicalTitle,
			})

		case DecisionAttach:
			err := store.AttachToEvent(ctx, decision.TargetEventID, post, simhash, normalized, decision.SimilarityScore)
			if err != nil {
				logJSON("ERROR", fmt.Sprintf("Failed to attach post %d to event %d: %v", post.ID, decision.TargetEventID, err), corrID, map[string]any{
					"raw_post_id": post.ID,
					"event_id":    decision.TargetEventID,
				})
				continue
			}
			logJSON("INFO", "Attached post to existing news_event", corrID, map[string]any{
				"raw_post_id":      post.ID,
				"event_id":         decision.TargetEventID,
				"decision":         "attached_to_event",
				"similarity_score": decision.SimilarityScore,
			})
		}

		processedCount++
	}

	return processedCount, nil
}

func main() {
	config, err := LoadConfig()
	if err != nil {
		logJSON("ERROR", fmt.Sprintf("Configuration error: %v", err), "", nil)
		os.Exit(1)
	}

	db, err := sql.Open("postgres", config.DatabaseURL)
	if err != nil {
		logJSON("ERROR", fmt.Sprintf("Database connection error: %v", err), "", nil)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		logJSON("ERROR", fmt.Sprintf("Database ping failed: %v", err), "", nil)
		os.Exit(1)
	}

	store := NewStore(db)

	logJSON("INFO", "V2 Processor service started", "", map[string]any{
		"poll_interval_seconds": config.PollInterval.Seconds(),
		"batch_size":            config.BatchSize,
		"simhash_threshold":     config.SimhashThreshold,
		"clustering_window_hrs": config.ClusteringWindow.Hours(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logJSON("INFO", fmt.Sprintf("Received shutdown signal (%v), stopping processor...", sig), "", nil)
		cancel()
	}()

	// Execute initial batch immediately
	if count, err := processBatch(ctx, store, config); err != nil {
		logJSON("ERROR", fmt.Sprintf("Batch processing error: %v", err), "", nil)
	} else if count > 0 {
		logJSON("INFO", fmt.Sprintf("Processed %d post(s) in batch", count), "", map[string]any{
			"processed_count": count,
		})
	}

	ticker := time.NewTicker(config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logJSON("INFO", "Processor service shutdown complete", "", nil)
			return
		case <-ticker.C:
			count, err := processBatch(ctx, store, config)
			if err != nil {
				logJSON("ERROR", fmt.Sprintf("Batch processing error: %v", err), "", nil)
			} else if count > 0 {
				logJSON("INFO", fmt.Sprintf("Processed %d post(s) in batch", count), "", map[string]any{
					"processed_count": count,
				})
			}
		}
	}
}
