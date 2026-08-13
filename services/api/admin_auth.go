package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountLockedOut   = errors.New("too many failed login attempts, please try again later")
	ErrSessionExpired     = errors.New("session has expired")
	ErrInvalidCSRF        = errors.New("invalid CSRF token")
)

const (
	AdminSessionCookieName = "efi_admin_session"
	DefaultSessionDuration = 24 * time.Hour
	MaxLoginFailures       = 5
	LoginLockoutWindow     = 5 * time.Minute
)

type contextKey string

const (
	contextKeyAdminUser    contextKey = "admin_user"
	contextKeyAdminSession contextKey = "admin_session"
)

// AdminUser represents an authenticated administrative user.
type AdminUser struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"` // "admin" or "moderator"
	CreatedAt time.Time `json:"created_at"`
}

// AdminSession represents an active authenticated session.
type AdminSession struct {
	Token     string    `json:"token"`
	UserID    int64     `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CSRFToken string    `json:"csrf_token"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SessionStore defines operations for managing admin sessions.
type SessionStore interface {
	CreateSession(user *AdminUser, duration time.Duration) (*AdminSession, error)
	GetSession(token string) (*AdminSession, bool)
	DeleteSession(token string)
	CleanupExpired()
}

// MemorySessionStore is a thread-safe in-memory session store.
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*AdminSession
}

// NewMemorySessionStore creates a new in-memory session manager.
func NewMemorySessionStore() *MemorySessionStore {
	store := &MemorySessionStore{
		sessions: make(map[string]*AdminSession),
	}
	// Periodic cleanup goroutine
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		for range ticker.C {
			store.CleanupExpired()
		}
	}()
	return store
}

// generateSecureToken creates a cryptographically secure random hex string.
func generateSecureToken(byteLength int) (string, error) {
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateSession allocates a new session and CSRF token for a verified user.
func (s *MemorySessionStore) CreateSession(user *AdminUser, duration time.Duration) (*AdminSession, error) {
	token, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}
	csrfToken, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	sess := &AdminSession{
		Token:     token,
		UserID:    user.ID,
		Email:     user.Email,
		Role:      user.Role,
		CSRFToken: csrfToken,
		CreatedAt: now,
		ExpiresAt: now.Add(duration),
	}

	s.mu.Lock()
	s.sessions[token] = sess
	s.mu.Unlock()

	return sess, nil
}

// GetSession retrieves an active session if it exists and has not expired.
func (s *MemorySessionStore) GetSession(token string) (*AdminSession, bool) {
	if token == "" {
		return nil, false
	}
	s.mu.RLock()
	sess, exists := s.sessions[token]
	s.mu.RUnlock()

	if !exists {
		return nil, false
	}

	if time.Now().UTC().After(sess.ExpiresAt) {
		s.DeleteSession(token)
		return nil, false
	}

	return sess, true
}

// DeleteSession removes a session by token.
func (s *MemorySessionStore) DeleteSession(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// CleanupExpired removes expired sessions.
func (s *MemorySessionStore) CleanupExpired() {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, token)
		}
	}
}

// HashPassword creates a bcrypt hash of a plaintext password.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash verifies a plaintext password against a bcrypt hash.
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// LoginRateLimiter enforces rate limiting on failed authentication attempts.
type LoginRateLimiter struct {
	mu           sync.Mutex
	ipFailures   map[string][]time.Time
	userFailures map[string][]time.Time
}

// NewLoginRateLimiter creates a new rate limiter instance.
func NewLoginRateLimiter() *LoginRateLimiter {
	return &LoginRateLimiter{
		ipFailures:   make(map[string][]time.Time),
		userFailures: make(map[string][]time.Time),
	}
}

// pruneOldAttempts removes attempts outside the window.
func pruneOldAttempts(attempts []time.Time, window time.Duration, now time.Time) []time.Time {
	cutoff := now.Add(-window)
	var valid []time.Time
	for _, t := range attempts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	return valid
}

// IsLockedOut checks if either the IP or the email account is currently locked out.
func (r *LoginRateLimiter) IsLockedOut(ip, email string) (bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	ip = strings.TrimSpace(ip)
	email = strings.ToLower(strings.TrimSpace(email))

	if ip != "" {
		validIP := pruneOldAttempts(r.ipFailures[ip], LoginLockoutWindow, now)
		r.ipFailures[ip] = validIP
		if len(validIP) >= MaxLoginFailures {
			oldest := validIP[0]
			remaining := oldest.Add(LoginLockoutWindow).Sub(now)
			if remaining > 0 {
				return true, remaining
			}
		}
	}

	if email != "" {
		validUser := pruneOldAttempts(r.userFailures[email], LoginLockoutWindow, now)
		r.userFailures[email] = validUser
		if len(validUser) >= MaxLoginFailures {
			oldest := validUser[0]
			remaining := oldest.Add(LoginLockoutWindow).Sub(now)
			if remaining > 0 {
				return true, remaining
			}
		}
	}

	return false, 0
}

// RecordFailure notes a failed login attempt for IP and email.
func (r *LoginRateLimiter) RecordFailure(ip, email string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	ip = strings.TrimSpace(ip)
	email = strings.ToLower(strings.TrimSpace(email))

	if ip != "" {
		r.ipFailures[ip] = append(pruneOldAttempts(r.ipFailures[ip], LoginLockoutWindow, now), now)
	}
	if email != "" {
		r.userFailures[email] = append(pruneOldAttempts(r.userFailures[email], LoginLockoutWindow, now), now)
	}
}

// RecordSuccess clears failure history for a successful login.
func (r *LoginRateLimiter) RecordSuccess(ip, email string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ip = strings.TrimSpace(ip)
	email = strings.ToLower(strings.TrimSpace(email))

	if ip != "" {
		delete(r.ipFailures, ip)
	}
	if email != "" {
		delete(r.userFailures, email)
	}
}

// GetClientIP extracts the client IP address from remote address or proxy headers.
func GetClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// SetSessionCookie writes the HTTP-only secure cookie to the response.
func SetSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time, isSecure bool) {
	cookie := &http.Cookie{
		Name:     AdminSessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
}

// ClearSessionCookie clears the session cookie.
func ClearSessionCookie(w http.ResponseWriter, isSecure bool) {
	cookie := &http.Cookie{
		Name:     AdminSessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
}

// AdminAuthMiddleware validates the session cookie and attaches the user to the request context.
func AdminAuthMiddleware(sessionStore SessionStore, isSecure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(AdminSessionCookieName)
			if err != nil || cookie.Value == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "unauthorized: admin authentication required",
				})
				return
			}

			sess, ok := sessionStore.GetSession(cookie.Value)
			if !ok {
				ClearSessionCookie(w, isSecure)
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "unauthorized: session expired or invalid",
				})
				return
			}

			user := &AdminUser{
				ID:    sess.UserID,
				Email: sess.Email,
				Role:  sess.Role,
			}

			ctx := context.WithValue(r.Context(), contextKeyAdminUser, user)
			ctx = context.WithValue(ctx, contextKeyAdminSession, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CSRFMiddleware validates X-CSRF-Token on mutating requests (POST, PATCH, PUT, DELETE).
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.ToUpper(r.Method)
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		sessVal := r.Context().Value(contextKeyAdminSession)
		sess, ok := sessVal.(*AdminSession)
		if !ok || sess == nil || sess.CSRFToken == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "forbidden: missing active session for CSRF validation",
			})
			return
		}

		csrfHeader := r.Header.Get("X-CSRF-Token")
		if csrfHeader == "" {
			csrfHeader = r.FormValue("csrf_token")
		}

		if subtle.ConstantTimeCompare([]byte(csrfHeader), []byte(sess.CSRFToken)) != 1 {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "forbidden: invalid CSRF token",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireRole enforces that the authenticated user has one of the allowed roles.
func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userVal := r.Context().Value(contextKeyAdminUser)
			user, ok := userVal.(*AdminUser)
			if !ok || user == nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "unauthorized: missing user context",
				})
				return
			}

			allowed := false
			for _, role := range allowedRoles {
				if user.Role == role {
					allowed = true
					break
				}
			}

			if !allowed {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error": "forbidden: insufficient privileges for this action",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetUserFromContext retrieves the authenticated user from the context.
func GetUserFromContext(ctx context.Context) (*AdminUser, bool) {
	user, ok := ctx.Value(contextKeyAdminUser).(*AdminUser)
	return user, ok
}

// GetSessionFromContext retrieves the active session from the context.
func GetSessionFromContext(ctx context.Context) (*AdminSession, bool) {
	sess, ok := ctx.Value(contextKeyAdminSession).(*AdminSession)
	return sess, ok
}
