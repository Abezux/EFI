package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashing(t *testing.T) {
	plain := "SuperSecretPassword123!"
	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if plain == hash {
		t.Fatal("password hash must not match plaintext")
	}

	if !CheckPasswordHash(plain, hash) {
		t.Fatal("expected password check to succeed with correct password")
	}

	if CheckPasswordHash("WrongPassword!", hash) {
		t.Fatal("expected password check to fail with incorrect password")
	}
}

func TestSessionStoreLifecycle(t *testing.T) {
	store := NewMemorySessionStore()
	user := &AdminUser{
		ID:    42,
		Email: "admin@example.com",
		Role:  "admin",
	}

	sess, err := store.CreateSession(user, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if sess.Token == "" || sess.CSRFToken == "" {
		t.Fatal("expected non-empty session token and CSRF token")
	}

	// Retrieve session
	retrieved, ok := store.GetSession(sess.Token)
	if !ok || retrieved.UserID != user.ID || retrieved.Role != user.Role {
		t.Fatalf("failed to retrieve valid session: %+v", retrieved)
	}

	// Delete session
	store.DeleteSession(sess.Token)
	if _, ok := store.GetSession(sess.Token); ok {
		t.Fatal("expected deleted session to not exist")
	}

	// Test expiration
	expSess, _ := store.CreateSession(user, 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	if _, ok := store.GetSession(expSess.Token); ok {
		t.Fatal("expected expired session to be invalid")
	}
}

func TestLoginRateLimiter(t *testing.T) {
	limiter := NewLoginRateLimiter()
	ip := "192.168.1.100"
	email := "test@example.com"

	// Initially not locked out
	if locked, _ := limiter.IsLockedOut(ip, email); locked {
		t.Fatal("expected not locked out initially")
	}

	// Record 4 failures - still not locked out
	for i := 0; i < 4; i++ {
		limiter.RecordFailure(ip, email)
	}
	if locked, _ := limiter.IsLockedOut(ip, email); locked {
		t.Fatal("expected not locked out after 4 failures")
	}

	// 5th failure triggers lockout
	limiter.RecordFailure(ip, email)
	locked, remaining := limiter.IsLockedOut(ip, email)
	if !locked || remaining <= 0 {
		t.Fatalf("expected locked out after 5 failures, got locked=%v, remaining=%v", locked, remaining)
	}

	// Record success clears lockout
	limiter.RecordSuccess(ip, email)
	if locked, _ := limiter.IsLockedOut(ip, email); locked {
		t.Fatal("expected lockout to be cleared after success")
	}
}

func TestAdminAuthMiddleware(t *testing.T) {
	store := NewMemorySessionStore()
	user := &AdminUser{
		ID:    1,
		Email: "admin@efi.et",
		Role:  "admin",
	}
	sess, err := store.CreateSession(user, time.Hour)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	handler := AdminAuthMiddleware(store, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := GetUserFromContext(r.Context())
		if !ok || u.Email != "admin@efi.et" {
			http.Error(w, "missing context user", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	// Case 1: Unauthenticated request -> 401
	req := httptest.NewRequest("GET", "/api/v1/admin/me", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for unauthenticated request, got %d", rr.Code)
	}

	// Case 2: Invalid cookie -> 401
	req = httptest.NewRequest("GET", "/api/v1/admin/me", nil)
	req.AddCookie(&http.Cookie{Name: AdminSessionCookieName, Value: "invalid-token"})
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for invalid cookie, got %d", rr.Code)
	}

	// Case 3: Valid cookie -> 200
	req = httptest.NewRequest("GET", "/api/v1/admin/me", nil)
	req.AddCookie(&http.Cookie{Name: AdminSessionCookieName, Value: sess.Token})
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for valid cookie, got %d", rr.Code)
	}
}

func TestCSRFMiddleware(t *testing.T) {
	sess := &AdminSession{
		Token:     "test-token",
		CSRFToken: "valid-csrf-token-12345",
	}

	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// GET request bypasses CSRF
	req := httptest.NewRequest("GET", "/api/v1/admin/channels", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected GET to bypass CSRF, got %d", rr.Code)
	}

	// POST without session in context -> 403
	req = httptest.NewRequest("POST", "/api/v1/admin/channels", strings.NewReader(`{}`))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected POST without session to be 403, got %d", rr.Code)
	}

	// POST with session but invalid CSRF header -> 403
	req = httptest.NewRequest("POST", "/api/v1/admin/channels", strings.NewReader(`{}`))
	ctx := context.WithValue(req.Context(), contextKeyAdminSession, sess)
	req = req.WithContext(ctx)
	req.Header.Set("X-CSRF-Token", "wrong-token")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected POST with invalid CSRF token to be 403, got %d", rr.Code)
	}

	// POST with valid CSRF header -> 200
	req = httptest.NewRequest("POST", "/api/v1/admin/channels", strings.NewReader(`{}`))
	ctx = context.WithValue(req.Context(), contextKeyAdminSession, sess)
	req = req.WithContext(ctx)
	req.Header.Set("X-CSRF-Token", "valid-csrf-token-12345")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected POST with valid CSRF token to be 200, got %d", rr.Code)
	}
}

func TestRequireRoleMiddleware(t *testing.T) {
	adminUser := &AdminUser{ID: 1, Role: "admin"}
	modUser := &AdminUser{ID: 2, Role: "moderator"}

	adminOnlyHandler := RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Case 1: Moderator accessing admin-only -> 403
	req := httptest.NewRequest("POST", "/api/v1/admin/channels", nil)
	ctx := context.WithValue(req.Context(), contextKeyAdminUser, modUser)
	rr := httptest.NewRecorder()
	adminOnlyHandler.ServeHTTP(rr, req.WithContext(ctx))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for moderator on admin-only route, got %d", rr.Code)
	}

	// Case 2: Admin accessing admin-only -> 200
	req = httptest.NewRequest("POST", "/api/v1/admin/channels", nil)
	ctx = context.WithValue(req.Context(), contextKeyAdminUser, adminUser)
	rr = httptest.NewRecorder()
	adminOnlyHandler.ServeHTTP(rr, req.WithContext(ctx))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for admin, got %d", rr.Code)
	}
}
