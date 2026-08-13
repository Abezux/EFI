package main

import (
	"net/http"
)

// SetupRouter configures all HTTP routes, middleware, and handlers.
func SetupRouter(
	cfg *Config,
	store StoreReader,
	adminStore AdminStore,
	sessionStore SessionStore,
	adminRateLimiter *LoginRateLimiter,
	hub *SSEHub,
	sseLimiter *SSEConnectionLimiter,
	logger *Logger,
) http.Handler {
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

	// 4. V9.1 Admin Panel Endpoints
	if adminStore != nil && sessionStore != nil && adminRateLimiter != nil {
		isSecure := false // Local / container development

		// Admin Auth Public Routes
		mux.HandleFunc("POST /api/v1/admin/login", AdminLoginHandler(adminStore, sessionStore, adminRateLimiter, isSecure, logger))

		// Admin Protected Route Middleware
		auth := AdminAuthMiddleware(sessionStore, isSecure)
		csrf := CSRFMiddleware
		adminOnly := RequireRole("admin")

		// Admin Profile & Session
		mux.Handle("POST /api/v1/admin/logout", auth(AdminLogoutHandler(sessionStore, isSecure, logger)))
		mux.Handle("GET /api/v1/admin/me", auth(AdminMeHandler()))
		mux.Handle("GET /api/v1/admin/csrf", auth(AdminCSRFHandler()))

		// Admin Channels Management
		mux.Handle("GET /api/v1/admin/channels", auth(ListAdminChannelsHandler(adminStore, logger)))
		mux.Handle("POST /api/v1/admin/channels", auth(adminOnly(csrf(AddAdminChannelHandler(adminStore, logger)))))
		mux.Handle("PATCH /api/v1/admin/channels/{id}/toggle", auth(adminOnly(csrf(ToggleAdminChannelHandler(adminStore, logger)))))

		// Admin Events & Moderation
		mux.Handle("GET /api/v1/admin/events", auth(ListAdminEventsHandler(adminStore, logger)))
		mux.Handle("GET /api/v1/admin/events/{id}", auth(GetAdminEventHandler(adminStore, logger)))
		mux.Handle("POST /api/v1/admin/events/{id}/hide", auth(csrf(HideAdminEventHandler(adminStore, logger))))
		mux.Handle("POST /api/v1/admin/events/{id}/restore", auth(csrf(RestoreAdminEventHandler(adminStore, logger))))
		mux.Handle("POST /api/v1/admin/events/{id}/detach-source", auth(csrf(DetachAdminSourceHandler(adminStore, logger))))

		// Admin Review Queue
		mux.Handle("GET /api/v1/admin/review-queue", auth(ListReviewQueueHandler(adminStore, logger)))
		mux.Handle("POST /api/v1/admin/review-queue/{id}/resolve", auth(csrf(ResolveReviewQueueHandler(adminStore, logger))))
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
