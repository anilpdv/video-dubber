// Package translation provides interfaces and implementations for text translation services.
package translation

import (
	"video-translator/internal/subtitle"
)

// ProgressCallback is called during translation to report progress.
type ProgressCallback func(current, total int)

// Translator is the interface for all translation services.
type Translator interface {
	// CheckInstalled verifies the translation service is available.
	CheckInstalled() error

	// CheckLanguagePair verifies the language pair is supported.
	CheckLanguagePair(sourceLang, targetLang string) error

	// Translate translates a single text string.
	Translate(text, sourceLang, targetLang string) (string, error)

	// TranslateSubtitles translates a list of subtitles.
	TranslateSubtitles(subs subtitle.List, sourceLang, targetLang string, onProgress ProgressCallback) (subtitle.List, error)
}

// Config contains settings for translation services.
type Config struct {
	// APIKey is the API key for cloud services.
	APIKey string

	// PythonPath is the path to Python interpreter (for Argos).
	PythonPath string

	// ChunkSize is the number of subtitles to translate per batch.
	ChunkSize int

	// MaxWorkers is the maximum number of concurrent workers.
	MaxWorkers int
}

// ProviderType identifies a translation provider.
type ProviderType string

const (
	ProviderArgos    ProviderType = "argos"
	ProviderOpenAI   ProviderType = "openai"
	ProviderDeepSeek ProviderType = "deepseek"
)

// BatchJob represents a batch of texts to translate.
type BatchJob struct {
	Index int
	Texts []string
}

// BatchResult contains the result of translating a batch.
type BatchResult struct {
	Index        int
	Translations []string
	Err          error
}
