package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"video-translator/internal/config"
	internalhttp "video-translator/internal/http"
	"video-translator/internal/logger"
	"video-translator/internal/media"
	"video-translator/internal/worker"
	"video-translator/models"
)

const openAITTSEndpoint = "https://api.openai.com/v1/audio/speech"

// Use shared HTTP client with connection pooling
var openaiTTSClient = internalhttp.OpenAIClient

// OpenAI TTS models
const (
	OpenAITTSModelStandard = "tts-1"    // Faster, lower quality
	OpenAITTSModelHD       = "tts-1-hd" // Slower, higher quality
)

// OpenAITTSService handles text-to-speech using OpenAI's API (high quality voices)
type OpenAITTSService struct {
	apiKey  string
	model   string  // tts-1 or tts-1-hd
	voice   string  // alloy, echo, fable, onyx, nova, shimmer
	speed   float64 // 0.25 to 4.0, default 1.15 for dubbing
	ffmpeg  *FFmpegService
	tempDir string
}

// OpenAI TTS voices with descriptions
var OpenAIVoices = map[string]string{
	"alloy":   "Alloy (Neutral, balanced)",
	"echo":    "Echo (Male, warm)",
	"fable":   "Fable (British, expressive)",
	"onyx":    "Onyx (Male, deep)",
	"nova":    "Nova (Female, friendly)",
	"shimmer": "Shimmer (Female, soft)",
}

// NewOpenAITTSService creates a new OpenAI TTS service
func NewOpenAITTSService(apiKey, model, voice string, speed float64) *OpenAITTSService {
	if model == "" {
		model = OpenAITTSModelStandard
	}
	if voice == "" {
		voice = "nova"
	}
	if speed <= 0 || speed > 4.0 {
		speed = 1.0 // Natural speed - atempo handles timing adjustments if needed
	}

	tempDir := filepath.Join(os.TempDir(), "video-translator-openai-tts")
	os.MkdirAll(tempDir, 0755)

	return &OpenAITTSService{
		apiKey:  apiKey,
		model:   model,
		voice:   voice,
		speed:   speed,
		ffmpeg:  NewFFmpegService(),
		tempDir: tempDir,
	}
}

// CheckInstalled verifies the API key is set (OpenAI TTS requires no local installation)
func (s *OpenAITTSService) CheckInstalled() error {
	if s.apiKey == "" {
		return fmt.Errorf("OpenAI API key is required for OpenAI TTS")
	}
	return nil
}

// SetVoice changes the voice for synthesis
func (s *OpenAITTSService) SetVoice(voice string) {
	if voice != "" {
		s.voice = voice
	}
}

// SetModel changes the TTS model
func (s *OpenAITTSService) SetModel(model string) {
	if model != "" {
		s.model = model
	}
}

// Synthesize generates audio from text using OpenAI TTS
func (s *OpenAITTSService) Synthesize(text, outputPath string) error {
	logger.LogInfo("OpenAI TTS: model=%s voice=%s speed=%.2f", s.model, s.voice, s.speed)

	if text == "" {
		return fmt.Errorf("empty text provided")
	}

	if s.apiKey == "" {
		return fmt.Errorf("OpenAI API key is required")
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":           s.model,
		"input":           text,
		"voice":           s.voice,
		"speed":           s.speed,
		"response_format": "mp3",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make request
	req, err := http.NewRequest("POST", openAITTSEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := openaiTTSClient.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return fmt.Errorf("OpenAI TTS API error: %s", errResp.Error.Message)
		}
		return fmt.Errorf("OpenAI TTS API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Response is raw audio bytes (MP3)
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read audio response: %w", err)
	}

	// Write to temp MP3 file
	mp3Path := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".mp3"
	if err := os.WriteFile(mp3Path, audioData, 0644); err != nil {
		return fmt.Errorf("failed to write audio file: %w", err)
	}

	// Convert MP3 to WAV for consistency with other TTS services
	if strings.HasSuffix(outputPath, ".wav") {
		if err := s.ffmpeg.ConvertToWAV(mp3Path, outputPath); err != nil {
			// Clean up MP3 on error
			os.Remove(mp3Path)
			return fmt.Errorf("failed to convert to WAV: %w", err)
		}
		// Clean up MP3
		os.Remove(mp3Path)
	} else if outputPath != mp3Path {
		// Rename MP3 to output path
		if err := os.Rename(mp3Path, outputPath); err != nil {
			return fmt.Errorf("failed to rename audio file: %w", err)
		}
	}

	return nil
}

// SynthesizeSubtitles generates audio for all subtitles with proper timing
func (s *OpenAITTSService) SynthesizeSubtitles(subs models.SubtitleList, outputPath string) error {
	return s.SynthesizeWithCallback(subs, outputPath, nil)
}

// openaiJobData contains data for an OpenAI TTS job.
type openaiJobData struct {
	index int
	text  string
	start time.Duration
	end   time.Duration
}

// SynthesizeWithCallback generates audio for subtitles with progress callback
// Uses internal worker pool for parallel processing (5-10x faster synthesis)
func (s *OpenAITTSService) SynthesizeWithCallback(
	subs models.SubtitleList,
	outputPath string,
	onProgress func(current, total int),
) error {
	if len(subs) == 0 {
		return fmt.Errorf("no subtitles provided")
	}

	if err := s.CheckInstalled(); err != nil {
		return err
	}

	// Create temp directory for segments
	segmentDir := filepath.Join(s.tempDir, fmt.Sprintf("segments_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(segmentDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(segmentDir)

	// Identify which subtitles need TTS (non-empty text)
	var jobs []openaiJobData
	for i, sub := range subs {
		if strings.TrimSpace(sub.Text) != "" {
			jobs = append(jobs, openaiJobData{
				index: i,
				text:  sub.Text,
				start: sub.StartTime,
				end:   sub.EndTime,
			})
		}
	}

	// Process TTS jobs in parallel using internal worker pool
	speechPaths := make(map[int]string)
	total := len(subs)

	if len(jobs) > 0 {
		// Process function for worker pool
		processJob := func(job worker.Job[openaiJobData]) (string, error) {
			data := job.Data
			speechPath := filepath.Join(segmentDir, fmt.Sprintf("speech_%04d.wav", data.index))

			// Synthesize the text
			if err := s.Synthesize(data.text, speechPath); err != nil {
				return "", err
			}

			// Adjust duration if needed
			targetDuration := (data.end - data.start).Seconds()
			if targetDuration > 0.2 {
				adjustedPath := filepath.Join(segmentDir, fmt.Sprintf("adjusted_%04d.wav", data.index))
				if err := s.ffmpeg.AdjustAudioDuration(speechPath, adjustedPath, targetDuration); err == nil {
					return adjustedPath, nil
				}
			}

			return speechPath, nil
		}

		// Progress callback adapter
		var progressCallback worker.ProgressFunc
		if onProgress != nil {
			progressCallback = func(completed, _ int) {
				onProgress(completed, total)
			}
		}

		// Run worker pool
		results, err := worker.Process(jobs, config.WorkersOpenAITTS, processJob, progressCallback)
		if err != nil {
			return fmt.Errorf("TTS synthesis failed: %w", err)
		}

		// Collect speech paths by index
		for i, path := range results {
			speechPaths[jobs[i].index] = path
		}
	}

	// Build final audio using AudioAssembler
	internalSubs := models.ToInternalSubtitles(subs)
	ffmpegMedia := media.NewFFmpegServiceWithPath(s.ffmpeg.GetPath())
	assembler := media.NewAudioAssembler(ffmpegMedia, segmentDir)

	if err := assembler.AssembleFromSpeechPaths(internalSubs, speechPaths, outputPath); err != nil {
		return fmt.Errorf("failed to assemble audio: %w", err)
	}

	return nil
}


// GetVoices returns the list of available OpenAI TTS voices
func GetOpenAIVoices() map[string]string {
	return OpenAIVoices
}

// GetVoiceList returns voice IDs as a slice
func GetOpenAIVoiceList() []string {
	return []string{"alloy", "echo", "fable", "onyx", "nova", "shimmer"}
}

// EstimateCost estimates the cost for synthesizing text
// OpenAI TTS pricing: $15/1M characters (tts-1), $30/1M characters (tts-1-hd)
func (s *OpenAITTSService) EstimateCost(charCount int) float64 {
	pricePerMillion := 15.0
	if s.model == OpenAITTSModelHD {
		pricePerMillion = 30.0
	}
	return float64(charCount) / 1000000.0 * pricePerMillion
}

// EstimateSubtitlesCost estimates the cost for synthesizing all subtitles
func (s *OpenAITTSService) EstimateSubtitlesCost(subs models.SubtitleList) float64 {
	totalChars := 0
	for _, sub := range subs {
		totalChars += len(sub.Text)
	}
	return s.EstimateCost(totalChars)
}
