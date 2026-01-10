package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
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

// cosyJobData contains data for a CosyVoice TTS job.
type cosyJobData struct {
	index int
	text  string
	start time.Duration
	end   time.Duration
}

// CosyVoiceService handles text-to-speech with voice cloning using CosyVoice
// CosyVoice is an open-source TTS system from Alibaba that supports zero-shot voice cloning
type CosyVoiceService struct {
	installPath     string // Path to CosyVoice installation
	mode            string // "local" or "api"
	apiURL          string // API endpoint if using api mode
	voiceSamplePath string // Path to voice sample for cloning
	pythonPath      string
	ffmpeg          *FFmpegService
	tempDir         string
}

// NewCosyVoiceService creates a new CosyVoice TTS service
func NewCosyVoiceService(installPath, mode, apiURL, voiceSamplePath, pythonPath string) *CosyVoiceService {
	if mode == "" {
		mode = "local"
	}
	if pythonPath == "" {
		pythonPath = "python3"
	}

	homeDir, _ := os.UserHomeDir()
	if installPath == "" {
		installPath = filepath.Join(homeDir, ".cosyvoice")
	}

	tempDir := filepath.Join(os.TempDir(), "video-translator-cosyvoice")
	os.MkdirAll(tempDir, 0755)

	return &CosyVoiceService{
		installPath:     installPath,
		mode:            mode,
		apiURL:          apiURL,
		voiceSamplePath: voiceSamplePath,
		pythonPath:      pythonPath,
		ffmpeg:          NewFFmpegService(),
		tempDir:         tempDir,
	}
}

// CheckInstalled verifies CosyVoice is available
func (s *CosyVoiceService) CheckInstalled() error {
	if s.mode == "api" {
		if s.apiURL == "" {
			return fmt.Errorf("CosyVoice API URL is required for API mode")
		}
		return nil
	}

	// Check if CosyVoice Python package is installed
	script := `
import sys
try:
    from cosyvoice.cli.cosyvoice import CosyVoice
    print("OK")
except ImportError:
    print("NOT_INSTALLED")
    sys.exit(1)
`
	cmd := exec.Command(s.pythonPath, "-c", script)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("CosyVoice not installed. See: https://github.com/FunAudioLLM/CosyVoice")
	}
	if strings.TrimSpace(string(output)) != "OK" {
		return fmt.Errorf("CosyVoice not installed. See: https://github.com/FunAudioLLM/CosyVoice")
	}
	return nil
}

// SetVoiceSample sets the voice sample path for cloning
func (s *CosyVoiceService) SetVoiceSample(samplePath string) error {
	if samplePath == "" {
		return fmt.Errorf("voice sample path is required")
	}
	if _, err := os.Stat(samplePath); os.IsNotExist(err) {
		return fmt.Errorf("voice sample not found: %s", samplePath)
	}
	s.voiceSamplePath = samplePath
	return nil
}

// ExtractVoiceSample extracts a voice sample from a video/audio file
func (s *CosyVoiceService) ExtractVoiceSample(inputPath, outputPath string, startSec, durationSec float64) error {
	if durationSec == 0 {
		durationSec = 10 // Default 10 seconds
	}

	args := []string{
		"-i", inputPath,
		"-ss", fmt.Sprintf("%.2f", startSec),
		"-t", fmt.Sprintf("%.2f", durationSec),
		"-ar", "22050", // 22kHz for CosyVoice
		"-ac", "1",     // Mono
		"-y",
		outputPath,
	}

	cmd := exec.Command(s.ffmpeg.GetPath(), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to extract voice sample: %w\nOutput: %s", err, string(output))
	}

	s.voiceSamplePath = outputPath
	return nil
}

// Synthesize generates audio from text using CosyVoice with voice cloning
func (s *CosyVoiceService) Synthesize(text, outputPath string) error {
	logger.LogInfo("CosyVoice: mode=%s sample=%s", s.mode, s.voiceSamplePath)

	if text == "" {
		return fmt.Errorf("empty text provided")
	}

	if s.voiceSamplePath == "" {
		return fmt.Errorf("voice sample is required for voice cloning")
	}

	if s.mode == "api" {
		return s.synthesizeViaAPI(text, outputPath)
	}

	return s.synthesizeLocal(text, outputPath)
}

// synthesizeLocal uses local CosyVoice installation
func (s *CosyVoiceService) synthesizeLocal(text, outputPath string) error {
	// Python script to run CosyVoice for zero-shot voice cloning
	script := fmt.Sprintf(`
import sys
import torch
from cosyvoice.cli.cosyvoice import CosyVoice
from cosyvoice.utils.file_utils import load_wav
import torchaudio

# Load CosyVoice model
cosyvoice = CosyVoice('pretrained_models/CosyVoice-300M')

# Load voice sample for cloning
prompt_speech = load_wav("%s", 22050)

# Synthesize with voice cloning (zero-shot)
text = """%s"""

output = cosyvoice.inference_zero_shot(
    text,
    "Target voice sample",
    prompt_speech
)

# Save output
for i, audio in enumerate(output):
    torchaudio.save("%s", audio['tts_speech'], 22050)
    break  # Only need first output

print("DONE")
`, s.voiceSamplePath, text, outputPath)

	cmd := exec.Command(s.pythonPath, "-c", script)
	cmd.Dir = s.installPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("CosyVoice synthesis failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// synthesizeViaAPI uses CosyVoice API
func (s *CosyVoiceService) synthesizeViaAPI(text, outputPath string) error {
	// Create multipart form with text and voice sample
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add text field
	if err := writer.WriteField("text", text); err != nil {
		return fmt.Errorf("failed to write text field: %w", err)
	}

	// Add voice sample file
	sampleFile, err := os.Open(s.voiceSamplePath)
	if err != nil {
		return fmt.Errorf("failed to open voice sample: %w", err)
	}
	defer sampleFile.Close()

	part, err := writer.CreateFormFile("voice_sample", filepath.Base(s.voiceSamplePath))
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, sampleFile); err != nil {
		return fmt.Errorf("failed to copy voice sample: %w", err)
	}

	writer.Close()

	// Make request using shared HTTP client
	req, err := http.NewRequest("POST", s.apiURL+"/synthesize", &body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := internalhttp.CosyVoiceClient.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("CosyVoice API error: %s", errResp.Error)
		}
		return fmt.Errorf("CosyVoice API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Response is audio bytes
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read audio response: %w", err)
	}

	if err := os.WriteFile(outputPath, audioData, 0644); err != nil {
		return fmt.Errorf("failed to write audio file: %w", err)
	}

	return nil
}

// SynthesizeSubtitles generates audio for all subtitles with proper timing
func (s *CosyVoiceService) SynthesizeSubtitles(subs models.SubtitleList, outputPath string) error {
	return s.SynthesizeWithCallback(subs, outputPath, nil)
}

// SynthesizeWithCallback generates audio for subtitles with progress callback
// Uses internal worker pool for parallel processing (GPU-intensive, keep workers low)
func (s *CosyVoiceService) SynthesizeWithCallback(
	subs models.SubtitleList,
	outputPath string,
	onProgress func(current, total int),
) error {
	logger.LogInfo("CosyVoice: synthesizing %d subtitles with %d workers", len(subs), config.WorkersCosyVoice)

	if len(subs) == 0 {
		return fmt.Errorf("no subtitles provided")
	}

	if err := s.CheckInstalled(); err != nil {
		return err
	}

	if s.voiceSamplePath == "" {
		return fmt.Errorf("voice sample is required for voice cloning. Use SetVoiceSample() first")
	}

	// Create temp directory for segments
	segmentDir := filepath.Join(s.tempDir, fmt.Sprintf("segments_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(segmentDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(segmentDir)

	// Identify which subtitles need TTS (non-empty text)
	var jobs []cosyJobData
	for i, sub := range subs {
		if strings.TrimSpace(sub.Text) != "" {
			jobs = append(jobs, cosyJobData{
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
		processJob := func(job worker.Job[cosyJobData]) (string, error) {
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
		results, err := worker.Process(jobs, config.WorkersCosyVoice, processJob, progressCallback)
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


// GetVoiceSamplePath returns the current voice sample path
func (s *CosyVoiceService) GetVoiceSamplePath() string {
	return s.voiceSamplePath
}

// HasVoiceSample returns true if a voice sample is set
func (s *CosyVoiceService) HasVoiceSample() bool {
	return s.voiceSamplePath != ""
}
