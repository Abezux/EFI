package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ProcessSingleStableEvent enriches a single stable event with AI summary, category, and named entities.
func ProcessSingleStableEvent(
	ctx context.Context,
	store *Store,
	llm LLMClient,
	event *StableEvent,
	catMap map[string]int,
	catNames []string,
	cfg *Config,
	logger *Logger,
	corrID string,
) error {
	// Step 1: Fetch all source texts associated with this event
	sourceTexts, err := store.FetchEventSourceTexts(ctx, event.ID)
	if err != nil {
		return fmt.Errorf("fetch source texts for event %d: %w", event.ID, err)
	}
	if len(sourceTexts) == 0 {
		logger.Warn("Event has no source texts; skipping enrichment", corrID, map[string]any{
			"event_id": event.ID,
		})
		return nil
	}

	// Step 2: Call LLM with bounded retries
	var result *EnrichmentResult
	var llmErr error
	for attempt := 1; attempt <= cfg.MaxLLMRetries; attempt++ {
		start := time.Now()
		result, llmErr = llm.EnrichEvent(ctx, sourceTexts, catNames)
		latency := time.Since(start).Milliseconds()

		if llmErr == nil {
			logger.Debug("LLM enrichment call succeeded", corrID, map[string]any{
				"event_id":   event.ID,
				"attempt":    attempt,
				"latency_ms": latency,
				"category":   result.Category,
				"entities":   len(result.Entities),
			})
			break
		}

		logger.Warn(fmt.Sprintf("LLM enrichment attempt %d/%d failed: %v", attempt, cfg.MaxLLMRetries, llmErr), corrID, map[string]any{
			"event_id":   event.ID,
			"attempt":    attempt,
			"latency_ms": latency,
		})

		if attempt < cfg.MaxLLMRetries {
			backoff := time.Duration(200*(1<<attempt)) * time.Millisecond
			if strings.Contains(fmt.Sprintf("%v", llmErr), "429") || strings.Contains(fmt.Sprintf("%v", llmErr), "RESOURCE_EXHAUSTED") {
				backoff = 21 * time.Second
			}
			time.Sleep(backoff)
		}
	}

	if llmErr != nil || result == nil {
		logger.Error(fmt.Sprintf("LLM enrichment failed after %d retries; event %d remains published without summary", cfg.MaxLLMRetries, event.ID), corrID, map[string]any{
			"event_id": event.ID,
			"error":    fmt.Sprintf("%v", llmErr),
		})
		// Graceful degradation per ADR/Spec: do NOT fail pipeline, event remains published
		return nil
	}

	// Step 3: Match category name to category_id
	var categoryID *int
	matchedCat := strings.TrimSpace(result.Category)
	if id, ok := catMap[matchedCat]; ok {
		categoryID = &id
	} else {
		// Case-insensitive match attempt
		for name, id := range catMap {
			if strings.EqualFold(name, matchedCat) {
				val := id
				categoryID = &val
				break
			}
		}
		// Fallback to "General Economy" if present
		if categoryID == nil {
			if genID, ok := catMap["General Economy"]; ok {
				val := genID
				categoryID = &val
			}
		}
	}

	// Step 4: Persist enrichment atomically with processing_audit
	audit := AuditEntry{
		NewsEventID: &event.ID,
		Stage:       "enrichment",
		Decision:    "summary_generated",
		Confidence:  1.0,
		ModelUsed:   llm.ModelName(),
		RawResponse: result.RawResponse,
	}

	if err := store.SaveEnrichmentWithAudit(ctx, event.ID, categoryID, result.AISummary, result.Entities, audit); err != nil {
		return fmt.Errorf("save enrichment with audit for event %d: %w", event.ID, err)
	}

	logger.Info("Event enriched successfully with AI summary and entities", corrID, map[string]any{
		"event_id":       event.ID,
		"category":       result.Category,
		"category_id":    categoryID,
		"entities_count": len(result.Entities),
		"ai_summary":     result.AISummary,
	})

	return nil
}
