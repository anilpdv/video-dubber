package media

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"video-translator/internal/config"
	"video-translator/internal/subtitle"
)

// AudioAssembler handles building timed audio from speech segments.
// This is the common logic shared by all TTS services.
type AudioAssembler struct {
	ffmpeg   *FFmpegService
	tempDir  string
	segments []string
	lastEnd  time.Duration
}

// NewAudioAssembler creates a new audio assembler.
func NewAudioAssembler(ffmpeg *FFmpegService, tempDir string) *AudioAssembler {
	return &AudioAssembler{
		ffmpeg:   ffmpeg,
		tempDir:  tempDir,
		segments: make([]string, 0),
		lastEnd:  0,
	}
}

// Reset clears the assembler state for reuse.
func (a *AudioAssembler) Reset() {
	a.segments = make([]string, 0)
	a.lastEnd = 0
}

// AddSilence adds a silence segment for a gap.
func (a *AudioAssembler) AddSilence(duration time.Duration, index int) error {
	if duration <= config.SilenceGapThreshold {
		return nil
	}

	silencePath := filepath.Join(a.tempDir, fmt.Sprintf("silence_%04d.wav", index))
	if err := a.ffmpeg.GenerateSilence(duration.Seconds(), silencePath); err != nil {
		return fmt.Errorf("failed to generate silence: %w", err)
	}
	a.segments = append(a.segments, silencePath)
	return nil
}

// AddSpeechSegment adds a speech segment, optionally adjusting duration.
func (a *AudioAssembler) AddSpeechSegment(speechPath string, targetDuration time.Duration, index int) error {
	// Adjust duration if target is meaningful
	if targetDuration > 200*time.Millisecond {
		adjustedPath := filepath.Join(a.tempDir, fmt.Sprintf("adjusted_%04d.wav", index))
		if err := a.ffmpeg.AdjustAudioDuration(speechPath, adjustedPath, targetDuration.Seconds()); err == nil {
			speechPath = adjustedPath
		}
		// If adjustment fails, use original speech
	}

	a.segments = append(a.segments, speechPath)
	return nil
}

// AddPadding adds silence padding if audio is shorter than target window.
func (a *AudioAssembler) AddPadding(speechPath string, windowDuration time.Duration, index int) error {
	actualDuration, err := a.ffmpeg.GetAudioDuration(speechPath)
	if err != nil {
		return nil // Ignore errors, padding is optional
	}

	tolerance := config.AudioDurationTolerance.Seconds()
	if actualDuration < windowDuration.Seconds()-tolerance {
		paddingDuration := windowDuration.Seconds() - actualDuration
		paddingPath := filepath.Join(a.tempDir, fmt.Sprintf("padding_%04d.wav", index))
		if err := a.ffmpeg.GenerateSilence(paddingDuration, paddingPath); err == nil {
			a.segments = append(a.segments, paddingPath)
		}
	}
	return nil
}

// ProcessSubtitle handles a single subtitle entry.
// It adds silence for gaps, generates speech or silence, and maintains timing.
func (a *AudioAssembler) ProcessSubtitle(sub subtitle.Subtitle, speechPath string, index int) error {
	// Add silence for gap before subtitle
	if sub.StartTime > a.lastEnd {
		gap := sub.StartTime - a.lastEnd
		if err := a.AddSilence(gap, index); err != nil {
			return err
		}
	}

	// Handle the subtitle content
	if sub.IsEmpty() {
		// Empty subtitle - add silence for the duration
		duration := sub.EndTime - sub.StartTime
		if duration > 0 {
			silencePath := filepath.Join(a.tempDir, fmt.Sprintf("silence_sub_%04d.wav", index))
			if err := a.ffmpeg.GenerateSilence(duration.Seconds(), silencePath); err != nil {
				return fmt.Errorf("failed to generate silence for empty subtitle %d: %w", index+1, err)
			}
			a.segments = append(a.segments, silencePath)
		}
	} else if speechPath != "" {
		// Add the speech segment
		targetDuration := sub.EndTime - sub.StartTime
		if err := a.AddSpeechSegment(speechPath, targetDuration, index); err != nil {
			return err
		}

		// Add padding if needed
		if err := a.AddPadding(speechPath, targetDuration, index); err != nil {
			return err
		}
	}

	a.lastEnd = sub.EndTime
	return nil
}

// Concatenate combines all segments into final output.
func (a *AudioAssembler) Concatenate(outputPath string) error {
	if len(a.segments) == 0 {
		return fmt.Errorf("no segments to concatenate")
	}
	return a.ffmpeg.ConcatAudioFiles(a.segments, outputPath)
}

// GetSegments returns the current list of segment paths.
func (a *AudioAssembler) GetSegments() []string {
	return a.segments
}

// AddSegment manually adds a segment path.
func (a *AudioAssembler) AddSegment(path string) {
	a.segments = append(a.segments, path)
}

// ProcessSubtitles processes all subtitles using a map of speech paths.
// This is the main entry point for building the final audio.
func (a *AudioAssembler) ProcessSubtitles(subs subtitle.List, speechPaths map[int]string) error {
	for i, sub := range subs {
		speechPath := speechPaths[i]
		if err := a.ProcessSubtitle(sub, speechPath, i); err != nil {
			return err
		}
	}
	return nil
}

// AssembleFromSpeechPaths builds final audio from subtitles and pre-synthesized speech.
// This is a convenience method that combines ProcessSubtitles and Concatenate.
func (a *AudioAssembler) AssembleFromSpeechPaths(subs subtitle.List, speechPaths map[int]string, outputPath string) error {
	if err := a.ProcessSubtitles(subs, speechPaths); err != nil {
		return err
	}
	return a.Concatenate(outputPath)
}

// IsSpeechNeeded returns true if a subtitle needs TTS synthesis.
func IsSpeechNeeded(sub subtitle.Subtitle) bool {
	return !sub.IsEmpty() && strings.TrimSpace(sub.Text) != ""
}

// FilterSpeechNeeded returns indices of subtitles that need TTS.
func FilterSpeechNeeded(subs subtitle.List) []int {
	indices := make([]int, 0)
	for i, sub := range subs {
		if IsSpeechNeeded(sub) {
			indices = append(indices, i)
		}
	}
	return indices
}
