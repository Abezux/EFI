package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// RawPost represents a row from the raw_posts table.
type RawPost struct {
	ID                int64
	ChannelID         int64
	TelegramMessageID int64
	RawText           string
	PostedAt          time.Time
}

// Store provides database access methods for processing and clustering.
type Store struct {
	db *sql.DB
}

// NewStore creates a new Store instance.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// FetchUnprocessedPosts retrieves the next batch of raw_posts with processing_status = 'ingested'.
func (s *Store) FetchUnprocessedPosts(ctx context.Context, limit int) ([]*RawPost, error) {
	query := `
		SELECT id, channel_id, telegram_message_id, raw_text, posted_at
		FROM raw_posts
		WHERE processing_status = 'ingested'
		ORDER BY posted_at ASC, id ASC
		LIMIT $1
	`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query unprocessed posts: %w", err)
	}
	defer rows.Close()

	var posts []*RawPost
	for rows.Next() {
		p := &RawPost{}
		if err := rows.Scan(&p.ID, &p.ChannelID, &p.TelegramMessageID, &p.RawText, &p.PostedAt); err != nil {
			return nil, fmt.Errorf("scan unprocessed post: %w", err)
		}
		posts = append(posts, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return posts, nil
}

// FetchRecentEventCandidates retrieves active news_events and their source posts within the time window,
// including their embedding centroids.
func (s *Store) FetchRecentEventCandidates(ctx context.Context, window time.Duration) ([]*EventCandidate, error) {
	windowInterval := fmt.Sprintf("%d seconds", int(window.Seconds()))
	eventQuery := `
		SELECT id, canonical_title, first_seen_at, last_updated_at, COALESCE(embedding_centroid::text, '')
		FROM news_events
		WHERE status = 'active' AND last_updated_at >= NOW() - $1::interval
		ORDER BY last_updated_at DESC
	`
	rows, err := s.db.QueryContext(ctx, eventQuery, windowInterval)
	if err != nil {
		return nil, fmt.Errorf("query recent events: %w", err)
	}
	defer rows.Close()

	var candidates []*EventCandidate
	var eventIDs []int64
	candidateMap := make(map[int64]*EventCandidate)

	for rows.Next() {
		c := &EventCandidate{Sources: []EventSourceMember{}}
		var centroidStr string
		if err := rows.Scan(&c.EventID, &c.CanonicalTitle, &c.FirstSeenAt, &c.LastUpdatedAt, &centroidStr); err != nil {
			return nil, fmt.Errorf("scan recent event: %w", err)
		}
		if centroidStr != "" {
			vec, err := ParsePgVector(centroidStr)
			if err == nil {
				c.EmbeddingCentroid = vec
			}
		}
		candidates = append(candidates, c)
		eventIDs = append(eventIDs, c.EventID)
		candidateMap[c.EventID] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("event rows error: %w", err)
	}

	if len(eventIDs) == 0 {
		return candidates, nil
	}

	// Fetch all sources for these candidates
	sourceQuery := `
		SELECT es.event_id, rp.id, COALESCE(rp.normalized_text, ''), COALESCE(rp.simhash, 0)
		FROM event_sources es
		JOIN raw_posts rp ON es.raw_post_id = rp.id
		WHERE es.event_id = ANY($1)
	`
	srcRows, err := s.db.QueryContext(ctx, sourceQuery, pq.Array(eventIDs))
	if err != nil {
		return nil, fmt.Errorf("query event sources: %w", err)
	}
	defer srcRows.Close()

	for srcRows.Next() {
		var eventID int64
		var member EventSourceMember
		if err := srcRows.Scan(&eventID, &member.RawPostID, &member.NormalizedText, &member.Simhash); err != nil {
			return nil, fmt.Errorf("scan event source: %w", err)
		}
		if cand, ok := candidateMap[eventID]; ok {
			cand.Sources = append(cand.Sources, member)
		}
	}
	if err := srcRows.Err(); err != nil {
		return nil, fmt.Errorf("source rows error: %w", err)
	}

	return candidates, nil
}

// CreateEvent atomically creates a news_events row, links the founding raw_post in event_sources,
// sets the initial embedding centroid, and updates the raw_post with normalized_text, simhash, embedding,
// and processing_status = 'processed'.
func (s *Store) CreateEvent(
	ctx context.Context,
	post *RawPost,
	simhash int64,
	normalizedText string,
	embedding []float32,
	canonicalTitle string,
) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // nolint:errcheck

	var eventID int64
	var vecParam sql.NullString
	if len(embedding) > 0 {
		vecParam = sql.NullString{String: FormatPgVector(embedding), Valid: true}
	}

	eventQuery := `
		INSERT INTO news_events (canonical_title, status, first_seen_at, last_updated_at, source_count, embedding_centroid)
		VALUES ($1, 'active', $2, $2, 1, $3::vector)
		RETURNING id
	`
	if err := tx.QueryRowContext(ctx, eventQuery, canonicalTitle, post.PostedAt, vecParam).Scan(&eventID); err != nil {
		return 0, fmt.Errorf("insert news_events: %w", err)
	}

	sourceQuery := `
		INSERT INTO event_sources (event_id, raw_post_id, similarity_score, added_at)
		VALUES ($1, $2, NULL, NOW())
	`
	if _, err := tx.ExecContext(ctx, sourceQuery, eventID, post.ID); err != nil {
		return 0, fmt.Errorf("insert event_sources: %w", err)
	}

	updatePostQuery := `
		UPDATE raw_posts
		SET normalized_text = $1, simhash = $2, embedding = $3::vector, processing_status = 'processed'
		WHERE id = $4
	`
	if _, err := tx.ExecContext(ctx, updatePostQuery, normalizedText, simhash, vecParam, post.ID); err != nil {
		return 0, fmt.Errorf("update raw_posts: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return eventID, nil
}

// AttachToEvent atomically links a raw_post to an existing news_event, increments source_count,
// updates last_updated_at, recomputes the centroid vector from attached posts, and marks the raw_post as processed.
func (s *Store) AttachToEvent(
	ctx context.Context,
	eventID int64,
	post *RawPost,
	simhash int64,
	normalizedText string,
	embedding []float32,
	similarityScore int,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // nolint:errcheck

	sourceQuery := `
		INSERT INTO event_sources (event_id, raw_post_id, similarity_score, added_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (event_id, raw_post_id) DO NOTHING
	`
	if _, err := tx.ExecContext(ctx, sourceQuery, eventID, post.ID, similarityScore); err != nil {
		return fmt.Errorf("insert event_sources: %w", err)
	}

	var vecParam sql.NullString
	if len(embedding) > 0 {
		vecParam = sql.NullString{String: FormatPgVector(embedding), Valid: true}
	}

	updatePostQuery := `
		UPDATE raw_posts
		SET normalized_text = $1, simhash = $2, embedding = $3::vector, processing_status = 'processed'
		WHERE id = $4
	`
	if _, err := tx.ExecContext(ctx, updatePostQuery, normalizedText, simhash, vecParam, post.ID); err != nil {
		return fmt.Errorf("update raw_posts: %w", err)
	}

	// Recompute embedding centroid and update news_events transactionally
	updateEventQuery := `
		UPDATE news_events
		SET source_count = (SELECT COUNT(*) FROM event_sources WHERE event_id = $1),
		    last_updated_at = GREATEST(last_updated_at, $2),
		    embedding_centroid = COALESCE(
		        (SELECT avg(rp.embedding)::vector 
		         FROM event_sources es 
		         JOIN raw_posts rp ON es.raw_post_id = rp.id 
		         WHERE es.event_id = $1 AND rp.embedding IS NOT NULL),
		        embedding_centroid
		    )
		WHERE id = $1
	`
	if _, err := tx.ExecContext(ctx, updateEventQuery, eventID, post.PostedAt); err != nil {
		return fmt.Errorf("update news_events: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

// MarkPostNeedsReview updates a raw_post's normalized text, simhash, embedding,
// and sets processing_status = 'needs_review' without attaching or creating an event.
func (s *Store) MarkPostNeedsReview(
	ctx context.Context,
	postID int64,
	simhash int64,
	normalizedText string,
	embedding []float32,
) error {
	var vecParam sql.NullString
	if len(embedding) > 0 {
		vecParam = sql.NullString{String: FormatPgVector(embedding), Valid: true}
	}

	query := `
		UPDATE raw_posts
		SET normalized_text = $1, simhash = $2, embedding = $3::vector, processing_status = 'needs_review'
		WHERE id = $4
	`
	_, err := s.db.ExecContext(ctx, query, normalizedText, simhash, vecParam, postID)
	if err != nil {
		return fmt.Errorf("mark post needs_review: %w", err)
	}
	return nil
}
