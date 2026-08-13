package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFormatExcerpt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		wantEnd  string
		maxLen   int
	}{
		{
			name:     "empty string",
			input:    "",
			maxRunes: 160,
			wantEnd:  "",
			maxLen:   0,
		},
		{
			name:     "short string under limit",
			input:    "This is a short post.",
			maxRunes: 160,
			wantEnd:  "This is a short post.",
			maxLen:   21,
		},
		{
			name:     "long English text truncated cleanly with ellipsis",
			input:    "The National Bank of Ethiopia announced a comprehensive set of foreign exchange reforms aimed at liberalizing the foreign exchange market and stabilizing the Ethiopian Birr against major foreign currencies in the region.",
			maxRunes: 100,
			wantEnd:  "...",
			maxLen:   103, // Runes <= 100 + 3
		},
		{
			name:     "long Amharic text truncated safely without corrupting runes",
			input:    "የኢትዮጵያ ብሔራዊ ባንክ አዲስ የውጭ ምንዛሬ ፖሊሲ ይፋ አደረገ። በዚህም መሠረት የውጭ ምንዛሬ ተመን በገበያ እንዲወሰን ተወስኗል። ይህ ውሳኔ በኢኮኖሚው ላይ ከፍተኛ ለውጥ እንደሚያመጣ ይጠበቃል።",
			maxRunes: 50,
			wantEnd:  "...",
			maxLen:   53,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatExcerpt(tt.input, tt.maxRunes)
			if tt.input == "" {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
				return
			}
			if !strings.HasSuffix(got, tt.wantEnd) {
				t.Errorf("expected excerpt to end with %q, got %q", tt.wantEnd, got)
			}
			runeLen := utf8.RuneCountInString(got)
			if runeLen > tt.maxLen {
				t.Errorf("expected rune length <= %d, got %d (%q)", tt.maxLen, runeLen, got)
			}
		})
	}
}

func TestSQLStoreLiveRead(t *testing.T) {
	dbURL := os.Getenv("API_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://efi_api:efi_api_pass@localhost:5432/efi_dev?sslmode=disable"
	}

	store, err := NewSQLStore(dbURL)
	if err != nil {
		t.Skipf("skipping live database test: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.Ping(ctx); err != nil {
		t.Skipf("skipping live database test (ping failed): %v", err)
	}

	// 1. Test GetCategories
	categories, err := store.GetCategories(ctx)
	if err != nil {
		t.Fatalf("GetCategories failed: %v", err)
	}
	if len(categories) != 9 {
		t.Errorf("expected 9 categories, got %d", len(categories))
	}

	// 2. Test GetEvents
	eventsRes, err := store.GetEvents(ctx, EventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if eventsRes.Total == 0 {
		t.Logf("note: no active events in database currently")
	} else {
		if len(eventsRes.Events) == 0 {
			t.Errorf("expected non-empty events slice when total > 0")
		}
		for _, ev := range eventsRes.Events {
			if !ev.AISummaryGenerated {
				t.Errorf("expected AISummaryGenerated = true for event %d", ev.ID)
			}
			if ev.Slug == "" {
				t.Errorf("expected Slug to be populated for event %d, got empty string", ev.ID)
			}
		}

		// 3. Test GetEventByID
		firstID := eventsRes.Events[0].ID
		detail, err := store.GetEventByID(ctx, firstID)
		if err != nil {
			t.Fatalf("GetEventByID(%d) failed: %v", firstID, err)
		}
		if detail.ID != firstID {
			t.Errorf("expected detail.ID = %d, got %d", firstID, detail.ID)
		}
		if detail.Slug == "" {
			t.Errorf("expected detail.Slug to be populated, got empty string")
		}
		if !detail.AISummaryGenerated {
			t.Errorf("expected detail.AISummaryGenerated = true")
		}
	}

	// 4. Test GetEventByID with non-existent ID
	_, err = store.GetEventByID(ctx, 999999999)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent ID, got %v", err)
	}

	// 5. Test SearchEvents
	searchRes, err := store.SearchEvents(ctx, "bank", 10, 0)
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	t.Logf("Search for 'bank' returned %d results (total: %d)", len(searchRes.Events), searchRes.Total)
}
