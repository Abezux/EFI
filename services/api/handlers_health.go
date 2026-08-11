package main

import (
	"encoding/json"
	"net/http"
)

type HealthResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func HealthHandler(store StoreReader, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Correlation-ID")
		w.Header().Set("Content-Type", "application/json")

		if err := store.Ping(r.Context()); err != nil {
			logger.Error("Health check failed: database unreachable", corrID, map[string]any{
				"error": err.Error(),
			})
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(HealthResponse{
				Status: "unhealthy",
				Error:  "database unreachable",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(HealthResponse{
			Status: "healthy",
		})
	}
}
