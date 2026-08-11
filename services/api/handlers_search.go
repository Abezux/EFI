package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// SearchHandler handles GET /api/v1/search?q=...
func SearchHandler(store StoreReader, maxLimit int, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		w.Header().Set("Content-Type", "application/json")

		query := r.URL.Query().Get("q")
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

		if query == "" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(EventListResult{
				Events: []EventSummary{},
				Total:  0,
				Limit:  limit,
				Offset: offset,
			})
			return
		}

		result, err := store.SearchEvents(r.Context(), query, limit, offset)
		if err != nil {
			logger.Error("Failed to execute search", corrID, map[string]any{
				"query": query,
				"error": err.Error(),
			})
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "failed to execute search",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}
