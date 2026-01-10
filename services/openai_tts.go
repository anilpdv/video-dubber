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
	"sync"
	"time"
	"video-translator/models"
)

// maxTTSWorkers is the number of concurrent TTS API calls
// Reduced to avoid rate limiting and excessive resource usage
const maxTTSWorkers = 6 // Increased from 4 - OpenAI can handle more concurrent requests

const openAITTSEndpoint = "https://api.openai.com/v1/audio/speech"

// Package-level HTTP client with connection pooling (reused across requests)
var openaiTTSClient = &http.Client{
	Timeout: 1 * time.Minute,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
	},
}

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
	LogInfo("OpenAI TTS: model=%s voice=%s speed=%.2f", s.model, s.voice, s.speed)

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
		os.Rename(mp3Path, outputPath)
	}

	return nil
}

// SynthesizeSubtitles generates audio for all subtitles with proper timing
func (s *OpenAITTSService) SynthesizeSubtitles(subs models.SubtitleList, outputPath string) error {
	return s.SynthesizeWithCallback(subs, outputPath, nil)
}

// ttsJob represents a TTS synthesis job
type ttsJob struct {
	index int
	sub   models.Subtitle
}

// ttsResult represents the result of a TTS synthesis job
type ttsResult struct {
	index int
	path  string
	err   error
}

// SynthesizeWithCallback generates audio for subtitles with progress callback
// Uses parallel processing with worker pool for 5-10x faster synthesis
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
	var ttsJobs []ttsJob
	for i, sub := range subs {
		if strings.TrimSpace(sub.Text) != "" {
			ttsJobs = append(ttsJobs, ttsJob{index: i, sub: sub})
		}
	}

	// Process TTS jobs in parallel
	speechPaths := make(map[int]string)
	var speechMutex sync.Mutex
	var progressCount int
	var progressMutex sync.Mutex

	if len(ttsJobs) > 0 {
		jobs := make(chan ttsJob, len(ttsJobs))
		results := make(chan ttsResult, len(ttsJobs))

		// Start worker pool
		var wg sync.WaitGroup
		numWorkers := maxTTSWorkers
		if len(ttsJobs) < numWorkers {
			numWorkers = len(ttsJobs)
		}

		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					speechPath := filepath.Join(segmentDir, fmt.Sprintf("speech_%04d.wav", job.index))
					err := s.Synthesize(job.sub.Text, speechPath)

					// Adjust duration if synthesis succeeded
					if err == nil {
						targetDuration := (job.sub.EndTime - job.sub.StartTime).Seconds()
						if targetDuration > 0.2 {
							adjustedPath := filepath.Join(segmentDir, fmt.Sprintf("adjusted_%04d.wav", job.index))
							if adjErr := s.ffmpeg.AdjustAudioDuration(speechPath, adjustedPath, targetDuration); adjErr == nil {
								speechPath = adjustedPath
							}
						}
					}

					results <- ttsResult{index: job.index, path: speechPath, err: err}
				}
			}()
		}

		// Send jobs to workers
		go func() {
			for _, job := range ttsJobs {
				jobs <- job
			}
			close(jobs)
		}()

		// Collect results
		go func() {
			wg.Wait()
			close(results)
		}()

		// Process results as they come in
		for result := range results {
			if result.err != nil {
				return fmt.Errorf("failed to synthesize subtitle %d: %w", result.index+1, result.err)
			}

			speechMutex.Lock()
			speechPaths[result.index] = result.path
			speechMutex.Unlock()

			// Report progress
			if onProgress != nil {
				progressMutex.Lock()
				progressCount++
				onProgress(progressCount, len(subs))
				progressMutex.Unlock()
			}
		}
	}

	// Build final segment list in order (silence + speech)
	var segmentPaths []string
	var lastEndTime time.Duration

	for i, sub := range subs {
		// Add silence for gap between subtitles
		if sub.StartTime > lastEndTime {
			gap := sub.StartTime - lastEndTime
			if gap > 10*time.Millisecond {
				silencePath := filepath.Join(segmentDir, fmt.Sprintf("silence_%04d.wav", i))
				if err := s.ffmpeg.GenerateSilence(gap.Seconds(), silencePath); err != nil {
					return fmt.Errorf("failed to generate silence: %w", err)
				}
				segmentPaths = append(segmentPaths, silencePath)
			}
		}

		// Check if we have speech for this subtitle
		if speechPath, ok := speechPaths[i]; ok {
			segmentPaths = append(segmentPaths, speechPath)

			// Pad audio with silence if shorter than window to maintain sync
			windowDuration := (sub.EndTime - sub.StartTime).Seconds()
			if actualDuration, err := s.ffmpeg.GetAudioDuration(speechPath); err == nil {
				if actualDuration < windowDuration-0.05 { // 50ms tolerance
					paddingDuration := windowDuration - actualDuration
					paddingPath := filepath.Join(segmentDir, fmt.Sprintf("padding_%04d.wav", i))
					if err := s.ffmpeg.GenerateSilence(paddingDuration, paddingPath); err == nil {
						segmentPaths = append(segmentPaths, paddingPath)
					}
				}
			}
		} else {
			// Empty subtitle - add silence for the duration
			duration := sub.EndTime - sub.StartTime
			if duration > 0 {
				silencePath := filepath.Join(segmentDir, fmt.Sprintf("silence_sub_%04d.wav", i))
				if err := s.ffmpeg.GenerateSilence(duration.Seconds(), silencePath); err != nil {
					return fmt.Errorf("failed to generate silence for empty subtitle %d: %w", i+1, err)
				}
				segmentPaths = append(segmentPaths, silencePath)
			}
		}

		lastEndTime = sub.EndTime
	}

	// Concatenate all segments
	if err := s.ffmpeg.ConcatAudioFiles(segmentPaths, outputPath); err != nil {
		return fmt.Errorf("failed to concatenate audio: %w", err)
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
