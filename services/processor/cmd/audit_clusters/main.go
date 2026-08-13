package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// AuditStatus describes the anti-drift compliance of an attached source.
type AuditStatus string

const (
	StatusFoundingPost AuditStatus = "FOUNDING_POST"
	StatusPass         AuditStatus = "PASS"
	StatusReview       AuditStatus = "REVIEW"
	StatusFlaggedDrift AuditStatus = "FLAGGED_DRIFT"
)

// SourceAuditResult holds audit findings for a single attached source.
type SourceAuditResult struct {
	RawPostID       int64
	TelegramMsgID   int64
	ChannelName     string
	ChannelHandle   string
	RawText         string
	AddedAt         time.Time
	CentroidSim     float64
	FoundingSim     float64
	Status          AuditStatus
	IsFoundingPost  bool
	SimilarityScore sql.NullInt64
}

// EventAuditReport holds audit findings for a news_event.
type EventAuditReport struct {
	EventID           int64
	CanonicalTitle    string
	FirstSeenAt       time.Time
	LastUpdatedAt     time.Time
	SourceCount       int
	FoundingRawPostID int64
	FoundingText      string
	Sources           []SourceAuditResult
	FlaggedCount      int
	ReviewCount       int
	PassCount         int
}

// CosineSimilarity computes cosine similarity between two float32 slices.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// ParsePgVector parses a PostgreSQL vector text format (e.g. "[0.1,0.2,...]").
func ParsePgVector(s string) ([]float32, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("invalid vector format: %s", s)
	}
	s = s[1 : len(s)-1]
	if len(s) == 0 {
		return []float32{}, nil
	}
	parts := strings.Split(s, ",")
	vec := make([]float32, len(parts))
	for i, p := range parts {
		val, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return nil, fmt.Errorf("invalid float at index %d: %w", i, err)
		}
		vec[i] = float32(val)
	}
	return vec, nil
}

// EvaluateSourceAudit assesses an attached source against centroid and founding embeddings.
func EvaluateSourceAudit(
	sourceID int64,
	foundingID int64,
	sourceEmbedding []float32,
	foundingEmbedding []float32,
	centroidEmbedding []float32,
	highThreshold float64,
	lowThreshold float64,
) (float64, float64, AuditStatus) {
	if sourceID == foundingID {
		return 1.0, 1.0, StatusFoundingPost
	}

	centroidSim := 0.0
	if len(sourceEmbedding) > 0 && len(centroidEmbedding) > 0 {
		centroidSim = CosineSimilarity(sourceEmbedding, centroidEmbedding)
	}

	foundingSim := 0.0
	if len(sourceEmbedding) > 0 && len(foundingEmbedding) > 0 {
		foundingSim = CosineSimilarity(sourceEmbedding, foundingEmbedding)
	} else {
		foundingSim = centroidSim
	}

	if centroidSim >= highThreshold && foundingSim >= highThreshold {
		return centroidSim, foundingSim, StatusPass
	}
	if centroidSim >= lowThreshold && foundingSim >= lowThreshold {
		return centroidSim, foundingSim, StatusReview
	}
	return centroidSim, foundingSim, StatusFlaggedDrift
}

func main() {
	dbURL := os.Getenv("APP_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://efi_app:efi_app_pass@localhost:5432/efi_dev?sslmode=disable"
	}

	fmt.Println("================================================================================")
	fmt.Println("       ETHIOPIA NEWS AGGREGATOR — CLUSTERING ANTI-DRIFT AUDIT TOOL (READ-ONLY)")
	fmt.Println("================================================================================")
	fmt.Printf("Connecting to database: %s\n", maskURL(dbURL))

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Database ping failed: %v\n", err)
		os.Exit(1)
	}

	highThreshold := 0.82
	lowThreshold := 0.65

	reports, err := runAudit(ctx, db, highThreshold, lowThreshold)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Audit execution failed: %v\n", err)
		os.Exit(1)
	}

	printReport(reports, highThreshold, lowThreshold)
}

func runAudit(ctx context.Context, db *sql.DB, highThreshold, lowThreshold float64) ([]*EventAuditReport, error) {
	eventQuery := `
		SELECT ne.id, ne.canonical_title, ne.first_seen_at, ne.last_updated_at, ne.source_count,
		       COALESCE(ne.founding_raw_post_id, 0),
		       COALESCE(ne.embedding_centroid::text, ''),
		       COALESCE(rp_founding.embedding::text, ''),
		       COALESCE(rp_founding.raw_text, '')
		FROM news_events ne
		LEFT JOIN raw_posts rp_founding ON ne.founding_raw_post_id = rp_founding.id
		ORDER BY ne.id ASC
	`
	rows, err := db.QueryContext(ctx, eventQuery)
	if err != nil {
		return nil, fmt.Errorf("query news_events: %w", err)
	}
	defer rows.Close()

	type eventMeta struct {
		report            *EventAuditReport
		centroidVec       []float32
		foundingVec       []float32
	}

	var events []*eventMeta
	for rows.Next() {
		rep := &EventAuditReport{Sources: []SourceAuditResult{}}
		var centroidStr, foundingStr string
		if err := rows.Scan(
			&rep.EventID,
			&rep.CanonicalTitle,
			&rep.FirstSeenAt,
			&rep.LastUpdatedAt,
			&rep.SourceCount,
			&rep.FoundingRawPostID,
			&centroidStr,
			&foundingStr,
			&rep.FoundingText,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		var centroidVec, foundingVec []float32
		if centroidStr != "" {
			centroidVec, _ = ParsePgVector(centroidStr)
		}
		if foundingStr != "" {
			foundingVec, _ = ParsePgVector(foundingStr)
		}

		events = append(events, &eventMeta{
			report:      rep,
			centroidVec: centroidVec,
			foundingVec: foundingVec,
		})
	}

	var results []*EventAuditReport

	for _, em := range events {
		rep := em.report

		srcQuery := `
			SELECT rp.id, rp.telegram_message_id, COALESCE(c.name, ''), COALESCE(c.handle, ''),
			       rp.raw_text, es.added_at, es.similarity_score, COALESCE(rp.embedding::text, '')
			FROM event_sources es
			JOIN raw_posts rp ON es.raw_post_id = rp.id
			LEFT JOIN channels c ON rp.channel_id = c.id
			WHERE es.event_id = $1
			ORDER BY es.added_at ASC, es.raw_post_id ASC
		`
		srcRows, err := db.QueryContext(ctx, srcQuery, rep.EventID)
		if err != nil {
			return nil, fmt.Errorf("query sources for event %d: %w", rep.EventID, err)
		}

		for srcRows.Next() {
			var src SourceAuditResult
			var srcEmbeddingStr string
			if err := srcRows.Scan(
				&src.RawPostID,
				&src.TelegramMsgID,
				&src.ChannelName,
				&src.ChannelHandle,
				&src.RawText,
				&src.AddedAt,
				&src.SimilarityScore,
				&srcEmbeddingStr,
			); err != nil {
				srcRows.Close()
				return nil, fmt.Errorf("scan source: %w", err)
			}

			src.IsFoundingPost = (src.RawPostID == rep.FoundingRawPostID)

			var srcVec []float32
			if srcEmbeddingStr != "" {
				srcVec, _ = ParsePgVector(srcEmbeddingStr)
			}

			centSim, fndSim, status := EvaluateSourceAudit(
				src.RawPostID,
				rep.FoundingRawPostID,
				srcVec,
				em.foundingVec,
				em.centroidVec,
				highThreshold,
				lowThreshold,
			)

			src.CentroidSim = centSim
			src.FoundingSim = fndSim
			src.Status = status

			switch status {
			case StatusPass, StatusFoundingPost:
				rep.PassCount++
			case StatusReview:
				rep.ReviewCount++
			case StatusFlaggedDrift:
				rep.FlaggedCount++
			}

			rep.Sources = append(rep.Sources, src)
		}
		srcRows.Close()

		results = append(results, rep)
	}

	return results, nil
}

func printReport(reports []*EventAuditReport, highThreshold, lowThreshold float64) {
	totalEvents := len(reports)
	totalSources := 0
	totalFlagged := 0
	totalReview := 0
	totalPassed := 0

	for _, r := range reports {
		totalSources += len(r.Sources)
		totalFlagged += r.FlaggedCount
		totalReview += r.ReviewCount
		totalPassed += r.PassCount
	}

	fmt.Println("\n--- AUDIT SUMMARY ---")
	fmt.Printf("Total Events Audited:       %d\n", totalEvents)
	fmt.Printf("Total Sources Evaluated:    %d\n", totalSources)
	fmt.Printf("Sources Meeting Thresholds: %d (%.1f%%)\n", totalPassed, pct(totalPassed, totalSources))
	fmt.Printf("Sources in Review Band:     %d (%.1f%%)\n", totalReview, pct(totalReview, totalSources))
	fmt.Printf("Sources FLAGGED FOR DRIFT:  %d (%.1f%%)\n", totalFlagged, pct(totalFlagged, totalSources))
	fmt.Printf("High Threshold: %.2f | Low Threshold: %.2f\n", highThreshold, lowThreshold)
	fmt.Println("--------------------------------------------------------------------------------")

	for _, r := range reports {
		fmt.Printf("\n=== EVENT #%d: %s ===\n", r.EventID, truncate(r.CanonicalTitle, 80))
		fmt.Printf("  First Seen:      %s\n", r.FirstSeenAt.Format(time.RFC3339))
		fmt.Printf("  Last Updated:    %s\n", r.LastUpdatedAt.Format(time.RFC3339))
		fmt.Printf("  Founding Post:   ID=%d\n", r.FoundingRawPostID)
		fmt.Printf("  Sources Summary: %d total (Pass: %d, Review: %d, Flagged: %d)\n", len(r.Sources), r.PassCount, r.ReviewCount, r.FlaggedCount)
		fmt.Printf("  Founding Text:   %q\n", truncate(r.FoundingText, 100))

		fmt.Println("  Attached Sources:")
		for i, s := range r.Sources {
			statusTag := string(s.Status)
			switch s.Status {
			case StatusFoundingPost:
				statusTag = "[FOUNDING]"
			case StatusPass:
				statusTag = "[ PASS ]"
			case StatusReview:
				statusTag = "[REVIEW]"
			case StatusFlaggedDrift:
				statusTag = "[FLAGGED_DRIFT]"
			}

			chInfo := s.ChannelName
			if s.ChannelHandle != "" {
				chInfo = fmt.Sprintf("%s (@%s)", s.ChannelName, s.ChannelHandle)
			}

			fmt.Printf("    #%d | ID: %-4d | %-15s | Centroid: %5.3f | Founding: %5.3f | %s\n",
				i+1, s.RawPostID, statusTag, s.CentroidSim, s.FoundingSim, chInfo)
			fmt.Printf("        Added: %s | Excerpt: %q\n",
				s.AddedAt.Format(time.RFC3339), truncate(s.RawText, 90))
		}
	}
	fmt.Println("\n================================================================================")
	fmt.Println("Audit complete. Zero database modifications performed.")
	fmt.Println("================================================================================")
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

func pct(part, total int) float64 {
	if total == 0 {
		return 0.0
	}
	return (float64(part) / float64(total)) * 100.0
}

func maskURL(u string) string {
	parts := strings.Split(u, "@")
	if len(parts) == 2 {
		return "postgres://***:***@" + parts[1]
	}
	return u
}
