// Package config provides centralized configuration and constants for the video-translator application.
package config

import (
	"runtime"
	"time"
)

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
	WorkersOpenAI     = 20 // API calls, OpenAI handles high concurrency
	WorkersOpenAITTS  = 25 // API calls, TTS is fast
	WorkersDeepSeek   = 25 // API calls, generous limits
	WorkersEdgeTTS    = 30 // Free API, very generous rate limits
)

// Audio chunking settings for parallel transcription
const (
	AudioChunkDuration = 5 * time.Minute  // Split audio into 5-minute chunks
	AudioChunkOverlap  = 2 * time.Second  // 2-second overlap to avoid word cutoff
	MinChunkDuration   = 30 * time.Second // Don't chunk audio shorter than this
)

// Global resource limits (across all videos)
const (
	// MaxConcurrentTranscriptions limits total concurrent transcription processes.
	// This prevents CPU overload when batch processing multiple videos.
	// Each video's chunks compete for these slots via a global semaphore.
	MaxConcurrentTranscriptions = 7

	// MaxConcurrentCPUOperations limits total concurrent CPU-intensive operations.
	// This includes FFmpeg operations (silence generation, duration adjustment),
	// local TTS (Piper), and local translation (Argos).
	// With maxParallelVideos=2, 8 slots gives good parallelism without overload.
	MaxConcurrentCPUOperations = 8
)

// Translation chunk sizes (subtitles per batch)
const (
	ChunkSizeArgos    = 50 // Local processing, moderate batch
	ChunkSizeOpenAI   = 50 // API token limits
	ChunkSizeDeepSeek = 20 // Smaller batches = more parallelism (200 subs → 10 batches → 10 workers active)
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

// DynamicWorkerCount returns the optimal worker count based on task type and CPU cores.
// This allows scaling workers based on system resources rather than fixed values.
func DynamicWorkerCount(taskType string) int {
	cpus := runtime.NumCPU()

	switch taskType {
	case "transcription":
		// CPU-bound, use all cores but cap at 8 for memory
		return minInt(cpus, 8)
	case "translation-api":
		// I/O-bound API calls, can use more workers
		return minInt(cpus*3, 30)
	case "tts-api":
		// I/O-bound TTS API calls
		return minInt(cpus*4, 40)
	case "tts-local":
		// CPU/GPU intensive local TTS
		return maxInt(cpus/2, 2)
	case "silence-generation":
		// Light FFmpeg operations
		return minInt(cpus*2, 16)
	default:
		return cpus
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
