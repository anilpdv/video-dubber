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
	"video-translator/internal/logger"
	"video-translator/internal/media"
	"video-translator/internal/worker"
	"video-translator/models"
)

const fishAudioTTSEndpoint = "https://api.fish.audio/v1/tts"

// fishAudioClient is a shared HTTP client for Fish Audio API calls
var fishAudioClient = &http.Client{
	Timeout: 2 * time.Minute,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	},
}

// Fish Audio TTS models
const (
	FishAudioModelS1       = "s1"         // OpenAudio S1 (flagship)
	FishAudioModelSpeech15 = "speech-1.5" // Stable
	FishAudioModelSpeech16 = "speech-1.6" // Latest
)

// FishAudioTTSService handles text-to-speech using Fish Audio's API (Fish Speech quality)
type FishAudioTTSService struct {
	apiKey      string
	model       string  // s1, speech-1.5, speech-1.6
	referenceID string  // Voice model ID
	speed       float64 // Speech speed (0.5-2.0)
	ffmpeg      *FFmpegService
	tempDir     string
}

// FishAudioVoices contains pre-built voice options from Fish Audio
// Users can also use custom voice IDs from their Fish Audio account
var FishAudioVoices = map[string]string{
	// Pre-built high-quality voices (common reference IDs)
	"default":  "Default Fish Audio voice",
	"custom":   "Use custom reference ID from your account",
}

// NewFishAudioTTSService creates a new Fish Audio TTS service
func NewFishAudioTTSService(apiKey, model, referenceID string, speed float64) *FishAudioTTSService {
	if model == "" {
		model = FishAudioModelS1 // S1 is the flagship model with best quality (same price)
	}
	if speed <= 0 || speed > 2.0 {
		speed = 1.0
	}

	tempDir := filepath.Join(os.TempDir(), "video-translator-fish-audio")
	os.MkdirAll(tempDir, 0755)

	return &FishAudioTTSService{
		apiKey:      apiKey,
		model:       model,
		referenceID: referenceID,
		speed:       speed,
		ffmpeg:      NewFFmpegService(),
		tempDir:     tempDir,
	}
}

// CheckInstalled verifies the API key is set
func (s *FishAudioTTSService) CheckInstalled() error {
	if s.apiKey == "" {
		return fmt.Errorf("Fish Audio API key is required")
	}
	return nil
}

// SetVoice changes the voice reference ID for synthesis
func (s *FishAudioTTSService) SetVoice(referenceID string) {
	if referenceID != "" {
		s.referenceID = referenceID
	}
}

// SetModel changes the TTS model
func (s *FishAudioTTSService) SetModel(model string) {
	if model != "" {
		s.model = model
	}
}

// fishAudioRequest represents the Fish Audio TTS API request body
type fishAudioRequest struct {
	Text              string            `json:"text"`
	ReferenceID       string            `json:"reference_id,omitempty"`
	Temperature       float64           `json:"temperature,omitempty"`
	TopP              float64           `json:"top_p,omitempty"`
	ChunkLength       int               `json:"chunk_length,omitempty"`
	RepetitionPenalty float64           `json:"repetition_penalty,omitempty"`
	MaxNewTokens      int               `json:"max_new_tokens,omitempty"`
	Prosody           *fishAudioProsody `json:"prosody,omitempty"`
	Format            string            `json:"format,omitempty"`
	Mp3Bitrate        int               `json:"mp3_bitrate,omitempty"`
	Normalize         bool              `json:"normalize,omitempty"`
	Latency           string            `json:"latency,omitempty"`
}

type fishAudioProsody struct {
	Speed  float64 `json:"speed"`
	Volume float64 `json:"volume"`
}

// SynthesizeWithEmotion generates audio from text with an emotion tag for Fish Audio TTS
func (s *FishAudioTTSService) SynthesizeWithEmotion(text, emotion, outputPath string) error {
	// Prepend emotion tag if provided (Fish Audio emotion control)
	// Format: (emotion) text - e.g., "(happy) Hello world!"
	if emotion != "" && emotion != "calm" {
		text = fmt.Sprintf("(%s) %s", emotion, text)
	}
	return s.Synthesize(text, outputPath)
}

// Synthesize generates audio from text using Fish Audio TTS
func (s *FishAudioTTSService) Synthesize(text, outputPath string) error {
	logger.LogInfo("Fish Audio TTS: model=%s reference_id=%s speed=%.2f", s.model, s.referenceID, s.speed)

	if text == "" {
		return fmt.Errorf("empty text provided")
	}

	if s.apiKey == "" {
		return fmt.Errorf("Fish Audio API key is required")
	}

	// Build request body with optimized parameters for voice consistency
	// Lower temperature/topP = more consistent voice (0.9 was too random)
	// repetition_penalty prevents glitches/stuttering
	// chunk_length controls text segmentation for stability
	reqBody := fishAudioRequest{
		Text:              text,
		Temperature:       0.4,   // Reduced from 0.9 for voice consistency
		TopP:              0.4,   // Reduced from 0.9 for voice consistency
		ChunkLength:       300,   // Optimal chunk size for stability
		RepetitionPenalty: 1.2,   // Prevents glitches/stuttering
		MaxNewTokens:      1024,  // Safe generation limit
		Prosody: &fishAudioProsody{
			Speed:  s.speed,
			Volume: 0,
		},
		Format:     "mp3",
		Mp3Bitrate: 192, // Higher quality audio
		Normalize:  true,
		Latency:    "normal",
	}

	// Add reference ID if specified
	if s.referenceID != "" && s.referenceID != "default" {
		reqBody.ReferenceID = s.referenceID
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make request
	req, err := http.NewRequest("POST", fishAudioTTSEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("model", s.model)

	resp, err := fishAudioClient.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
			Detail  string `json:"detail"`
		}
		if json.Unmarshal(respBody, &errResp) == nil {
			if errResp.Message != "" {
				return fmt.Errorf("Fish Audio API error: %s", errResp.Message)
			}
			if errResp.Error != "" {
				return fmt.Errorf("Fish Audio API error: %s", errResp.Error)
			}
			if errResp.Detail != "" {
				return fmt.Errorf("Fish Audio API error: %s", errResp.Detail)
			}
		}
		return fmt.Errorf("Fish Audio API error (status %d): %s", resp.StatusCode, string(respBody))
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
			os.Remove(mp3Path)
			return fmt.Errorf("failed to convert to WAV: %w", err)
		}
		os.Remove(mp3Path)
	} else if outputPath != mp3Path {
		if err := os.Rename(mp3Path, outputPath); err != nil {
			return fmt.Errorf("failed to rename audio file: %w", err)
		}
	}

	return nil
}

// SynthesizeSubtitles generates audio for all subtitles with proper timing
func (s *FishAudioTTSService) SynthesizeSubtitles(subs models.SubtitleList, outputPath string) error {
	return s.SynthesizeWithCallback(subs, outputPath, nil)
}

// fishAudioJobData contains data for a Fish Audio TTS job
type fishAudioJobData struct {
	index   int
	text    string
	emotion string // Fish Audio emotion tag (happy, sad, excited, etc.)
	start   time.Duration
	end     time.Duration
}

// SynthesizeWithCallback generates audio for subtitles with progress callback
func (s *FishAudioTTSService) SynthesizeWithCallback(
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
	var jobs []fishAudioJobData
	for i, sub := range subs {
		if strings.TrimSpace(sub.Text) != "" {
			jobs = append(jobs, fishAudioJobData{
				index:   i,
				text:    sub.Text,
				emotion: sub.Emotion, // Fish Audio emotion tag for expressive speech
				start:   sub.StartTime,
				end:     sub.EndTime,
			})
		}
	}

	// Process TTS jobs in parallel using internal worker pool
	speechPaths := make(map[int]string)
	total := len(subs)

	if len(jobs) > 0 {
		// Process function for worker pool
		processJob := func(job worker.Job[fishAudioJobData]) (string, error) {
			data := job.Data
			speechPath := filepath.Join(segmentDir, fmt.Sprintf("speech_%04d.wav", data.index))

			// Synthesize the text with emotion (if set)
			if err := s.SynthesizeWithEmotion(data.text, data.emotion, speechPath); err != nil {
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

		// Run worker pool with limited workers (Fish Audio starter tier = 5 concurrent)
		workers := config.WorkersFishAudio
		if workers == 0 {
			workers = 5 // Default to starter tier
		}
		results, err := worker.Process(jobs, workers, processJob, progressCallback)
		if err != nil {
			return fmt.Errorf("TTS synthesis failed: %w", err)
		}

		// Collect speech paths by index
		for i, path := range results {
			speechPaths[jobs[i].index] = path
		}
	}

	// Build final audio using AudioAssembler with parallel gap processing
	internalSubs := models.ToInternalSubtitles(subs)
	ffmpegMedia := media.NewFFmpegServiceWithPath(s.ffmpeg.GetPath())
	assembler := media.NewAudioAssembler(ffmpegMedia, segmentDir)

	if err := assembler.AssembleFromSpeechPathsParallel(internalSubs, speechPaths, outputPath); err != nil {
		return fmt.Errorf("failed to assemble audio: %w", err)
	}

	return nil
}

// GetFishAudioVoices returns the list of available Fish Audio voices
func GetFishAudioVoices() map[string]string {
	return FishAudioVoices
}

// GetFishAudioVoiceList returns voice IDs as a slice
func GetFishAudioVoiceList() []string {
	return []string{"default", "custom"}
}

// GetFishAudioModels returns the list of available Fish Audio TTS models
func GetFishAudioModels() []string {
	return []string{FishAudioModelSpeech16, FishAudioModelSpeech15, FishAudioModelS1}
}

// EstimateCost estimates the cost for synthesizing text
// Fish Audio pricing: $15/1M UTF-8 bytes
func (s *FishAudioTTSService) EstimateCost(byteCount int) float64 {
	return float64(byteCount) / 1000000.0 * 15.0
}

// EstimateSubtitlesCost estimates the cost for synthesizing all subtitles
func (s *FishAudioTTSService) EstimateSubtitlesCost(subs models.SubtitleList) float64 {
	totalBytes := 0
	for _, sub := range subs {
		totalBytes += len(sub.Text)
	}
	return s.EstimateCost(totalBytes)
}
