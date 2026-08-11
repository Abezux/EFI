package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitMiddleware(t *testing.T) {
	logger := NewLogger("ERROR")
	limiter := NewIPRateLimiter(2.0, 3) // 2 req/sec, burst of 3
	middleware := RateLimitMiddleware(limiter, logger)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := middleware(okHandler)

	// First 3 requests should succeed (burst capacity = 3)
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected status 200, got %d", i, rec.Code)
		}
	}

	// 4th request should immediately be rejected with 429
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 Too Many Requests, got %d", rec.Code)
	}

	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter != "1" {
		t.Errorf("expected Retry-After header '1', got %q", retryAfter)
	}

	var errResp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode 429 response body: %v", err)
	}
	if errResp["error"] != "rate limit exceeded" {
		t.Errorf("unexpected error message: %v", errResp)
	}

	// Different IP should still have its own independent bucket
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.1:54321"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("different IP expected status 200, got %d", rec2.Code)
	}

	// Wait 600ms for token refill (2 req/sec -> 1 token per 500ms)
	time.Sleep(600 * time.Millisecond)

	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "192.168.1.100:12345"
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusOK {
		t.Errorf("after refill, expected status 200, got %d", rec3.Code)
	}
}

func TestPanicRecoveryMiddleware(t *testing.T) {
	logger := NewLogger("ERROR")
	middleware := PanicRecoveryMiddleware(logger)

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("simulated critical crash")
	})

	handler := middleware(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	// Should not crash the test process
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 Internal Server Error, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode panic response body: %v", err)
	}
	if resp["error"] != "internal server error" {
		t.Errorf("expected 'internal server error', got %q", resp["error"])
	}
}

func TestCORSMiddleware(t *testing.T) {
	middleware := CORSMiddleware("*")

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("cors test"))
	})

	handler := middleware(nextHandler)

	t.Run("OPTIONS preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/events", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("expected status 204 No Content, got %d", rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("expected Access-Control-Allow-Origin: *")
		}
		if rec.Header().Get("Access-Control-Allow-Methods") != "GET, OPTIONS" {
			t.Errorf("expected Access-Control-Allow-Methods: GET, OPTIONS")
		}
	})

	t.Run("GET request with CORS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("expected Access-Control-Allow-Origin: *")
		}
	})
}

func TestRequestLoggerMiddleware(t *testing.T) {
	logger := NewLogger("ERROR")
	middleware := RequestLoggerMiddleware(logger)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/test-log", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	corrID := rec.Header().Get("X-Correlation-ID")
	if corrID == "" {
		t.Errorf("expected X-Correlation-ID header in response")
	}
}
