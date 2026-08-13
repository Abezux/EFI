package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockAdminStore struct {
	users            map[string]*AdminUserWithHash
	channels         []AdminChannel
	events           map[int64]*AdminEventDetail
	reviewPosts      []NeedsReviewPost
	auditActions     []ModerationActionRecord
	toggledChannel   int64
	hiddenEvent      int64
	restoredEvent    int64
	detachedSource   int64
	resolvedPost     int64
	resolvedDecision string
}

func (m *mockAdminStore) Close() error {
	return nil
}

func newMockAdminStore() *mockAdminStore {
	hash, _ := HashPassword("ValidAdminPass123!")
	m := &mockAdminStore{
		users: map[string]*AdminUserWithHash{
			"admin@efi.et": {
				ID:           1,
				Email:        "admin@efi.et",
				PasswordHash: hash,
				Role:         "admin",
				CreatedAt:    time.Now().UTC(),
			},
			"mod@efi.et": {
				ID:           2,
				Email:        "mod@efi.et",
				PasswordHash: hash,
				Role:         "moderator",
				CreatedAt:    time.Now().UTC(),
			},
		},
		channels: []AdminChannel{
			{
				ID:                1,
				TelegramChannelID: -1001234567,
				Name:              "Test Channel",
				IsActive:          true,
				AddedAt:           time.Now().UTC(),
				PostCount:         10,
			},
		},
		events: map[int64]*AdminEventDetail{
			100: {
				ID:             100,
				CanonicalTitle: "Major Breaking News Story",
				Slug:           "major-breaking-news-story",
				AISummary:      "Summary of the major news story.",
				Status:         "active",
				IsHidden:       false,
				SourceCount:    2,
				FirstSeenAt:    time.Now().UTC(),
				LastUpdatedAt:  time.Now().UTC(),
				Sources: []AdminEventSource{
					{
						EventID:     100,
						RawPostID:   501,
						ChannelName: "Test Channel",
						RawText:     "First report text",
					},
					{
						EventID:     100,
						RawPostID:   502,
						ChannelName: "Test Channel",
						RawText:     "Second report text",
					},
				},
			},
		},
		reviewPosts: []NeedsReviewPost{
			{
				RawPostID:         901,
				ChannelID:         1,
				ChannelName:       "Test Channel",
				TelegramMessageID: 101,
				RawText:           "Ambiguous post needing review",
				PostedAt:          time.Now().UTC(),
				IngestedAt:        time.Now().UTC(),
			},
		},
	}
	return m
}

// Implement AdminStore
func (m *mockAdminStore) GetEvents(ctx context.Context, filter EventFilter) (*EventListResult, error) {
	return &EventListResult{Events: []EventSummary{}, Total: 0}, nil
}
func (m *mockAdminStore) GetEventByID(ctx context.Context, id int64) (*EventDetail, error) {
	if ev, ok := m.events[id]; ok && !ev.IsHidden {
		return &EventDetail{ID: ev.ID, CanonicalTitle: ev.CanonicalTitle}, nil
	}
	return nil, ErrNotFound
}
func (m *mockAdminStore) GetCategories(ctx context.Context) ([]CategoryWithCount, error) {
	return []CategoryWithCount{}, nil
}
func (m *mockAdminStore) SearchEvents(ctx context.Context, query string, limit, offset int) (*EventListResult, error) {
	return &EventListResult{Events: []EventSummary{}, Total: 0}, nil
}
func (m *mockAdminStore) Ping(ctx context.Context) error {
	return nil
}

func (m *mockAdminStore) GetUserByEmail(ctx context.Context, email string) (*AdminUserWithHash, error) {
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, ErrNotFound
}
func (m *mockAdminStore) GetUserByID(ctx context.Context, id int64) (*AdminUser, error) {
	for _, u := range m.users {
		if u.ID == id {
			return &AdminUser{ID: u.ID, Email: u.Email, Role: u.Role}, nil
		}
	}
	return nil, ErrNotFound
}
func (m *mockAdminStore) ListAdminChannels(ctx context.Context) ([]AdminChannel, error) {
	return m.channels, nil
}
func (m *mockAdminStore) AddChannel(ctx context.Context, telegramChannelID int64, name, handle string, actorUserID int64, reason string) (*AdminChannel, error) {
	ch := AdminChannel{
		ID:                int64(len(m.channels) + 1),
		TelegramChannelID: telegramChannelID,
		Name:              name,
		IsActive:          true,
		AddedAt:           time.Now().UTC(),
	}
	m.channels = append(m.channels, ch)
	return &ch, nil
}
func (m *mockAdminStore) ToggleChannel(ctx context.Context, channelID int64, actorUserID int64, reason string) (*AdminChannel, error) {
	for i := range m.channels {
		if m.channels[i].ID == channelID {
			m.channels[i].IsActive = !m.channels[i].IsActive
			m.toggledChannel = channelID
			return &m.channels[i], nil
		}
	}
	return nil, ErrNotFound
}
func (m *mockAdminStore) ListAdminEvents(ctx context.Context, limit, offset int, statusFilter string, hiddenFilter *bool) ([]AdminEventSummary, int, error) {
	var res []AdminEventSummary
	for _, ev := range m.events {
		if hiddenFilter != nil && ev.IsHidden != *hiddenFilter {
			continue
		}
		res = append(res, AdminEventSummary{
			ID:             ev.ID,
			CanonicalTitle: ev.CanonicalTitle,
			Status:         ev.Status,
			IsHidden:       ev.IsHidden,
		})
	}
	return res, len(res), nil
}
func (m *mockAdminStore) GetAdminEventByID(ctx context.Context, id int64) (*AdminEventDetail, error) {
	if ev, ok := m.events[id]; ok {
		return ev, nil
	}
	return nil, ErrNotFound
}
func (m *mockAdminStore) HideEvent(ctx context.Context, eventID int64, actorUserID int64, reason string) error {
	if ev, ok := m.events[eventID]; ok {
		ev.IsHidden = true
		m.hiddenEvent = eventID
		return nil
	}
	return ErrNotFound
}
func (m *mockAdminStore) RestoreEvent(ctx context.Context, eventID int64, actorUserID int64, reason string) error {
	if ev, ok := m.events[eventID]; ok {
		ev.IsHidden = false
		m.restoredEvent = eventID
		return nil
	}
	return ErrNotFound
}
func (m *mockAdminStore) DetachSource(ctx context.Context, eventID int64, rawPostID int64, actorUserID int64, reason string) error {
	ev, ok := m.events[eventID]
	if !ok {
		return ErrNotFound
	}
	var newSources []AdminEventSource
	for _, s := range ev.Sources {
		if s.RawPostID != rawPostID {
			newSources = append(newSources, s)
		}
	}
	ev.Sources = newSources
	ev.SourceCount = len(newSources)
	m.detachedSource = rawPostID
	return nil
}
func (m *mockAdminStore) GetModerationHistory(ctx context.Context, targetType string, targetID int64) ([]ModerationActionRecord, error) {
	return m.auditActions, nil
}
func (m *mockAdminStore) ListNeedsReviewPosts(ctx context.Context, limit, offset int) (*ReviewQueueResult, error) {
	return &ReviewQueueResult{
		Posts:  m.reviewPosts,
		Total:  len(m.reviewPosts),
		Limit:  limit,
		Offset: offset,
	}, nil
}
func (m *mockAdminStore) ResolveNeedsReviewPost(ctx context.Context, rawPostID int64, decision string, targetEventID *int64, actorEmail string, actorUserID int64, reason string) error {
	m.resolvedPost = rawPostID
	m.resolvedDecision = decision
	var remaining []NeedsReviewPost
	for _, p := range m.reviewPosts {
		if p.RawPostID != rawPostID {
			remaining = append(remaining, p)
		}
	}
	m.reviewPosts = remaining
	return nil
}

func TestAdminLoginAndSession(t *testing.T) {
	store := newMockAdminStore()
	sessionStore := NewMemorySessionStore()
	rateLimiter := NewLoginRateLimiter()
	logger := NewLogger("ERROR")

	handler := AdminLoginHandler(store, sessionStore, rateLimiter, false, logger)

	// Test 1: Invalid password -> 401
	body, _ := json.Marshal(LoginRequest{Email: "admin@efi.et", Password: "WrongPassword!"})
	req := httptest.NewRequest("POST", "/api/v1/admin/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", rr.Code)
	}

	// Test 2: Valid credentials -> 200 + cookie + csrf token
	body, _ = json.Marshal(LoginRequest{Email: "admin@efi.et", Password: "ValidAdminPass123!"})
	req = httptest.NewRequest("POST", "/api/v1/admin/login", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid login, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}

	csrfToken, ok := resp["csrf_token"].(string)
	if !ok || csrfToken == "" {
		t.Fatal("expected csrf_token in login response")
	}

	cookies := rr.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == AdminSessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected session cookie to be set")
	}
}

func TestEventTakedownAndRestore(t *testing.T) {
	store := newMockAdminStore()
	logger := NewLogger("ERROR")
	adminUser := &AdminUser{ID: 1, Email: "admin@efi.et", Role: "admin"}

	// Hide event handler
	hideHandler := HideAdminEventHandler(store, logger)

	body, _ := json.Marshal(ReasonRequest{Reason: "Legal defamation claim"})
	req := httptest.NewRequest("POST", "/api/v1/admin/events/100/hide", bytes.NewReader(body))
	req.SetPathValue("id", "100")
	ctx := context.WithValue(req.Context(), contextKeyAdminUser, adminUser)
	rr := httptest.NewRecorder()
	hideHandler.ServeHTTP(rr, req.WithContext(ctx))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for hide event, got %d: %s", rr.Code, rr.Body.String())
	}
	if store.hiddenEvent != 100 || !store.events[100].IsHidden {
		t.Fatal("expected event 100 to be hidden")
	}

	// Restore event handler
	restoreHandler := RestoreAdminEventHandler(store, logger)
	body, _ = json.Marshal(ReasonRequest{Reason: "Claim cleared on appeal"})
	req = httptest.NewRequest("POST", "/api/v1/admin/events/100/restore", bytes.NewReader(body))
	req.SetPathValue("id", "100")
	ctx = context.WithValue(req.Context(), contextKeyAdminUser, adminUser)
	rr = httptest.NewRecorder()
	restoreHandler.ServeHTTP(rr, req.WithContext(ctx))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for restore event, got %d: %s", rr.Code, rr.Body.String())
	}
	if store.events[100].IsHidden {
		t.Fatal("expected event 100 to be restored (not hidden)")
	}
}

func TestDetachSourceHandler(t *testing.T) {
	store := newMockAdminStore()
	logger := NewLogger("ERROR")
	adminUser := &AdminUser{ID: 1, Email: "admin@efi.et", Role: "admin"}

	detachHandler := DetachAdminSourceHandler(store, logger)

	body, _ := json.Marshal(DetachSourceRequest{RawPostID: 502, Reason: "Off-topic source accidentally clustered"})
	req := httptest.NewRequest("POST", "/api/v1/admin/events/100/detach-source", bytes.NewReader(body))
	req.SetPathValue("id", "100")
	ctx := context.WithValue(req.Context(), contextKeyAdminUser, adminUser)
	rr := httptest.NewRecorder()
	detachHandler.ServeHTTP(rr, req.WithContext(ctx))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for detach source, got %d: %s", rr.Code, rr.Body.String())
	}
	if store.detachedSource != 502 || store.events[100].SourceCount != 1 {
		t.Fatalf("expected source 502 detached, remaining count 1, got count %d", store.events[100].SourceCount)
	}
}

func TestResolveReviewQueueHandler(t *testing.T) {
	store := newMockAdminStore()
	logger := NewLogger("ERROR")
	adminUser := &AdminUser{ID: 1, Email: "admin@efi.et", Role: "admin"}

	resolveHandler := ResolveReviewQueueHandler(store, logger)

	targetID := int64(100)
	body, _ := json.Marshal(ResolveReviewRequest{
		Decision:      "attach_to_event",
		TargetEventID: &targetID,
		Reason:        "Verified manual match to event 100",
	})
	req := httptest.NewRequest("POST", "/api/v1/admin/review-queue/901/resolve", bytes.NewReader(body))
	req.SetPathValue("id", "901")
	ctx := context.WithValue(req.Context(), contextKeyAdminUser, adminUser)
	rr := httptest.NewRecorder()
	resolveHandler.ServeHTTP(rr, req.WithContext(ctx))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for resolve review, got %d: %s", rr.Code, rr.Body.String())
	}
	if store.resolvedPost != 901 || store.resolvedDecision != "attach_to_event" {
		t.Fatalf("expected post 901 resolved with attach_to_event, got post=%d decision=%s", store.resolvedPost, store.resolvedDecision)
	}
}
