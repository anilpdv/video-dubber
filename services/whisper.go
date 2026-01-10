package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"video-translator/internal/config"
	"video-translator/internal/logger"
	"video-translator/internal/subtitle"
	"video-translator/internal/text"
	"video-translator/internal/worker"
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
	logger.LogInfo("Whisper: model=%s lang=%s file=%s", filepath.Base(s.modelPath), language, filepath.Base(audioPath))

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

	ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutWhisper)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.whisperPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("whisper transcription failed: %w\nOutput: %s", err, string(output))
	}

	// Parse the SRT file using internal/subtitle package
	internalSubs, err := subtitle.ParseSRTFile(srtPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SRT output: %w", err)
	}

	return models.FromInternalSubtitles(internalSubs), nil
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

	ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutWhisper)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.whisperPath, args...)

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
	// Using pre-compiled regex for performance

	// Read from combined stdout and stderr
	reader := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Text()
		if matches := text.WhisperTimestampRegex.FindStringSubmatch(line); len(matches) > 1 {
			currentSec := subtitle.ParseTimestampToSeconds(matches[1])
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

	// Parse the SRT file using internal/subtitle package
	internalSubs, err := subtitle.ParseSRTFile(srtPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SRT output: %w", err)
	}

	return models.FromInternalSubtitles(internalSubs), nil
}

// TranscribeWithOpenAI uses OpenAI's Whisper API for fast transcription
// Cost: $0.006/minute = ~$1.80 for 5 hours of audio
func (s *WhisperService) TranscribeWithOpenAI(audioPath, apiKey, language string, onProgress func(percent int, message string)) (models.SubtitleList, error) {
	logger.LogInfo("OpenAI Whisper API: model=whisper-1 lang=%s file=%s", language, filepath.Base(audioPath))

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

	// Parse SRT response using internal/subtitle package
	internalSubs, err := subtitle.ParseSRTString(string(respBody))
	if err != nil {
		return nil, fmt.Errorf("failed to parse SRT response: %w", err)
	}
	subtitles := models.FromInternalSubtitles(internalSubs)

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
	ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutFFmpeg)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg.ffmpegPath,
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


// TranscribeChunksParallel transcribes multiple audio chunks in parallel using worker pools.
// This provides 3-6x speedup for long videos on multi-core systems.
// Each chunk's subtitles are adjusted with the correct offset timestamp.
func (s *WhisperService) TranscribeChunksParallel(
	chunks []ChunkInfo,
	language string,
	onProgress func(completed, total int),
) (models.SubtitleList, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks to transcribe")
	}

	// Single chunk - use regular transcription
	if len(chunks) == 1 {
		return s.Transcribe(chunks[0].Path, language)
	}

	logger.LogInfo("Whisper: transcribing %d chunks in parallel", len(chunks))

	// Determine worker count based on CPU cores
	workers := config.DynamicWorkerCount("transcription")
	if workers > len(chunks) {
		workers = len(chunks)
	}

	// Define processing function for each chunk
	processChunk := func(job worker.Job[ChunkInfo]) (models.SubtitleList, error) {
		chunk := job.Data
		subs, err := s.Transcribe(chunk.Path, language)
		if err != nil {
			return nil, fmt.Errorf("chunk %d transcription failed: %w", chunk.Index, err)
		}

		// Adjust timestamps with chunk offset
		offsetDuration := time.Duration(chunk.StartTime * float64(time.Second))
		for i := range subs {
			subs[i].StartTime += offsetDuration
			subs[i].EndTime += offsetDuration
		}

		return subs, nil
	}

	// Process chunks in parallel
	results, err := worker.Process(chunks, workers, processChunk, onProgress)
	if err != nil {
		return nil, err
	}

	// Merge all results and handle overlap
	return mergeChunkSubtitles(results, chunks), nil
}

// mergeChunkSubtitles merges subtitles from multiple chunks, handling overlap regions.
// Subtitles in overlap regions are deduplicated by preferring the first chunk's version.
func mergeChunkSubtitles(chunkResults []models.SubtitleList, _ []ChunkInfo) models.SubtitleList {
	if len(chunkResults) == 0 {
		return nil
	}

	// Flatten all subtitles
	var allSubs models.SubtitleList
	for _, subs := range chunkResults {
		allSubs = append(allSubs, subs...)
	}

	// Sort by start time
	sort.Slice(allSubs, func(i, j int) bool {
		return allSubs[i].StartTime < allSubs[j].StartTime
	})

	// Deduplicate overlapping subtitles
	// Two subtitles are considered duplicates if they overlap significantly (>80%)
	var merged models.SubtitleList
	for _, sub := range allSubs {
		if len(merged) == 0 {
			merged = append(merged, sub)
			continue
		}

		last := merged[len(merged)-1]

		// Check if this subtitle overlaps significantly with the last one
		overlapStart := maxDuration(last.StartTime, sub.StartTime)
		overlapEnd := minDuration(last.EndTime, sub.EndTime)

		if overlapEnd > overlapStart {
			// There is overlap
			overlapDuration := overlapEnd - overlapStart
			subDuration := sub.EndTime - sub.StartTime

			// If overlap is >80% of subtitle duration, skip (duplicate)
			if subDuration > 0 && float64(overlapDuration)/float64(subDuration) > 0.8 {
				continue
			}
		}

		// Not a duplicate, add to merged list
		merged = append(merged, sub)
	}

	return merged
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
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
