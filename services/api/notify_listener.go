package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/lib/pq"
)

// NotifyListener manages a persistent PostgreSQL LISTEN connection on news_events_channel.
type NotifyListener struct {
	dbURL       string
	channelName string
	hub         *SSEHub
	logger      *Logger
	listener    *pq.Listener
	stopOnce    sync.Once
	stopChan    chan struct{}
	stoppedChan chan struct{}
}

// NewNotifyListener initializes a NotifyListener with reconnection backoff parameters.
func NewNotifyListener(dbURL string, channelName string, hub *SSEHub, logger *Logger) *NotifyListener {
	if channelName == "" {
		channelName = "news_events_channel"
	}

	nl := &NotifyListener{
		dbURL:       dbURL,
		channelName: channelName,
		hub:         hub,
		logger:      logger,
		stopChan:    make(chan struct{}),
		stoppedChan: make(chan struct{}),
	}

	eventCallback := func(event pq.ListenerEventType, err error) {
		corrID := GenerateCorrelationID()
		switch event {
		case pq.ListenerEventConnected:
			logger.Info("Postgres LISTEN subscriber connected to channel", corrID, map[string]any{
				"channel": channelName,
			})
		case pq.ListenerEventDisconnected:
			logger.Warn("Postgres LISTEN subscriber disconnected; attempting automatic reconnection", corrID, map[string]any{
				"channel": channelName,
				"error":   fmt.Sprintf("%v", err),
			})
		case pq.ListenerEventReconnected:
			logger.Info("Postgres LISTEN subscriber reconnected successfully", corrID, map[string]any{
				"channel": channelName,
			})
		case pq.ListenerEventConnectionAttemptFailed:
			logger.Error("Postgres LISTEN subscriber connection attempt failed; backing off", corrID, map[string]any{
				"channel": channelName,
				"error":   fmt.Sprintf("%v", err),
			})
		}
	}

	// Min reconnect 2s, max reconnect 30s as specified in ADR-0011 / spec
	nl.listener = pq.NewListener(dbURL, 2*time.Second, 30*time.Second, eventCallback)
	return nl
}

// Start begins listening on the channel and processing incoming notifications.
func (nl *NotifyListener) Start(ctx context.Context) error {
	initCorrID := GenerateCorrelationID()
	if err := nl.listener.Listen(nl.channelName); err != nil {
		nl.logger.Error("Failed to issue LISTEN command on postgres channel", initCorrID, map[string]any{
			"channel": nl.channelName,
			"error":   err.Error(),
		})
		return fmt.Errorf("listen %q: %w", nl.channelName, err)
	}

	nl.logger.Info("Postgres LISTEN subscriber started", initCorrID, map[string]any{
		"channel": nl.channelName,
	})

	go nl.eventLoop(ctx)
	return nil
}

func (nl *NotifyListener) eventLoop(ctx context.Context) {
	defer close(nl.stoppedChan)

	for {
		select {
		case <-ctx.Done():
			_ = nl.listener.Close()
			return
		case <-nl.stopChan:
			_ = nl.listener.Close()
			return
		case notification, ok := <-nl.listener.NotificationChannel():
			if !ok {
				nl.logger.Warn("Postgres notification channel closed", GenerateCorrelationID(), nil)
				return
			}
			if notification == nil {
				// pq.Listener periodically sends nil as a ping/heartbeat
				continue
			}

			nl.handleNotification(notification)
		}
	}
}

func (nl *NotifyListener) handleNotification(n *pq.Notification) {
	corrID := GenerateCorrelationID()

	var msg StreamMessage
	if err := json.Unmarshal([]byte(n.Extra), &msg); err != nil {
		nl.logger.Warn("Received invalid JSON payload in Postgres notification", corrID, map[string]any{
			"channel": n.Channel,
			"payload": n.Extra,
			"error":   err.Error(),
		})
		return
	}

	if msg.Type != "new_event" && msg.Type != "event_updated" {
		nl.logger.Warn("Received unrecognized message type in Postgres notification", corrID, map[string]any{
			"type":     msg.Type,
			"event_id": msg.EventID,
		})
		return
	}

	if msg.EventID <= 0 {
		nl.logger.Warn("Received invalid event_id in Postgres notification", corrID, map[string]any{
			"event_id": msg.EventID,
			"type":     msg.Type,
		})
		return
	}

	nl.logger.Info("Forwarding Postgres notification to SSE hub", corrID, map[string]any{
		"channel":  n.Channel,
		"type":     msg.Type,
		"event_id": msg.EventID,
	})

	if nl.hub != nil {
		nl.hub.Broadcast(msg)
	}
}

// Close gracefully stops the Postgres listener.
func (nl *NotifyListener) Close() error {
	nl.stopOnce.Do(func() {
		close(nl.stopChan)
		_ = nl.listener.Close()
	})
	<-nl.stoppedChan
	return nil
}
