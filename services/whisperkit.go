package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"video-translator/internal/logger"
	"video-translator/models"
)

// WhisperKitService handles transcription using WhisperKit (native macOS Apple Silicon)
type WhisperKitService struct {
	cliPath string
	model   string // large-v2
}

// WhisperKitOutput represents the JSON output from whisperkit-cli
type WhisperKitOutput struct {
	Text     string              `json:"text"`
	Language string              `json:"language"`
	Segments []WhisperKitSegment `json:"segments"`
}

// WhisperKitSegment represents a transcription segment
type WhisperKitSegment struct {
	Start float64          `json:"start"`
	End   float64          `json:"end"`
	Text  string           `json:"text"`
	Words []WhisperKitWord `json:"words"`
}

// WhisperKitWord represents a word with timing
type WhisperKitWord struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// NewWhisperKitService creates a new WhisperKit transcription service
// Available models: tiny, base, small, medium, large-v2, large-v3
func NewWhisperKitService(model string) *WhisperKitService {
	if model == "" {
		model = "base" // ~150MB download, good balance of speed/quality
	}
	return &WhisperKitService{
		cliPath: findExecutable("whisperkit-cli"),
		model:   model,
	}
}

// CheckInstalled verifies whisperkit-cli is available
func (s *WhisperKitService) CheckInstalled() error {
	// Check if the path exists (cliPath may already be full path from findExecutable)
	if _, err := os.Stat(s.cliPath); err == nil {
		return nil
	}
	// Fallback to LookPath
	if _, err := exec.LookPath(s.cliPath); err != nil {
		return fmt.Errorf("whisperkit-cli not found. Install with: brew install whisperkit-cli")
	}
	return nil
}

// GetModel returns the current model name
func (s *WhisperKitService) GetModel() string {
	return s.model
}

// GetModelSize returns the approximate download size for the model
func (s *WhisperKitService) GetModelSize() string {
	sizes := map[string]string{
		"tiny":     "~75MB",
		"base":     "~150MB",
		"small":    "~500MB",
		"medium":   "~1.5GB",
		"large-v2": "~3GB",
		"large-v3": "~3GB",
	}
	if size, ok := sizes[s.model]; ok {
		return size
	}
	return "~150MB"
}

// IsModelDownloaded checks if the WhisperKit model is already cached
func (s *WhisperKitService) IsModelDownloaded() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	// WhisperKit stores models at ~/Documents/huggingface/models/argmaxinc/whisperkit-coreml/
	// Model naming: openai_whisper-{model} (e.g., openai_whisper-tiny, openai_whisper-base)
	modelName := fmt.Sprintf("openai_whisper-%s", s.model)
	modelPath := filepath.Join(homeDir, "Documents", "huggingface", "models",
		"argmaxinc", "whisperkit-coreml", modelName)

	// Check if model directory exists
	info, err := os.Stat(modelPath)
	if err != nil {
		return false
	}

	if !info.IsDir() {
		return false
	}

	// Check for required model files (AudioEncoder.mlmodelc, TextDecoder.mlmodelc)
	encoderPath := filepath.Join(modelPath, "AudioEncoder.mlmodelc")
	decoderPath := filepath.Join(modelPath, "TextDecoder.mlmodelc")

	if _, err := os.Stat(encoderPath); err != nil {
		return false
	}
	if _, err := os.Stat(decoderPath); err != nil {
		return false
	}

	return true
}

// DownloadModel downloads the WhisperKit model with progress callback
func (s *WhisperKitService) DownloadModel(onProgress func(line string)) error {
	if err := s.CheckInstalled(); err != nil {
		return err
	}

	logger.LogInfo("WhisperKit: downloading model %s...", s.model)

	// Create a temporary audio file to trigger model download
	// whisperkit-cli downloads the model before processing
	tmpDir := os.TempDir()
	silenceFile := filepath.Join(tmpDir, "whisperkit_download_trigger.wav")

	args := []string{
		"transcribe",
		"--audio-path", silenceFile,
		"--model", s.model,
		"--verbose",
	}

	// First create a tiny valid wav file
	if err := createMinimalWAV(silenceFile); err != nil {
		args[2] = "/dev/null"
	}
	defer os.Remove(silenceFile)

	cmd := exec.Command(s.cliPath, args...)
	stderr, _ := cmd.StderrPipe()
	stdout, _ := cmd.StdoutPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start whisperkit-cli: %w", err)
	}

	downloadComplete := false

	// Read stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if onProgress != nil {
				onProgress(line)
			}
		}
	}()

	// Read stderr and detect model loading
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		logger.LogInfo("WhisperKit: %s", line)
		if onProgress != nil {
			onProgress(line)
		}
		// Detect when model is ready (download complete)
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, "loaded") || strings.Contains(lineLower, "transcribing") {
			downloadComplete = true
		}
	}

	// Wait for process to finish (will error on invalid audio, that's OK)
	cmd.Wait()

	// If we saw model loaded, or IsModelDownloaded returns true, success
	if downloadComplete || s.IsModelDownloaded() {
		logger.LogInfo("WhisperKit: model %s downloaded successfully", s.model)
		return nil
	}

	return fmt.Errorf("model download may have failed, please try again")
}

// createMinimalWAV creates a minimal valid WAV file for triggering model download
func createMinimalWAV(path string) error {
	// Minimal WAV header for 1 second of silence at 16kHz mono
	header := []byte{
		'R', 'I', 'F', 'F', // ChunkID
		0x24, 0x00, 0x00, 0x00, // ChunkSize (36 + data size)
		'W', 'A', 'V', 'E', // Format
		'f', 'm', 't', ' ', // Subchunk1ID
		0x10, 0x00, 0x00, 0x00, // Subchunk1Size (16 for PCM)
		0x01, 0x00, // AudioFormat (1 = PCM)
		0x01, 0x00, // NumChannels (1 = mono)
		0x80, 0x3E, 0x00, 0x00, // SampleRate (16000)
		0x00, 0x7D, 0x00, 0x00, // ByteRate (16000 * 1 * 2)
		0x02, 0x00, // BlockAlign (1 * 2)
		0x10, 0x00, // BitsPerSample (16)
		'd', 'a', 't', 'a', // Subchunk2ID
		0x00, 0x00, 0x00, 0x00, // Subchunk2Size (0 - empty data)
	}

	return os.WriteFile(path, header, 0644)
}

// Transcribe converts audio to text with timestamps using WhisperKit
func (s *WhisperKitService) Transcribe(audioPath, language string) (models.SubtitleList, error) {
	logger.LogInfo("WhisperKit: transcribing %s (lang=%s, model=%s)", filepath.Base(audioPath), language, s.model)

	if err := s.CheckInstalled(); err != nil {
		return nil, err
	}

	// Create output directory for JSON report
	workDir := filepath.Dir(audioPath)
	baseName := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))

	args := []string{
		"transcribe",
		"--audio-path", audioPath,
		"--model", s.model,
		"--verbose",
		"--report",
		"--report-path", workDir,
	}

	if language != "" && language != "auto" {
		args = append(args, "--language", language)
	}

	logger.LogInfo("WhisperKit: running whisperkit-cli %s", strings.Join(args, " "))
	logger.LogInfo("WhisperKit: first run may download model (~1.5GB), please wait...")

	cmd := exec.Command(s.cliPath, args...)

	// Stream output to show progress
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("whisperkit transcription failed: %w", err)
	}

	// Find the JSON report file (whisperkit names it based on audio file)
	jsonPath := filepath.Join(workDir, baseName+".json")

	// Try alternative naming patterns if not found
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		// Try looking for any JSON file in the directory
		files, _ := filepath.Glob(filepath.Join(workDir, "*.json"))
		if len(files) > 0 {
			jsonPath = files[0]
			logger.LogInfo("WhisperKit: found report at %s", jsonPath)
		}
	}

	return s.parseOutput(jsonPath)
}

// parseOutput reads and parses the WhisperKit JSON output file
func (s *WhisperKitService) parseOutput(jsonPath string) (models.SubtitleList, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read whisperkit output: %w", err)
	}

	// Clean up the JSON file after reading
	defer os.Remove(jsonPath)

	var result WhisperKitOutput
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse whisperkit output: %w", err)
	}

	logger.LogInfo("WhisperKit: parsed %d segments", len(result.Segments))

	var subs models.SubtitleList
	for _, seg := range result.Segments {
		subs = append(subs, models.Subtitle{
			StartTime: time.Duration(seg.Start * float64(time.Second)),
			EndTime:   time.Duration(seg.End * float64(time.Second)),
			Text:      strings.TrimSpace(seg.Text),
		})
	}

	return subs, nil
}
