package models

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds application settings
// Supports multiple providers for transcription, translation, and TTS
type Config struct {
	// Basic settings
	DefaultSourceLang string `json:"default_source_lang"`
	DefaultTargetLang string `json:"default_target_lang"`
	DefaultVoice      string `json:"default_voice"`
	OutputDirectory   string `json:"output_directory"`

	// Tool paths
	FFmpegPath     string `json:"ffmpeg_path"`
	WhisperPath    string `json:"whisper_path"`
	PiperPath      string `json:"piper_path"`
	PiperVoicesDir string `json:"piper_voices_dir"`
	PythonPath     string `json:"python_path"`

	// Provider selection (whisper-cpp, faster-whisper, openai)
	TranscriptionProvider string `json:"transcription_provider"`
	// Provider selection (argos, openai, deepseek)
	TranslationProvider string `json:"translation_provider"`
	// Provider selection (piper, openai, cosyvoice)
	TTSProvider string `json:"tts_provider"`

	// OpenAI API settings
	OpenAIKey     string `json:"openai_key"`
	UseOpenAIAPIs bool   `json:"use_openai_apis"` // Legacy flag for backward compatibility

	// DeepSeek API settings (10x cheaper than GPT-4o-mini)
	DeepSeekKey string `json:"deepseek_key"`

	// Groq API settings (ultra-fast transcription using LPU hardware)
	GroqAPIKey string `json:"groq_api_key"`

	// Grok API settings (xAI's Grok for translation - cheap)
	GrokAPIKey string `json:"grok_api_key"`

	// Whisper settings (for whisper-cpp)
	WhisperModel string `json:"whisper_model"`

	// FasterWhisper settings (4-10x faster than whisper-cpp)
	FasterWhisperModel  string `json:"faster_whisper_model"`  // tiny, base, small, medium, large-v2, large-v3
	FasterWhisperDevice string `json:"faster_whisper_device"` // auto, cuda, cpu

	// WhisperKit settings (native macOS Apple Silicon)
	WhisperKitModel string `json:"whisperkit_model"` // tiny, base, small, medium, large-v2, large-v3

	// OpenAI TTS settings (high quality voices)
	OpenAITTSModel string  `json:"openai_tts_model"` // tts-1, tts-1-hd
	OpenAITTSVoice string  `json:"openai_tts_voice"` // alloy, echo, fable, onyx, nova, shimmer
	OpenAITTSSpeed float64 `json:"openai_tts_speed"` // 0.25 to 4.0, default 1.15 for dubbing

	// CosyVoice settings (voice cloning)
	CosyVoicePath        string `json:"cosyvoice_path"`
	CosyVoiceMode        string `json:"cosyvoice_mode"`    // local, api
	CosyVoiceAPIURL      string `json:"cosyvoice_api_url"` // API endpoint if using api mode
	VoiceCloneSamplePath string `json:"voice_clone_sample"`

	// Edge TTS settings (FREE neural TTS from Microsoft)
	EdgeTTSVoice string `json:"edge_tts_voice"` // en-US-AriaNeural, en-US-GuyNeural, etc.

	// Smart segmentation (uses LLM to improve sentence boundaries)
	UseSmartSegmentation bool `json:"use_smart_segmentation"`

	// Audio mixing settings (keep background music/sounds)
	KeepBackgroundAudio   bool    `json:"keep_background_audio"`
	BackgroundAudioVolume float64 `json:"background_audio_volume"` // 0.0-1.0, default 0.3
}

func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		// Basic settings
		DefaultSourceLang: "ru",
		DefaultTargetLang: "en",
		DefaultVoice:      "en_US-amy-medium",
		OutputDirectory:   filepath.Join(homeDir, "Desktop", "Translated"),

		// Tool paths
		FFmpegPath:     "/opt/homebrew/bin/ffmpeg",
		WhisperPath:    "/opt/homebrew/bin/whisper-cpp",
		PiperPath:      "piper",
		PiperVoicesDir: filepath.Join(homeDir, ".piper", "voices"),
		PythonPath:     "python3",

		// Provider selection (default to local/free options)
		TranscriptionProvider: "whisperkit", // Native macOS (Apple Silicon)
		TranslationProvider:   "argos",
		TTSProvider:           "piper",

		// OpenAI settings
		OpenAIKey:     "",
		UseOpenAIAPIs: false, // Legacy flag

		// DeepSeek settings
		DeepSeekKey: "",

		// Groq settings
		GroqAPIKey: "",

		// Whisper settings
		WhisperModel: "base",

		// FasterWhisper settings
		FasterWhisperModel:  "base",
		FasterWhisperDevice: "auto",

		// WhisperKit settings (native macOS Apple Silicon)
		WhisperKitModel: "base",

		// OpenAI TTS settings
		OpenAITTSModel: "tts-1",
		OpenAITTSVoice: "nova",
		OpenAITTSSpeed: 1.15, // Slightly faster for natural dubbing

		// CosyVoice settings
		CosyVoicePath:        filepath.Join(homeDir, ".cosyvoice"),
		CosyVoiceMode:        "local",
		CosyVoiceAPIURL:      "",
		VoiceCloneSamplePath: "",

		// Edge TTS settings (FREE)
		EdgeTTSVoice: "en-US-AriaNeural",

		// Smart segmentation
		UseSmartSegmentation: false,

		// Audio mixing (keep background music at 30% volume)
		KeepBackgroundAudio:   true,
		BackgroundAudioVolume: 0.3,
	}
}

func (c *Config) ConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "video-translator", "config.json")
}

func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	data, err := os.ReadFile(config.ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) Save() error {
	configPath := c.ConfigPath()

	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600) // User-only permissions for security
}
