package main

import (
	"encoding/json"
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

		t.Logf("Hamming distance post 0 vs post 2: %d", dist02)
		t.Logf("Hamming distance post 0 vs post 3: %d", dist03)
		t.Logf("Hamming distance post 2 vs post 3: %d", dist23)

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

func TestClusteringDecision(t *testing.T) {
	fixtures := loadFixtures(t)
	threshold := 10

	// Post 0: ENA fuel price (founding post of Event 1)
	norm0 := NormalizeText(fixtures[0].RawText)
	hash0 := ComputeSimhash(norm0)

	// Step 1: Post 0 processed with no existing candidates -> Creates Event 1
	var candidates []*EventCandidate
	dec0 := DecideClustering(1, norm0, hash0, candidates, threshold)
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

	// Step 2: Repost of Post 0 (with footer) -> MUST attach to Event 101
	repostText := fixtures[0].RawText + "\n\nFollow us on @TeleChannel"
	normRepost := NormalizeText(repostText)
	hashRepost := ComputeSimhash(normRepost)

	decRepost := DecideClustering(2, normRepost, hashRepost, candidates, threshold)
	if decRepost.Type != DecisionAttach {
		t.Fatalf("expected repost to attach to Event 101, got %v", decRepost.Type)
	}
	if decRepost.TargetEventID != 101 {
		t.Fatalf("expected target event 101, got %d", decRepost.TargetEventID)
	}
	t.Logf("Repost successfully attached to Event 101 with similarity score %d", decRepost.SimilarityScore)

	// Step 3: Distinct posts (Post 2 NBE Forex, Post 3 Coffee, Post 4 Road project) -> MUST create new events
	for idx := 2; idx < len(fixtures); idx++ {
		norm := NormalizeText(fixtures[idx].RawText)
		hash := ComputeSimhash(norm)

		dec := DecideClustering(int64(idx+1), norm, hash, candidates, threshold)
		if dec.Type != DecisionCreateNew {
			t.Fatalf("expected distinct post %d to create new event, but got attached to event %d with score %d",
				idx, dec.TargetEventID, dec.SimilarityScore)
		}

		// Add new event
		newEvent := &EventCandidate{
			EventID:        int64(100 + idx),
			CanonicalTitle: dec.CanonicalTitle,
			FirstSeenAt:    time.Now(),
			LastUpdatedAt:  time.Now(),
			Sources: []EventSourceMember{
				{RawPostID: int64(idx + 1), NormalizedText: norm, Simhash: hash},
			},
		}
		candidates = append(candidates, newEvent)
	}
}

func TestClusteringExactDuplicate(t *testing.T) {
	threshold := 10
	text := "the national bank of ethiopia announced new foreign exchange rules today"
	norm := NormalizeText(text)
	hash := ComputeSimhash(norm)

	candidates := []*EventCandidate{
		{
			EventID:        201,
			CanonicalTitle: norm,
			Sources: []EventSourceMember{
				{RawPostID: 50, NormalizedText: norm, Simhash: hash},
			},
		},
	}

	dec := DecideClustering(51, norm, hash, candidates, threshold)
	if dec.Type != DecisionAttach {
		t.Fatalf("expected exact duplicate to attach, got %v", dec.Type)
	}
	if dec.TargetEventID != 201 {
		t.Fatalf("expected target event 201, got %d", dec.TargetEventID)
	}
	if dec.SimilarityScore != 0 {
		t.Fatalf("expected similarity score 0 for exact duplicate, got %d", dec.SimilarityScore)
	}
}

func TestConfigDefaults(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/testdb")
	defer os.Unsetenv("DATABASE_URL")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.SimhashThreshold != 10 {
		t.Errorf("expected default SimhashThreshold 10, got %d", cfg.SimhashThreshold)
	}
	if cfg.ClusteringWindow != 48*time.Hour {
		t.Errorf("expected default ClusteringWindow 48h, got %v", cfg.ClusteringWindow)
	}
	if cfg.PollInterval != 2*time.Second {
		t.Errorf("expected default PollInterval 2s, got %v", cfg.PollInterval)
	}
	if cfg.BatchSize != 50 {
		t.Errorf("expected default BatchSize 50, got %d", cfg.BatchSize)
	}
}
