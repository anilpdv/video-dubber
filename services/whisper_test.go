package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"video-translator/internal/subtitle"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"00:00:00,000", 0},
		{"00:00:01,000", time.Second},
		{"00:01:00,000", time.Minute},
		{"01:00:00,000", time.Hour},
		{"00:00:01,500", 1500 * time.Millisecond},
		{"01:30:45,123", time.Hour + 30*time.Minute + 45*time.Second + 123*time.Millisecond},
		{"00:00:00.000", 0}, // Test with period instead of comma
		{"00:00:01.500", 1500 * time.Millisecond},
	}

	for _, tt := range tests {
		got := subtitle.ParseTimestamp(tt.input)
		if got != tt.expected {
			t.Errorf("ParseTimestamp(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestParseTimestamp_Invalid(t *testing.T) {
	tests := []string{
		"",
		"invalid",
		"00:00",
	}

	for _, tt := range tests {
		got := subtitle.ParseTimestamp(tt)
		if got != 0 {
			t.Errorf("ParseTimestamp(%q) = %v, want 0", tt, got)
		}
	}
}

func TestParseTimestamp_ShortFormat(t *testing.T) {
	// "1:2:3" is valid - parses as 1h 2m 3s
	got := subtitle.ParseTimestamp("1:2:3")
	expected := time.Hour + 2*time.Minute + 3*time.Second
	if got != expected {
		t.Errorf("ParseTimestamp(%q) = %v, want %v", "1:2:3", got, expected)
	}
}

func TestParseTimestampToSeconds(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"00:00:00.000", 0},
		{"00:00:01.000", 1},
		{"00:01:00.000", 60},
		{"01:00:00.000", 3600},
		{"00:05:30.500", 330.5},
		{"01:30:45.123", 5445.123},
	}

	for _, tt := range tests {
		got := subtitle.ParseTimestampToSeconds(tt.input)
		if got != tt.expected {
			t.Errorf("ParseTimestampToSeconds(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestParseTimestampToSeconds_Invalid(t *testing.T) {
	tests := []string{
		"",
		"invalid",
		"00:00",
	}

	for _, tt := range tests {
		got := subtitle.ParseTimestampToSeconds(tt)
		if got != 0 {
			t.Errorf("ParseTimestampToSeconds(%q) = %v, want 0", tt, got)
		}
	}
}

func TestNewWhisperService(t *testing.T) {
	s := NewWhisperService()
	if s == nil {
		t.Fatal("NewWhisperService() returned nil")
	}
	if s.whisperPath == "" {
		t.Error("whisperPath should not be empty")
	}
	if s.modelPath == "" {
		t.Error("modelPath should not be empty")
	}
}

func TestNewWhisperServiceWithPaths(t *testing.T) {
	whisperPath := "/custom/whisper"
	modelPath := "/custom/model.bin"

	s := NewWhisperServiceWithPaths(whisperPath, modelPath)
	if s == nil {
		t.Fatal("NewWhisperServiceWithPaths() returned nil")
	}
	if s.whisperPath != whisperPath {
		t.Errorf("whisperPath = %q, want %q", s.whisperPath, whisperPath)
	}
	if s.modelPath != modelPath {
		t.Errorf("modelPath = %q, want %q", s.modelPath, modelPath)
	}
}

func TestGetAvailableModels(t *testing.T) {
	models := GetAvailableModels()

	if len(models) == 0 {
		t.Error("GetAvailableModels() returned empty list")
	}

	// Check for expected models
	expectedModels := []string{"tiny", "base", "small", "medium", "large"}
	for _, expected := range expectedModels {
		found := false
		for _, m := range models {
			if m == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected model %q not found", expected)
		}
	}
}

func TestDownloadModelURL(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"tiny", "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin"},
		{"base", "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin"},
		{"large", "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large.bin"},
	}

	for _, tt := range tests {
		got := DownloadModelURL(tt.model)
		if got != tt.expected {
			t.Errorf("DownloadModelURL(%q) = %q, want %q", tt.model, got, tt.expected)
		}
	}
}

func TestWhisperService_CheckModel_NotFound(t *testing.T) {
	s := &WhisperService{
		whisperPath: "whisper-cli",
		modelPath:   "/nonexistent/path/model.bin",
	}

	err := s.CheckModel()
	if err == nil {
		t.Error("CheckModel() should return error for nonexistent model")
	}
}

func TestWhisperService_CheckInstalled_NotFound(t *testing.T) {
	s := &WhisperService{
		whisperPath: "/nonexistent/whisper-cli",
		modelPath:   "/some/model.bin",
	}

	err := s.CheckInstalled()
	if err == nil {
		t.Error("CheckInstalled() should return error for nonexistent whisper binary")
	}
}

func TestParseSRTFile(t *testing.T) {
	// Create a temp SRT file
	tmpDir, err := os.MkdirTemp("", "whisper_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srtContent := `1
00:00:00,000 --> 00:00:02,500
Hello world

2
00:00:03,000 --> 00:00:05,500
This is a test

3
00:00:06,000 --> 00:00:08,000
Third subtitle
`

	srtPath := filepath.Join(tmpDir, "test.srt")
	if err := os.WriteFile(srtPath, []byte(srtContent), 0644); err != nil {
		t.Fatalf("failed to write SRT file: %v", err)
	}

	subs, err := subtitle.ParseSRTFile(srtPath)
	if err != nil {
		t.Fatalf("ParseSRTFile() error = %v", err)
	}

	if len(subs) != 3 {
		t.Errorf("ParseSRTFile() returned %d subtitles, want 3", len(subs))
	}

	// Check first subtitle
	if subs[0].Index != 1 {
		t.Errorf("subs[0].Index = %d, want 1", subs[0].Index)
	}
	if subs[0].Text != "Hello world" {
		t.Errorf("subs[0].Text = %q, want 'Hello world'", subs[0].Text)
	}
	if subs[0].StartTime != 0 {
		t.Errorf("subs[0].StartTime = %v, want 0", subs[0].StartTime)
	}
	if subs[0].EndTime != 2500*time.Millisecond {
		t.Errorf("subs[0].EndTime = %v, want 2.5s", subs[0].EndTime)
	}

	// Check second subtitle
	if subs[1].Index != 2 {
		t.Errorf("subs[1].Index = %d, want 2", subs[1].Index)
	}
	if subs[1].Text != "This is a test" {
		t.Errorf("subs[1].Text = %q, want 'This is a test'", subs[1].Text)
	}
}

func TestParseSRTFile_NotFound(t *testing.T) {
	_, err := subtitle.ParseSRTFile("/nonexistent/file.srt")
	if err == nil {
		t.Error("ParseSRTFile() should return error for nonexistent file")
	}
}

func TestParseSRTFile_Empty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "whisper_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srtPath := filepath.Join(tmpDir, "empty.srt")
	if err := os.WriteFile(srtPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write SRT file: %v", err)
	}

	subs, err := subtitle.ParseSRTFile(srtPath)
	if err != nil {
		t.Fatalf("ParseSRTFile() error = %v", err)
	}

	if len(subs) != 0 {
		t.Errorf("ParseSRTFile() returned %d subtitles, want 0", len(subs))
	}
}

func TestParseSRTFile_WithPeriodSeparator(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "whisper_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Some SRT files use period instead of comma
	srtContent := `1
00:00:00.000 --> 00:00:02.500
Test subtitle
`

	srtPath := filepath.Join(tmpDir, "test.srt")
	if err := os.WriteFile(srtPath, []byte(srtContent), 0644); err != nil {
		t.Fatalf("failed to write SRT file: %v", err)
	}

	subs, err := subtitle.ParseSRTFile(srtPath)
	if err != nil {
		t.Fatalf("ParseSRTFile() error = %v", err)
	}

	if len(subs) != 1 {
		t.Errorf("ParseSRTFile() returned %d subtitles, want 1", len(subs))
	}
}

func TestParseSRTFile_MultilineText(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "whisper_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srtContent := `1
00:00:00,000 --> 00:00:02,500
Line one
Line two
Line three

`

	srtPath := filepath.Join(tmpDir, "test.srt")
	if err := os.WriteFile(srtPath, []byte(srtContent), 0644); err != nil {
		t.Fatalf("failed to write SRT file: %v", err)
	}

	subs, err := subtitle.ParseSRTFile(srtPath)
	if err != nil {
		t.Fatalf("ParseSRTFile() error = %v", err)
	}

	if len(subs) != 1 {
		t.Fatalf("ParseSRTFile() returned %d subtitles, want 1", len(subs))
	}

	// Text should contain all lines
	expected := "Line one Line two Line three"
	if subs[0].Text != expected {
		t.Errorf("subs[0].Text = %q, want %q", subs[0].Text, expected)
	}
}

func TestWhisperService_Transcribe_InvalidInput(t *testing.T) {
	s := &WhisperService{
		whisperPath: "/nonexistent/whisper-cli",
		modelPath:   "/some/model.bin",
	}

	// Should fail because whisper is not installed
	_, err := s.Transcribe("/nonexistent/audio.wav", "en")
	if err == nil {
		t.Error("Transcribe() should return error for nonexistent whisper")
	}
}

func TestWhisperService_TranscribeWithProgress_InvalidInput(t *testing.T) {
	s := &WhisperService{
		whisperPath: "/nonexistent/whisper-cli",
		modelPath:   "/some/model.bin",
	}

	// Should fail because whisper is not installed
	_, err := s.TranscribeWithProgress("/nonexistent/audio.wav", "en", 60.0, nil)
	if err == nil {
		t.Error("TranscribeWithProgress() should return error for nonexistent whisper")
	}
}

func TestWhisperService_TranscribeToText_InvalidInput(t *testing.T) {
	s := &WhisperService{
		whisperPath: "/nonexistent/whisper-cli",
		modelPath:   "/some/model.bin",
	}

	// Should fail because whisper is not installed
	_, err := s.TranscribeToText("/nonexistent/audio.wav", "en")
	if err == nil {
		t.Error("TranscribeToText() should return error for nonexistent whisper")
	}
}
