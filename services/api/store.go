package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/lib/pq"
)

var (
	ErrNotFound = errors.New("resource not found")
)

// CategoryInfo represents a category attached to an event.
type CategoryInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// CategoryWithCount represents a category with active event count for /api/v1/categories.
type CategoryWithCount struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	EventCount int    `json:"event_count"`
}

// EntityInfo represents a named entity attached to an event.
type EntityInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// SourceDetail represents a source attribution item in event detail.
type SourceDetail struct {
	ChannelName   string    `json:"channel_name"`
	ChannelHandle string    `json:"channel_handle,omitempty"`
	PostedAt      time.Time `json:"posted_at"`
	Excerpt       string    `json:"excerpt"`
}

// EventSummary represents an event in the list or search response.
type EventSummary struct {
	ID                 int64         `json:"id"`
	CanonicalTitle     string        `json:"canonical_title"`
	AISummary          string        `json:"ai_summary"`
	AISummaryGenerated bool          `json:"ai_summary_generated"`
	Category           *CategoryInfo `json:"category,omitempty"`
	Entities           []EntityInfo  `json:"entities"`
	SourceCount        int           `json:"source_count"`
	FirstSeenAt        time.Time     `json:"first_seen_at"`
	LastUpdatedAt      time.Time     `json:"last_updated_at"`
}

// EventDetail represents the full details of an event for /api/v1/events/{id}.
type EventDetail struct {
	ID                 int64          `json:"id"`
	CanonicalTitle     string         `json:"canonical_title"`
	AISummary          string         `json:"ai_summary"`
	AISummaryGenerated bool           `json:"ai_summary_generated"`
	Category           *CategoryInfo  `json:"category,omitempty"`
	Entities           []EntityInfo   `json:"entities"`
	Sources            []SourceDetail `json:"sources"`
	SourceCount        int            `json:"source_count"`
	FirstSeenAt        time.Time      `json:"first_seen_at"`
	LastUpdatedAt      time.Time      `json:"last_updated_at"`
}

// EventFilter specifies filtering and pagination parameters for listing events.
type EventFilter struct {
	CategorySlug string
	Since        *time.Time
	Limit        int
	Offset       int
}

// EventListResult represents the paginated result of events.
type EventListResult struct {
	Events []EventSummary `json:"events"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// StoreReader defines the read-only persistence interface for the API service.
type StoreReader interface {
	GetEvents(ctx context.Context, filter EventFilter) (*EventListResult, error)
	GetEventByID(ctx context.Context, id int64) (*EventDetail, error)
	GetCategories(ctx context.Context) ([]CategoryWithCount, error)
	SearchEvents(ctx context.Context, query string, limit, offset int) (*EventListResult, error)
	Ping(ctx context.Context) error
	Close() error
}

// SQLStore implements StoreReader against PostgreSQL using least-privilege efi_api role.
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore creates a new SQLStore with configured connection pool.
func NewSQLStore(dbURL string) (*SQLStore, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	return &SQLStore{db: db}, nil
}

// Ping checks database connectivity.
func (s *SQLStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database connection pool.
func (s *SQLStore) Close() error {
	return s.db.Close()
}

// FormatExcerpt safely truncates text to a maximum number of runes, preserving UTF-8 integrity.
func FormatExcerpt(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if maxRunes <= 0 {
		maxRunes = 160
	}

	runeCount := utf8.RuneCountInString(text)
	if runeCount <= maxRunes {
		return text
	}

	runes := []rune(text)
	truncated := string(runes[:maxRunes])

	// Attempt to cut at last whitespace boundary if available
	lastSpace := strings.LastIndexAny(truncated, " \t\n\r")
	if lastSpace > maxRunes/2 {
		truncated = strings.TrimSpace(truncated[:lastSpace])
	} else {
		truncated = strings.TrimSpace(truncated)
	}

	return truncated + "..."
}

// GetEvents retrieves a paginated list of active published events with optional filtering.
func (s *SQLStore) GetEvents(ctx context.Context, filter EventFilter) (*EventListResult, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var whereClauses []string
	var args []any
	argIdx := 1

	// Strict filter: ONLY active events are returned (never needs_review)
	whereClauses = append(whereClauses, "ne.status = 'active'")

	if filter.CategorySlug != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("c.slug = $%d", argIdx))
		args = append(args, filter.CategorySlug)
		argIdx++
	}

	if filter.Since != nil && !filter.Since.IsZero() {
		whereClauses = append(whereClauses, fmt.Sprintf("ne.last_updated_at > $%d", argIdx))
		args = append(args, *filter.Since)
		argIdx++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// 1. Get total count
	countQuery := fmt.Sprintf(`
		SELECT count(*)
		FROM news_events ne
		LEFT JOIN categories c ON ne.category_id = c.id
		WHERE %s
	`, whereSQL)

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count events: %w", err)
	}

	if total == 0 {
		return &EventListResult{
			Events: []EventSummary{},
			Total:  0,
			Limit:  limit,
			Offset: offset,
		}, nil
	}

	// 2. Query paginated events
	eventsQuery := fmt.Sprintf(`
		SELECT 
			ne.id,
			ne.canonical_title,
			coalesce(ne.ai_summary, ''),
			ne.source_count,
			ne.first_seen_at,
			ne.last_updated_at,
			c.id,
			c.name,
			c.slug
		FROM news_events ne
		LEFT JOIN categories c ON ne.category_id = c.id
		WHERE %s
		ORDER BY ne.last_updated_at DESC, ne.id DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, argIdx, argIdx+1)

	queryArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, eventsQuery, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []EventSummary
	var eventIDs []int64

	for rows.Next() {
		var (
			ev           EventSummary
			catID        sql.NullInt64
			catName      sql.NullString
			catSlug      sql.NullString
			aiSummaryStr string
		)

		if err := rows.Scan(
			&ev.ID,
			&ev.CanonicalTitle,
			&aiSummaryStr,
			&ev.SourceCount,
			&ev.FirstSeenAt,
			&ev.LastUpdatedAt,
			&catID,
			&catName,
			&catSlug,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		ev.AISummary = aiSummaryStr
		ev.AISummaryGenerated = true // Explicit transparency flag
		if catID.Valid {
			ev.Category = &CategoryInfo{
				ID:   catID.Int64,
				Name: catName.String,
				Slug: catSlug.String,
			}
		}
		ev.Entities = []EntityInfo{} // Initialized empty slice

		events = append(events, ev)
		eventIDs = append(eventIDs, ev.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}

	// 3. Batch load entities for retrieved events
	if len(eventIDs) > 0 {
		entitiesMap, err := s.getEntitiesForEvents(ctx, eventIDs)
		if err != nil {
			return nil, fmt.Errorf("get entities for events: %w", err)
		}
		for i := range events {
			if ents, ok := entitiesMap[events[i].ID]; ok {
				events[i].Entities = ents
			}
		}
	}

	return &EventListResult{
		Events: events,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// GetEventByID retrieves full details of a single active event including sources and entities.
func (s *SQLStore) GetEventByID(ctx context.Context, id int64) (*EventDetail, error) {
	// Base filter: MUST be status = 'active'
	query := `
		SELECT 
			ne.id,
			ne.canonical_title,
			coalesce(ne.ai_summary, ''),
			ne.source_count,
			ne.first_seen_at,
			ne.last_updated_at,
			c.id,
			c.name,
			c.slug
		FROM news_events ne
		LEFT JOIN categories c ON ne.category_id = c.id
		WHERE ne.id = $1 AND ne.status = 'active'
	`

	var (
		ev           EventDetail
		catID        sql.NullInt64
		catName      sql.NullString
		catSlug      sql.NullString
		aiSummaryStr string
	)

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&ev.ID,
		&ev.CanonicalTitle,
		&aiSummaryStr,
		&ev.SourceCount,
		&ev.FirstSeenAt,
		&ev.LastUpdatedAt,
		&catID,
		&catName,
		&catSlug,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query event by id: %w", err)
	}

	ev.AISummary = aiSummaryStr
	ev.AISummaryGenerated = true // Explicit transparency flag
	if catID.Valid {
		ev.Category = &CategoryInfo{
			ID:   catID.Int64,
			Name: catName.String,
			Slug: catSlug.String,
		}
	}
	ev.Entities = []EntityInfo{}
	ev.Sources = []SourceDetail{}

	// Load entities for this event
	entitiesMap, err := s.getEntitiesForEvents(ctx, []int64{ev.ID})
	if err != nil {
		return nil, fmt.Errorf("load entities: %w", err)
	}
	if ents, ok := entitiesMap[ev.ID]; ok {
		ev.Entities = ents
	}

	// Load sources with bounded excerpts
	sourcesQuery := `
		SELECT 
			ch.name,
			coalesce(ch.handle, ''),
			rp.posted_at,
			coalesce(rp.normalized_text, rp.raw_text)
		FROM event_sources es
		JOIN raw_posts rp ON es.raw_post_id = rp.id
		JOIN channels ch ON rp.channel_id = ch.id
		WHERE es.event_id = $1
		ORDER BY rp.posted_at ASC, es.raw_post_id ASC
	`
	sRows, err := s.db.QueryContext(ctx, sourcesQuery, ev.ID)
	if err != nil {
		return nil, fmt.Errorf("load sources: %w", err)
	}
	defer sRows.Close()

	for sRows.Next() {
		var (
			src     SourceDetail
			handle  string
			rawBody string
		)
		if err := sRows.Scan(&src.ChannelName, &handle, &src.PostedAt, &rawBody); err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		src.ChannelHandle = handle
		src.Excerpt = FormatExcerpt(rawBody, 160) // Bounded excerpt for copyright/attribution safety
		ev.Sources = append(ev.Sources, src)
	}
	if err := sRows.Err(); err != nil {
		return nil, fmt.Errorf("sources rows err: %w", err)
	}

	return &ev, nil
}

// GetCategories retrieves all categories with active event count.
func (s *SQLStore) GetCategories(ctx context.Context) ([]CategoryWithCount, error) {
	query := `
		SELECT 
			c.id,
			c.name,
			c.slug,
			count(ne.id) FILTER (WHERE ne.status = 'active') as event_count
		FROM categories c
		LEFT JOIN news_events ne ON ne.category_id = c.id
		GROUP BY c.id, c.name, c.slug
		ORDER BY c.id ASC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query categories: %w", err)
	}
	defer rows.Close()

	var categories []CategoryWithCount
	for rows.Next() {
		var c CategoryWithCount
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.EventCount); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("categories rows err: %w", err)
	}

	return categories, nil
}

// SearchEvents performs full-text search over active news events.
func (s *SQLStore) SearchEvents(ctx context.Context, query string, limit, offset int) (*EventListResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return &EventListResult{
			Events: []EventSummary{},
			Total:  0,
			Limit:  limit,
			Offset: offset,
		}, nil
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// Multi-mode full-text search:
	// Matches against tsvector using 'simple' configuration (handles Amharic & English),
	// with ILIKE fallback for exact substrings.
	countQuery := `
		SELECT count(*)
		FROM news_events ne
		LEFT JOIN categories c ON ne.category_id = c.id
		WHERE ne.status = 'active'
		  AND (
		    to_tsvector('simple', ne.canonical_title || ' ' || coalesce(ne.ai_summary, '')) @@ plainto_tsquery('simple', $1)
		    OR ne.canonical_title ILIKE '%' || $1 || '%'
		    OR coalesce(ne.ai_summary, '') ILIKE '%' || $1 || '%'
		  )
	`

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, query).Scan(&total); err != nil {
		return nil, fmt.Errorf("count search results: %w", err)
	}

	if total == 0 {
		return &EventListResult{
			Events: []EventSummary{},
			Total:  0,
			Limit:  limit,
			Offset: offset,
		}, nil
	}

	searchQuery := `
		SELECT 
			ne.id,
			ne.canonical_title,
			coalesce(ne.ai_summary, ''),
			ne.source_count,
			ne.first_seen_at,
			ne.last_updated_at,
			c.id,
			c.name,
			c.slug
		FROM news_events ne
		LEFT JOIN categories c ON ne.category_id = c.id
		WHERE ne.status = 'active'
		  AND (
		    to_tsvector('simple', ne.canonical_title || ' ' || coalesce(ne.ai_summary, '')) @@ plainto_tsquery('simple', $1)
		    OR ne.canonical_title ILIKE '%' || $1 || '%'
		    OR coalesce(ne.ai_summary, '') ILIKE '%' || $1 || '%'
		  )
		ORDER BY 
		  ts_rank_cd(to_tsvector('simple', ne.canonical_title || ' ' || coalesce(ne.ai_summary, '')), plainto_tsquery('simple', $1)) DESC,
		  ne.last_updated_at DESC, 
		  ne.id DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(ctx, searchQuery, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query search results: %w", err)
	}
	defer rows.Close()

	var events []EventSummary
	var eventIDs []int64

	for rows.Next() {
		var (
			ev           EventSummary
			catID        sql.NullInt64
			catName      sql.NullString
			catSlug      sql.NullString
			aiSummaryStr string
		)

		if err := rows.Scan(
			&ev.ID,
			&ev.CanonicalTitle,
			&aiSummaryStr,
			&ev.SourceCount,
			&ev.FirstSeenAt,
			&ev.LastUpdatedAt,
			&catID,
			&catName,
			&catSlug,
		); err != nil {
			return nil, fmt.Errorf("scan search event: %w", err)
		}

		ev.AISummary = aiSummaryStr
		ev.AISummaryGenerated = true
		if catID.Valid {
			ev.Category = &CategoryInfo{
				ID:   catID.Int64,
				Name: catName.String,
				Slug: catSlug.String,
			}
		}
		ev.Entities = []EntityInfo{}

		events = append(events, ev)
		eventIDs = append(eventIDs, ev.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search rows err: %w", err)
	}

	if len(eventIDs) > 0 {
		entitiesMap, err := s.getEntitiesForEvents(ctx, eventIDs)
		if err != nil {
			return nil, fmt.Errorf("get entities for search results: %w", err)
		}
		for i := range events {
			if ents, ok := entitiesMap[events[i].ID]; ok {
				events[i].Entities = ents
			}
		}
	}

	return &EventListResult{
		Events: events,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (s *SQLStore) getEntitiesForEvents(ctx context.Context, eventIDs []int64) (map[int64][]EntityInfo, error) {
	result := make(map[int64][]EntityInfo)
	if len(eventIDs) == 0 {
		return result, nil
	}

	// Format IDs for ANY($1) query
	idStrs := make([]string, len(eventIDs))
	for i, id := range eventIDs {
		idStrs[i] = fmt.Sprintf("%d", id)
	}
	arrayLiteral := fmt.Sprintf("{%s}", strings.Join(idStrs, ","))

	query := `
		SELECT 
			ee.event_id,
			e.id,
			e.name,
			e.type
		FROM event_entities ee
		JOIN entities e ON ee.entity_id = e.id
		WHERE ee.event_id = ANY($1::bigint[])
		ORDER BY e.name ASC
	`

	rows, err := s.db.QueryContext(ctx, query, arrayLiteral)
	if err != nil {
		return nil, fmt.Errorf("query entities: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			eventID int64
			ent     EntityInfo
		)
		if err := rows.Scan(&eventID, &ent.ID, &ent.Name, &ent.Type); err != nil {
			return nil, fmt.Errorf("scan entity: %w", err)
		}
		result[eventID] = append(result[eventID], ent)
	}

	return result, rows.Err()
}
