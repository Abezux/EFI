package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

	// Broadcast notification on news_events_channel within the same transaction
	notifyPayload := fmt.Sprintf(`{"type":"new_event","event_id":%d}`, eventID)
	if _, err := tx.ExecContext(ctx, "SELECT pg_notify('news_events_channel', $1)", notifyPayload); err != nil {
		return 0, fmt.Errorf("notify news_events_channel: %w", err)
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

	// Broadcast notification on news_events_channel within the same transaction
	notifyPayload := fmt.Sprintf(`{"type":"event_updated","event_id":%d}`, eventID)
	if _, err := tx.ExecContext(ctx, "SELECT pg_notify('news_events_channel', $1)", notifyPayload); err != nil {
		return fmt.Errorf("notify news_events_channel: %w", err)
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

// AuditEntry captures metadata for the processing_audit table.
type AuditEntry struct {
	RawPostID   *int64
	NewsEventID *int64
	Stage       string // 'verification' | 'enrichment'
	Decision    string // 'same_event' | 'different_event' | 'low_confidence_unresolved' | 'summary_generated'
	Confidence  float32
	ModelUsed   string
	RawResponse string
}

// StableEvent represents an active event ready for AI summary and enrichment.
type StableEvent struct {
	ID             int64
	CanonicalTitle string
	LastUpdatedAt  time.Time
	LastEnrichedAt *time.Time
}

// FetchNeedsReviewPosts retrieves raw_posts with processing_status = 'needs_review'.
func (s *Store) FetchNeedsReviewPosts(ctx context.Context, limit int) ([]*RawPost, error) {
	query := `
		SELECT id, channel_id, telegram_message_id, raw_text, posted_at, COALESCE(normalized_text, ''), COALESCE(simhash, 0)
		FROM raw_posts
		WHERE processing_status = 'needs_review'
		ORDER BY posted_at ASC, id ASC
		LIMIT $1
	`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query needs_review posts: %w", err)
	}
	defer rows.Close()

	var posts []*RawPost
	for rows.Next() {
		p := &RawPost{}
		var normText string
		var simhash int64
		if err := rows.Scan(&p.ID, &p.ChannelID, &p.TelegramMessageID, &p.RawText, &p.PostedAt, &normText, &simhash); err != nil {
			return nil, fmt.Errorf("scan needs_review post: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return posts, nil
}

// FindNearestCandidateEvent finds the closest event within the clustering window using vector cosine distance.
func (s *Store) FindNearestCandidateEvent(ctx context.Context, postID int64, window time.Duration) (*EventCandidate, float32, error) {
	windowInterval := fmt.Sprintf("%d seconds", int(window.Seconds()))
	query := `
		SELECT ne.id, ne.canonical_title, ne.first_seen_at, ne.last_updated_at,
		       COALESCE(ne.embedding_centroid::text, ''),
		       1 - (ne.embedding_centroid <=> rp.embedding) AS cosine_sim
		FROM news_events ne, raw_posts rp
		WHERE rp.id = $1
		  AND rp.embedding IS NOT NULL
		  AND ne.embedding_centroid IS NOT NULL
		  AND ne.status = 'active'
		  AND ne.last_updated_at >= NOW() - $2::interval
		ORDER BY ne.embedding_centroid <=> rp.embedding ASC
		LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, query, postID, windowInterval)
	c := &EventCandidate{Sources: []EventSourceMember{}}
	var centroidStr string
	var similarity float32
	err := row.Scan(&c.EventID, &c.CanonicalTitle, &c.FirstSeenAt, &c.LastUpdatedAt, &centroidStr, &similarity)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("query nearest event: %w", err)
	}

	if centroidStr != "" {
		vec, err := ParsePgVector(centroidStr)
		if err == nil {
			c.EmbeddingCentroid = vec
		}
	}

	// Fetch source texts for candidate
	srcRows, err := s.db.QueryContext(ctx, `
		SELECT rp.id, COALESCE(rp.normalized_text, ''), COALESCE(rp.simhash, 0)
		FROM event_sources es
		JOIN raw_posts rp ON es.raw_post_id = rp.id
		WHERE es.event_id = $1
	`, c.EventID)
	if err == nil {
		defer srcRows.Close()
		for srcRows.Next() {
			var m EventSourceMember
			if err := srcRows.Scan(&m.RawPostID, &m.NormalizedText, &m.Simhash); err == nil {
				c.Sources = append(c.Sources, m)
			}
		}
	}

	return c, similarity, nil
}

// AttachToEventWithAudit transactionally attaches a post to an event, updates centroid, marks status as 'processed',
// and writes an audit row.
func (s *Store) AttachToEventWithAudit(
	ctx context.Context,
	eventID int64,
	postID int64,
	postedAt time.Time,
	audit AuditEntry,
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
	simScore := int(audit.Confidence * 100)
	if _, err := tx.ExecContext(ctx, sourceQuery, eventID, postID, simScore); err != nil {
		return fmt.Errorf("insert event_sources: %w", err)
	}

	updatePostQuery := `
		UPDATE raw_posts
		SET processing_status = 'processed'
		WHERE id = $1
	`
	if _, err := tx.ExecContext(ctx, updatePostQuery, postID); err != nil {
		return fmt.Errorf("update raw_posts: %w", err)
	}

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
	if _, err := tx.ExecContext(ctx, updateEventQuery, eventID, postedAt); err != nil {
		return fmt.Errorf("update news_events: %w", err)
	}

	auditQuery := `
		INSERT INTO processing_audit (raw_post_id, news_event_id, stage, decision, confidence, model_used, raw_response, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`
	if _, err := tx.ExecContext(ctx, auditQuery, audit.RawPostID, eventID, audit.Stage, audit.Decision, audit.Confidence, audit.ModelUsed, audit.RawResponse); err != nil {
		return fmt.Errorf("insert processing_audit: %w", err)
	}

	// Broadcast notification on news_events_channel within the same transaction
	notifyPayload := fmt.Sprintf(`{"type":"event_updated","event_id":%d}`, eventID)
	if _, err := tx.ExecContext(ctx, "SELECT pg_notify('news_events_channel', $1)", notifyPayload); err != nil {
		return fmt.Errorf("notify news_events_channel: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

// CreateEventWithAudit transactionally creates a new news_events row, links the founding post,
// updates post status to 'processed', and writes the audit row.
func (s *Store) CreateEventWithAudit(
	ctx context.Context,
	post *RawPost,
	canonicalTitle string,
	audit AuditEntry,
) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // nolint:errcheck

	var eventID int64
	eventQuery := `
		INSERT INTO news_events (canonical_title, status, first_seen_at, last_updated_at, source_count, embedding_centroid)
		VALUES ($1, 'active', $2, $2, 1, (SELECT embedding FROM raw_posts WHERE id = $3))
		RETURNING id
	`
	if err := tx.QueryRowContext(ctx, eventQuery, canonicalTitle, post.PostedAt, post.ID).Scan(&eventID); err != nil {
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
		SET processing_status = 'processed'
		WHERE id = $1
	`
	if _, err := tx.ExecContext(ctx, updatePostQuery, post.ID); err != nil {
		return 0, fmt.Errorf("update raw_posts: %w", err)
	}

	auditQuery := `
		INSERT INTO processing_audit (raw_post_id, news_event_id, stage, decision, confidence, model_used, raw_response, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`
	if _, err := tx.ExecContext(ctx, auditQuery, audit.RawPostID, eventID, audit.Stage, audit.Decision, audit.Confidence, audit.ModelUsed, audit.RawResponse); err != nil {
		return 0, fmt.Errorf("insert processing_audit: %w", err)
	}

	// Broadcast notification on news_events_channel within the same transaction
	notifyPayload := fmt.Sprintf(`{"type":"new_event","event_id":%d}`, eventID)
	if _, err := tx.ExecContext(ctx, "SELECT pg_notify('news_events_channel', $1)", notifyPayload); err != nil {
		return 0, fmt.Errorf("notify news_events_channel: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return eventID, nil
}

// RecordAudit writes a standalone row to processing_audit (e.g. for low_confidence_unresolved).
func (s *Store) RecordAudit(ctx context.Context, audit AuditEntry) error {
	auditQuery := `
		INSERT INTO processing_audit (raw_post_id, news_event_id, stage, decision, confidence, model_used, raw_response, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`
	_, err := s.db.ExecContext(ctx, auditQuery, audit.RawPostID, audit.NewsEventID, audit.Stage, audit.Decision, audit.Confidence, audit.ModelUsed, audit.RawResponse)
	if err != nil {
		return fmt.Errorf("insert processing_audit: %w", err)
	}
	return nil
}

// FetchStableEventsForEnrichment retrieves active news_events whose last source addition was at least
// stabilityWindow ago, and either have not yet been enriched or gained newer sources since last enrichment.
func (s *Store) FetchStableEventsForEnrichment(ctx context.Context, stabilityWindow time.Duration, limit int) ([]*StableEvent, error) {
	windowInterval := fmt.Sprintf("%d seconds", int(stabilityWindow.Seconds()))
	query := `
		SELECT id, canonical_title, last_updated_at, last_enriched_at
		FROM news_events
		WHERE status = 'active'
		  AND last_updated_at <= NOW() - $1::interval
		  AND (last_enriched_at IS NULL OR last_updated_at > last_enriched_at)
		ORDER BY last_updated_at ASC
		LIMIT $2
	`
	rows, err := s.db.QueryContext(ctx, query, windowInterval, limit)
	if err != nil {
		return nil, fmt.Errorf("query stable events: %w", err)
	}
	defer rows.Close()

	var events []*StableEvent
	for rows.Next() {
		e := &StableEvent{}
		var enrichedAt sql.NullTime
		if err := rows.Scan(&e.ID, &e.CanonicalTitle, &e.LastUpdatedAt, &enrichedAt); err != nil {
			return nil, fmt.Errorf("scan stable event: %w", err)
		}
		if enrichedAt.Valid {
			e.LastEnrichedAt = &enrichedAt.Time
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return events, nil
}

// FetchEventSourceTexts retrieves unnormalized raw texts from raw_posts attached to the given news_event,
// formatted with channel attribution metadata so direct quotes can be preserved and attributed.
func (s *Store) FetchEventSourceTexts(ctx context.Context, eventID int64) ([]string, error) {
	query := `
		SELECT COALESCE(c.name, ''), COALESCE(c.handle, ''), rp.raw_text
		FROM event_sources es
		JOIN raw_posts rp ON es.raw_post_id = rp.id
		LEFT JOIN channels c ON rp.channel_id = c.id
		WHERE es.event_id = $1
		ORDER BY es.added_at ASC
	`
	rows, err := s.db.QueryContext(ctx, query, eventID)
	if err != nil {
		return nil, fmt.Errorf("query event source texts: %w", err)
	}
	defer rows.Close()

	var texts []string
	idx := 1
	for rows.Next() {
		var name, handle, rawText string
		if err := rows.Scan(&name, &handle, &rawText); err != nil {
			return nil, fmt.Errorf("scan source text: %w", err)
		}
		rawText = strings.TrimSpace(rawText)
		if rawText == "" {
			continue
		}

		var header string
		if name != "" {
			if handle != "" {
				header = fmt.Sprintf("[Source %d — %s (@%s)]", idx, name, handle)
			} else {
				header = fmt.Sprintf("[Source %d — %s]", idx, name)
			}
		} else {
			header = fmt.Sprintf("[Source %d]", idx)
		}
		texts = append(texts, fmt.Sprintf("%s\n%s", header, rawText))
		idx++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return texts, nil
}

// FetchCategories returns a map of category name -> id and a slice of valid category names.
func (s *Store) FetchCategories(ctx context.Context) (map[string]int, []string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name FROM categories ORDER BY id ASC")
	if err != nil {
		return nil, nil, fmt.Errorf("query categories: %w", err)
	}
	defer rows.Close()

	catMap := make(map[string]int)
	var catNames []string
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, fmt.Errorf("scan category: %w", err)
		}
		catMap[name] = id
		catNames = append(catNames, name)
	}
	return catMap, catNames, nil
}

// SaveEnrichmentWithAudit transactionally writes ai_headline, ai_summary, category_id, extracted entities,
// updates last_enriched_at, and writes a processing_audit record.
func (s *Store) SaveEnrichmentWithAudit(
	ctx context.Context,
	eventID int64,
	categoryID *int,
	aiHeadline string,
	aiSummary string,
	entities []ExtractedEntity,
	audit AuditEntry,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // nolint:errcheck

	var headlineParam sql.NullString
	if trimmed := strings.TrimSpace(aiHeadline); trimmed != "" {
		headlineParam = sql.NullString{String: trimmed, Valid: true}
	}

	updateEventQuery := `
		UPDATE news_events
		SET ai_headline = $1,
		    ai_summary = $2,
		    category_id = $3,
		    last_enriched_at = NOW()
		WHERE id = $4
	`
	if _, err := tx.ExecContext(ctx, updateEventQuery, headlineParam, aiSummary, categoryID, eventID); err != nil {
		return fmt.Errorf("update news_events enrichment: %w", err)
	}

	// Insert or find entities, then link in event_entities
	for _, ent := range entities {
		if strings.TrimSpace(ent.Name) == "" {
			continue
		}
		entType := strings.ToLower(strings.TrimSpace(ent.Type))
		if entType != "person" && entType != "place" && entType != "organization" {
			entType = "organization"
		}

		var entityID int64
		upsertEntityQuery := `
			INSERT INTO entities (name, type)
			VALUES ($1, $2)
			ON CONFLICT (name, type) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`
		if err := tx.QueryRowContext(ctx, upsertEntityQuery, strings.TrimSpace(ent.Name), entType).Scan(&entityID); err != nil {
			return fmt.Errorf("upsert entity %q: %w", ent.Name, err)
		}

		linkQuery := `
			INSERT INTO event_entities (event_id, entity_id)
			VALUES ($1, $2)
			ON CONFLICT (event_id, entity_id) DO NOTHING
		`
		if _, err := tx.ExecContext(ctx, linkQuery, eventID, entityID); err != nil {
			return fmt.Errorf("link event entity: %w", err)
		}
	}

	auditQuery := `
		INSERT INTO processing_audit (raw_post_id, news_event_id, stage, decision, confidence, model_used, raw_response, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`
	if _, err := tx.ExecContext(ctx, auditQuery, audit.RawPostID, eventID, audit.Stage, audit.Decision, audit.Confidence, audit.ModelUsed, audit.RawResponse); err != nil {
		return fmt.Errorf("insert processing_audit: %w", err)
	}

	// Broadcast notification on news_events_channel within the same transaction
	notifyPayload := fmt.Sprintf(`{"type":"event_updated","event_id":%d}`, eventID)
	if _, err := tx.ExecContext(ctx, "SELECT pg_notify('news_events_channel', $1)", notifyPayload); err != nil {
		return fmt.Errorf("notify news_events_channel: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
