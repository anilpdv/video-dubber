package tts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"video-translator/internal/media"
	"video-translator/internal/subtitle"
	"video-translator/internal/worker"
)

// BaseTTS provides shared functionality for TTS services.
// Embed this in concrete TTS implementations to inherit common behavior.
type BaseTTS struct {
	Voice    string
	FFmpeg   *media.FFmpegService
	TempDir  string
	Workers  int
}

// NewBaseTTS creates a new BaseTTS with defaults.
func NewBaseTTS(voice string, workers int) *BaseTTS {
	tempDir := filepath.Join(os.TempDir(), "video-translator-tts")
	os.MkdirAll(tempDir, 0755)

	return &BaseTTS{
		Voice:   voice,
		FFmpeg:  media.NewFFmpegService(),
		TempDir: tempDir,
		Workers: workers,
	}
}

// SetVoice sets the voice for synthesis.
func (b *BaseTTS) SetVoice(voice string) {
	b.Voice = voice
}

// GetVoice returns the current voice.
func (b *BaseTTS) GetVoice() string {
	return b.Voice
}

// EstimateCost returns 0 by default (free services).
func (b *BaseTTS) EstimateCost(charCount int) float64 {
	return 0.0
}

// Cleanup removes temporary files.
func (b *BaseTTS) Cleanup() error {
	return os.RemoveAll(b.TempDir)
}

// SynthesizeFunc is the function signature for synthesizing a single text.
type SynthesizeFunc func(text, outputPath string) error

// SynthesizeWithCallbackGeneric provides a generic implementation of parallel TTS synthesis.
// It takes a synthesize function and handles the worker pool, progress tracking, and audio assembly.
func (b *BaseTTS) SynthesizeWithCallbackGeneric(
	subs subtitle.List,
	outputPath string,
	onProgress ProgressCallback,
	synthesize SynthesizeFunc,
) error {
	if len(subs) == 0 {
		return fmt.Errorf("no subtitles provided")
	}

	// Create unique temp directory for this job
	segmentDir := filepath.Join(b.TempDir, fmt.Sprintf("segments_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(segmentDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(segmentDir)

	// Identify subtitles that need TTS
	var jobs []Job
	for i, sub := range subs {
		if !sub.IsEmpty() && strings.TrimSpace(sub.Text) != "" {
			jobs = append(jobs, Job{Index: i, Sub: sub})
		}
	}

	// Process TTS jobs in parallel
	speechPaths := make(map[int]string)
	total := len(subs)

	if len(jobs) > 0 {
		processFunc := func(job worker.Job[Job]) (string, error) {
			ttsJob := job.Data
			speechPath := filepath.Join(segmentDir, fmt.Sprintf("speech_%04d.wav", ttsJob.Index))
			err := synthesize(ttsJob.Sub.Text, speechPath)
			if err != nil {
				return "", err
			}

			// Adjust duration if needed
			targetDuration := (ttsJob.Sub.EndTime - ttsJob.Sub.StartTime).Seconds()
			if targetDuration > 0.2 {
				adjustedPath := filepath.Join(segmentDir, fmt.Sprintf("adjusted_%04d.wav", ttsJob.Index))
				if adjErr := b.FFmpeg.AdjustAudioDuration(speechPath, adjustedPath, targetDuration); adjErr == nil {
					return adjustedPath, nil
				}
			}

			return speechPath, nil
		}

		// Create worker pool
		workerJobs := make([]worker.Job[Job], len(jobs))
		for i, job := range jobs {
			workerJobs[i] = worker.Job[Job]{Index: i, Data: job}
		}

		// Progress tracking
		progressCount := 0
		progressCallback := func(completed, _ int) {
			progressCount = completed
			if onProgress != nil {
				onProgress(progressCount, total)
			}
		}

		// Run worker pool
		results, err := worker.Process(jobs, b.Workers, processFunc, progressCallback)
		if err != nil {
			return err
		}

		// Collect speech paths
		for i, path := range results {
			if path != "" {
				speechPaths[jobs[i].Index] = path
			}
		}
	}

	// Build final audio using AudioAssembler
	assembler := media.NewAudioAssembler(b.FFmpeg, segmentDir)
	if err := assembler.AssembleFromSpeechPaths(subs, speechPaths, outputPath); err != nil {
		return fmt.Errorf("failed to assemble audio: %w", err)
	}

	return nil
}

// CreateSegmentDir creates a unique temp directory for segments.
func (b *BaseTTS) CreateSegmentDir() (string, error) {
	segmentDir := filepath.Join(b.TempDir, fmt.Sprintf("segments_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(segmentDir, 0755); err != nil {
		return "", err
	}
	return segmentDir, nil
}

// SpeechPath returns the path for a speech segment.
func SpeechPath(segmentDir string, index int) string {
	return filepath.Join(segmentDir, fmt.Sprintf("speech_%04d.wav", index))
}

// AdjustedPath returns the path for an adjusted speech segment.
func AdjustedPath(segmentDir string, index int) string {
	return filepath.Join(segmentDir, fmt.Sprintf("adjusted_%04d.wav", index))
}

// SilencePath returns the path for a silence segment.
func SilencePath(segmentDir string, index int) string {
	return filepath.Join(segmentDir, fmt.Sprintf("silence_%04d.wav", index))
}
