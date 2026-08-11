package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// ListEventsHandler handles GET /api/v1/events with pagination and filtering.
func ListEventsHandler(store StoreReader, maxLimit int, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		w.Header().Set("Content-Type", "application/json")

		// Query params
		categorySlug := r.URL.Query().Get("category")
		sinceStr := r.URL.Query().Get("since")
		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")

		limit := 20
		if limitStr != "" {
			if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
				limit = parsedLimit
			}
		}
		if limit > maxLimit {
			limit = maxLimit
		}

		offset := 0
		if offsetStr != "" {
			if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
				offset = parsedOffset
			}
		}

		var sinceTime *time.Time
		if sinceStr != "" {
			parsedTime, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "invalid 'since' timestamp format, must be RFC3339",
				})
				return
			}
			sinceTime = &parsedTime
		}

		filter := EventFilter{
			CategorySlug: categorySlug,
			Since:        sinceTime,
			Limit:        limit,
			Offset:       offset,
		}

		result, err := store.GetEvents(r.Context(), filter)
		if err != nil {
			logger.Error("Failed to list events", corrID, map[string]any{
				"error": err.Error(),
			})
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "failed to retrieve events",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// GetEventHandler handles GET /api/v1/events/{id}.
func GetEventHandler(store StoreReader, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		w.Header().Set("Content-Type", "application/json")

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "invalid event id, must be positive integer",
			})
			return
		}

		event, err := store.GetEventByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "event not found",
				})
				return
			}
			logger.Error("Failed to get event by ID", corrID, map[string]any{
				"event_id": id,
				"error":    err.Error(),
			})
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "failed to retrieve event",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(event)
	}
}
