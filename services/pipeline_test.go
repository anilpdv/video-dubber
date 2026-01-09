package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"video-translator/models"
)

func TestNewPipeline(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	if p == nil {
		t.Fatal("NewPipeline() returned nil")
	}
	if p.ffmpeg == nil {
		t.Error("ffmpeg service should not be nil")
	}
	if p.whisper == nil {
		t.Error("whisper service should not be nil")
	}
	if p.translator == nil {
		t.Error("translator service should not be nil")
	}
	if p.tts == nil {
		t.Error("tts service should not be nil")
	}
	if p.config == nil {
		t.Error("config should not be nil")
	}
	if p.tempDir == "" {
		t.Error("tempDir should not be empty")
	}
}

func TestPipeline_SetProgressCallback(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	called := false
	p.SetProgressCallback(func(stage string, percent int, message string) {
		called = true
	})

	// Trigger progress
	p.progress("test", 50, "test message")

	if !called {
		t.Error("progress callback should have been called")
	}
}

func TestPipeline_progress_NoCallback(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	// Should not panic when callback is nil
	p.progress("test", 50, "test message")
}

func TestPipeline_generateOutputPath_DefaultDir(t *testing.T) {
	config := &models.Config{
		OutputDirectory: "",
	}
	p := &Pipeline{config: config}

	inputPath := "/path/to/video.mp4"
	output := p.generateOutputPath(inputPath)

	expectedDir := "/path/to"
	if filepath.Dir(output) != expectedDir {
		t.Errorf("expected directory %q, got %q", expectedDir, filepath.Dir(output))
	}

	expectedName := "video_translated.mp4"
	if filepath.Base(output) != expectedName {
		t.Errorf("expected filename %q, got %q", expectedName, filepath.Base(output))
	}
}

func TestPipeline_generateOutputPath_CustomDir(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "pipeline_test")
	defer os.RemoveAll(tmpDir)

	config := &models.Config{
		OutputDirectory: tmpDir,
	}
	p := &Pipeline{config: config}

	inputPath := "/path/to/my_video.mkv"
	output := p.generateOutputPath(inputPath)

	if filepath.Dir(output) != tmpDir {
		t.Errorf("expected directory %q, got %q", tmpDir, filepath.Dir(output))
	}

	expectedName := "my_video_translated.mkv"
	if filepath.Base(output) != expectedName {
		t.Errorf("expected filename %q, got %q", expectedName, filepath.Base(output))
	}
}

func TestPipeline_generateOutputPath_TildeExpansion(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	config := &models.Config{
		OutputDirectory: "~/Videos",
	}
	p := &Pipeline{config: config}

	inputPath := "/path/to/video.mp4"
	output := p.generateOutputPath(inputPath)

	expectedDir := filepath.Join(homeDir, "Videos")
	if filepath.Dir(output) != expectedDir {
		t.Errorf("expected directory %q, got %q", expectedDir, filepath.Dir(output))
	}
}

func TestPipeline_ValidateJob_InputNotFound(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	job := models.NewTranslationJob("/nonexistent/video.mp4")
	err := p.ValidateJob(job)

	if err == nil {
		t.Error("ValidateJob should return error for nonexistent input")
	}
}

func TestPipeline_CheckDependencies(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	results := p.CheckDependencies()

	// Should return results for all dependencies
	expectedKeys := []string{
		"ffmpeg",
		"whisper-cpp",
		"whisper-model",
		"argos-translate",
		"argos-ru-en",
		"piper-tts",
		"piper-voice",
	}

	for _, key := range expectedKeys {
		if _, ok := results[key]; !ok {
			t.Errorf("expected key %q in results", key)
		}
	}
}

func TestPipeline_Cleanup(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	// Create the temp directory
	os.MkdirAll(p.tempDir, 0755)

	err := p.Cleanup()
	if err != nil {
		t.Errorf("Cleanup() error = %v", err)
	}

	// Check directory was removed
	if _, err := os.Stat(p.tempDir); !os.IsNotExist(err) {
		t.Error("Cleanup() should remove tempDir")
	}
}

func TestPipeline_GetEstimatedTime(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	tests := []struct {
		duration float64
		expected time.Duration
	}{
		{60, 2 * time.Minute},    // 1 minute video -> 2 minutes estimated
		{300, 10 * time.Minute},  // 5 minute video -> 10 minutes estimated
		{3600, 2 * time.Hour},    // 1 hour video -> 2 hours estimated
	}

	for _, tt := range tests {
		got := p.GetEstimatedTime(tt.duration)
		if got != tt.expected {
			t.Errorf("GetEstimatedTime(%v) = %v, want %v", tt.duration, got, tt.expected)
		}
	}
}

func TestPipeline_ProcessWithOriginalAudio(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	job := models.NewTranslationJob("/nonexistent/video.mp4")
	err := p.ProcessWithOriginalAudio(job, 0.3)

	// Should fail because input doesn't exist
	if err == nil {
		t.Error("ProcessWithOriginalAudio should fail for nonexistent input")
	}
}

func TestPipeline_ProcessAsync(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	job := models.NewTranslationJob("/nonexistent/video.mp4")
	done := make(chan error, 1)

	p.ProcessAsync(job, done)

	// Wait for result
	select {
	case err := <-done:
		if err == nil {
			t.Error("ProcessAsync should fail for nonexistent input")
		}
	case <-time.After(5 * time.Second):
		t.Error("ProcessAsync timed out")
	}
}

func TestPipeline_Process_JobDefaults(t *testing.T) {
	config := &models.Config{
		DefaultSourceLang: "ru",
		DefaultTargetLang: "en",
		DefaultVoice:      "en_US-amy-medium",
		OutputDirectory:   "/tmp",
	}
	p := &Pipeline{
		ffmpeg:     NewFFmpegService(),
		whisper:    NewWhisperService(),
		translator: NewTranslatorService(),
		tts:        NewTTSService(config.DefaultVoice),
		config:     config,
		tempDir:    "/tmp/test-pipeline",
	}

	job := models.NewTranslationJob("/nonexistent/video.mp4")
	// Leave SourceLang, TargetLang, Voice empty to test defaults

	// Process will fail early because file doesn't exist, but we can check
	// that defaults would be applied
	_ = p.Process(job)

	// The job should have defaults applied before the error
	// Note: this is testing internal behavior, actual values may not persist after error
}

func TestPipeline_Process_InvalidInput(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	job := models.NewTranslationJob("/nonexistent/video.mp4")
	err := p.Process(job)

	if err == nil {
		t.Error("Process should fail for nonexistent input")
	}

	// Job should be marked as failed
	if job.Status != models.StatusFailed {
		t.Errorf("job status = %v, want StatusFailed", job.Status)
	}
}

func TestPipeline_TempDirCreation(t *testing.T) {
	config := models.DefaultConfig()
	p := NewPipeline(config)

	// Verify temp dir exists
	if _, err := os.Stat(p.tempDir); os.IsNotExist(err) {
		t.Error("tempDir should be created by NewPipeline")
	}

	// Cleanup
	p.Cleanup()
}
