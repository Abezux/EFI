package main

import (
	"encoding/json"
	"net/http"
)

// ListCategoriesHandler handles GET /api/v1/categories.
func ListCategoriesHandler(store StoreReader, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		w.Header().Set("Content-Type", "application/json")

		categories, err := store.GetCategories(r.Context())
		if err != nil {
			logger.Error("Failed to list categories", corrID, map[string]any{
				"error": err.Error(),
			})
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "failed to retrieve categories",
			})
			return
		}

		if categories == nil {
			categories = []CategoryWithCount{}
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(categories)
	}
}
