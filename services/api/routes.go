package main

import (
	"net/http"
)

// SetupRouter configures all HTTP routes, middleware, and handlers.
func SetupRouter(cfg *Config, store StoreReader, hub *SSEHub, sseLimiter *SSEConnectionLimiter, logger *Logger) http.Handler {
	mux := http.NewServeMux()

	// 1. Health check (unauthenticated, operational probe)
	mux.HandleFunc("GET /healthz", HealthHandler(store, logger))

	// 2. V1 Public REST API routes
	mux.HandleFunc("GET /api/v1/events", ListEventsHandler(store, cfg.MaxPaginationLimit, logger))
	mux.HandleFunc("GET /api/v1/events/{id}", GetEventHandler(store, logger))
	mux.HandleFunc("GET /api/v1/categories", ListCategoriesHandler(store, logger))
	mux.HandleFunc("GET /api/v1/search", SearchHandler(store, cfg.MaxPaginationLimit, logger))

	// 3. V7 Real-Time SSE Stream endpoint
	if hub != nil && sseLimiter != nil {
		mux.HandleFunc("GET /api/v1/stream", StreamHandler(hub, sseLimiter, logger))
	}

	// Middleware stack: RateLimit -> CORS -> RequestLogger -> PanicRecovery
	limiter := NewIPRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)

	var handler http.Handler = mux
	handler = RateLimitMiddleware(limiter, logger)(handler)
	handler = CORSMiddleware(cfg.CORSAllowedOrigins)(handler)
	handler = RequestLoggerMiddleware(logger)(handler)
	handler = PanicRecoveryMiddleware(logger)(handler)

	return handler
}
