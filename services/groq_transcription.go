package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"video-translator/internal/logger"
	"video-translator/models"
)

const (
	groqTranscriptionEndpoint = "https://api.groq.com/openai/v1/audio/transcriptions"
	groqWhisperModel          = "whisper-large-v3" // Groq's fastest Whisper model
)

// GroqTranscriptionService handles transcription using Groq's ultra-fast Whisper API.
// Groq uses specialized LPU (Language Processing Unit) hardware for 5-10x faster inference.
// Cost: ~$0.03/hour of audio
type GroqTranscriptionService struct {
	apiKey string
	ffmpeg *FFmpegService
}

// NewGroqTranscriptionService creates a new Groq transcription service.
func NewGroqTranscriptionService(apiKey string) *GroqTranscriptionService {
	return &GroqTranscriptionService{
		apiKey: apiKey,
		ffmpeg: NewFFmpegService(),
	}
}

// CheckInstalled verifies the API key is set.
func (s *GroqTranscriptionService) CheckInstalled() error {
	if s.apiKey == "" {
		return fmt.Errorf("Groq API key is required. Get one at https://console.groq.com")
	}
	return nil
}

// Transcribe transcribes audio using Groq's Whisper API.
// Returns subtitles with timestamps.
func (s *GroqTranscriptionService) Transcribe(audioPath, language string) (models.SubtitleList, error) {
	return s.TranscribeWithProgress(audioPath, language, nil)
}

// TranscribeWithProgress transcribes audio with progress callbacks.
func (s *GroqTranscriptionService) TranscribeWithProgress(
	audioPath, language string,
	onProgress func(percent int, message string),
) (models.SubtitleList, error) {
	logger.LogInfo("Groq Whisper API: model=%s lang=%s file=%s", groqWhisperModel, language, filepath.Base(audioPath))

	if err := s.CheckInstalled(); err != nil {
		return nil, err
	}

	// Check file size - Groq has a 25MB limit like OpenAI
	fileInfo, err := os.Stat(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// 25MB limit
	const maxFileSize = 25 * 1024 * 1024
	if fileInfo.Size() > maxFileSize {
		// Compress audio for upload
		return s.transcribeCompressed(audioPath, language, onProgress)
	}

	return s.transcribeDirect(audioPath, language, onProgress)
}

// transcribeDirect transcribes audio directly without compression.
func (s *GroqTranscriptionService) transcribeDirect(
	audioPath, language string,
	onProgress func(percent int, message string),
) (models.SubtitleList, error) {
	if onProgress != nil {
		onProgress(20, "Uploading audio to Groq...")
	}

	// Open the audio file
	file, err := os.Open(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the file
	part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Add fields
	writer.WriteField("model", groqWhisperModel)
	if language != "" {
		writer.WriteField("language", language)
	}
	writer.WriteField("response_format", "verbose_json") // Get timestamps
	writer.Close()

	if onProgress != nil {
		onProgress(25, "Transcribing with Groq (ultra-fast)...")
	}

	// Make the API request
	req, err := http.NewRequest("POST", groqTranscriptionEndpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Groq is fast, but use reasonable timeout
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Groq API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("Groq API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("Groq API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if onProgress != nil {
		onProgress(38, "Parsing transcription...")
	}

	// Parse verbose_json response
	subtitles, err := s.parseVerboseJSON(respBody)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Groq response: %w", err)
	}

	if onProgress != nil {
		onProgress(40, fmt.Sprintf("Transcribed %d segments", len(subtitles)))
	}

	return subtitles, nil
}

// parseVerboseJSON parses Groq's verbose_json response format.
func (s *GroqTranscriptionService) parseVerboseJSON(data []byte) (models.SubtitleList, error) {
	var response struct {
		Segments []struct {
			Start float64 `json:"start"`
			End   float64 `json:"end"`
			Text  string  `json:"text"`
		} `json:"segments"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	var subtitles models.SubtitleList
	for _, seg := range response.Segments {
		subtitles = append(subtitles, models.Subtitle{
			StartTime: time.Duration(seg.Start * float64(time.Second)),
			EndTime:   time.Duration(seg.End * float64(time.Second)),
			Text:      seg.Text,
		})
	}

	return subtitles, nil
}

// transcribeCompressed compresses audio before upload for large files.
func (s *GroqTranscriptionService) transcribeCompressed(
	audioPath, language string,
	onProgress func(percent int, message string),
) (models.SubtitleList, error) {
	if onProgress != nil {
		onProgress(15, "Compressing audio for upload...")
	}

	// Compress to MP3 with lower bitrate
	compressedPath := audioPath + ".compressed.mp3"
	if err := s.ffmpeg.CompressToMP3(audioPath, compressedPath, 64); err != nil {
		return nil, fmt.Errorf("failed to compress audio: %w", err)
	}
	defer os.Remove(compressedPath)

	return s.transcribeDirect(compressedPath, language, onProgress)
}

// EstimateTime estimates transcription time for audio duration.
// Groq is extremely fast - typically real-time or faster.
func (s *GroqTranscriptionService) EstimateTime(audioDurationSeconds float64) float64 {
	// Groq processes at roughly 10x real-time speed
	return audioDurationSeconds / 10.0
}

// EstimateCost estimates cost for audio duration.
// Groq pricing: approximately $0.0005 per minute of audio.
func (s *GroqTranscriptionService) EstimateCost(audioDurationMinutes float64) float64 {
	return audioDurationMinutes * 0.0005
}
