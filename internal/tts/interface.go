// Package tts provides interfaces and implementations for text-to-speech services.
package tts

import (
	"video-translator/internal/subtitle"
)

// ProgressCallback is called during synthesis to report progress.
type ProgressCallback func(current, total int)

// Service is the interface for all TTS services.
type Service interface {
	// CheckInstalled verifies the TTS service is available.
	CheckInstalled() error

	// SetVoice sets the voice for synthesis.
	SetVoice(voice string)

	// GetVoice returns the current voice.
	GetVoice() string

	// Synthesize generates audio from text.
	Synthesize(text, outputPath string) error

	// SynthesizeSubtitles generates timed audio for subtitles.
	SynthesizeSubtitles(subs subtitle.List, outputPath string) error

	// SynthesizeWithCallback generates audio with progress callback.
	SynthesizeWithCallback(subs subtitle.List, outputPath string, onProgress ProgressCallback) error

	// EstimateCost estimates synthesis cost (0 for free services).
	EstimateCost(charCount int) float64
}

// Config contains settings for TTS services.
type Config struct {
	// Voice is the voice ID to use.
	Voice string

	// Model is the model to use (for services with multiple models).
	Model string

	// Speed is the speaking rate (1.0 = normal).
	Speed float64

	// APIKey is the API key for cloud services.
	APIKey string

	// TempDir is the directory for temporary files.
	TempDir string
}

// ProviderType identifies a TTS provider.
type ProviderType string

const (
	ProviderPiper     ProviderType = "piper"
	ProviderOpenAI    ProviderType = "openai"
	ProviderEdgeTTS   ProviderType = "edge-tts"
	ProviderCosyVoice ProviderType = "cosyvoice"
)

// Job represents a TTS synthesis job.
type Job struct {
	Index int
	Sub   subtitle.Subtitle
}

// Result represents the result of a TTS synthesis job.
type Result struct {
	Index int
	Path  string
	Err   error
}
