package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AdminUserWithHash represents an administrative user with their bcrypt hash for auth verification.
type AdminUserWithHash struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// AdminChannel represents channel metadata with ingestion statistics for the admin dashboard.
type AdminChannel struct {
	ID                int64      `json:"id"`
	TelegramChannelID int64      `json:"telegram_channel_id"`
	Name              string     `json:"name"`
	Handle            *string    `json:"handle,omitempty"`
	IsActive          bool       `json:"is_active"`
	AddedAt           time.Time  `json:"added_at"`
	LastSeenAt        *time.Time `json:"last_seen_at,omitempty"`
	PostCount         int        `json:"post_count"`
}

// AdminEventSummary represents an event summary with moderation status for admin browsing.
type AdminEventSummary struct {
	ID                   int64         `json:"id"`
	CanonicalTitle       string        `json:"canonical_title"`
	Slug                 string        `json:"slug"`
	AIHeadline           *string       `json:"ai_headline,omitempty"`
	AISummary            string        `json:"ai_summary"`
	Status               string        `json:"status"` // "active", "needs_review"
	IsHidden             bool          `json:"is_hidden"`
	SourceCount          int           `json:"source_count"`
	FirstSeenAt          time.Time     `json:"first_seen_at"`
	LastUpdatedAt        time.Time     `json:"last_updated_at"`
	Category             *CategoryInfo `json:"category,omitempty"`
	LastModerationReason *string       `json:"last_moderation_reason,omitempty"`
	LastModeratedAt      *time.Time    `json:"last_moderated_at,omitempty"`
}

// AdminEventSource represents a source report attached to an event.
type AdminEventSource struct {
	EventID           int64     `json:"event_id"`
	RawPostID         int64     `json:"raw_post_id"`
	ChannelID         int64     `json:"channel_id"`
	ChannelName       string    `json:"channel_name"`
	ChannelHandle     *string   `json:"channel_handle,omitempty"`
	TelegramMessageID int64     `json:"telegram_message_id"`
	RawText           string    `json:"raw_text"`
	PostedAt          time.Time `json:"posted_at"`
	AttachedAt        time.Time `json:"attached_at"`
}

// ModerationActionRecord represents an entry in the moderation audit trail.
type ModerationActionRecord struct {
	ID          int64     `json:"id"`
	ActorUserID int64     `json:"actor_user_id"`
	ActorEmail  string    `json:"actor_email"`
	ActionType  string    `json:"action_type"`
	TargetType  string    `json:"target_type"`
	TargetID    int64     `json:"target_id"`
	Reason      *string   `json:"reason,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// AdminEventDetail represents the complete detail of an event for the admin interface.
type AdminEventDetail struct {
	ID                int64                    `json:"id"`
	CanonicalTitle    string                   `json:"canonical_title"`
	Slug              string                   `json:"slug"`
	AIHeadline        *string                  `json:"ai_headline,omitempty"`
	AISummary         string                   `json:"ai_summary"`
	Status            string                   `json:"status"`
	IsHidden          bool                     `json:"is_hidden"`
	SourceCount       int                      `json:"source_count"`
	FirstSeenAt       time.Time                `json:"first_seen_at"`
	LastUpdatedAt     time.Time                `json:"last_updated_at"`
	Category          *CategoryInfo            `json:"category,omitempty"`
	Sources           []AdminEventSource       `json:"sources"`
	Entities          []EntityInfo             `json:"entities"`
	ModerationHistory []ModerationActionRecord `json:"moderation_history"`
}

// NeedsReviewPost represents an ambiguous post in the manual review queue.
type NeedsReviewPost struct {
	RawPostID           int64      `json:"raw_post_id"`
	ChannelID           int64      `json:"channel_id"`
	ChannelName         string     `json:"channel_name"`
	ChannelHandle       *string    `json:"channel_handle,omitempty"`
	TelegramMessageID   int64      `json:"telegram_message_id"`
	RawText             string     `json:"raw_text"`
	PostedAt            time.Time  `json:"posted_at"`
	IngestedAt          time.Time  `json:"ingested_at"`
	CandidateEventID    *int64     `json:"candidate_event_id,omitempty"`
	CandidateEventTitle *string    `json:"candidate_event_title,omitempty"`
	AIRunReason         *string    `json:"ai_run_reason,omitempty"`
	AuditCreatedAt      *time.Time `json:"audit_created_at,omitempty"`
}

// ReviewQueueResult represents the paginated result of the review queue.
type ReviewQueueResult struct {
	Posts  []NeedsReviewPost `json:"posts"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

// AdminStore defines all administrative database queries and mutations.
type AdminStore interface {
	StoreReader
	// Users & Auth
	GetUserByEmail(ctx context.Context, email string) (*AdminUserWithHash, error)
	GetUserByID(ctx context.Context, id int64) (*AdminUser, error)

	// Channels
	ListAdminChannels(ctx context.Context) ([]AdminChannel, error)
	AddChannel(ctx context.Context, telegramChannelID int64, name, handle string, actorUserID int64, reason string) (*AdminChannel, error)
	ToggleChannel(ctx context.Context, channelID int64, actorUserID int64, reason string) (*AdminChannel, error)

	// Events & Moderation
	ListAdminEvents(ctx context.Context, limit, offset int, statusFilter string, hiddenFilter *bool) ([]AdminEventSummary, int, error)
	GetAdminEventByID(ctx context.Context, id int64) (*AdminEventDetail, error)
	HideEvent(ctx context.Context, eventID int64, actorUserID int64, reason string) error
	RestoreEvent(ctx context.Context, eventID int64, actorUserID int64, reason string) error
	DetachSource(ctx context.Context, eventID int64, rawPostID int64, actorUserID int64, reason string) error
	GetModerationHistory(ctx context.Context, targetType string, targetID int64) ([]ModerationActionRecord, error)

	// Review Queue
	ListNeedsReviewPosts(ctx context.Context, limit, offset int) (*ReviewQueueResult, error)
	ResolveNeedsReviewPost(ctx context.Context, rawPostID int64, decision string, targetEventID *int64, actorEmail string, actorUserID int64, reason string) error
}

// GetUserByEmail retrieves a user and their password hash for authentication.
func (s *SQLStore) GetUserByEmail(ctx context.Context, email string) (*AdminUserWithHash, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	query := `
		SELECT id, email, password_hash, role, created_at
		FROM users
		WHERE email = $1
	`
	var u AdminUserWithHash
	err := s.db.QueryRowContext(ctx, query, email).Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.Role,
		&u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

// GetUserByID retrieves a user by their primary key ID.
func (s *SQLStore) GetUserByID(ctx context.Context, id int64) (*AdminUser, error) {
	query := `
		SELECT id, email, role, created_at
		FROM users
		WHERE id = $1
	`
	var u AdminUser
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID,
		&u.Email,
		&u.Role,
		&u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// ListAdminChannels retrieves all registered channels with their ingestion statistics.
func (s *SQLStore) ListAdminChannels(ctx context.Context) ([]AdminChannel, error) {
	query := `
		SELECT 
			c.id,
			c.telegram_channel_id,
			c.name,
			c.handle,
			c.is_active,
			c.added_at,
			c.last_seen_at,
			count(rp.id) as post_count
		FROM channels c
		LEFT JOIN raw_posts rp ON rp.channel_id = c.id
		GROUP BY c.id, c.telegram_channel_id, c.name, c.handle, c.is_active, c.added_at, c.last_seen_at
		ORDER BY c.is_active DESC, c.id ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list admin channels: %w", err)
	}
	defer rows.Close()

	var channels []AdminChannel
	for rows.Next() {
		var (
			ch         AdminChannel
			handleNull sql.NullString
			seenNull   sql.NullTime
		)
		if err := rows.Scan(
			&ch.ID,
			&ch.TelegramChannelID,
			&ch.Name,
			&handleNull,
			&ch.IsActive,
			&ch.AddedAt,
			&seenNull,
			&ch.PostCount,
		); err != nil {
			return nil, fmt.Errorf("scan admin channel: %w", err)
		}
		if handleNull.Valid {
			h := handleNull.String
			ch.Handle = &h
		}
		if seenNull.Valid {
			t := seenNull.Time
			ch.LastSeenAt = &t
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

// AddChannel registers a new channel or reactivates an existing one, logging the action.
func (s *SQLStore) AddChannel(ctx context.Context, telegramChannelID int64, name, handle string, actorUserID int64, reason string) (*AdminChannel, error) {
	name = strings.TrimSpace(name)
	handle = strings.TrimPrefix(strings.TrimSpace(handle), "@")

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO channels (telegram_channel_id, name, handle, is_active, added_at, last_seen_at)
		VALUES ($1, $2, NULLIF($3, ''), true, NOW(), NOW())
		ON CONFLICT (telegram_channel_id) DO UPDATE
		SET is_active = true,
		    name = EXCLUDED.name,
		    handle = COALESCE(EXCLUDED.handle, channels.handle)
		RETURNING id, telegram_channel_id, name, handle, is_active, added_at, last_seen_at
	`
	var (
		ch         AdminChannel
		handleNull sql.NullString
		seenNull   sql.NullTime
	)
	err = tx.QueryRowContext(ctx, query, telegramChannelID, name, handle).Scan(
		&ch.ID,
		&ch.TelegramChannelID,
		&ch.Name,
		&handleNull,
		&ch.IsActive,
		&ch.AddedAt,
		&seenNull,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert channel: %w", err)
	}
	if handleNull.Valid {
		h := handleNull.String
		ch.Handle = &h
	}
	if seenNull.Valid {
		t := seenNull.Time
		ch.LastSeenAt = &t
	}

	// Log moderation action
	auditQuery := `
		INSERT INTO moderation_actions (actor_user_id, action_type, target_type, target_id, reason, created_at)
		VALUES ($1, 'add_channel', 'channel', $2, NULLIF($3, ''), NOW())
	`
	if _, err := tx.ExecContext(ctx, auditQuery, actorUserID, ch.ID, reason); err != nil {
		return nil, fmt.Errorf("log moderation action: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &ch, nil
}

// ToggleChannel flips a channel's is_active status and logs the action to moderation_actions.
func (s *SQLStore) ToggleChannel(ctx context.Context, channelID int64, actorUserID int64, reason string) (*AdminChannel, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	query := `
		UPDATE channels
		SET is_active = NOT is_active
		WHERE id = $1
		RETURNING id, telegram_channel_id, name, handle, is_active, added_at, last_seen_at
	`
	var (
		ch         AdminChannel
		handleNull sql.NullString
		seenNull   sql.NullTime
	)
	err = tx.QueryRowContext(ctx, query, channelID).Scan(
		&ch.ID,
		&ch.TelegramChannelID,
		&ch.Name,
		&handleNull,
		&ch.IsActive,
		&ch.AddedAt,
		&seenNull,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("toggle channel: %w", err)
	}
	if handleNull.Valid {
		h := handleNull.String
		ch.Handle = &h
	}
	if seenNull.Valid {
		t := seenNull.Time
		ch.LastSeenAt = &t
	}

	auditQuery := `
		INSERT INTO moderation_actions (actor_user_id, action_type, target_type, target_id, reason, created_at)
		VALUES ($1, 'toggle_channel', 'channel', $2, NULLIF($3, ''), NOW())
	`
	if _, err := tx.ExecContext(ctx, auditQuery, actorUserID, channelID, reason); err != nil {
		return nil, fmt.Errorf("log moderation action: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &ch, nil
}

// ListAdminEvents retrieves a paginated list of events for administration, including hidden ones.
func (s *SQLStore) ListAdminEvents(ctx context.Context, limit, offset int, statusFilter string, hiddenFilter *bool) ([]AdminEventSummary, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var whereClauses []string
	var args []any
	argIdx := 1

	if statusFilter != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("ne.status = $%d", argIdx))
		args = append(args, statusFilter)
		argIdx++
	}

	if hiddenFilter != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("ne.is_hidden = $%d", argIdx))
		args = append(args, *hiddenFilter)
		argIdx++
	}

	whereSQL := "TRUE"
	if len(whereClauses) > 0 {
		whereSQL = strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf(`
		SELECT count(*)
		FROM news_events ne
		WHERE %s
	`, whereSQL)

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admin events: %w", err)
	}

	if total == 0 {
		return []AdminEventSummary{}, 0, nil
	}

	query := fmt.Sprintf(`
		SELECT 
			ne.id,
			ne.canonical_title,
			coalesce(ne.slug, ''),
			ne.ai_headline,
			coalesce(ne.ai_summary, ''),
			ne.status,
			ne.is_hidden,
			ne.source_count,
			ne.first_seen_at,
			ne.last_updated_at,
			c.id,
			c.name,
			c.slug,
			ma.reason,
			ma.created_at
		FROM news_events ne
		LEFT JOIN categories c ON ne.category_id = c.id
		LEFT JOIN LATERAL (
			SELECT reason, created_at
			FROM moderation_actions
			WHERE target_type = 'event' AND target_id = ne.id
			ORDER BY created_at DESC
			LIMIT 1
		) ma ON true
		WHERE %s
		ORDER BY ne.last_updated_at DESC, ne.id DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, argIdx, argIdx+1)

	queryArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin events: %w", err)
	}
	defer rows.Close()

	var events []AdminEventSummary
	for rows.Next() {
		var (
			ev         AdminEventSummary
			aiHeadline sql.NullString
			catID      sql.NullInt64
			catName    sql.NullString
			catSlug    sql.NullString
			modReason  sql.NullString
			modAt      sql.NullTime
		)
		if err := rows.Scan(
			&ev.ID,
			&ev.CanonicalTitle,
			&ev.Slug,
			&aiHeadline,
			&ev.AISummary,
			&ev.Status,
			&ev.IsHidden,
			&ev.SourceCount,
			&ev.FirstSeenAt,
			&ev.LastUpdatedAt,
			&catID,
			&catName,
			&catSlug,
			&modReason,
			&modAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan admin event: %w", err)
		}
		if aiHeadline.Valid && strings.TrimSpace(aiHeadline.String) != "" {
			h := aiHeadline.String
			ev.AIHeadline = &h
		}
		if catID.Valid {
			ev.Category = &CategoryInfo{
				ID:   catID.Int64,
				Name: catName.String,
				Slug: catSlug.String,
			}
		}
		if modReason.Valid {
			r := modReason.String
			ev.LastModerationReason = &r
		}
		if modAt.Valid {
			t := modAt.Time
			ev.LastModeratedAt = &t
		}
		events = append(events, ev)
	}

	return events, total, rows.Err()
}

// GetAdminEventByID retrieves full details of an event including all sources and moderation history.
func (s *SQLStore) GetAdminEventByID(ctx context.Context, id int64) (*AdminEventDetail, error) {
	query := `
		SELECT 
			ne.id,
			ne.canonical_title,
			coalesce(ne.slug, ''),
			ne.ai_headline,
			coalesce(ne.ai_summary, ''),
			ne.status,
			ne.is_hidden,
			ne.source_count,
			ne.first_seen_at,
			ne.last_updated_at,
			c.id,
			c.name,
			c.slug
		FROM news_events ne
		LEFT JOIN categories c ON ne.category_id = c.id
		WHERE ne.id = $1
	`
	var (
		ev         AdminEventDetail
		aiHeadline sql.NullString
		catID      sql.NullInt64
		catName    sql.NullString
		catSlug    sql.NullString
	)

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&ev.ID,
		&ev.CanonicalTitle,
		&ev.Slug,
		&aiHeadline,
		&ev.AISummary,
		&ev.Status,
		&ev.IsHidden,
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
		return nil, fmt.Errorf("get admin event by id: %w", err)
	}

	if aiHeadline.Valid && strings.TrimSpace(aiHeadline.String) != "" {
		h := aiHeadline.String
		ev.AIHeadline = &h
	}
	if catID.Valid {
		ev.Category = &CategoryInfo{
			ID:   catID.Int64,
			Name: catName.String,
			Slug: catSlug.String,
		}
	}
	ev.Sources = []AdminEventSource{}
	ev.Entities = []EntityInfo{}
	ev.ModerationHistory = []ModerationActionRecord{}

	// Load entities
	entitiesMap, err := s.getEntitiesForEvents(ctx, []int64{ev.ID})
	if err == nil {
		if ents, ok := entitiesMap[ev.ID]; ok {
			ev.Entities = ents
		}
	}

	// Load full sources
	srcQuery := `
		SELECT 
			es.event_id,
			es.raw_post_id,
			ch.id,
			ch.name,
			ch.handle,
			rp.telegram_message_id,
			rp.raw_text,
			rp.posted_at,
			es.attached_at
		FROM event_sources es
		JOIN raw_posts rp ON es.raw_post_id = rp.id
		JOIN channels ch ON rp.channel_id = ch.id
		WHERE es.event_id = $1
		ORDER BY rp.posted_at ASC
	`
	sRows, err := s.db.QueryContext(ctx, srcQuery, ev.ID)
	if err == nil {
		defer sRows.Close()
		for sRows.Next() {
			var (
				src    AdminEventSource
				handle sql.NullString
			)
			if err := sRows.Scan(
				&src.EventID,
				&src.RawPostID,
				&src.ChannelID,
				&src.ChannelName,
				&handle,
				&src.TelegramMessageID,
				&src.RawText,
				&src.PostedAt,
				&src.AttachedAt,
			); err == nil {
				if handle.Valid {
					h := handle.String
					src.ChannelHandle = &h
				}
				ev.Sources = append(ev.Sources, src)
			}
		}
	}

	// Load moderation history
	history, err := s.GetModerationHistory(ctx, "event", ev.ID)
	if err == nil && history != nil {
		ev.ModerationHistory = history
	} else {
		ev.ModerationHistory = []ModerationActionRecord{}
	}

	return &ev, nil
}

// HideEvent soft-takedowns an event and creates a moderation audit record.
func (s *SQLStore) HideEvent(ctx context.Context, eventID int64, actorUserID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("a reason is required for hiding an event")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, "UPDATE news_events SET is_hidden = true WHERE id = $1", eventID)
	if err != nil {
		return fmt.Errorf("update event is_hidden: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}

	auditQuery := `
		INSERT INTO moderation_actions (actor_user_id, action_type, target_type, target_id, reason, created_at)
		VALUES ($1, 'hide_event', 'event', $2, $3, NOW())
	`
	if _, err := tx.ExecContext(ctx, auditQuery, actorUserID, eventID, reason); err != nil {
		return fmt.Errorf("log moderation action: %w", err)
	}

	return tx.Commit()
}

// RestoreEvent restores a previously hidden event and records the action in moderation_actions.
func (s *SQLStore) RestoreEvent(ctx context.Context, eventID int64, actorUserID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("a reason is required for restoring an event")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, "UPDATE news_events SET is_hidden = false WHERE id = $1", eventID)
	if err != nil {
		return fmt.Errorf("update event is_hidden: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}

	auditQuery := `
		INSERT INTO moderation_actions (actor_user_id, action_type, target_type, target_id, reason, created_at)
		VALUES ($1, 'restore_event', 'event', $2, $3, NOW())
	`
	if _, err := tx.ExecContext(ctx, auditQuery, actorUserID, eventID, reason); err != nil {
		return fmt.Errorf("log moderation action: %w", err)
	}

	return tx.Commit()
}

// DetachSource removes a source post from an event without taking down the event itself.
func (s *SQLStore) DetachSource(ctx context.Context, eventID int64, rawPostID int64, actorUserID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("a reason is required for detaching a source")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, "DELETE FROM event_sources WHERE event_id = $1 AND raw_post_id = $2", eventID, rawPostID)
	if err != nil {
		return fmt.Errorf("detach source: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errors.New("source not attached to this event")
	}

	// Update source count
	updateQuery := `
		UPDATE news_events 
		SET source_count = (SELECT count(*) FROM event_sources WHERE event_id = $1)
		WHERE id = $1
	`
	if _, err := tx.ExecContext(ctx, updateQuery, eventID); err != nil {
		return fmt.Errorf("update source count: %w", err)
	}

	auditQuery := `
		INSERT INTO moderation_actions (actor_user_id, action_type, target_type, target_id, reason, created_at)
		VALUES ($1, 'detach_source', 'source', $2, $3, NOW())
	`
	if _, err := tx.ExecContext(ctx, auditQuery, actorUserID, rawPostID, reason); err != nil {
		return fmt.Errorf("log moderation action: %w", err)
	}

	return tx.Commit()
}

// GetModerationHistory returns audit actions for a specific target.
func (s *SQLStore) GetModerationHistory(ctx context.Context, targetType string, targetID int64) ([]ModerationActionRecord, error) {
	query := `
		SELECT 
			ma.id,
			ma.actor_user_id,
			u.email,
			ma.action_type,
			ma.target_type,
			ma.target_id,
			ma.reason,
			ma.created_at
		FROM moderation_actions ma
		JOIN users u ON ma.actor_user_id = u.id
		WHERE ma.target_type = $1 AND ma.target_id = $2
		ORDER BY ma.created_at DESC
	`
	rows, err := s.db.QueryContext(ctx, query, targetType, targetID)
	if err != nil {
		return nil, fmt.Errorf("query moderation history: %w", err)
	}
	defer rows.Close()

	records := []ModerationActionRecord{}
	for rows.Next() {
		var (
			rec    ModerationActionRecord
			reason sql.NullString
		)
		if err := rows.Scan(
			&rec.ID,
			&rec.ActorUserID,
			&rec.ActorEmail,
			&rec.ActionType,
			&rec.TargetType,
			&rec.TargetID,
			&reason,
			&rec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan moderation history: %w", err)
		}
		if reason.Valid {
			r := reason.String
			rec.Reason = &r
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// ListNeedsReviewPosts retrieves unresolved posts with candidate event context.
func (s *SQLStore) ListNeedsReviewPosts(ctx context.Context, limit, offset int) (*ReviewQueueResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	countQuery := `
		SELECT count(*)
		FROM raw_posts
		WHERE processing_status = 'needs_review'
	`
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return nil, fmt.Errorf("count review queue: %w", err)
	}

	if total == 0 {
		return &ReviewQueueResult{
			Posts:  []NeedsReviewPost{},
			Total:  0,
			Limit:  limit,
			Offset: offset,
		}, nil
	}

	query := `
		SELECT 
			rp.id,
			rp.channel_id,
			ch.name,
			ch.handle,
			rp.telegram_message_id,
			rp.raw_text,
			rp.posted_at,
			rp.ingested_at,
			pa.event_id,
			ne.canonical_title,
			pa.reasoning,
			pa.created_at
		FROM raw_posts rp
		JOIN channels ch ON rp.channel_id = ch.id
		LEFT JOIN LATERAL (
			SELECT event_id, reasoning, created_at
			FROM processing_audit
			WHERE raw_post_id = rp.id
			ORDER BY created_at DESC
			LIMIT 1
		) pa ON true
		LEFT JOIN news_events ne ON pa.event_id = ne.id
		WHERE rp.processing_status = 'needs_review'
		ORDER BY rp.ingested_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query review queue: %w", err)
	}
	defer rows.Close()

	var posts []NeedsReviewPost
	for rows.Next() {
		var (
			p         NeedsReviewPost
			handle    sql.NullString
			candID    sql.NullInt64
			candTitle sql.NullString
			aiReason  sql.NullString
			auditTime sql.NullTime
		)
		if err := rows.Scan(
			&p.RawPostID,
			&p.ChannelID,
			&p.ChannelName,
			&handle,
			&p.TelegramMessageID,
			&p.RawText,
			&p.PostedAt,
			&p.IngestedAt,
			&candID,
			&candTitle,
			&aiReason,
			&auditTime,
		); err != nil {
			return nil, fmt.Errorf("scan review post: %w", err)
		}
		if handle.Valid {
			h := handle.String
			p.ChannelHandle = &h
		}
		if candID.Valid {
			id := candID.Int64
			p.CandidateEventID = &id
		}
		if candTitle.Valid {
			t := candTitle.String
			p.CandidateEventTitle = &t
		}
		if aiReason.Valid {
			r := aiReason.String
			p.AIRunReason = &r
		}
		if auditTime.Valid {
			t := auditTime.Time
			p.AuditCreatedAt = &t
		}
		posts = append(posts, p)
	}

	return &ReviewQueueResult{
		Posts:  posts,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, rows.Err()
}

// ResolveNeedsReviewPost manually resolves an ambiguous post, logging to processing_audit and moderation_actions.
func (s *SQLStore) ResolveNeedsReviewPost(ctx context.Context, rawPostID int64, decision string, targetEventID *int64, actorEmail string, actorUserID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	decision = strings.ToLower(strings.TrimSpace(decision))

	if decision != "attach_to_event" && decision != "create_new_event" && decision != "discard" {
		return fmt.Errorf("invalid decision %q: must be 'attach_to_event', 'create_new_event', or 'discard'", decision)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Verify post is currently needs_review
	var rawText string
	err = tx.QueryRowContext(ctx, "SELECT raw_text FROM raw_posts WHERE id = $1 AND processing_status = 'needs_review'", rawPostID).Scan(&rawText)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("post is not in needs_review queue")
		}
		return fmt.Errorf("check post status: %w", err)
	}

	var finalEventID *int64

	switch decision {
	case "attach_to_event":
		if targetEventID == nil || *targetEventID <= 0 {
			return errors.New("target_event_id is required when attaching to event")
		}
		finalEventID = targetEventID

		// Verify event exists
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM news_events WHERE id = $1)", *targetEventID).Scan(&exists); err != nil || !exists {
			return errors.New("target event not found")
		}

		// Insert event_source
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO event_sources (event_id, raw_post_id, attached_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (event_id, raw_post_id) DO NOTHING
		`, *targetEventID, rawPostID); err != nil {
			return fmt.Errorf("insert event source: %w", err)
		}

		// Update event source count & last_updated_at
		if _, err := tx.ExecContext(ctx, `
			UPDATE news_events 
			SET source_count = (SELECT count(*) FROM event_sources WHERE event_id = $1),
			    last_updated_at = NOW()
			WHERE id = $1
		`, *targetEventID); err != nil {
			return fmt.Errorf("update event: %w", err)
		}

	case "create_new_event":
		title := rawText
		if len([]rune(title)) > 100 {
			title = string([]rune(title)[:100])
		}
		// Create new event
		var newID int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO news_events (canonical_title, status, source_count, founding_raw_post_id, first_seen_at, last_updated_at)
			VALUES ($1, 'active', 1, $2, NOW(), NOW())
			RETURNING id
		`, title, rawPostID).Scan(&newID)
		if err != nil {
			return fmt.Errorf("create event: %w", err)
		}
		finalEventID = &newID

		// Attach source
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO event_sources (event_id, raw_post_id, attached_at)
			VALUES ($1, $2, NOW())
		`, newID, rawPostID); err != nil {
			return fmt.Errorf("attach founding source: %w", err)
		}

	case "discard":
		// No event attached
	}

	// Update raw_post status to processed
	if _, err := tx.ExecContext(ctx, "UPDATE raw_posts SET processing_status = 'processed' WHERE id = $1", rawPostID); err != nil {
		return fmt.Errorf("update raw_post status: %w", err)
	}

	// Insert into processing_audit with model_used = human:{email}
	auditModel := fmt.Sprintf("human:%s", actorEmail)
	auditQuery := `
		INSERT INTO processing_audit (raw_post_id, event_id, stage, decision, confidence, model_used, reasoning, created_at)
		VALUES ($1, $2, 'manual_verification', $3, 1.0, $4, $5, NOW())
	`
	if _, err := tx.ExecContext(ctx, auditQuery, rawPostID, finalEventID, decision, auditModel, reason); err != nil {
		return fmt.Errorf("log processing audit: %w", err)
	}

	// Insert into moderation_actions
	modQuery := `
		INSERT INTO moderation_actions (actor_user_id, action_type, target_type, target_id, reason, created_at)
		VALUES ($1, 'resolve_needs_review', 'raw_post', $2, NULLIF($3, ''), NOW())
	`
	if _, err := tx.ExecContext(ctx, modQuery, actorUserID, rawPostID, reason); err != nil {
		return fmt.Errorf("log moderation action: %w", err)
	}

	return tx.Commit()
}
