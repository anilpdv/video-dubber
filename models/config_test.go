package models

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.WhisperModel != "base" {
		t.Errorf("WhisperModel = %q, want 'base'", config.WhisperModel)
	}
	if config.DefaultSourceLang != "ru" {
		t.Errorf("DefaultSourceLang = %q, want 'ru'", config.DefaultSourceLang)
	}
	if config.DefaultTargetLang != "en" {
		t.Errorf("DefaultTargetLang = %q, want 'en'", config.DefaultTargetLang)
	}
	if config.DefaultVoice != "en_US-amy-medium" {
		t.Errorf("DefaultVoice = %q, want 'en_US-amy-medium'", config.DefaultVoice)
	}
	if config.FFmpegPath != "/opt/homebrew/bin/ffmpeg" {
		t.Errorf("FFmpegPath = %q, want '/opt/homebrew/bin/ffmpeg'", config.FFmpegPath)
	}
	if config.WhisperPath != "/opt/homebrew/bin/whisper-cpp" {
		t.Errorf("WhisperPath = %q, want '/opt/homebrew/bin/whisper-cpp'", config.WhisperPath)
	}
	if config.PiperPath != "piper" {
		t.Errorf("PiperPath = %q, want 'piper'", config.PiperPath)
	}
	if config.PythonPath != "python3" {
		t.Errorf("PythonPath = %q, want 'python3'", config.PythonPath)
	}
}

func TestDefaultConfig_HomeDir(t *testing.T) {
	config := DefaultConfig()
	homeDir, _ := os.UserHomeDir()

	expectedOutputDir := filepath.Join(homeDir, "Desktop", "Translated")
	if config.OutputDirectory != expectedOutputDir {
		t.Errorf("OutputDirectory = %q, want %q", config.OutputDirectory, expectedOutputDir)
	}

	expectedPiperDir := filepath.Join(homeDir, ".piper", "voices")
	if config.PiperVoicesDir != expectedPiperDir {
		t.Errorf("PiperVoicesDir = %q, want %q", config.PiperVoicesDir, expectedPiperDir)
	}
}

func TestConfigPath(t *testing.T) {
	config := DefaultConfig()
	homeDir, _ := os.UserHomeDir()

	expected := filepath.Join(homeDir, ".config", "video-translator", "config.json")
	if got := config.ConfigPath(); got != expected {
		t.Errorf("ConfigPath() = %q, want %q", got, expected)
	}
}

func TestLoadConfig_DefaultWhenMissing(t *testing.T) {
	// LoadConfig should return default config when file doesn't exist
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if config.WhisperModel != "base" {
		t.Errorf("expected default WhisperModel 'base', got %q", config.WhisperModel)
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	// Create a temp directory for testing
	tmpDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a custom config
	config := &Config{
		WhisperModel:      "large",
		DefaultSourceLang: "es",
		DefaultTargetLang: "de",
		DefaultVoice:      "de-voice",
		OutputDirectory:   tmpDir,
		FFmpegPath:        "/custom/ffmpeg",
		WhisperPath:       "/custom/whisper",
		PiperPath:         "/custom/piper",
		PiperVoicesDir:    tmpDir,
		PythonPath:        "/custom/python",
	}

	// Test that config fields are set correctly
	if config.WhisperModel != "large" {
		t.Errorf("WhisperModel = %q, want 'large'", config.WhisperModel)
	}
	if config.DefaultSourceLang != "es" {
		t.Errorf("DefaultSourceLang = %q, want 'es'", config.DefaultSourceLang)
	}
}

func TestConfig_Fields(t *testing.T) {
	config := &Config{
		WhisperModel:      "tiny",
		DefaultSourceLang: "fr",
		DefaultTargetLang: "en",
		DefaultVoice:      "en-voice",
		OutputDirectory:   "/output",
		FFmpegPath:        "/bin/ffmpeg",
		WhisperPath:       "/bin/whisper",
		PiperPath:         "/bin/piper",
		PiperVoicesDir:    "/voices",
		PythonPath:        "/bin/python",
	}

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"WhisperModel", config.WhisperModel, "tiny"},
		{"DefaultSourceLang", config.DefaultSourceLang, "fr"},
		{"DefaultTargetLang", config.DefaultTargetLang, "en"},
		{"DefaultVoice", config.DefaultVoice, "en-voice"},
		{"OutputDirectory", config.OutputDirectory, "/output"},
		{"FFmpegPath", config.FFmpegPath, "/bin/ffmpeg"},
		{"WhisperPath", config.WhisperPath, "/bin/whisper"},
		{"PiperPath", config.PiperPath, "/bin/piper"},
		{"PiperVoicesDir", config.PiperVoicesDir, "/voices"},
		{"PythonPath", config.PythonPath, "/bin/python"},
	}

	for _, tt := range tests {
		if tt.got != tt.expected {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
		}
	}
}
