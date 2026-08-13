package main

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	nonAlphaNumRegex  = regexp.MustCompile(`[^a-zA-Z0-9\x{1200}-\x{137F}]+`)
	multipleDashRegex = regexp.MustCompile(`-+`)
)

// GenerateSlug produces a clean, url-friendly slug from an AI headline or canonical title.
// Supports English, numbers, and Ethiopic script (Amharic).
func GenerateSlug(title string) string {
	title = strings.TrimSpace(strings.ToLower(title))
	if title == "" {
		return "event"
	}

	// Replace non-alphanumerics with dash
	slug := nonAlphaNumRegex.ReplaceAllString(title, "-")
	slug = multipleDashRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")

	if slug == "" {
		return "event"
	}

	// Bounded length: max 80 runes, cutting at dash boundary if possible
	if utf8.RuneCountInString(slug) > 80 {
		runes := []rune(slug)
		truncated := string(runes[:80])
		if lastDash := strings.LastIndex(truncated, "-"); lastDash > 40 {
			truncated = truncated[:lastDash]
		}
		slug = strings.Trim(truncated, "-")
	}

	if slug == "" {
		return "event"
	}

	return slug
}
