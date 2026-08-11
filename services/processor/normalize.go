package main

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	urlRegex      = regexp.MustCompile(`(?i)\b(?:https?://|www\.|t\.me/)\S+`)
	markdownRegex = regexp.MustCompile(`[*_~` + "`" + `\[\]\(\)]`)
	punctRegex    = regexp.MustCompile(`[፡።፣፤፥፦፧፨.,!?:;"'""''\-/\\]`)
)

// NormalizeText normalizes raw post text for consistent similarity comparison:
// - Lowercases English characters while preserving UTF-8 Amharic/Ethiopic text
// - Removes HTTP/HTTPS and Telegram URLs
// - Strips markdown/formatting symbols (*, _, ~, `, [, ], (, ))
// - Normalizes Ethiopic and standard punctuation to spaces
// - Collapses whitespace runs and newlines into single spaces
// - Trims leading and trailing spaces
func NormalizeText(raw string) string {
	if raw == "" {
		return ""
	}

	// 1. Lowercase
	text := strings.ToLower(raw)

	// 2. Remove URLs
	text = urlRegex.ReplaceAllString(text, " ")

	// 3. Remove Markdown formatting markers
	text = markdownRegex.ReplaceAllString(text, "")

	// 4. Normalize punctuation
	text = punctRegex.ReplaceAllString(text, " ")

	// 5. Collapse whitespace and trim
	var builder strings.Builder
	builder.Grow(len(text))

	inSpace := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if !inSpace {
				builder.WriteRune(' ')
				inSpace = true
			}
		} else {
			builder.WriteRune(r)
			inSpace = false
		}
	}

	return strings.TrimSpace(builder.String())
}
