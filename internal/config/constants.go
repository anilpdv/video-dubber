// Package config provides centralized configuration and constants for the video-translator application.
package config

import "time"

// Progress stage boundaries (0-100%)
const (
	ProgressExtractStart    = 0
	ProgressExtractEnd      = 15
	ProgressTranscribeStart = 15
	ProgressTranscribeEnd   = 40
	ProgressTranslateStart  = 40
	ProgressTranslateEnd    = 60
	ProgressSynthesizeStart = 60
	ProgressSynthesizeEnd   = 85
	ProgressMuxStart        = 85
	ProgressMuxEnd          = 100
)

// Worker pool sizes (tuned for each provider type)
const (
	WorkersPiperTTS   = 2  // CPU-intensive, local processing
	WorkersCosyVoice  = 2  // GPU-intensive, local processing
	WorkersArgos      = 4  // Python subprocess overhead
	WorkersOpenAI     = 4  // API calls, moderate concurrency
	WorkersOpenAITTS  = 6  // API calls, can handle more
	WorkersDeepSeek   = 8  // API calls, generous limits
	WorkersEdgeTTS    = 10 // Free API, generous rate limits
)

// Translation chunk sizes (subtitles per batch)
const (
	ChunkSizeArgos    = 50  // Local processing, moderate batch
	ChunkSizeOpenAI   = 50  // API token limits
	ChunkSizeDeepSeek = 120 // Larger batches, fewer API calls
)

// Retry settings
const (
	DefaultMaxRetries     = 3
	DefaultRetryDelayBase = time.Second
)

// HTTP client settings
const (
	HTTPTimeout             = 2 * time.Minute
	HTTPMaxIdleConns        = 10
	HTTPMaxIdleConnsPerHost = 10
	HTTPIdleConnTimeout     = 90 * time.Second
)

// Audio settings
const (
	AudioSampleRate16k    = 16000 // Whisper requirement
	AudioSampleRate24k    = 24000 // TTS output
	SilenceGapThreshold   = 10 * time.Millisecond
	AudioDurationTolerance = 50 * time.Millisecond
)

// TTS speed settings
const (
	PiperLengthScale = 1.0   // Normal speaking rate
	PiperNoiseScale  = 0.667 // Balanced variability
	PiperNoiseW      = 0.8   // Natural phoneme variation
)

// Whisper model defaults
const (
	WhisperDefaultModel = "base"
	WhisperModelTiny    = "tiny"   // 75MB, fastest
	WhisperModelBase    = "base"   // 150MB, balanced
	WhisperModelSmall   = "small"  // 500MB, better quality
	WhisperModelMedium  = "medium" // 1.5GB, high quality
	WhisperModelLarge   = "large"  // 3GB, best quality
)

// API endpoints
const (
	OpenAIAPIEndpoint     = "https://api.openai.com/v1"
	DeepSeekAPIEndpoint   = "https://api.deepseek.com/v1/chat/completions"
	OpenAIChatEndpoint    = "https://api.openai.com/v1/chat/completions"
	OpenAIWhisperEndpoint = "https://api.openai.com/v1/audio/transcriptions"
	OpenAITTSEndpoint     = "https://api.openai.com/v1/audio/speech"
)

// API models
const (
	OpenAITranslationModel = "gpt-4o-mini"
	DeepSeekModel          = "deepseek-chat"
	OpenAITTSModelTTS1     = "tts-1"
	OpenAITTSModelTTS1HD   = "tts-1-hd"
)

// Translation delimiter
const TranslationDelimiter = "|||SUBTITLE|||"

// Temperature settings for LLM calls
const (
	TranslationTemperature = 0.3
	TranslationMaxTokens   = 4096
	DeepSeekMaxTokens      = 8192
)

// Default TTS voices
const (
	DefaultPiperVoice  = "en_US-amy-medium"
	DefaultEdgeTTSVoice = "en-US-AriaNeural"
	DefaultOpenAIVoice  = "alloy"
)

// Default languages
const (
	DefaultSourceLang = "ru"
	DefaultTargetLang = "en"
)

// Processing timeouts
const (
	EdgeTTSTimeout    = 60 * time.Second
	WhisperAPITimeout = 10 * time.Minute
)

// Exec command timeouts (for os/exec calls)
const (
	ExecTimeoutPython  = 5 * time.Minute   // Python script execution
	ExecTimeoutFFmpeg  = 10 * time.Minute  // Audio/video processing
	ExecTimeoutWhisper = 30 * time.Minute  // Transcription (can be long)
)
