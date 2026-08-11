package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TelegramPostFixture represents the schema of a sample Telegram message fixture.
type TelegramPostFixture struct {
	ChannelID     int64          `json:"channel_id"`
	ChannelHandle string         `json:"channel_handle"`
	ChannelName   string         `json:"channel_name"`
	MessageID     int64          `json:"message_id"`
	RawText       string         `json:"raw_text"`
	PostedAt      string         `json:"posted_at"`
	RawEntities   []EntityItem   `json:"raw_entities"`
	MediaRefs     []MediaRefItem `json:"media_refs"`
}

type EntityItem struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

type MediaRefItem struct {
	Type    string `json:"type"`
	FileID  string `json:"file_id"`
	Caption string `json:"caption"`
}

func loadFixtureFile(t *testing.T) []TelegramPostFixture {
	t.Helper()

	candidates := []string{
		"fixtures/sample_telegram_posts.json",
		"tests/fixtures/sample_telegram_posts.json",
		"../tests/fixtures/sample_telegram_posts.json",
	}

	var data []byte
	var err error
	var foundPath string

	for _, path := range candidates {
		absPath, pathErr := filepath.Abs(path)
		if pathErr == nil {
			data, err = os.ReadFile(absPath)
			if err == nil {
				foundPath = absPath
				break
			}
		}
	}

	if err != nil || len(data) == 0 {
		t.Fatalf("Failed to locate or read sample_telegram_posts.json fixture: %v", err)
	}

	t.Logf("Loaded fixture from: %s", foundPath)

	var posts []TelegramPostFixture
	if err := json.Unmarshal(data, &posts); err != nil {
		t.Fatalf("Failed to unmarshal fixture JSON: %v", err)
	}

	return posts
}

func TestSampleTelegramPostsFixture(t *testing.T) {
	posts := loadFixtureFile(t)

	if len(posts) < 3 {
		t.Fatalf("Expected at least 3 sample posts in fixture, got %d", len(posts))
	}

	seenIDs := make(map[string]bool)
	hasNearDuplicateFuelStory := false

	for i, post := range posts {
		if post.ChannelID == 0 {
			t.Errorf("Post [%d]: missing or invalid channel_id", i)
		}
		if post.MessageID == 0 {
			t.Errorf("Post [%d]: missing or invalid message_id", i)
		}
		if strings.TrimSpace(post.RawText) == "" {
			t.Errorf("Post [%d]: raw_text must not be empty", i)
		}

		// Verify timestamp format (RFC 3339)
		if _, err := time.Parse(time.RFC3339, post.PostedAt); err != nil {
			t.Errorf("Post [%d]: posted_at is not valid RFC 3339 (%s): %v", i, post.PostedAt, err)
		}

		key := string(rune(post.ChannelID)) + ":" + string(rune(post.MessageID))
		if seenIDs[key] {
			t.Errorf("Post [%d]: duplicate (channel_id, message_id) found in fixture: %s", i, key)
		}
		seenIDs[key] = true
	}

	// Verify the deliberate near-duplicate pair exists across different channels
	fuelPosts := 0
	for _, post := range posts {
		if strings.Contains(post.RawText, "101") && (strings.Contains(post.RawText, "ነዳጅ") || strings.Contains(post.RawText, "ቤንዚን")) {
			fuelPosts++
		}
	}

	if fuelPosts >= 2 {
		hasNearDuplicateFuelStory = true
	}

	if !hasNearDuplicateFuelStory {
		t.Errorf("Fixture must include at least one deliberate near-duplicate pair describing the same event across different channels")
	}
}
