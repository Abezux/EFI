package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// LoginRequest defines the payload for authentication.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ChannelCreateRequest defines the payload for registering a new channel.
type ChannelCreateRequest struct {
	TelegramChannelID int64  `json:"telegram_channel_id"`
	Name              string `json:"name"`
	Handle            string `json:"handle"`
	Reason            string `json:"reason"`
}

// ReasonRequest defines payloads that require an administrative rationale.
type ReasonRequest struct {
	Reason string `json:"reason"`
}

// DetachSourceRequest defines the payload for detaching a single post from an event.
type DetachSourceRequest struct {
	RawPostID int64  `json:"raw_post_id"`
	Reason    string `json:"reason"`
}

// ResolveReviewRequest defines the payload for manually resolving an ambiguous post.
type ResolveReviewRequest struct {
	Decision      string `json:"decision"` // "attach_to_event", "create_new_event", "discard"
	TargetEventID *int64 `json:"target_event_id,omitempty"`
	Reason        string `json:"reason"`
}

// AdminLoginHandler authenticates credentials and creates an admin session.
func AdminLoginHandler(store AdminStore, sessionStore SessionStore, rateLimiter *LoginRateLimiter, isSecure bool, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		clientIP := GetClientIP(r)

		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid request body",
			})
			return
		}

		req.Email = strings.ToLower(strings.TrimSpace(req.Email))

		if locked, remaining := rateLimiter.IsLockedOut(clientIP, req.Email); locked {
			w.Header().Set("Retry-After", strconv.Itoa(int(remaining.Seconds())))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "too many failed login attempts, please try again later",
			})
			return
		}

		if req.Email == "" || req.Password == "" {
			rateLimiter.RecordFailure(clientIP, req.Email)
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "invalid email or password",
			})
			return
		}

		userWithHash, err := store.GetUserByEmail(r.Context(), req.Email)
		if err != nil || !CheckPasswordHash(req.Password, userWithHash.PasswordHash) {
			rateLimiter.RecordFailure(clientIP, req.Email)
			logger.Warn("Failed admin login attempt", corrID, map[string]any{
				"email":     req.Email,
				"remote_ip": clientIP,
			})
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "invalid email or password",
			})
			return
		}

		rateLimiter.RecordSuccess(clientIP, req.Email)

		adminUser := &AdminUser{
			ID:        userWithHash.ID,
			Email:     userWithHash.Email,
			Role:      userWithHash.Role,
			CreatedAt: userWithHash.CreatedAt,
		}

		sess, err := sessionStore.CreateSession(adminUser, DefaultSessionDuration)
		if err != nil {
			logger.Error("Failed to create session", corrID, map[string]any{
				"error": err.Error(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal error allocating session",
			})
			return
		}

		SetSessionCookie(w, sess.Token, sess.ExpiresAt, isSecure)

		logger.Info("Admin login successful", corrID, map[string]any{
			"user_id": adminUser.ID,
			"email":   adminUser.Email,
			"role":    adminUser.Role,
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"user":       adminUser,
			"csrf_token": sess.CSRFToken,
		})
	}
}

// AdminLogoutHandler terminates an admin session.
func AdminLogoutHandler(sessionStore SessionStore, isSecure bool, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		cookie, err := r.Cookie(AdminSessionCookieName)
		if err == nil && cookie.Value != "" {
			sessionStore.DeleteSession(cookie.Value)
		}
		ClearSessionCookie(w, isSecure)

		logger.Info("Admin logout completed", corrID, nil)
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "logged out successfully",
		})
	}
}

// AdminMeHandler returns current authenticated session profile.
func AdminMeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetUserFromContext(r.Context())
		if !ok || user == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user": user,
		})
	}
}

// AdminCSRFHandler returns the current CSRF token for the authenticated session.
func AdminCSRFHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := GetSessionFromContext(r.Context())
		if !ok || sess == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"csrf_token": sess.CSRFToken,
		})
	}
}

// ListAdminChannelsHandler lists all monitored channels.
func ListAdminChannelsHandler(store AdminStore, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		channels, err := store.ListAdminChannels(r.Context())
		if err != nil {
			logger.Error("Failed to list admin channels", corrID, map[string]any{
				"error": err.Error(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to list channels",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"channels": channels,
		})
	}
}

// AddAdminChannelHandler creates or reactivates a channel (admin role only).
func AddAdminChannelHandler(store AdminStore, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		user, _ := GetUserFromContext(r.Context())

		var req ChannelCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid request body",
			})
			return
		}

		if req.TelegramChannelID == 0 || strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "telegram_channel_id and name are required",
			})
			return
		}

		channel, err := store.AddChannel(r.Context(), req.TelegramChannelID, req.Name, req.Handle, user.ID, req.Reason)
		if err != nil {
			logger.Error("Failed to add channel", corrID, map[string]any{
				"error": err.Error(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to add channel",
			})
			return
		}

		logger.Info("Admin registered new channel", corrID, map[string]any{
			"channel_id": channel.ID,
			"name":       channel.Name,
			"actor":      user.Email,
		})

		writeJSON(w, http.StatusCreated, channel)
	}
}

// ToggleAdminChannelHandler enables or disables channel ingestion (admin role only).
func ToggleAdminChannelHandler(store AdminStore, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		user, _ := GetUserFromContext(r.Context())

		idStr := r.PathValue("id")
		channelID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || channelID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid channel id",
			})
			return
		}

		var req ReasonRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		channel, err := store.ToggleChannel(r.Context(), channelID, user.ID, req.Reason)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error": "channel not found",
				})
				return
			}
			logger.Error("Failed to toggle channel", corrID, map[string]any{
				"error": err.Error(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to toggle channel",
			})
			return
		}

		logger.Info("Admin toggled channel status", corrID, map[string]any{
			"channel_id": channel.ID,
			"is_active":  channel.IsActive,
			"actor":      user.Email,
		})

		writeJSON(w, http.StatusOK, channel)
	}
}

// ListAdminEventsHandler retrieves events with moderation status and filters.
func ListAdminEventsHandler(store AdminStore, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")

		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")
		statusFilter := r.URL.Query().Get("status")
		hiddenStr := r.URL.Query().Get("hidden")

		limit := 20
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
		offset := 0
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}

		var hiddenFilter *bool
		if hiddenStr != "" {
			b, err := strconv.ParseBool(hiddenStr)
			if err == nil {
				hiddenFilter = &b
			}
		}

		events, total, err := store.ListAdminEvents(r.Context(), limit, offset, statusFilter, hiddenFilter)
		if err != nil {
			logger.Error("Failed to list admin events", corrID, map[string]any{
				"error": err.Error(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to list events",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"events": events,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
}

// GetAdminEventHandler returns full details for a single event in the admin panel.
func GetAdminEventHandler(store AdminStore, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")

		idStr := r.PathValue("id")
		eventID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || eventID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid event id",
			})
			return
		}

		event, err := store.GetAdminEventByID(r.Context(), eventID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error": "event not found",
				})
				return
			}
			logger.Error("Failed to get admin event", corrID, map[string]any{
				"error": err.Error(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to retrieve event",
			})
			return
		}

		writeJSON(w, http.StatusOK, event)
	}
}

// HideAdminEventHandler soft-takedowns an event.
func HideAdminEventHandler(store AdminStore, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		user, _ := GetUserFromContext(r.Context())

		idStr := r.PathValue("id")
		eventID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || eventID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid event id",
			})
			return
		}

		var req ReasonRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Reason) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "a non-empty reason is required to take down an event",
			})
			return
		}

		if err := store.HideEvent(r.Context(), eventID, user.ID, req.Reason); err != nil {
			if errors.Is(err, ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error": "event not found",
				})
				return
			}
			logger.Error("Failed to hide event", corrID, map[string]any{
				"event_id": eventID,
				"error":    err.Error(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to hide event",
			})
			return
		}

		logger.Info("Event hidden by moderator", corrID, map[string]any{
			"event_id": eventID,
			"actor":    user.Email,
			"reason":   req.Reason,
		})

		writeJSON(w, http.StatusOK, map[string]string{
			"message": "event successfully hidden",
		})
	}
}

// RestoreAdminEventHandler restores a previously hidden event.
func RestoreAdminEventHandler(store AdminStore, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		user, _ := GetUserFromContext(r.Context())

		idStr := r.PathValue("id")
		eventID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || eventID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid event id",
			})
			return
		}

		var req ReasonRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Reason) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "a non-empty reason is required to restore an event",
			})
			return
		}

		if err := store.RestoreEvent(r.Context(), eventID, user.ID, req.Reason); err != nil {
			if errors.Is(err, ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error": "event not found",
				})
				return
			}
			logger.Error("Failed to restore event", corrID, map[string]any{
				"event_id": eventID,
				"error":    err.Error(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to restore event",
			})
			return
		}

		logger.Info("Event restored by moderator", corrID, map[string]any{
			"event_id": eventID,
			"actor":    user.Email,
			"reason":   req.Reason,
		})

		writeJSON(w, http.StatusOK, map[string]string{
			"message": "event successfully restored",
		})
	}
}

// DetachAdminSourceHandler removes a source post from an event.
func DetachAdminSourceHandler(store AdminStore, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		user, _ := GetUserFromContext(r.Context())

		idStr := r.PathValue("id")
		eventID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || eventID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid event id",
			})
			return
		}

		var req DetachSourceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RawPostID <= 0 || strings.TrimSpace(req.Reason) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "raw_post_id and a non-empty reason are required to detach a source",
			})
			return
		}

		if err := store.DetachSource(r.Context(), eventID, req.RawPostID, user.ID, req.Reason); err != nil {
			logger.Error("Failed to detach source", corrID, map[string]any{
				"event_id":    eventID,
				"raw_post_id": req.RawPostID,
				"error":       err.Error(),
			})
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("failed to detach source: %s", err.Error()),
			})
			return
		}

		logger.Info("Source detached from event", corrID, map[string]any{
			"event_id":    eventID,
			"raw_post_id": req.RawPostID,
			"actor":       user.Email,
			"reason":      req.Reason,
		})

		writeJSON(w, http.StatusOK, map[string]string{
			"message": "source detached successfully",
		})
	}
}

// ListReviewQueueHandler lists ambiguous posts pending manual decision.
func ListReviewQueueHandler(store AdminStore, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")

		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")

		limit := 20
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
		offset := 0
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}

		result, err := store.ListNeedsReviewPosts(r.Context(), limit, offset)
		if err != nil {
			logger.Error("Failed to list review queue", corrID, map[string]any{
				"error": err.Error(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to list review queue",
			})
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// ResolveReviewQueueHandler handles manual resolution of an ambiguous post.
func ResolveReviewQueueHandler(store AdminStore, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		user, _ := GetUserFromContext(r.Context())

		idStr := r.PathValue("id")
		rawPostID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || rawPostID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid raw_post id",
			})
			return
		}

		var req ResolveReviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid request body",
			})
			return
		}

		if err := store.ResolveNeedsReviewPost(r.Context(), rawPostID, req.Decision, req.TargetEventID, user.Email, user.ID, req.Reason); err != nil {
			logger.Error("Failed to resolve review post", corrID, map[string]any{
				"raw_post_id": rawPostID,
				"decision":    req.Decision,
				"error":       err.Error(),
			})
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("failed to resolve review post: %s", err.Error()),
			})
			return
		}

		logger.Info("Review queue post resolved by human", corrID, map[string]any{
			"raw_post_id": rawPostID,
			"decision":    req.Decision,
			"actor":       user.Email,
		})

		writeJSON(w, http.StatusOK, map[string]string{
			"message": "post resolved successfully",
		})
	}
}
