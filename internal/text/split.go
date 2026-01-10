package text

import "strings"

// SplitByDelimiter splits text by a delimiter and returns cleaned, non-empty parts.
// This is useful for parsing batch translation results that use delimiters.
func SplitByDelimiter(text, delimiter string) []string {
	parts := strings.Split(text, delimiter)
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return cleaned
}
