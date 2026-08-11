package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// StructuredLog represents the standard JSON log format defined in AGENTS.md.
type StructuredLog struct {
	Timestamp     string         `json:"timestamp"`
	Level         string         `json:"level"`
	Service       string         `json:"service"`
	Message       string         `json:"message"`
	CorrelationID string         `json:"correlation_id"`
	Extra         map[string]any `json:"extra,omitempty"`
}

// GenerateCorrelationID generates a unique hexadecimal correlation identifier.
func GenerateCorrelationID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("corr-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// Logger provides structured logging with level filtering.
type Logger struct {
	level string
}

// NewLogger creates a new Logger instance for the API service.
func NewLogger(level string) *Logger {
	return &Logger{
		level: strings.ToUpper(level),
	}
}

func (l *Logger) shouldLog(msgLevel string) bool {
	levels := map[string]int{
		"DEBUG": 0,
		"INFO":  1,
		"WARN":  2,
		"ERROR": 3,
	}
	currentThreshold, ok := levels[l.level]
	if !ok {
		currentThreshold = 1 // Default to INFO
	}
	targetLevel, ok := levels[msgLevel]
	if !ok {
		targetLevel = 1
	}
	return targetLevel >= currentThreshold
}

func (l *Logger) log(level, message, correlationID string, extra map[string]any) {
	if !l.shouldLog(level) {
		return
	}
	if correlationID == "" {
		correlationID = GenerateCorrelationID()
	}
	entry := StructuredLog{
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Level:         level,
		Service:       "api",
		Message:       message,
		CorrelationID: correlationID,
		Extra:         extra,
	}
	data, err := json.Marshal(entry)
	if err == nil {
		fmt.Fprintln(os.Stdout, string(data))
	}
}

// Debug logs a debug-level message.
func (l *Logger) Debug(message, correlationID string, extra map[string]any) {
	l.log("DEBUG", message, correlationID, extra)
}

// Info logs an info-level message.
func (l *Logger) Info(message, correlationID string, extra map[string]any) {
	l.log("INFO", message, correlationID, extra)
}

// Warn logs a warning-level message.
func (l *Logger) Warn(message, correlationID string, extra map[string]any) {
	l.log("WARN", message, correlationID, extra)
}

// Error logs an error-level message.
func (l *Logger) Error(message, correlationID string, extra map[string]any) {
	l.log("ERROR", message, correlationID, extra)
}
