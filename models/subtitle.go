package models

import "time"

type Subtitle struct {
	Index     int
	StartTime time.Duration
	EndTime   time.Duration
	Text      string
	Emotion   string // Fish Audio emotion tag (happy, sad, excited, etc.)
}

type SubtitleList []Subtitle

func (s SubtitleList) TotalDuration() time.Duration {
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1].EndTime
}

func (s SubtitleList) GetText() string {
	var text string
	for _, sub := range s {
		text += sub.Text + " "
	}
	return text
}
