package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// StreamMessage represents a real-time event push payload over SSE.
type StreamMessage struct {
	Type    string `json:"type"` // "new_event" | "event_updated"
	EventID int64  `json:"event_id"`
}

// SSEClient represents a single connected HTTP client receiving SSE events.
type SSEClient struct {
	id       string
	ip       string
	sendChan chan StreamMessage
	closed   bool
	mu       sync.Mutex
}

// Close safely closes the client's send channel once.
func (c *SSEClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.sendChan)
	}
}

// SSEHub coordinates all active SSE clients and handles non-blocking broadcast fan-out.
type SSEHub struct {
	mu          sync.RWMutex
	clients     map[*SSEClient]bool
	register    chan *SSEClient
	unregister  chan *SSEClient
	broadcast   chan StreamMessage
	stopChan    chan struct{}
	stoppedChan chan struct{}
	logger      *Logger
}

// NewSSEHub creates a new SSEHub instance.
func NewSSEHub(logger *Logger) *SSEHub {
	return &SSEHub{
		clients:     make(map[*SSEClient]bool),
		register:    make(chan *SSEClient),
		unregister:  make(chan *SSEClient),
		broadcast:   make(chan StreamMessage, 64),
		stopChan:    make(chan struct{}),
		stoppedChan: make(chan struct{}),
		logger:      logger,
	}
}

// Run begins the hub coordinator loop.
func (h *SSEHub) Run(ctx context.Context) {
	defer close(h.stoppedChan)

	for {
		select {
		case <-ctx.Done():
			h.cleanupAll()
			return
		case <-h.stopChan:
			h.cleanupAll()
			return
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			count := len(h.clients)
			h.mu.Unlock()

			corrID := GenerateCorrelationID()
			h.logger.Info("SSE client connected", corrID, map[string]any{
				"client_id":         client.id,
				"remote_ip":         client.ip,
				"connected_clients": count,
			})

		case client := <-h.unregister:
			h.mu.Lock()
			if _, exists := h.clients[client]; exists {
				delete(h.clients, client)
				client.Close()
			}
			count := len(h.clients)
			h.mu.Unlock()

			corrID := GenerateCorrelationID()
			h.logger.Info("SSE client disconnected", corrID, map[string]any{
				"client_id":         client.id,
				"remote_ip":         client.ip,
				"connected_clients": count,
			})

		case msg := <-h.broadcast:
			h.mu.RLock()
			count := len(h.clients)
			corrID := GenerateCorrelationID()
			h.logger.Info("Broadcasting SSE message to clients", corrID, map[string]any{
				"type":              msg.Type,
				"event_id":          msg.EventID,
				"connected_clients": count,
			})

			for client := range h.clients {
				select {
				case client.sendChan <- msg:
				default:
					// Slow or unresponsive client: drop message to prevent hub stall
					h.logger.Warn("SSE client buffer full, dropping message", corrID, map[string]any{
						"client_id": client.id,
						"remote_ip": client.ip,
					})
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *SSEHub) cleanupAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		delete(h.clients, client)
		client.Close()
	}
}

// Register adds a new client to the hub.
func (h *SSEHub) Register(client *SSEClient) {
	select {
	case h.register <- client:
	case <-h.stopChan:
	}
}

// Unregister removes a client from the hub.
func (h *SSEHub) Unregister(client *SSEClient) {
	select {
	case h.unregister <- client:
	case <-h.stopChan:
	}
}

// Broadcast dispatches a stream message to all connected clients.
func (h *SSEHub) Broadcast(msg StreamMessage) {
	select {
	case h.broadcast <- msg:
	default:
		corrID := GenerateCorrelationID()
		h.logger.Warn("Hub broadcast buffer full; dropping real-time message", corrID, map[string]any{
			"type":     msg.Type,
			"event_id": msg.EventID,
		})
	}
}

// ClientCount returns the current number of registered clients.
func (h *SSEHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Stop terminates the hub.
func (h *SSEHub) Stop() {
	close(h.stopChan)
	<-h.stoppedChan
}

// SSEConnectionLimiter tracks and bounds concurrent SSE connections per client IP.
type SSEConnectionLimiter struct {
	mu          sync.Mutex
	maxPerIP    int
	connections map[string]int
}

// NewSSEConnectionLimiter creates a limiter with the specified per-IP concurrent limit.
func NewSSEConnectionLimiter(maxPerIP int) *SSEConnectionLimiter {
	if maxPerIP <= 0 {
		maxPerIP = 5
	}
	return &SSEConnectionLimiter{
		maxPerIP:    maxPerIP,
		connections: make(map[string]int),
	}
}

// Acquire attempts to claim a connection slot for the given IP. Returns true if permitted.
func (l *SSEConnectionLimiter) Acquire(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	current := l.connections[ip]
	if current >= l.maxPerIP {
		return false
	}
	l.connections[ip] = current + 1
	return true
}

// Release releases a connection slot for the given IP.
func (l *SSEConnectionLimiter) Release(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	current := l.connections[ip]
	if current <= 1 {
		delete(l.connections, ip)
	} else {
		l.connections[ip] = current - 1
	}
}

// Count returns active connections for a given IP.
func (l *SSEConnectionLimiter) Count(ip string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.connections[ip]
}

func generateClientID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// StreamHandler handles GET /api/v1/stream Server-Sent Events requests.
func StreamHandler(hub *SSEHub, limiter *SSEConnectionLimiter, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported by response writer", http.StatusInternalServerError)
			return
		}

		clientIP := getClientIP(r)
		corrID := r.Header.Get("X-Correlation-ID")
		if corrID == "" {
			corrID = GenerateCorrelationID()
			r.Header.Set("X-Correlation-ID", corrID)
		}

		// Enforce per-IP concurrent connection limit
		if !limiter.Acquire(clientIP) {
			logger.Warn("SSE connection limit exceeded for IP", corrID, map[string]any{
				"remote_ip": clientIP,
				"limit":     limiter.maxPerIP,
			})
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":       "too many concurrent stream connections",
				"retry_after": 5,
			})
			return
		}
		defer limiter.Release(clientIP)

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Disable response write timeout for streaming connection if supported
		rc := http.NewResponseController(w)
		_ = rc.SetWriteDeadline(time.Time{})

		// Flush initial SSE comment to confirm stream establishment to client
		if _, err := fmt.Fprintf(w, ": connected\n\n"); err != nil {
			return
		}
		flusher.Flush()

		client := &SSEClient{
			id:       generateClientID(),
			ip:       clientIP,
			sendChan: make(chan StreamMessage, 32),
		}

		hub.Register(client)
		defer hub.Unregister(client)

		// Heartbeat ticker to prevent proxies / NATs from dropping idle SSE streams
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case msg, ok := <-client.sendChan:
				if !ok {
					return
				}
				payload, err := json.Marshal(msg)
				if err != nil {
					continue
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", string(payload)); err != nil {
					return
				}
				flusher.Flush()
			case <-ticker.C:
				if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}
