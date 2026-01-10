package subtitle

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseTimestamp converts an SRT timestamp string to time.Duration.
// Supports both comma and dot as millisecond separators.
// Format: 00:00:00,000 or 00:00:00.000
func ParseTimestamp(ts string) time.Duration {
	// Normalize separator (SRT uses comma, some use dot)
	ts = strings.Replace(ts, ",", ".", 1)

	parts := strings.Split(ts, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])

	secParts := strings.Split(parts[2], ".")
	seconds, _ := strconv.Atoi(secParts[0])
	millis := 0
	if len(secParts) > 1 {
		millis, _ = strconv.Atoi(secParts[1])
	}

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(millis)*time.Millisecond
}

// ParseTimestampToSeconds converts timestamp like "00:05:30.500" to seconds as float64.
// Useful for progress calculations.
func ParseTimestampToSeconds(ts string) float64 {
	parts := strings.Split(ts, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.ParseFloat(parts[0], 64)
	minutes, _ := strconv.ParseFloat(parts[1], 64)

	secParts := strings.Split(parts[2], ".")
	seconds, _ := strconv.ParseFloat(secParts[0], 64)
	millis := 0.0
	if len(secParts) > 1 {
		millis, _ = strconv.ParseFloat("0."+secParts[1], 64)
	}

	return hours*3600 + minutes*60 + seconds + millis
}

// FormatTimestamp converts a time.Duration to SRT timestamp format.
// Output format: 00:00:00,000
func FormatTimestamp(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	millis := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

// FormatTimestampDot converts a time.Duration to timestamp format with dot separator.
// Output format: 00:00:00.000
func FormatTimestampDot(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	millis := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

// DurationToSeconds converts a time.Duration to seconds as float64.
func DurationToSeconds(d time.Duration) float64 {
	return d.Seconds()
}

// SecondsToDuration converts seconds as float64 to time.Duration.
func SecondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
