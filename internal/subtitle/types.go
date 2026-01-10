// Package subtitle provides types and utilities for handling subtitles.
package subtitle

import (
	"strings"
	"time"
)

// Subtitle represents a single subtitle entry with timing and text.
type Subtitle struct {
	Index     int
	StartTime time.Duration
	EndTime   time.Duration
	Text      string
}

// Duration returns the duration of this subtitle.
func (s Subtitle) Duration() time.Duration {
	return s.EndTime - s.StartTime
}

// IsEmpty returns true if the subtitle has no text.
func (s Subtitle) IsEmpty() bool {
	return strings.TrimSpace(s.Text) == ""
}

// List is a slice of subtitles with utility methods.
type List []Subtitle

// TotalDuration returns the total duration from start to end of all subtitles.
func (l List) TotalDuration() time.Duration {
	if len(l) == 0 {
		return 0
	}
	return l[len(l)-1].EndTime
}

// GetText returns all subtitle text concatenated with spaces.
func (l List) GetText() string {
	var text strings.Builder
	for _, sub := range l {
		if sub.Text != "" {
			text.WriteString(sub.Text)
			text.WriteString(" ")
		}
	}
	return strings.TrimSpace(text.String())
}

// GetTexts returns all subtitle texts as a slice.
func (l List) GetTexts() []string {
	texts := make([]string, len(l))
	for i, sub := range l {
		texts[i] = sub.Text
	}
	return texts
}

// NonEmpty returns a new list containing only subtitles with non-empty text.
func (l List) NonEmpty() List {
	result := make(List, 0, len(l))
	for _, sub := range l {
		if !sub.IsEmpty() {
			result = append(result, sub)
		}
	}
	return result
}

// WithUpdatedTexts returns a new list with updated texts while preserving timing.
func (l List) WithUpdatedTexts(texts []string) List {
	if len(texts) != len(l) {
		return l
	}
	result := make(List, len(l))
	for i, sub := range l {
		result[i] = Subtitle{
			Index:     sub.Index,
			StartTime: sub.StartTime,
			EndTime:   sub.EndTime,
			Text:      texts[i],
		}
	}
	return result
}

// Clone returns a deep copy of the list.
func (l List) Clone() List {
	result := make(List, len(l))
	copy(result, l)
	return result
}

// ConvertSubtitle converts external subtitle fields to internal Subtitle.
func ConvertSubtitle(index int, startTime, endTime time.Duration, text string) Subtitle {
	return Subtitle{
		Index:     index,
		StartTime: startTime,
		EndTime:   endTime,
		Text:      text,
	}
}
