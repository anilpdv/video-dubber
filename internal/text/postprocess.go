package text

import (
	"strings"
	"unicode"
)

// Postprocess cleans up translated text for better quality.
// It capitalizes the first letter, normalizes whitespace, and ensures proper formatting.
func Postprocess(text string) string {
	if text == "" {
		return ""
	}

	// Trim whitespace
	text = strings.TrimSpace(text)

	// Capitalize first letter if it's lowercase
	if len(text) > 0 {
		runes := []rune(text)
		if unicode.IsLower(runes[0]) {
			runes[0] = unicode.ToUpper(runes[0])
			text = string(runes)
		}
	}

	// Remove any double spaces that might have been introduced
	text = whitespaceRegex.ReplaceAllString(text, " ")

	return text
}
