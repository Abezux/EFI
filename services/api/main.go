package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger := NewLogger(cfg.LogLevel)
	initCorrID := GenerateCorrelationID()

	logger.Info("Starting Ethiopia News Public API service", initCorrID, map[string]any{
		"port":                 cfg.Port,
		"rate_limit_rps":       cfg.RateLimitRPS,
		"rate_limit_burst":     cfg.RateLimitBurst,
		"max_pagination_limit": cfg.MaxPaginationLimit,
		"cors_allowed_origins": cfg.CORSAllowedOrigins,
	})

	// Initialize least-privilege read-only SQL store
	store, err := NewSQLStore(cfg.DatabaseURL)
	if err != nil {
		logger.Error("Failed to initialize database store", initCorrID, map[string]any{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	defer store.Close()

	// Initial connectivity check
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPing()
	if err := store.Ping(pingCtx); err != nil {
		logger.Warn("Initial database ping failed (will retry on health checks)", initCorrID, map[string]any{
			"error": err.Error(),
		})
	} else {
		logger.Info("Database connection established as efi_api", initCorrID, nil)
	}

	// Initialize real-time SSE Hub and connection limiter
	hub := NewSSEHub(logger)
	go hub.Run(context.Background())
	defer hub.Stop()

	sseLimiter := NewSSEConnectionLimiter(cfg.MaxSSEPerIP)

	// Initialize and start Postgres LISTEN subscriber on news_events_channel
	notifyListener := NewNotifyListener(cfg.DatabaseURL, "news_events_channel", hub, logger)
	if err := notifyListener.Start(context.Background()); err != nil {
		logger.Warn("Failed to start Postgres LISTEN subscriber (will continue with REST API)", initCorrID, map[string]any{
			"error": err.Error(),
		})
	} else {
		logger.Info("Postgres LISTEN subscriber active on news_events_channel", initCorrID, nil)
	}
	defer notifyListener.Close()

	// Build HTTP router
	router := SetupRouter(cfg, store, hub, sseLimiter, logger)

	srv := &http.Server{
		Addr:        fmt.Sprintf(":%d", cfg.Port),
		Handler:     router,
		ReadTimeout: 10 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	// Server shutdown channel
	shutdownErr := make(chan error, 1)

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		sig := <-sigChan

		shutdownCorrID := GenerateCorrelationID()
		logger.Info(fmt.Sprintf("Received shutdown signal: %v, initiating graceful shutdown", sig), shutdownCorrID, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_ = notifyListener.Close()
		hub.Stop()

		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("Graceful shutdown failed", shutdownCorrID, map[string]any{
				"error": err.Error(),
			})
			shutdownErr <- err
			return
		}

		logger.Info("Public API server stopped gracefully", shutdownCorrID, nil)
		shutdownErr <- nil
	}()

	logger.Info(fmt.Sprintf("Listening for HTTP requests on :%d", cfg.Port), initCorrID, nil)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("Server ListenAndServe failed", initCorrID, map[string]any{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	if err := <-shutdownErr; err != nil {
		os.Exit(1)
	}
}
