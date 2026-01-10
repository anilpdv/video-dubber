package models

import (
	"video-translator/internal/subtitle"
)

// ToInternalSubtitles converts models.SubtitleList to internal subtitle.List.
// This is used by TTS services that need to pass subtitles to internal/media.
func ToInternalSubtitles(subs SubtitleList) subtitle.List {
	result := make(subtitle.List, len(subs))
	for i, sub := range subs {
		result[i] = subtitle.Subtitle{
			Index:     sub.Index,
			StartTime: sub.StartTime,
			EndTime:   sub.EndTime,
			Text:      sub.Text,
		}
	}
	return result
}

// FromInternalSubtitles converts internal subtitle.List to models.SubtitleList.
// This is used by transcription services that return internal subtitle types.
func FromInternalSubtitles(subs subtitle.List) SubtitleList {
	result := make(SubtitleList, len(subs))
	for i, sub := range subs {
		result[i] = Subtitle{
			Index:     sub.Index,
			StartTime: sub.StartTime,
			EndTime:   sub.EndTime,
			Text:      sub.Text,
		}
	}
	return result
}
