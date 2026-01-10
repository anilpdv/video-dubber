// Package transcription provides interfaces and implementations for speech-to-text services.
package transcription

import (
	"video-translator/internal/subtitle"
)

// ProgressCallback is called during transcription to report progress.
// currentSec is the current position in the audio, percent is the overall progress.
type ProgressCallback func(currentSec float64, percent int)

// Transcriber is the interface for all transcription services.
type Transcriber interface {
	// CheckInstalled verifies the transcription service is available.
	CheckInstalled() error

	// Transcribe converts audio to subtitles.
	Transcribe(audioPath, language string) (subtitle.List, error)

	// TranscribeWithProgress transcribes with progress reporting.
	TranscribeWithProgress(audioPath, language string, audioDuration float64, onProgress ProgressCallback) (subtitle.List, error)
}

// Config contains settings for transcription services.
type Config struct {
	// Model specifies the transcription model to use.
	Model string

	// Device specifies the compute device (cpu, cuda, etc.).
	Device string

	// PythonPath is the path to Python interpreter (for FasterWhisper).
	PythonPath string

	// APIKey is the API key for cloud services (OpenAI).
	APIKey string
}

// ProviderType identifies a transcription provider.
type ProviderType string

const (
	ProviderWhisperCPP    ProviderType = "whisper-cpp"
	ProviderFasterWhisper ProviderType = "faster-whisper"
	ProviderOpenAI        ProviderType = "openai"
)

// AvailableModels returns the list of available Whisper model sizes.
func AvailableModels() []string {
	return []string{
		"tiny",   // ~75MB, fastest
		"base",   // ~150MB, good balance
		"small",  // ~500MB, better accuracy
		"medium", // ~1.5GB, high accuracy
		"large",  // ~3GB, best accuracy
	}
}

// ModelDownloadURL returns the URL to download a Whisper model.
func ModelDownloadURL(model string) string {
	return "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-" + model + ".bin"
}
