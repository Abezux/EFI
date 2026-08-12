package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// responseWriterWrapper wraps http.ResponseWriter to capture the HTTP status code.
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// RequestLoggerMiddleware logs every HTTP request as structured JSON.
func RequestLoggerMiddleware(logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			corrID := r.Header.Get("X-Correlation-ID")
			if corrID == "" {
				corrID = GenerateCorrelationID()
				r.Header.Set("X-Correlation-ID", corrID)
			}
			w.Header().Set("X-Correlation-ID", corrID)

			wrapped := &responseWriterWrapper{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)
			clientIP := getClientIP(r)

			logger.Info("http_request", corrID, map[string]any{
				"method":      r.Method,
				"path":        r.URL.Path,
				"query":       r.URL.RawQuery,
				"remote_ip":   clientIP,
				"status":      wrapped.statusCode,
				"duration_ms": duration.Milliseconds(),
			})
		})
	}
}

// PanicRecoveryMiddleware catches panics, logs the stack trace in structured JSON, and returns 500.
func PanicRecoveryMiddleware(logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					corrID := r.Header.Get("X-Correlation-ID")
					stack := string(debug.Stack())
					logger.Error("Unhandled panic recovered in HTTP handler", corrID, map[string]any{
						"panic": fmt.Sprintf("%v", rec),
						"stack": stack,
						"path":  r.URL.Path,
					})

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error": "internal server error",
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware sets standard CORS headers and handles OPTIONS preflight.
func CORSMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	if allowedOrigins == "" {
		allowedOrigins = "*"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigins)
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Correlation-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// tokenBucket represents a token-bucket rate limiter for a single IP.
type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

// IPRateLimiter provides per-IP rate limiting using the token bucket algorithm.
type IPRateLimiter struct {
	mu           sync.Mutex
	rps          float64
	burst        int
	clients      map[string]*tokenBucket
	lastCleaned  time.Time
	cleanupEvery time.Duration
}

// NewIPRateLimiter creates a new IPRateLimiter with specified RPS and burst capacity.
func NewIPRateLimiter(rps float64, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		rps:          rps,
		burst:        burst,
		clients:      make(map[string]*tokenBucket),
		lastCleaned:  time.Now(),
		cleanupEvery: 5 * time.Minute,
	}
}

// Allow checks if a request from the given IP is permitted under the rate limit.
func (lim *IPRateLimiter) Allow(ip string) bool {
	lim.mu.Lock()
	defer lim.mu.Unlock()

	now := time.Now()

	// Periodic cleanup of stale clients
	if now.Sub(lim.lastCleaned) > lim.cleanupEvery {
		for k, tb := range lim.clients {
			if now.Sub(tb.lastRefill) > 10*time.Minute {
				delete(lim.clients, k)
			}
		}
		lim.lastCleaned = now
	}

	tb, exists := lim.clients[ip]
	if !exists {
		tb = &tokenBucket{
			tokens:     float64(lim.burst) - 1.0, // consume 1 initial token
			lastRefill: now,
		}
		lim.clients[ip] = tb
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * lim.rps
	if tb.tokens > float64(lim.burst) {
		tb.tokens = float64(lim.burst)
	}
	tb.lastRefill = now

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// RateLimitMiddleware enforces token bucket rate limiting per client IP.
func RateLimitMiddleware(limiter *IPRateLimiter, logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r)
			if !limiter.Allow(ip) {
				corrID := r.Header.Get("X-Correlation-ID")
				logger.Warn("Rate limit exceeded", corrID, map[string]any{
					"remote_ip": ip,
					"path":      r.URL.Path,
				})

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":       "rate limit exceeded",
					"retry_after": 1,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fallback to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
