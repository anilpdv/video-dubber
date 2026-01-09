package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"video-translator/models"
)

func TestNewTTSService(t *testing.T) {
	s := NewTTSService("")
	if s == nil {
		t.Fatal("NewTTSService() returned nil")
	}
	if s.voiceModel != "en_US-amy-medium" {
		t.Errorf("voiceModel = %q, want 'en_US-amy-medium'", s.voiceModel)
	}
	if s.piperPath != "piper" {
		t.Errorf("piperPath = %q, want 'piper'", s.piperPath)
	}
}

func TestNewTTSService_WithVoice(t *testing.T) {
	voice := "en_GB-alba-medium"
	s := NewTTSService(voice)
	if s == nil {
		t.Fatal("NewTTSService() returned nil")
	}
	if s.voiceModel != voice {
		t.Errorf("voiceModel = %q, want %q", s.voiceModel, voice)
	}
}

func TestTTSService_SetVoice(t *testing.T) {
	s := NewTTSService("")
	newVoice := "de_DE-thorsten-medium"
	s.SetVoice(newVoice)
	if s.GetVoice() != newVoice {
		t.Errorf("GetVoice() = %q, want %q", s.GetVoice(), newVoice)
	}
}

func TestTTSService_GetVoice(t *testing.T) {
	s := NewTTSService("fr_FR-upmc-medium")
	if s.GetVoice() != "fr_FR-upmc-medium" {
		t.Errorf("GetVoice() = %q, want 'fr_FR-upmc-medium'", s.GetVoice())
	}
}

func TestTTSService_getModelPath(t *testing.T) {
	s := NewTTSService("en_US-amy-medium")
	modelPath := s.getModelPath()
	if !filepath.IsAbs(modelPath) {
		t.Errorf("getModelPath() should return absolute path, got %q", modelPath)
	}
	if !containsString(modelPath, "en_US-amy-medium.onnx") {
		t.Errorf("getModelPath() = %q, should contain 'en_US-amy-medium.onnx'", modelPath)
	}
}

func TestTTSService_CheckVoiceModel_NotFound(t *testing.T) {
	s := &TTSService{
		voicesDir:  "/nonexistent/path",
		voiceModel: "test-voice",
	}
	err := s.CheckVoiceModel()
	if err == nil {
		t.Error("CheckVoiceModel() should return error for nonexistent model")
	}
}

func TestTTSService_CheckInstalled_NotFound(t *testing.T) {
	s := &TTSService{
		piperPath: "/nonexistent/piper",
	}
	err := s.CheckInstalled()
	if err == nil {
		t.Error("CheckInstalled() should return error for nonexistent piper")
	}
}

func TestTTSService_Synthesize_EmptyText(t *testing.T) {
	s := NewTTSService("")
	err := s.Synthesize("", "/tmp/output.wav")
	if err == nil {
		t.Error("Synthesize() should return error for empty text")
	}
}

func TestTTSService_SynthesizeSubtitles_Empty(t *testing.T) {
	s := NewTTSService("")
	subs := models.SubtitleList{}
	err := s.SynthesizeSubtitles(subs, "/tmp/output.wav")
	if err == nil {
		t.Error("SynthesizeSubtitles() should return error for empty subtitles")
	}
}

func TestTTSService_SynthesizeWithCallback_Empty(t *testing.T) {
	s := NewTTSService("")
	subs := models.SubtitleList{}
	err := s.SynthesizeWithCallback(subs, "/tmp/output.wav", nil)
	if err == nil {
		t.Error("SynthesizeWithCallback() should return error for empty subtitles")
	}
}

func TestTTSService_Cleanup(t *testing.T) {
	s := NewTTSService("")
	// Create temp dir if it doesn't exist
	os.MkdirAll(s.tempDir, 0755)

	err := s.Cleanup()
	if err != nil {
		t.Errorf("Cleanup() error = %v", err)
	}

	// Check that tempDir was removed
	if _, err := os.Stat(s.tempDir); !os.IsNotExist(err) {
		t.Error("Cleanup() should remove tempDir")
	}
}

func TestGetVoicesForLanguage_English(t *testing.T) {
	voices := GetVoicesForLanguage("en")
	if len(voices) == 0 {
		t.Error("GetVoicesForLanguage('en') returned empty map")
	}

	// Check for expected English voices
	expectedVoices := []string{"en_US-amy-medium", "en_US-ryan-medium", "en_GB-alba-medium"}
	for _, voice := range expectedVoices {
		if _, ok := voices[voice]; !ok {
			t.Errorf("expected voice %q not found", voice)
		}
	}
}

func TestGetVoicesForLanguage_German(t *testing.T) {
	voices := GetVoicesForLanguage("de")
	if len(voices) == 0 {
		t.Error("GetVoicesForLanguage('de') returned empty map")
	}
	if _, ok := voices["de_DE-thorsten-medium"]; !ok {
		t.Error("expected German voice not found")
	}
}

func TestGetVoicesForLanguage_Unknown(t *testing.T) {
	voices := GetVoicesForLanguage("xx")
	if len(voices) != 0 {
		t.Errorf("GetVoicesForLanguage('xx') should return empty map, got %d voices", len(voices))
	}
}

func TestDownloadVoiceModelURL(t *testing.T) {
	tests := []struct {
		voice    string
		expected string
	}{
		{
			"en_US-amy-medium",
			"https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_US/amy/medium/en_US-amy-medium.onnx",
		},
		{
			"de_DE-thorsten-medium",
			"https://huggingface.co/rhasspy/piper-voices/resolve/main/de/de_DE/thorsten/medium/de_DE-thorsten-medium.onnx",
		},
	}

	for _, tt := range tests {
		got := DownloadVoiceModelURL(tt.voice)
		if got != tt.expected {
			t.Errorf("DownloadVoiceModelURL(%q) = %q, want %q", tt.voice, got, tt.expected)
		}
	}
}

func TestDownloadVoiceModelURL_Invalid(t *testing.T) {
	tests := []string{
		"",
		"invalid",
		"en-amy",
	}

	for _, tt := range tests {
		got := DownloadVoiceModelURL(tt)
		if got != "" {
			t.Errorf("DownloadVoiceModelURL(%q) = %q, want empty string", tt, got)
		}
	}
}

func TestPiperVoices(t *testing.T) {
	if len(PiperVoices) == 0 {
		t.Error("PiperVoices map should not be empty")
	}

	// Check some expected voices exist
	expectedVoices := []string{
		"en_US-amy-medium",
		"en_US-ryan-medium",
		"de_DE-thorsten-medium",
		"fr_FR-upmc-medium",
	}

	for _, voice := range expectedVoices {
		if _, ok := PiperVoices[voice]; !ok {
			t.Errorf("expected voice %q not found in PiperVoices", voice)
		}
	}
}

func TestTTSService_SynthesizeSubtitles_WithGaps(t *testing.T) {
	// This test verifies the logic handles gaps between subtitles
	// Without actually calling piper (which may not be installed)
	s := &TTSService{
		piperPath:  "/nonexistent/piper",
		voiceModel: "test",
		voicesDir:  "/tmp",
		ffmpeg:     NewFFmpegService(),
		tempDir:    "/tmp/tts-test",
	}

	subs := models.SubtitleList{
		{Index: 1, StartTime: 0, EndTime: time.Second, Text: "First"},
		{Index: 2, StartTime: 2 * time.Second, EndTime: 3 * time.Second, Text: "Second"}, // 1 second gap
	}

	// This will fail because piper doesn't exist, but exercises the code path
	err := s.SynthesizeSubtitles(subs, "/tmp/output.wav")
	if err == nil {
		t.Error("SynthesizeSubtitles() should fail with nonexistent piper")
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
