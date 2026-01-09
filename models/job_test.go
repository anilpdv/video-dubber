package models

import (
	"errors"
	"testing"
	"time"
)

func TestNewTranslationJob(t *testing.T) {
	job := NewTranslationJob("/path/to/video.mp4")

	if job.ID == "" {
		t.Error("expected non-empty ID")
	}
	if job.InputPath != "/path/to/video.mp4" {
		t.Errorf("expected InputPath /path/to/video.mp4, got %s", job.InputPath)
	}
	if job.FileName != "video.mp4" {
		t.Errorf("expected FileName video.mp4, got %s", job.FileName)
	}
	if job.Status != StatusPending {
		t.Errorf("expected StatusPending, got %s", job.Status)
	}
	if job.Progress != 0 {
		t.Errorf("expected Progress 0, got %d", job.Progress)
	}
	if job.SourceLang != "ru" {
		t.Errorf("expected default SourceLang ru, got %s", job.SourceLang)
	}
	if job.TargetLang != "en" {
		t.Errorf("expected default TargetLang en, got %s", job.TargetLang)
	}
	if job.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestSetStatus(t *testing.T) {
	job := NewTranslationJob("/path/to/video.mp4")

	job.SetStatus(StatusProcessing, "Starting...", 10)

	if job.Status != StatusProcessing {
		t.Errorf("expected StatusProcessing, got %s", job.Status)
	}
	if job.CurrentStage != "Starting..." {
		t.Errorf("expected stage 'Starting...', got %s", job.CurrentStage)
	}
	if job.Progress != 10 {
		t.Errorf("expected progress 10, got %d", job.Progress)
	}
}

func TestComplete(t *testing.T) {
	job := NewTranslationJob("/path/to/video.mp4")
	job.SetStatus(StatusProcessing, "Processing", 50)

	job.Complete("/output/video_translated.mp4")

	if job.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", job.Status)
	}
	if job.OutputPath != "/output/video_translated.mp4" {
		t.Errorf("expected OutputPath /output/video_translated.mp4, got %s", job.OutputPath)
	}
	if job.Progress != 100 {
		t.Errorf("expected Progress 100, got %d", job.Progress)
	}
	if job.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestFail(t *testing.T) {
	job := NewTranslationJob("/path/to/video.mp4")
	job.SetStatus(StatusProcessing, "Processing", 50)

	testErr := errors.New("test error")
	job.Fail(testErr)

	if job.Status != StatusFailed {
		t.Errorf("expected StatusFailed, got %s", job.Status)
	}
	if job.Error != testErr {
		t.Errorf("expected error to be set")
	}
	if job.Progress != 0 {
		t.Errorf("expected Progress 0 after failure, got %d", job.Progress)
	}
	if job.CurrentStage != "Failed" {
		t.Errorf("expected CurrentStage 'Failed', got %s", job.CurrentStage)
	}
}

func TestStatusText(t *testing.T) {
	tests := []struct {
		status   JobStatus
		err      error
		expected string
	}{
		{StatusPending, nil, "Ready to translate"},
		{StatusProcessing, nil, "Starting..."},
		{StatusExtracting, nil, "Extracting audio..."},
		{StatusTranscribing, nil, "Transcribing..."},
		{StatusTranslating, nil, "Translating..."},
		{StatusSynthesizing, nil, "Generating speech..."},
		{StatusMuxing, nil, "Creating video..."},
		{StatusCompleted, nil, "Completed!"},
		{StatusFailed, nil, "Failed"},
		{StatusFailed, errors.New("some error"), "Failed: some error"},
	}

	for _, tt := range tests {
		job := &TranslationJob{Status: tt.status, Error: tt.err}
		got := job.StatusText()
		if got != tt.expected {
			t.Errorf("StatusText(%s) = %q, want %q", tt.status, got, tt.expected)
		}
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status JobStatus
		icon   string
	}{
		{StatusPending, "⏳"},
		{StatusProcessing, "🔄"},
		{StatusExtracting, "🔄"},
		{StatusTranscribing, "🔄"},
		{StatusTranslating, "🔄"},
		{StatusSynthesizing, "🔄"},
		{StatusMuxing, "🔄"},
		{StatusCompleted, "✅"},
		{StatusFailed, "❌"},
	}

	for _, tt := range tests {
		job := &TranslationJob{Status: tt.status}
		if got := job.StatusIcon(); got != tt.icon {
			t.Errorf("StatusIcon(%s) = %q, want %q", tt.status, got, tt.icon)
		}
	}
}

func TestStatusIcon_UnknownStatus(t *testing.T) {
	job := &TranslationJob{Status: "unknown"}
	if got := job.StatusIcon(); got != "📄" {
		t.Errorf("StatusIcon(unknown) = %q, want 📄", got)
	}
}

func TestJobFields(t *testing.T) {
	job := NewTranslationJob("/path/to/video.mp4")
	job.AudioPath = "/tmp/audio.wav"
	job.TranscriptPath = "/tmp/transcript.srt"
	job.DubbedAudioPath = "/tmp/dubbed.wav"
	job.Voice = "en-US-amy"

	if job.AudioPath != "/tmp/audio.wav" {
		t.Error("AudioPath not set correctly")
	}
	if job.TranscriptPath != "/tmp/transcript.srt" {
		t.Error("TranscriptPath not set correctly")
	}
	if job.DubbedAudioPath != "/tmp/dubbed.wav" {
		t.Error("DubbedAudioPath not set correctly")
	}
	if job.Voice != "en-US-amy" {
		t.Error("Voice not set correctly")
	}
}

func TestJobTimestamps(t *testing.T) {
	before := time.Now()
	job := NewTranslationJob("/path/to/video.mp4")
	after := time.Now()

	if job.CreatedAt.Before(before) || job.CreatedAt.After(after) {
		t.Error("CreatedAt should be set to current time")
	}

	job.Complete("/output/video.mp4")

	if job.CompletedAt == nil {
		t.Error("CompletedAt should be set after Complete()")
	}
	if job.CompletedAt.Before(job.CreatedAt) {
		t.Error("CompletedAt should be after CreatedAt")
	}
}

func TestStatusText_UnknownStatus(t *testing.T) {
	job := &TranslationJob{Status: "unknown"}
	got := job.StatusText()
	// Unknown status should return the status string itself
	if got != "unknown" {
		t.Errorf("StatusText(unknown) = %q, want 'unknown'", got)
	}
}

func TestNewTranslationJob_DefaultVoice(t *testing.T) {
	job := NewTranslationJob("/path/to/video.mp4")
	if job.Voice != "en-US-AriaNeural" {
		t.Errorf("expected default Voice 'en-US-AriaNeural', got %s", job.Voice)
	}
}

func TestTranslationJob_SetAllStatuses(t *testing.T) {
	job := NewTranslationJob("/path/to/video.mp4")

	// Test all status transitions
	statuses := []struct {
		status JobStatus
		stage  string
		progress int
	}{
		{StatusExtracting, "Extracting audio", 0},
		{StatusTranscribing, "Transcribing audio", 15},
		{StatusTranslating, "Translating text", 40},
		{StatusSynthesizing, "Generating speech", 60},
		{StatusMuxing, "Creating video", 85},
	}

	for _, s := range statuses {
		job.SetStatus(s.status, s.stage, s.progress)
		if job.Status != s.status {
			t.Errorf("expected status %s, got %s", s.status, job.Status)
		}
		if job.CurrentStage != s.stage {
			t.Errorf("expected stage %s, got %s", s.stage, job.CurrentStage)
		}
		if job.Progress != s.progress {
			t.Errorf("expected progress %d, got %d", s.progress, job.Progress)
		}
	}
}
