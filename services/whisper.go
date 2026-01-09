package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"video-translator/models"
)

type WhisperService struct {
	whisperPath string
	modelPath   string
}

func NewWhisperService() *WhisperService {
	homeDir, _ := os.UserHomeDir()

	// Try to find whisper-cpp/whisper-cli in common locations
	// Note: Homebrew installs it as "whisper-cli" not "whisper-cpp"
	paths := []string{
		"/opt/homebrew/bin/whisper-cli",
		"/opt/homebrew/bin/whisper-cpp",
		"/usr/local/bin/whisper-cli",
		"/usr/local/bin/whisper-cpp",
		"whisper-cli",
		"whisper-cpp",
	}

	whisperPath := "whisper-cli"
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			whisperPath = p
			break
		}
	}

	// Default model path
	modelPath := filepath.Join(homeDir, ".whisper", "models", "ggml-base.bin")

	return &WhisperService{
		whisperPath: whisperPath,
		modelPath:   modelPath,
	}
}

func NewWhisperServiceWithPaths(whisperPath, modelPath string) *WhisperService {
	return &WhisperService{
		whisperPath: whisperPath,
		modelPath:   modelPath,
	}
}

// CheckInstalled verifies whisper-cpp/whisper-cli is available
func (s *WhisperService) CheckInstalled() error {
	// Check if path exists or command is in PATH
	if _, err := exec.LookPath(s.whisperPath); err != nil {
		if _, err := os.Stat(s.whisperPath); os.IsNotExist(err) {
			return fmt.Errorf("whisper-cli not found. Install with: brew install whisper-cpp")
		}
	}
	return nil
}

// CheckModel verifies the model file exists
func (s *WhisperService) CheckModel() error {
	if _, err := os.Stat(s.modelPath); os.IsNotExist(err) {
		return fmt.Errorf("whisper model not found at %s. Download from HuggingFace", s.modelPath)
	}
	return nil
}

// Transcribe converts audio to text with timestamps
func (s *WhisperService) Transcribe(audioPath, language string) (models.SubtitleList, error) {
	LogInfo("Whisper: model=%s lang=%s file=%s", filepath.Base(s.modelPath), language, filepath.Base(audioPath))

	if err := s.CheckInstalled(); err != nil {
		return nil, err
	}

	if err := s.CheckModel(); err != nil {
		return nil, err
	}

	// Output SRT to a temp file
	outputDir := filepath.Dir(audioPath)
	baseName := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	srtPath := filepath.Join(outputDir, baseName+".srt")

	// whisper-cpp -m model.bin -l ru -f audio.wav -osrt
	args := []string{
		"-m", s.modelPath,
		"-l", language,
		"-f", audioPath,
		"-osrt",
		"-of", filepath.Join(outputDir, baseName),
	}

	cmd := exec.Command(s.whisperPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("whisper transcription failed: %w\nOutput: %s", err, string(output))
	}

	// Parse the SRT file
	subtitles, err := parseSRTFile(srtPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SRT output: %w", err)
	}

	return subtitles, nil
}

// TranscribeWithProgress transcribes audio while reporting progress via callback
func (s *WhisperService) TranscribeWithProgress(audioPath, language string, audioDuration float64, onProgress func(currentSec float64, percent int)) (models.SubtitleList, error) {
	if err := s.CheckInstalled(); err != nil {
		return nil, err
	}

	if err := s.CheckModel(); err != nil {
		return nil, err
	}

	// Output SRT to a temp file
	outputDir := filepath.Dir(audioPath)
	baseName := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	srtPath := filepath.Join(outputDir, baseName+".srt")

	args := []string{
		"-m", s.modelPath,
		"-l", language,
		"-f", audioPath,
		"-osrt",
		"-of", filepath.Join(outputDir, baseName),
	}

	cmd := exec.Command(s.whisperPath, args...)

	// Get pipes for streaming output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start whisper: %w", err)
	}

	// Parse timestamps from output in real-time
	// Whisper outputs: [00:05:00.000 --> 00:05:02.500] text
	timestampRegex := regexp.MustCompile(`\[(\d{2}:\d{2}:\d{2}\.\d{3})`)

	// Read from combined stdout and stderr
	reader := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Text()
		if matches := timestampRegex.FindStringSubmatch(line); len(matches) > 1 {
			currentSec := parseTimestampToSeconds(matches[1])
			if audioDuration > 0 && onProgress != nil {
				// Calculate progress in the 15-40% range (transcription stage)
				percent := int((currentSec/audioDuration)*25) + 15
				if percent > 40 {
					percent = 40
				}
				onProgress(currentSec, percent)
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("whisper transcription failed: %w", err)
	}

	// Parse the SRT file
	subtitles, err := parseSRTFile(srtPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SRT output: %w", err)
	}

	return subtitles, nil
}

// TranscribeWithOpenAI uses OpenAI's Whisper API for fast transcription
// Cost: $0.006/minute = ~$1.80 for 5 hours of audio
func (s *WhisperService) TranscribeWithOpenAI(audioPath, apiKey, language string, onProgress func(percent int, message string)) (models.SubtitleList, error) {
	LogInfo("OpenAI Whisper API: model=whisper-1 lang=%s file=%s", language, filepath.Base(audioPath))

	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	// OpenAI Whisper API has a 25MB file size limit
	// Check file size first
	fileInfo, err := os.Stat(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// 25MB limit for OpenAI API
	const maxFileSize = 25 * 1024 * 1024
	if fileInfo.Size() > maxFileSize {
		// For large files, we need to split into chunks
		return s.transcribeWithOpenAIChunked(audioPath, apiKey, language, onProgress)
	}

	if onProgress != nil {
		onProgress(20, "Uploading audio to OpenAI...")
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

	// Add other fields
	writer.WriteField("model", "whisper-1")
	writer.WriteField("language", language)
	writer.WriteField("response_format", "srt") // Get SRT directly!
	writer.Close()

	if onProgress != nil {
		onProgress(25, "Transcribing with OpenAI Whisper...")
	}

	// Make the API request
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/transcriptions", body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Use a client with longer timeout for large files
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
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
			return nil, fmt.Errorf("OpenAI API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("OpenAI API error: %s", string(respBody))
	}

	if onProgress != nil {
		onProgress(38, "Parsing transcription...")
	}

	// Parse SRT response
	subtitles, err := parseSRTFromString(string(respBody))
	if err != nil {
		return nil, fmt.Errorf("failed to parse SRT response: %w", err)
	}

	if onProgress != nil {
		onProgress(40, fmt.Sprintf("Transcribed %d segments", len(subtitles)))
	}

	return subtitles, nil
}

// transcribeWithOpenAIChunked handles files larger than 25MB by splitting them
func (s *WhisperService) transcribeWithOpenAIChunked(audioPath, apiKey, language string, onProgress func(percent int, message string)) (models.SubtitleList, error) {
	// For now, convert to MP3 with lower bitrate to reduce file size
	// This is a workaround - ideally we'd split the audio into chunks

	if onProgress != nil {
		onProgress(18, "Compressing audio for upload...")
	}

	// Create a compressed version
	compressedPath := audioPath + ".compressed.mp3"
	defer os.Remove(compressedPath)

	// Use FFmpeg to compress - 64kbps mono should be fine for speech
	ffmpeg := NewFFmpegService()
	cmd := exec.Command(ffmpeg.ffmpegPath,
		"-i", audioPath,
		"-ac", "1",        // Mono
		"-ar", "16000",    // 16kHz
		"-b:a", "64k",     // 64kbps
		"-y",
		compressedPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to compress audio: %w\nOutput: %s", err, string(output))
	}

	// Check if compressed file is small enough
	fileInfo, err := os.Stat(compressedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get compressed file info: %w", err)
	}

	const maxFileSize = 25 * 1024 * 1024
	if fileInfo.Size() > maxFileSize {
		return nil, fmt.Errorf("audio file is too large for OpenAI API (>25MB even after compression). Please use a shorter video or local Whisper")
	}

	// Transcribe the compressed file
	return s.TranscribeWithOpenAI(compressedPath, apiKey, language, onProgress)
}

// parseSRTFromString parses SRT content from a string
func parseSRTFromString(content string) (models.SubtitleList, error) {
	var subtitles models.SubtitleList
	lines := strings.Split(content, "\n")

	var currentSub *models.Subtitle
	lineNum := 0
	timeRegex := regexp.MustCompile(`(\d{2}:\d{2}:\d{2}[,\.]\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2}[,\.]\d{3})`)

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)

		if line == "" {
			if currentSub != nil && currentSub.Text != "" {
				subtitles = append(subtitles, *currentSub)
			}
			currentSub = nil
			lineNum = 0
			continue
		}

		lineNum++

		switch lineNum {
		case 1:
			// Index line
			index, err := strconv.Atoi(line)
			if err == nil {
				currentSub = &models.Subtitle{Index: index}
			}
		case 2:
			// Timestamp line
			if currentSub != nil {
				matches := timeRegex.FindStringSubmatch(line)
				if len(matches) == 3 {
					currentSub.StartTime = parseTimestamp(matches[1])
					currentSub.EndTime = parseTimestamp(matches[2])
				}
			}
		default:
			// Text lines
			if currentSub != nil {
				if currentSub.Text != "" {
					currentSub.Text += " "
				}
				currentSub.Text += line
			}
		}
	}

	// Don't forget the last subtitle
	if currentSub != nil && currentSub.Text != "" {
		subtitles = append(subtitles, *currentSub)
	}

	return subtitles, nil
}

// parseTimestampToSeconds converts timestamp like "00:05:30.500" to seconds
func parseTimestampToSeconds(ts string) float64 {
	parts := strings.Split(ts, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.ParseFloat(parts[0], 64)
	minutes, _ := strconv.ParseFloat(parts[1], 64)

	secParts := strings.Split(parts[2], ".")
	seconds, _ := strconv.ParseFloat(secParts[0], 64)
	millis := 0.0
	if len(secParts) > 1 {
		millis, _ = strconv.ParseFloat("0."+secParts[1], 64)
	}

	return hours*3600 + minutes*60 + seconds + millis
}

// TranscribeToText returns just the text without timestamps
func (s *WhisperService) TranscribeToText(audioPath, language string) (string, error) {
	subtitles, err := s.Transcribe(audioPath, language)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	for _, sub := range subtitles {
		text.WriteString(sub.Text)
		text.WriteString(" ")
	}

	return strings.TrimSpace(text.String()), nil
}

// parseSRTFile parses an SRT subtitle file
func parseSRTFile(path string) (models.SubtitleList, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var subtitles models.SubtitleList
	scanner := bufio.NewScanner(file)

	// SRT format:
	// 1
	// 00:00:00,000 --> 00:00:02,500
	// Text here
	//
	// 2
	// ...

	var currentSub *models.Subtitle
	lineNum := 0
	timeRegex := regexp.MustCompile(`(\d{2}:\d{2}:\d{2}[,\.]\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2}[,\.]\d{3})`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			if currentSub != nil && currentSub.Text != "" {
				subtitles = append(subtitles, *currentSub)
			}
			currentSub = nil
			lineNum = 0
			continue
		}

		lineNum++

		switch lineNum {
		case 1:
			// Index line
			index, err := strconv.Atoi(line)
			if err == nil {
				currentSub = &models.Subtitle{Index: index}
			}
		case 2:
			// Timestamp line
			if currentSub != nil {
				matches := timeRegex.FindStringSubmatch(line)
				if len(matches) == 3 {
					currentSub.StartTime = parseTimestamp(matches[1])
					currentSub.EndTime = parseTimestamp(matches[2])
				}
			}
		default:
			// Text lines
			if currentSub != nil {
				if currentSub.Text != "" {
					currentSub.Text += " "
				}
				currentSub.Text += line
			}
		}
	}

	// Don't forget the last subtitle
	if currentSub != nil && currentSub.Text != "" {
		subtitles = append(subtitles, *currentSub)
	}

	return subtitles, scanner.Err()
}

// parseTimestamp converts SRT timestamp to time.Duration
// Format: 00:00:00,000 or 00:00:00.000
func parseTimestamp(ts string) time.Duration {
	// Normalize separator
	ts = strings.Replace(ts, ",", ".", 1)

	parts := strings.Split(ts, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])

	secParts := strings.Split(parts[2], ".")
	seconds, _ := strconv.Atoi(secParts[0])
	millis := 0
	if len(secParts) > 1 {
		millis, _ = strconv.Atoi(secParts[1])
	}

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(millis)*time.Millisecond
}

// GetAvailableModels returns list of available model sizes
func GetAvailableModels() []string {
	return []string{
		"tiny",   // ~75MB, fastest
		"base",   // ~150MB, good balance
		"small",  // ~500MB, better accuracy
		"medium", // ~1.5GB, high accuracy
		"large",  // ~3GB, best accuracy
	}
}

// DownloadModelURL returns the URL to download a model
func DownloadModelURL(model string) string {
	return fmt.Sprintf("https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-%s.bin", model)
}
