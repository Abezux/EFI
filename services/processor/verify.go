package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// VerificationOutcome represents the result of evaluating a needs_review post.
type VerificationOutcome string

const (
	OutcomeAttachedToEvent VerificationOutcome = "attached_to_event"
	OutcomeCreatedNewEvent VerificationOutcome = "created_new_event"
	OutcomeRemainsInReview VerificationOutcome = "remains_in_review"
	OutcomeSkippedNoCand   VerificationOutcome = "created_event_no_candidate"
)

// ProcessSingleNeedsReviewPost verifies a single needs_review post against candidate events.
func ProcessSingleNeedsReviewPost(
	ctx context.Context,
	store *Store,
	llm LLMClient,
	post *RawPost,
	cfg *Config,
	logger *Logger,
	corrID string,
) (VerificationOutcome, error) {
	// Step 1: Find nearest event candidate
	candidate, similarity, err := store.FindNearestCandidateEvent(ctx, post.ID, cfg.ClusteringWindow)
	if err != nil {
		return OutcomeRemainsInReview, fmt.Errorf("find candidate for post %d: %w", post.ID, err)
	}

	// If no candidate exists within clustering window, create a new event
	if candidate == nil {
		canonicalTitle := GenerateCanonicalTitle(post.RawText)
		audit := AuditEntry{
			RawPostID:   &post.ID,
			Stage:       "verification",
			Decision:    "different_event",
			Confidence:  1.0,
			ModelUsed:   "none_no_candidate",
			RawResponse: "No candidate event found within clustering window; created initial event",
		}
		eventID, err := store.CreateEventWithAudit(ctx, post, canonicalTitle, audit)
		if err != nil {
			return OutcomeRemainsInReview, fmt.Errorf("create event for post %d without candidate: %w", post.ID, err)
		}
		logger.Info("Created new event for needs_review post (no candidates in window)", corrID, map[string]any{
			"raw_post_id": post.ID,
			"event_id":    eventID,
		})
		return OutcomeSkippedNoCand, nil
	}

	// Step 2: Assemble candidate context for the narrow verification prompt
	var candidateTextBuilder strings.Builder
	candidateTextBuilder.WriteString(fmt.Sprintf("Event Title: %s\n", candidate.CanonicalTitle))
	for i, src := range candidate.Sources {
		if i >= 3 {
			break // include up to 3 representative source snippets
		}
		if src.NormalizedText != "" {
			candidateTextBuilder.WriteString(fmt.Sprintf("Source %d: %s\n", i+1, src.NormalizedText))
		}
	}
	candidateContext := candidateTextBuilder.String()

	postText := post.RawText
	if pNorm := NormalizeText(post.RawText); pNorm != "" {
		postText = pNorm
	}

	// Step 3: Call LLM with retry
	var result *VerificationResult
	var llmErr error
	for attempt := 1; attempt <= cfg.MaxLLMRetries; attempt++ {
		start := time.Now()
		result, llmErr = llm.VerifySameEvent(ctx, postText, candidateContext)
		latency := time.Since(start).Milliseconds()

		if llmErr == nil {
			logger.Debug("LLM verification call succeeded", corrID, map[string]any{
				"raw_post_id":  post.ID,
				"candidate_id": candidate.EventID,
				"attempt":      attempt,
				"latency_ms":   latency,
				"decision":     result.Decision,
				"confidence":   result.Confidence,
			})
			break
		}

		logger.Warn(fmt.Sprintf("LLM verification attempt %d/%d failed: %v", attempt, cfg.MaxLLMRetries, llmErr), corrID, map[string]any{
			"raw_post_id": post.ID,
			"attempt":     attempt,
			"latency_ms":  latency,
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
		logger.Error(fmt.Sprintf("LLM verification failed after %d retries; post %d remains in needs_review", cfg.MaxLLMRetries, post.ID), corrID, map[string]any{
			"raw_post_id": post.ID,
			"error":       fmt.Sprintf("%v", llmErr),
		})
		return OutcomeRemainsInReview, nil
	}

	// Step 4: Evaluate verification decision against confidence threshold
	audit := AuditEntry{
		RawPostID:   &post.ID,
		NewsEventID: &candidate.EventID,
		Stage:       "verification",
		Decision:    result.Decision,
		Confidence:  result.Confidence,
		ModelUsed:   llm.ModelName(),
		RawResponse: result.RawResponse,
	}

	// Rule 1: High-confidence "same_event" -> Attach to candidate event
	if result.Decision == "same_event" && result.Confidence >= cfg.VerifyConfidenceThreshold {
		if err := store.AttachToEventWithAudit(ctx, candidate.EventID, post.ID, post.PostedAt, audit); err != nil {
			return OutcomeRemainsInReview, fmt.Errorf("attach verified post %d to event %d: %w", post.ID, candidate.EventID, err)
		}
		logger.Info("LLM verified same_event: attached to event", corrID, map[string]any{
			"raw_post_id": post.ID,
			"event_id":    candidate.EventID,
			"confidence":  result.Confidence,
			"cosine_sim":  similarity,
			"decision":    "attached_to_event",
			"reasoning":   result.Reasoning,
		})
		return OutcomeAttachedToEvent, nil
	}

	// Rule 2: High-confidence "different_event" -> Create new event
	if result.Decision == "different_event" && result.Confidence >= cfg.VerifyConfidenceThreshold {
		canonicalTitle := GenerateCanonicalTitle(post.RawText)
		eventID, err := store.CreateEventWithAudit(ctx, post, canonicalTitle, audit)
		if err != nil {
			return OutcomeRemainsInReview, fmt.Errorf("create verified new event for post %d: %w", post.ID, err)
		}
		logger.Info("LLM verified different_event: created new event", corrID, map[string]any{
			"raw_post_id": post.ID,
			"event_id":    eventID,
			"confidence":  result.Confidence,
			"cosine_sim":  similarity,
			"decision":    "created_new_event",
			"reasoning":   result.Reasoning,
		})
		return OutcomeCreatedNewEvent, nil
	}

	// Rule 3: Low confidence or uncertain -> NEVER force decision, leave in needs_review, write audit row
	audit.Decision = "low_confidence_unresolved"
	if err := store.RecordAudit(ctx, audit); err != nil {
		logger.Error("Failed to record audit for low-confidence verification", corrID, map[string]any{
			"raw_post_id": post.ID,
			"error":       err.Error(),
		})
	}

	logger.Warn("Low-confidence LLM verification result; post remains in needs_review without force-deciding", corrID, map[string]any{
		"raw_post_id": post.ID,
		"decision":    result.Decision,
		"confidence":  result.Confidence,
		"threshold":   cfg.VerifyConfidenceThreshold,
		"reasoning":   result.Reasoning,
	})

	return OutcomeRemainsInReview, nil
}
