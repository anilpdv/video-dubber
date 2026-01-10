// Package text provides text processing utilities for subtitle translation.
package text

import (
	"regexp"
	"strings"
)

// Pre-compiled regex patterns (created once at package init for performance)
var (
	fillerRegex       = regexp.MustCompile(`(?i)\b(uh|um|er|ah|hmm|erm)\b`)
	whitespaceRegex   = regexp.MustCompile(`\s+`)
	dotsRegex         = regexp.MustCompile(`\.{2,}`)
	exclamationsRegex = regexp.MustCompile(`!{2,}`)
	questionsRegex    = regexp.MustCompile(`\?{2,}`)
	commasRegex       = regexp.MustCompile(`,{2,}`)
)

// Preprocess cleans up text before translation for better accuracy.
// It removes filler words, normalizes whitespace, and simplifies punctuation.
func Preprocess(text string) string {
	if text == "" {
		return ""
	}

	// Remove common filler sounds/words that don't translate well
	text = fillerRegex.ReplaceAllString(text, "")

	// Normalize whitespace (multiple spaces, tabs, etc. to single space)
	text = whitespaceRegex.ReplaceAllString(text, " ")

	// Remove leading/trailing whitespace
	text = strings.TrimSpace(text)

	// Simplify repeated punctuation
	text = dotsRegex.ReplaceAllString(text, ".")
	text = exclamationsRegex.ReplaceAllString(text, "!")
	text = questionsRegex.ReplaceAllString(text, "?")
	text = commasRegex.ReplaceAllString(text, ",")

	return text
}
