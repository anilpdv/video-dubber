package text

import "strings"

// EscapeForPython escapes a string for use in Python single-quoted strings.
// Uses a single-pass approach for efficiency instead of multiple ReplaceAll calls.
func EscapeForPython(s string) string {
	if s == "" {
		return ""
	}

	// Pre-allocate with extra capacity for potential escapes
	var b strings.Builder
	b.Grow(len(s) + 10)

	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '\'':
			b.WriteString("\\'")
		case '\n':
			b.WriteString("\\n")
		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}
