package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type FixturePost struct {
	ChannelID     int64  `json:"channel_id"`
	ChannelHandle string `json:"channel_handle"`
	MessageID     int64  `json:"message_id"`
	RawText       string `json:"raw_text"`
	PostedAt      string `json:"posted_at"`
}

func loadFixtures(t *testing.T) []FixturePost {
	t.Helper()
	path := filepath.Join("..", "..", "tests", "fixtures", "sample_telegram_posts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture file at %s: %v", path, err)
	}

	var posts []FixturePost
	if err := json.Unmarshal(data, &posts); err != nil {
		t.Fatalf("failed to parse fixture json: %v", err)
	}

	if len(posts) < 5 {
		t.Fatalf("expected at least 5 fixture posts, got %d", len(posts))
	}

	return posts
}

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "lowercase English",
			input:    "The National Bank OF Ethiopia",
			expected: "the national bank of ethiopia",
		},
		{
			name:     "strip URLs",
			input:    "Breaking news https://t.me/news_et/123 check www.ethiopia.gov.et/update for info",
			expected: "breaking news check for info",
		},
		{
			name:     "strip Markdown formatting",
			input:    "*Bold* _Italic_ ~Strike~ `Code` [Link Text](https://example.com)",
			expected: "bold italic strike code link text",
		},
		{
			name:     "collapse whitespace, newlines, and punctuation",
			input:    "  First line.\n\nSecond line.   Third   word!  \n",
			expected: "first line second line third word",
		},
		{
			name:     "preserve Amharic characters",
			input:    "ሰበር ዜና፡ **የነዳጅ ዋጋ** ማስተካከያ ተደረገ!",
			expected: "ሰበር ዜና የነዳጅ ዋጋ ማስተካከያ ተደረገ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeText(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeText(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSimhash(t *testing.T) {
	fixtures := loadFixtures(t)

	norm0 := NormalizeText(fixtures[0].RawText)
	norm2 := NormalizeText(fixtures[2].RawText)
	norm3 := NormalizeText(fixtures[3].RawText)

	hash0 := ComputeSimhash(norm0)
	hash2 := ComputeSimhash(norm2)
	hash3 := ComputeSimhash(norm3)

	t.Run("identical strings have distance 0", func(t *testing.T) {
		dist := HammingDistance(hash0, hash0)
		if dist != 0 {
			t.Errorf("expected Hamming distance 0 for identical strings, got %d", dist)
		}
	})

	t.Run("lightly edited repost produces low Hamming distance", func(t *testing.T) {
		edited := fixtures[0].RawText + "\n\n@EthiopianNews"
		normEdited := NormalizeText(edited)
		hashEdited := ComputeSimhash(normEdited)

		dist := HammingDistance(hash0, hashEdited)
		t.Logf("Hamming distance for lightly edited forward: %d", dist)
		if dist > 10 {
			t.Errorf("expected distance <= 10 for lightly edited repost, got %d", dist)
		}
	})

	t.Run("distinct stories produce high Hamming distance", func(t *testing.T) {
		dist02 := HammingDistance(hash0, hash2) // Fuel price vs NBE forex
		dist03 := HammingDistance(hash0, hash3) // Fuel price vs Coffee export
		dist23 := HammingDistance(hash2, hash3) // NBE forex vs Coffee export

		if dist02 <= 10 {
			t.Errorf("expected distance > 10 between fuel price and NBE forex, got %d", dist02)
		}
		if dist03 <= 10 {
			t.Errorf("expected distance > 10 between fuel price and coffee export, got %d", dist03)
		}
		if dist23 <= 10 {
			t.Errorf("expected distance > 10 between NBE forex and coffee export, got %d", dist23)
		}
	})
}

func TestClusteringDecision_SimhashFastPath(t *testing.T) {
	fixtures := loadFixtures(t)
	threshold := 10

	// Post 0: ENA fuel price (founding post of Event 1)
	norm0 := NormalizeText(fixtures[0].RawText)
	hash0 := ComputeSimhash(norm0)

	// Step 1: Post 0 processed with no existing candidates -> Creates Event 1
	var candidates []*EventCandidate
	dec0 := DecideClustering(1, norm0, hash0, nil, candidates, threshold, 0.82, 0.65)
	if dec0.Type != DecisionCreateNew {
		t.Fatalf("expected post 0 to create new event, got %v", dec0.Type)
	}

	// Add Event 1 to candidates
	event1 := &EventCandidate{
		EventID:        101,
		CanonicalTitle: dec0.CanonicalTitle,
		FirstSeenAt:    time.Now().Add(-10 * time.Minute),
		LastUpdatedAt:  time.Now().Add(-10 * time.Minute),
		Sources: []EventSourceMember{
			{RawPostID: 1, NormalizedText: norm0, Simhash: hash0},
		},
	}
	candidates = append(candidates, event1)

	// Step 2: Repost of Post 0 (with footer) -> MUST attach to Event 101 via simhash fast path
	repostText := fixtures[0].RawText + "\n\nFollow us on @TeleChannel"
	normRepost := NormalizeText(repostText)
	hashRepost := ComputeSimhash(normRepost)

	decRepost := DecideClustering(2, normRepost, hashRepost, nil, candidates, threshold, 0.82, 0.65)
	if decRepost.Type != DecisionAttach {
		t.Fatalf("expected repost to attach to Event 101, got %v", decRepost.Type)
	}
	if decRepost.TargetEventID != 101 {
		t.Fatalf("expected target event 101, got %d", decRepost.TargetEventID)
	}
	if decRepost.MatchReason != "simhash" {
		t.Fatalf("expected match_reason simhash, got %s", decRepost.MatchReason)
	}
}

// TestClusteringDecision_FuelPriceSemanticMatch verifies the crucial Architecture Spec test case:
// Three differently-worded sentences reporting the same fuel price hike (high Hamming distance > 10)
// are clustered into ONE event via high embedding cosine similarity (>= 0.82).
func TestClusteringDecision_FuelPriceSemanticMatch(t *testing.T) {
	threshold := 10
	highThreshold := 0.82
	lowThreshold := 0.65

	// Sentence 1 (Founding event)
	text1 := "Government announces new retail fuel prices for the upcoming month starting midnight."
	norm1 := NormalizeText(text1)
	hash1 := ComputeSimhash(norm1)
	vec1 := make([]float32, 768)
	vec1[0] = 0.90
	vec1[1] = 0.40

	// Sentence 2 (Paraphrase 1)
	text2 := "Fuel prices to increase starting Monday according to Ministry of Trade and Regional Integration."
	norm2 := NormalizeText(text2)
	hash2 := ComputeSimhash(norm2)
	vec2 := make([]float32, 768)
	vec2[0] = 0.89
	vec2[1] = 0.41

	// Sentence 3 (Paraphrase 2)
	text3 := "The ministry announced revised retail petroleum and gasoline tariff adjustments across all regional stations."
	norm3 := NormalizeText(text3)
	hash3 := ComputeSimhash(norm3)
	vec3 := make([]float32, 768)
	vec3[0] = 0.88
	vec3[1] = 0.42

	// Verify that literal Hamming distances are > 10 (failing Simhash alone)
	dist12 := HammingDistance(hash1, hash2)
	dist13 := HammingDistance(hash1, hash3)
	t.Logf("Hamming distance (1 vs 2): %d (must be > 10)", dist12)
	t.Logf("Hamming distance (1 vs 3): %d (must be > 10)", dist13)
	if dist12 <= threshold || dist13 <= threshold {
		t.Errorf("Simhashes unexpectedly close; test requires literal divergence: dist12=%d, dist13=%d", dist12, dist13)
	}

	// 1. Sentence 1 creates Event 501
	var candidates []*EventCandidate
	dec1 := DecideClustering(1, norm1, hash1, vec1, candidates, threshold, highThreshold, lowThreshold)
	if dec1.Type != DecisionCreateNew {
		t.Fatalf("expected sentence 1 to create new event, got %v", dec1.Type)
	}

	event501 := &EventCandidate{
		EventID:           501,
		CanonicalTitle:    dec1.CanonicalTitle,
		FirstSeenAt:       time.Now(),
		LastUpdatedAt:     time.Now(),
		EmbeddingCentroid: vec1,
		Sources: []EventSourceMember{
			{RawPostID: 1, NormalizedText: norm1, Simhash: hash1},
		},
	}
	candidates = append(candidates, event501)

	// 2. Sentence 2 evaluated -> Must attach to Event 501 via Embedding similarity
	cos12 := CosineSimilarity(vec2, event501.EmbeddingCentroid)
	t.Logf("Cosine similarity (Sentence 2 to Centroid): %f", cos12)
	if cos12 < highThreshold {
		t.Fatalf("test vector setup error: cosine similarity %f < %f", cos12, highThreshold)
	}

	dec2 := DecideClustering(2, norm2, hash2, vec2, candidates, threshold, highThreshold, lowThreshold)
	if dec2.Type != DecisionAttach {
		t.Fatalf("expected sentence 2 to attach to Event 501 via embedding, got %v", dec2.Type)
	}
	if dec2.TargetEventID != 501 {
		t.Fatalf("expected target event 501, got %d", dec2.TargetEventID)
	}
	if dec2.MatchReason != "embedding" {
		t.Fatalf("expected match_reason embedding, got %s", dec2.MatchReason)
	}

	// 3. Sentence 3 evaluated -> Must also attach to Event 501
	cos13 := CosineSimilarity(vec3, event501.EmbeddingCentroid)
	t.Logf("Cosine similarity (Sentence 3 to Centroid): %f", cos13)
	if cos13 < highThreshold {
		t.Fatalf("test vector setup error: cosine similarity %f < %f", cos13, highThreshold)
	}

	dec3 := DecideClustering(3, norm3, hash3, vec3, candidates, threshold, highThreshold, lowThreshold)
	if dec3.Type != DecisionAttach {
		t.Fatalf("expected sentence 3 to attach to Event 501 via embedding, got %v", dec3.Type)
	}
	if dec3.TargetEventID != 501 {
		t.Fatalf("expected target event 501, got %d", dec3.TargetEventID)
	}
	if dec3.MatchReason != "embedding" {
		t.Fatalf("expected match_reason embedding, got %s", dec3.MatchReason)
	}

	t.Log("Successfully clustered all 3 paraphrased fuel price posts into single news_event!")
}

// TestClusteringDecision_FalsePositiveDifferentStory verifies that distinct news stories
// do NOT cluster into the same event.
func TestClusteringDecision_FalsePositiveDifferentStory(t *testing.T) {
	threshold := 10
	highThreshold := 0.82
	lowThreshold := 0.65

	// Existing Event: Fuel price hike
	centroid := make([]float32, 768)
	centroid[0] = 1.0

	event := &EventCandidate{
		EventID:           601,
		CanonicalTitle:    "Fuel price hike announced",
		EmbeddingCentroid: centroid,
		Sources: []EventSourceMember{
			{RawPostID: 10, NormalizedText: "fuel price hike announced", Simhash: 12345678},
		},
	}
	candidates := []*EventCandidate{event}

	// Incoming Post: Addis Ababa electric bus transit corridor (Orthogonal vector)
	unrelatedText := "Addis Ababa city administration launches new electric bus transit corridor"
	normUnrelated := NormalizeText(unrelatedText)
	hashUnrelated := ComputeSimhash(normUnrelated)
	vecUnrelated := make([]float32, 768)
	vecUnrelated[100] = 1.0 // Orthogonal: Cosine similarity 0.0

	dec := DecideClustering(11, normUnrelated, hashUnrelated, vecUnrelated, candidates, threshold, highThreshold, lowThreshold)
	if dec.Type != DecisionCreateNew {
		t.Fatalf("expected unrelated post to create new event, got %v (reason: %s)", dec.Type, dec.MatchReason)
	}
}

// TestClusteringDecision_AmbiguousBandNeedsReview verifies that posts falling into the ambiguous
// similarity window (0.65 <= similarity < 0.82) are routed to "needs_review".
func TestClusteringDecision_AmbiguousBandNeedsReview(t *testing.T) {
	threshold := 10
	highThreshold := 0.82
	lowThreshold := 0.65

	// Centroid vector
	centroid := make([]float32, 768)
	centroid[0] = 1.0

	event := &EventCandidate{
		EventID:           701,
		CanonicalTitle:    "Trade ministry update",
		EmbeddingCentroid: centroid,
		Sources: []EventSourceMember{
			{RawPostID: 20, NormalizedText: "trade ministry update", Simhash: 999999},
		},
	}
	candidates := []*EventCandidate{event}

	// Incoming Post with cosine similarity ~0.707 (45 degrees from centroid)
	borderlineText := "Trade and investment conference scheduled for next week"
	normBorderline := NormalizeText(borderlineText)
	hashBorderline := ComputeSimhash(normBorderline)
	vecBorderline := make([]float32, 768)
	vecBorderline[0] = 1.0
	vecBorderline[1] = 1.0 // Cosine similarity with centroid is 1/sqrt(2) ≈ 0.7071

	cosSim := CosineSimilarity(vecBorderline, centroid)
	t.Logf("Cosine similarity for borderline post: %f", cosSim)
	if cosSim < lowThreshold || cosSim >= highThreshold {
		t.Fatalf("expected cosine similarity between %f and %f, got %f", lowThreshold, highThreshold, cosSim)
	}

	dec := DecideClustering(21, normBorderline, hashBorderline, vecBorderline, candidates, threshold, highThreshold, lowThreshold)
	if dec.Type != DecisionNeedsReview {
		t.Fatalf("expected borderline post to be marked needs_review, got %v", dec.Type)
	}
	if dec.TargetEventID != 701 {
		t.Fatalf("expected target event 701 for review candidate, got %d", dec.TargetEventID)
	}
	if dec.MatchReason != "embedding_ambiguous" {
		t.Fatalf("expected match_reason embedding_ambiguous, got %s", dec.MatchReason)
	}
}

// TestClusteringDecision_CentroidDriftAntiDriftProtection tests Section 5's drift simulation:
// A series of marginal attachments causes the event's centroid to drift far from the founding post.
// Under old single-centroid logic, a new post matching the drifted centroid (cos >= 0.82) would attach.
// Under new V3.1 anti-drift logic, because similarity to the founding post is < 0.65, the post is
// REJECTED from attaching and creates a new event.
func TestClusteringDecision_CentroidDriftAntiDriftProtection(t *testing.T) {
	threshold := 10
	highThreshold := 0.82
	lowThreshold := 0.65

	// 1. Founding post embedding: [1.0, 0, 0, ...]
	foundingVec := make([]float32, 768)
	foundingVec[0] = 1.0

	// 2. Drifted Centroid: drifted over multiple attachments to [0.30, 0.954, 0, ...]
	driftedCentroid := make([]float32, 768)
	driftedCentroid[0] = 0.30
	driftedCentroid[1] = 0.954

	event := &EventCandidate{
		EventID:           801,
		CanonicalTitle:    "Verified Event #1 - Original Report",
		FoundingRawPostID: 1,
		FoundingEmbedding: foundingVec,
		EmbeddingCentroid: driftedCentroid,
		Sources: []EventSourceMember{
			{RawPostID: 1, NormalizedText: "original founding report", Simhash: 111111},
			{RawPostID: 2, NormalizedText: "intermediate drifted update", Simhash: 222222},
		},
	}
	candidates := []*EventCandidate{event}

	// 3. Incoming post matching the drifted centroid closely, but completely unrelated to founding post:
	// Vector: [0.10, 0.995, 0, ...]
	incomingText := "Unrelated topic that matches drifted centroid"
	normIncoming := NormalizeText(incomingText)
	hashIncoming := ComputeSimhash(normIncoming)
	incomingVec := make([]float32, 768)
	incomingVec[0] = 0.10
	incomingVec[1] = 0.995

	simToCentroid := CosineSimilarity(incomingVec, driftedCentroid)
	simToFounding := CosineSimilarity(incomingVec, foundingVec)

	t.Logf("Cosine similarity to Drifted Centroid: %f (>= %f high threshold)", simToCentroid, highThreshold)
	t.Logf("Cosine similarity to Founding Post:     %f (< %f low threshold)", simToFounding, lowThreshold)

	if simToCentroid < highThreshold {
		t.Fatalf("setup error: simToCentroid %f must be >= %f", simToCentroid, highThreshold)
	}
	if simToFounding >= lowThreshold {
		t.Fatalf("setup error: simToFounding %f must be < %f", simToFounding, lowThreshold)
	}

	// Evaluate clustering under V3.1 dual-check logic
	dec := DecideClustering(99, normIncoming, hashIncoming, incomingVec, candidates, threshold, highThreshold, lowThreshold)

	// PROOF: Post MUST NOT attach to Event 801, and MUST create a new event
	if dec.Type != DecisionCreateNew {
		t.Fatalf("ANTI-DRIFT FAILED: Expected DecisionCreateNew for drifted post, got %v (reason: %s, target: %d)",
			dec.Type, dec.MatchReason, dec.TargetEventID)
	}
	if dec.MatchReason != "new_event" {
		t.Fatalf("expected match_reason 'new_event', got %s", dec.MatchReason)
	}
	t.Log("SUCCESS: Anti-drift safeguard correctly rejected drifted attachment and created new event!")
}

// TestClusteringDecision_AmbiguousFoundingPostDriftProtection verifies that if a post matches the centroid
// with high confidence but matches the founding post only moderately (ambiguous band), it is routed to
// needs_review for verification rather than blindly auto-attached.
func TestClusteringDecision_AmbiguousFoundingPostDriftProtection(t *testing.T) {
	threshold := 10
	highThreshold := 0.82
	lowThreshold := 0.65

	// Founding post: [1.0, 0, 0, ...]
	foundingVec := make([]float32, 768)
	foundingVec[0] = 1.0

	// Slightly drifted centroid: [0.85, 0.527, 0, ...]
	centroidVec := make([]float32, 768)
	centroidVec[0] = 0.85
	centroidVec[1] = 0.527

	event := &EventCandidate{
		EventID:           802,
		CanonicalTitle:    "Original Event Title",
		FoundingRawPostID: 10,
		FoundingEmbedding: foundingVec,
		EmbeddingCentroid: centroidVec,
		Sources: []EventSourceMember{
			{RawPostID: 10, NormalizedText: "founding report text", Simhash: 333333},
		},
	}
	candidates := []*EventCandidate{event}

	// Incoming post with:
	// simToCentroid = 0.90 (>= 0.82)
	// simToFounding = 0.72 (0.65 <= sim < 0.82, ambiguous)
	// Vector: [0.72, 0.694, 0, ...]
	// Dot with founding [1.0, 0]: 0.72
	// Dot with centroid [0.85, 0.527]: 0.72*0.85 + 0.694*0.527 = 0.612 + 0.3657 = 0.9777
	normIncoming := "Borderline related post needing review"
	hashIncoming := ComputeSimhash(normIncoming)
	incomingVec := make([]float32, 768)
	incomingVec[0] = 0.72
	incomingVec[1] = 0.694

	simToCentroid := CosineSimilarity(incomingVec, centroidVec)
	simToFounding := CosineSimilarity(incomingVec, foundingVec)

	t.Logf("Cosine similarity to Centroid: %f (>= %f)", simToCentroid, highThreshold)
	t.Logf("Cosine similarity to Founding: %f (between %f and %f)", simToFounding, lowThreshold, highThreshold)

	dec := DecideClustering(100, normIncoming, hashIncoming, incomingVec, candidates, threshold, highThreshold, lowThreshold)

	if dec.Type != DecisionNeedsReview {
		t.Fatalf("Expected DecisionNeedsReview for ambiguous founding similarity, got %v (reason: %s)", dec.Type, dec.MatchReason)
	}
	if dec.TargetEventID != 802 {
		t.Fatalf("expected target event 802, got %d", dec.TargetEventID)
	}
	if dec.MatchReason != "embedding_ambiguous" {
		t.Fatalf("expected match_reason 'embedding_ambiguous', got %s", dec.MatchReason)
	}
}

type MockFailingEmbedder struct {
	failCount int
	calls     int
}

func (m *MockFailingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	m.calls++
	if m.calls <= m.failCount {
		return nil, errors.New("temporary upstream 503 error")
	}
	vec := make([]float32, 768)
	vec[0] = 0.5
	return vec, nil
}

func (m *MockFailingEmbedder) Dimension() int {
	return 768
}

func TestGenerateEmbeddingWithRetry(t *testing.T) {
	ctx := context.Background()

	t.Run("succeeds after retry", func(t *testing.T) {
		mock := &MockFailingEmbedder{failCount: 2}
		vec, err := generateEmbeddingWithRetry(ctx, mock, "sample text", 3, "test-corr-id")
		if err != nil {
			t.Fatalf("expected success after retry, got err: %v", err)
		}
		if len(vec) != 768 {
			t.Fatalf("expected vector len 768, got %d", len(vec))
		}
		if mock.calls != 3 {
			t.Fatalf("expected 3 calls, got %d", mock.calls)
		}
	})

	t.Run("fails permanently when retries exhausted", func(t *testing.T) {
		mock := &MockFailingEmbedder{failCount: 5}
		_, err := generateEmbeddingWithRetry(ctx, mock, "sample text", 2, "test-corr-id")
		if err == nil {
			t.Fatal("expected error after exhausted retries, got nil")
		}
		if mock.calls != 2 {
			t.Fatalf("expected 2 calls, got %d", mock.calls)
		}
	})
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Standard English headline",
			input:    "Commercial Bank of Ethiopia FX Directive",
			expected: "commercial-bank-of-ethiopia-fx-directive",
		},
		{
			name:     "Punctuation and symbols",
			input:    "New $250 Million Bond Fund Launched! (2026 Update)",
			expected: "new-250-million-bond-fund-launched-2026-update",
		},
		{
			name:     "Amharic text preservation",
			input:    "የአፍሪካ ሀገራትን የብድር ጫና ለመቀነስ",
			expected: "የአፍሪካ-ሀገራትን-የብድር-ጫና-ለመቀነስ",
		},
		{
			name:     "Leading and trailing dashes and spaces",
			input:    " --- Special Announcement: Ethiopia Market Trends --- ",
			expected: "special-announcement-ethiopia-market-trends",
		},
		{
			name:     "Empty string fallback",
			input:    "   ",
			expected: "event",
		},
		{
			name:     "Only special characters fallback",
			input:    "!@#$%^&*()_+",
			expected: "event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSlug(tt.input)
			if result != tt.expected {
				t.Errorf("GenerateSlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

