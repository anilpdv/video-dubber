package models

import (
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type JobStatus string

const (
	StatusPending     JobStatus = "pending"
	StatusProcessing  JobStatus = "processing"
	StatusExtracting  JobStatus = "extracting"
	StatusTranscribing JobStatus = "transcribing"
	StatusTranslating JobStatus = "translating"
	StatusSynthesizing JobStatus = "synthesizing"
	StatusMuxing      JobStatus = "muxing"
	StatusCompleted   JobStatus = "completed"
	StatusFailed      JobStatus = "failed"
)

type TranslationJob struct {
	ID           string
	InputPath    string
	OutputPath   string
	FileName     string
	Status       JobStatus
	Progress     int    // 0-100
	CurrentStage string
	Error        error
	CreatedAt    time.Time
	CompletedAt  *time.Time

	// Translation settings
	SourceLang string
	TargetLang string
	Voice      string

	// Intermediate files
	AudioPath      string
	TranscriptPath string
	DubbedAudioPath string
}

func NewTranslationJob(inputPath string) *TranslationJob {
	return &TranslationJob{
		ID:         uuid.New().String(),
		InputPath:  inputPath,
		FileName:   filepath.Base(inputPath),
		Status:     StatusPending,
		Progress:   0,
		CreatedAt:  time.Now(),
		SourceLang: "ru",
		TargetLang: "en",
		Voice:      "en-US-AriaNeural",
	}
}

func (j *TranslationJob) SetStatus(status JobStatus, stage string, progress int) {
	j.Status = status
	j.CurrentStage = stage
	j.Progress = progress
}

func (j *TranslationJob) Complete(outputPath string) {
	j.Status = StatusCompleted
	j.OutputPath = outputPath
	j.Progress = 100
	now := time.Now()
	j.CompletedAt = &now
}

func (j *TranslationJob) Fail(err error) {
	j.Status = StatusFailed
	j.Error = err
	j.Progress = 0
	j.CurrentStage = "Failed"
}

func (j *TranslationJob) StatusText() string {
	switch j.Status {
	case StatusPending:
		return "Ready to translate"
	case StatusProcessing:
		return "Starting..."
	case StatusExtracting:
		return "Extracting audio..."
	case StatusTranscribing:
		return "Transcribing..."
	case StatusTranslating:
		return "Translating..."
	case StatusSynthesizing:
		return "Generating speech..."
	case StatusMuxing:
		return "Creating video..."
	case StatusCompleted:
		return "Completed!"
	case StatusFailed:
		if j.Error != nil {
			return "Failed: " + j.Error.Error()
		}
		return "Failed"
	default:
		return string(j.Status)
	}
}

// StatusIcon returns an emoji icon representing the job status
func (j *TranslationJob) StatusIcon() string {
	switch j.Status {
	case StatusPending:
		return "⏳"
	case StatusProcessing, StatusExtracting, StatusTranscribing, StatusTranslating, StatusSynthesizing, StatusMuxing:
		return "🔄"
	case StatusCompleted:
		return "✅"
	case StatusFailed:
		return "❌"
	default:
		return "📄"
	}
}
