package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestSSEHub_LifecycleAndBroadcast(t *testing.T) {
	logger := NewLogger("ERROR")
	hub := NewSSEHub(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Run(ctx)

	if hub.ClientCount() != 0 {
		t.Fatalf("expected initial client count 0, got %d", hub.ClientCount())
	}

	client1 := &SSEClient{
		id:       "client-1",
		ip:       "127.0.0.1",
		sendChan: make(chan StreamMessage, 10),
	}
	client2 := &SSEClient{
		id:       "client-2",
		ip:       "127.0.0.1",
		sendChan: make(chan StreamMessage, 10),
	}

	hub.Register(client1)
	hub.Register(client2)

	// Wait briefly for register channel processing
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 2 {
		t.Fatalf("expected client count 2, got %d", hub.ClientCount())
	}

	// Broadcast test message
	msg := StreamMessage{
		Type:    "new_event",
		EventID: 42,
	}
	hub.Broadcast(msg)

	// Assert client 1 received
	select {
	case received := <-client1.sendChan:
		if received.Type != "new_event" || received.EventID != 42 {
			t.Fatalf("unexpected message on client1: %+v", received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for message on client1")
	}

	// Assert client 2 received
	select {
	case received := <-client2.sendChan:
		if received.Type != "new_event" || received.EventID != 42 {
			t.Fatalf("unexpected message on client2: %+v", received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for message on client2")
	}

	// Unregister client 1
	hub.Unregister(client1)
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Fatalf("expected client count 1 after unregister, got %d", hub.ClientCount())
	}

	// Broadcast update message
	msg2 := StreamMessage{
		Type:    "event_updated",
		EventID: 42,
	}
	hub.Broadcast(msg2)

	select {
	case received := <-client2.sendChan:
		if received.Type != "event_updated" || received.EventID != 42 {
			t.Fatalf("unexpected message on client2: %+v", received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for message on client2")
	}

	// Clean stop
	hub.Stop()
}

func TestSSEConnectionLimiter(t *testing.T) {
	limiter := NewSSEConnectionLimiter(2)

	ip := "192.168.1.50"

	// Connection 1: Allowed
	if !limiter.Acquire(ip) {
		t.Fatal("expected connection 1 to be permitted")
	}
	if count := limiter.Count(ip); count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	// Connection 2: Allowed
	if !limiter.Acquire(ip) {
		t.Fatal("expected connection 2 to be permitted")
	}
	if count := limiter.Count(ip); count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	// Connection 3: Denied (exceeds limit 2)
	if limiter.Acquire(ip) {
		t.Fatal("expected connection 3 to be rejected by limiter")
	}

	// Release connection 1
	limiter.Release(ip)
	if count := limiter.Count(ip); count != 1 {
		t.Fatalf("expected count 1 after release, got %d", count)
	}

	// Connection 3 retry: Allowed
	if !limiter.Acquire(ip) {
		t.Fatal("expected connection to succeed after release")
	}

	// Release all
	limiter.Release(ip)
	limiter.Release(ip)
	if count := limiter.Count(ip); count != 0 {
		t.Fatalf("expected count 0 after full release, got %d", count)
	}
}

func TestStreamHandler_SSEHeadersAndStreaming(t *testing.T) {
	logger := NewLogger("ERROR")
	hub := NewSSEHub(logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	defer hub.Stop()

	limiter := NewSSEConnectionLimiter(5)
	handler := StreamHandler(hub, limiter, logger)

	server := httptest.NewServer(handler)
	defer server.Close()

	// Connect SSE client
	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to stream handler: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("unexpected content type: %q", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Cache-Control") != "no-cache, no-transform" {
		t.Fatalf("unexpected cache-control: %q", resp.Header.Get("Cache-Control"))
	}

	reader := bufio.NewReader(resp.Body)

	// Line 1 should be initial comment ": connected"
	line1, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read initial comment: %v", err)
	}
	if strings.TrimSpace(line1) != ": connected" {
		t.Fatalf("expected ': connected', got %q", line1)
	}

	// Wait for client to be registered in hub
	time.Sleep(50 * time.Millisecond)
	if hub.ClientCount() != 1 {
		t.Fatalf("expected 1 registered hub client, got %d", hub.ClientCount())
	}

	// Broadcast new event
	hub.Broadcast(StreamMessage{Type: "new_event", EventID: 99})

	// Read stream line until data line
	var dataLine string
	for i := 0; i < 5; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read stream data: %v", err)
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data: ") {
			dataLine = trimmed
			break
		}
	}

	if dataLine == "" {
		t.Fatal("did not receive data line from SSE stream")
	}

	payloadJSON := strings.TrimPrefix(dataLine, "data: ")
	var msg StreamMessage
	if err := json.Unmarshal([]byte(payloadJSON), &msg); err != nil {
		t.Fatalf("failed to unmarshal stream data %q: %v", payloadJSON, err)
	}
	if msg.Type != "new_event" || msg.EventID != 99 {
		t.Fatalf("unexpected stream message content: %+v", msg)
	}
}

func TestStreamHandler_ConnectionLimiterEnforcement(t *testing.T) {
	logger := NewLogger("ERROR")
	hub := NewSSEHub(logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	defer hub.Stop()

	// Set limit to 1 per IP
	limiter := NewSSEConnectionLimiter(1)
	handler := StreamHandler(hub, limiter, logger)

	server := httptest.NewServer(handler)
	defer server.Close()

	// Connection 1
	reqCtx1, reqCancel1 := context.WithCancel(context.Background())
	defer reqCancel1()

	req1, _ := http.NewRequestWithContext(reqCtx1, http.MethodGet, server.URL, nil)
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("connection 1 failed: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("expected connection 1 status 200, got %d", resp1.StatusCode)
	}

	// Connection 2 from same IP should receive HTTP 429 Too Many Requests
	req2, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("connection 2 failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 for second concurrent connection, got %d", resp2.StatusCode)
	}
	if resp2.Header.Get("Retry-After") != "5" {
		t.Errorf("expected Retry-After header '5', got %q", resp2.Header.Get("Retry-After"))
	}
}

func TestNotifyListener_HandleNotification(t *testing.T) {
	logger := NewLogger("ERROR")
	hub := NewSSEHub(logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	defer hub.Stop()

	client := &SSEClient{
		id:       "test-client",
		ip:       "127.0.0.1",
		sendChan: make(chan StreamMessage, 10),
	}
	hub.Register(client)
	time.Sleep(30 * time.Millisecond)

	nl := &NotifyListener{
		channelName: "news_events_channel",
		hub:         hub,
		logger:      logger,
	}

	// 1. Valid new_event
	n1 := &pq.Notification{
		Channel: "news_events_channel",
		Extra:   `{"type":"new_event","event_id":77}`,
	}
	nl.handleNotification(n1)

	select {
	case msg := <-client.sendChan:
		if msg.Type != "new_event" || msg.EventID != 77 {
			t.Fatalf("unexpected message: %+v", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for message forwarding")
	}

	// 2. Valid event_updated
	n2 := &pq.Notification{
		Channel: "news_events_channel",
		Extra:   `{"type":"event_updated","event_id":77}`,
	}
	nl.handleNotification(n2)

	select {
	case msg := <-client.sendChan:
		if msg.Type != "event_updated" || msg.EventID != 77 {
			t.Fatalf("unexpected message: %+v", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for message forwarding")
	}

	// 3. Malformed payload should be ignored safely without panic
	n3 := &pq.Notification{
		Channel: "news_events_channel",
		Extra:   `invalid json`,
	}
	nl.handleNotification(n3)

	// 4. Invalid event_id <= 0 should be ignored safely
	n4 := &pq.Notification{
		Channel: "news_events_channel",
		Extra:   `{"type":"new_event","event_id":0}`,
	}
	nl.handleNotification(n4)
}
