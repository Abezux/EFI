package main

import (
	"math"
	"time"
)

// DecisionType represents the clustering decision for a raw post.
type DecisionType string

const (
	DecisionCreateNew   DecisionType = "create_new"
	DecisionAttach      DecisionType = "attach"
	DecisionNeedsReview DecisionType = "needs_review"
)

// ClusteringDecision encapsulates the decision made for a post.
type ClusteringDecision struct {
	Type            DecisionType
	TargetEventID   int64
	SimilarityScore int
	MatchReason     string // "exact_text", "simhash", "embedding", "embedding_ambiguous", "new_event"
	CosineScore     float64
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
	EventID           int64
	CanonicalTitle    string
	FirstSeenAt       time.Time
	LastUpdatedAt     time.Time
	FoundingRawPostID int64
	FoundingEmbedding []float32
	EmbeddingCentroid []float32
	Sources           []EventSourceMember
}

// truncateTitle creates a canonical title from normalized text up to maxRunes.
func truncateTitle(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

// GenerateCanonicalTitle generates a short title from raw or normalized text.
func GenerateCanonicalTitle(text string) string {
	return truncateTitle(text, 120)
}

// DecideClustering evaluates a post against active candidate events within the time window:
//  1. Exact Duplicate: If normalized text exactly matches an existing source in a candidate event,
//     attaches immediately with similarity score 0 and match_reason="exact_text".
//  2. Near Duplicate (Simhash): If minimum Hamming distance is <= simhashThreshold (e.g. 10),
//     attaches immediately to the best-matching event with match_reason="simhash".
//  3. Semantic Similarity (Anti-Drift Embedding Check): If post embedding is available and candidate has an embedding centroid:
//     - Compares post embedding against BOTH candidate's centroid AND candidate's founding post embedding.
//     - If CosineSimilarity >= highThreshold against BOTH centroid and founding post:
//       attaches with match_reason="embedding".
//     - If CosineSimilarity >= lowThreshold against BOTH centroid and founding post (ambiguous band):
//       routes to "needs_review" with match_reason="embedding_ambiguous".
//     - If similarity against either falls below lowThreshold: does not attach (creates a new news_event).
//  4. Otherwise: Creates a new news_events row with canonical_title set to truncated normalized text.
func DecideClustering(
	postID int64,
	normalizedText string,
	simhash int64,
	embedding []float32,
	candidates []*EventCandidate,
	simhashThreshold int,
	embeddingHighThreshold float64,
	embeddingLowThreshold float64,
) *ClusteringDecision {
	// 1. Exact-match check
	for _, candidate := range candidates {
		for _, source := range candidate.Sources {
			if source.NormalizedText != "" && source.NormalizedText == normalizedText {
				return &ClusteringDecision{
					Type:            DecisionAttach,
					TargetEventID:   candidate.EventID,
					SimilarityScore: 0,
					MatchReason:     "exact_text",
				}
			}
		}
	}

	// 2. Near-duplicate Simhash Hamming distance check (Fast Path)
	var bestSimhashCandidate *EventCandidate
	bestDistance := 65 // Larger than any 64-bit Hamming distance

	for _, candidate := range candidates {
		for _, source := range candidate.Sources {
			if source.Simhash == 0 {
				continue
			}
			dist := HammingDistance(simhash, source.Simhash)
			if dist < bestDistance {
				bestDistance = dist
				bestSimhashCandidate = candidate
			}
		}
	}

	if bestSimhashCandidate != nil && bestDistance <= simhashThreshold {
		return &ClusteringDecision{
			Type:            DecisionAttach,
			TargetEventID:   bestSimhashCandidate.EventID,
			SimilarityScore: bestDistance,
			MatchReason:     "simhash",
		}
	}

	// 3. Semantic Embedding Cosine Similarity Check with Anti-Drift Dual Verification (Fallback Path)
	if len(embedding) > 0 && len(candidates) > 0 {
		var bestSemanticCandidate *EventCandidate
		bestEffectiveScore := -1.0
		var bestCentroidScore float64
		var bestFoundingScore float64

		for _, candidate := range candidates {
			if len(candidate.EmbeddingCentroid) == 0 {
				continue
			}
			cosCentroid := CosineSimilarity(embedding, candidate.EmbeddingCentroid)
			cosFounding := cosCentroid
			if len(candidate.FoundingEmbedding) > 0 {
				cosFounding = CosineSimilarity(embedding, candidate.FoundingEmbedding)
			}

			effectiveScore := math.Min(cosCentroid, cosFounding)
			if effectiveScore > bestEffectiveScore {
				bestEffectiveScore = effectiveScore
				bestCentroidScore = cosCentroid
				bestFoundingScore = cosFounding
				bestSemanticCandidate = candidate
			}
		}

		if bestSemanticCandidate != nil {
			// Confidently same event: must meet high threshold against both centroid and founding post
			if bestCentroidScore >= embeddingHighThreshold && bestFoundingScore >= embeddingHighThreshold {
				return &ClusteringDecision{
					Type:            DecisionAttach,
					TargetEventID:   bestSemanticCandidate.EventID,
					SimilarityScore: int(math.Round(bestEffectiveScore * 100)),
					CosineScore:     bestEffectiveScore,
					MatchReason:     "embedding",
				}
			}

			// Ambiguous band: both scores at least low threshold -> needs_review
			if bestCentroidScore >= embeddingLowThreshold && bestFoundingScore >= embeddingLowThreshold {
				return &ClusteringDecision{
					Type:            DecisionNeedsReview,
					TargetEventID:   bestSemanticCandidate.EventID,
					SimilarityScore: int(math.Round(bestEffectiveScore * 100)),
					CosineScore:     bestEffectiveScore,
					MatchReason:     "embedding_ambiguous",
				}
			}
		}
	}

	// 4. No match or below low threshold -> Create new event
	title := truncateTitle(normalizedText, 120)
	if title == "" {
		title = "Untitled Event"
	}

	return &ClusteringDecision{
		Type:           DecisionCreateNew,
		CanonicalTitle: title,
		MatchReason:    "new_event",
	}
}
