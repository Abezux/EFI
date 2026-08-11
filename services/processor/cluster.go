package main

import (
	"time"
)

// DecisionType represents the clustering decision for a raw post.
type DecisionType string

const (
	DecisionCreateNew DecisionType = "create_new"
	DecisionAttach    DecisionType = "attach"
)

// ClusteringDecision encapsulates the decision made for a post.
type ClusteringDecision struct {
	Type            DecisionType
	TargetEventID   int64
	SimilarityScore int
	CanonicalTitle  string
}

// EventSourceMember represents an existing raw post attached to an event candidate.
type EventSourceMember struct {
	RawPostID      int64
	NormalizedText string
	Simhash        int64
}

// EventCandidate represents an active news_event candidate within the time window.
type EventCandidate struct {
	EventID        int64
	CanonicalTitle string
	FirstSeenAt    time.Time
	LastUpdatedAt  time.Time
	Sources        []EventSourceMember
}

// truncateTitle creates a canonical title from normalized text up to maxRunes.
func truncateTitle(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

// DecideClustering evaluates a post against active candidate events within the time window:
//  1. Exact Duplicate: If normalized text exactly matches an existing source in a candidate event,
//     attaches with similarity score 0.
//  2. Near Duplicate: Evaluates Hamming distance against all sources of each candidate event.
//     If the minimum distance is <= threshold, attaches to the best-matching event.
//  3. Otherwise: Creates a new news_events row with canonical_title set to truncated normalized text.
func DecideClustering(
	postID int64,
	normalizedText string,
	simhash int64,
	candidates []*EventCandidate,
	threshold int,
) *ClusteringDecision {
	// 1. Exact-match check
	for _, candidate := range candidates {
		for _, source := range candidate.Sources {
			if source.NormalizedText != "" && source.NormalizedText == normalizedText {
				return &ClusteringDecision{
					Type:            DecisionAttach,
					TargetEventID:   candidate.EventID,
					SimilarityScore: 0,
				}
			}
		}
	}

	// 2. Near-duplicate Simhash Hamming distance check
	var bestCandidate *EventCandidate
	bestDistance := 65 // Larger than any 64-bit Hamming distance

	for _, candidate := range candidates {
		for _, source := range candidate.Sources {
			if source.Simhash == 0 {
				continue
			}
			dist := HammingDistance(simhash, source.Simhash)
			if dist < bestDistance {
				bestDistance = dist
				bestCandidate = candidate
			}
		}
	}

	if bestCandidate != nil && bestDistance <= threshold {
		return &ClusteringDecision{
			Type:            DecisionAttach,
			TargetEventID:   bestCandidate.EventID,
			SimilarityScore: bestDistance,
		}
	}

	// 3. No match -> Create new event
	title := truncateTitle(normalizedText, 120)
	if title == "" {
		title = "Untitled Event"
	}

	return &ClusteringDecision{
		Type:           DecisionCreateNew,
		CanonicalTitle: title,
	}
}
