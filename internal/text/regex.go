package text

import "regexp"

// Pre-compiled regex patterns for parsing whisper output and progress.
// These patterns are used across multiple services.
var (
	// WhisperTimestampRegex matches timestamps in format [HH:MM:SS.mmm
	WhisperTimestampRegex = regexp.MustCompile(`\[(\d{2}:\d{2}:\d{2}\.\d{3})`)

	// WhisperProgressRegex matches progress output like PROGRESS:45.5
	WhisperProgressRegex = regexp.MustCompile(`PROGRESS:(\d+\.?\d*)`)
)
