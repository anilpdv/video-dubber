package subtitle

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Pre-compiled regex for timestamp parsing
var timeRegex = regexp.MustCompile(`(\d{2}:\d{2}:\d{2}[,\.]\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2}[,\.]\d{3})`)

// ParseSRT parses SRT content from a reader.
// This is the unified SRT parser that handles both file and string input.
func ParseSRT(r io.Reader) (List, error) {
	var subtitles List
	scanner := bufio.NewScanner(r)

	// SRT format:
	// 1
	// 00:00:00,000 --> 00:00:02,500
	// Text here
	//
	// 2
	// ...

	var currentSub *Subtitle
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			if currentSub != nil && currentSub.Text != "" {
				subtitles = append(subtitles, *currentSub)
			}
			currentSub = nil
			lineNum = 0
			continue
		}

		lineNum++

		switch lineNum {
		case 1:
			// Index line
			index, err := strconv.Atoi(line)
			if err == nil {
				currentSub = &Subtitle{Index: index}
			}
		case 2:
			// Timestamp line
			if currentSub != nil {
				matches := timeRegex.FindStringSubmatch(line)
				if len(matches) == 3 {
					currentSub.StartTime = ParseTimestamp(matches[1])
					currentSub.EndTime = ParseTimestamp(matches[2])
				}
			}
		default:
			// Text lines
			if currentSub != nil {
				if currentSub.Text != "" {
					currentSub.Text += " "
				}
				currentSub.Text += line
			}
		}
	}

	// Don't forget the last subtitle
	if currentSub != nil && currentSub.Text != "" {
		subtitles = append(subtitles, *currentSub)
	}

	return subtitles, scanner.Err()
}

// ParseSRTFile parses an SRT file from the given path.
func ParseSRTFile(path string) (List, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ParseSRT(file)
}

// ParseSRTString parses SRT content from a string.
func ParseSRTString(content string) (List, error) {
	return ParseSRT(strings.NewReader(content))
}

// FormatSRT formats a list of subtitles to SRT format.
func FormatSRT(subs List) string {
	var builder strings.Builder
	for i, sub := range subs {
		// Index
		builder.WriteString(strconv.Itoa(sub.Index))
		builder.WriteString("\n")

		// Timestamps
		builder.WriteString(FormatTimestamp(sub.StartTime))
		builder.WriteString(" --> ")
		builder.WriteString(FormatTimestamp(sub.EndTime))
		builder.WriteString("\n")

		// Text
		builder.WriteString(sub.Text)
		builder.WriteString("\n")

		// Blank line between entries
		if i < len(subs)-1 {
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

// WriteSRTFile writes subtitles to an SRT file.
func WriteSRTFile(path string, subs List) error {
	content := FormatSRT(subs)
	return os.WriteFile(path, []byte(content), 0644)
}
