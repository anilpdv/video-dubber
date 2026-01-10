package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"video-translator/internal/config"
	"video-translator/internal/logger"
)

type FFmpegService struct {
	ffmpegPath string
}

// ChunkInfo represents an audio chunk for parallel processing
type ChunkInfo struct {
	Index      int           // Chunk index (0-based)
	Path       string        // Path to the chunk audio file
	StartTime  float64       // Start time in the original audio (seconds)
	Duration   float64       // Duration of this chunk (seconds)
	HasOverlap bool          // True if this chunk has overlap with the next
}

// newFFmpegCmd creates a new command with timeout context
func (s *FFmpegService) newCmd(args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutFFmpeg)
	return exec.CommandContext(ctx, s.ffmpegPath, args...), cancel
}

func NewFFmpegService() *FFmpegService {
	// Try to find ffmpeg in common locations
	paths := []string{
		"/opt/homebrew/bin/ffmpeg",
		"/usr/local/bin/ffmpeg",
		"/usr/bin/ffmpeg",
		"ffmpeg", // Use PATH
	}

	ffmpegPath := "ffmpeg"
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			ffmpegPath = p
			break
		}
	}

	return &FFmpegService{
		ffmpegPath: ffmpegPath,
	}
}

func NewFFmpegServiceWithPath(path string) *FFmpegService {
	return &FFmpegService{
		ffmpegPath: path,
	}
}

// CheckInstalled verifies ffmpeg is available
func (s *FFmpegService) CheckInstalled() error {
	cmd, cancel := s.newCmd("-version")
	defer cancel()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg not found at %s: %w", s.ffmpegPath, err)
	}
	return nil
}

// GetPath returns the ffmpeg executable path
func (s *FFmpegService) GetPath() string {
	return s.ffmpegPath
}

// ExtractAudio extracts audio from video and converts to WAV format (16kHz mono for Whisper)
func (s *FFmpegService) ExtractAudio(videoPath, outputPath string) error {
	logger.LogInfo("FFmpeg: extracting audio → %s", filepath.Base(outputPath))

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// ffmpeg -i input.mp4 -vn -ar 16000 -ac 1 -acodec pcm_s16le output.wav
	args := []string{
		"-i", videoPath,
		"-vn",              // No video
		"-ar", "16000",     // 16kHz sample rate (Whisper requirement)
		"-ac", "1",         // Mono audio
		"-acodec", "pcm_s16le", // PCM 16-bit
		"-y",               // Overwrite output
		outputPath,
	}

	cmd, cancel := s.newCmd(args...)
	defer cancel()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg audio extraction failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// ExtractAudioMP3 extracts audio as MP3 (smaller file size)
func (s *FFmpegService) ExtractAudioMP3(videoPath, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	args := []string{
		"-i", videoPath,
		"-vn",
		"-acodec", "libmp3lame",
		"-q:a", "2", // High quality
		"-y",
		outputPath,
	}

	cmd, cancel := s.newCmd(args...)
	defer cancel()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg MP3 extraction failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// CompressToMP3 compresses audio to MP3 with specified bitrate (for API upload limits)
func (s *FFmpegService) CompressToMP3(inputPath, outputPath string, bitrate int) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	args := []string{
		"-i", inputPath,
		"-vn",
		"-acodec", "libmp3lame",
		"-b:a", fmt.Sprintf("%dk", bitrate), // e.g., "64k" for 64 kbps
		"-y",
		outputPath,
	}

	cmd, cancel := s.newCmd(args...)
	defer cancel()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg MP3 compression failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// MuxVideoAudio combines video (with original audio removed) and new audio
func (s *FFmpegService) MuxVideoAudio(videoPath, audioPath, outputPath string) error {
	logger.LogInfo("FFmpeg: muxing video + audio → %s", filepath.Base(outputPath))

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// ffmpeg -i video.mp4 -i dubbed_audio.mp3 -c:v copy -map 0:v -map 1:a output.mp4
	args := []string{
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",    // Copy video stream (no re-encoding)
		"-map", "0:v",     // Use video from first input
		"-map", "1:a",     // Use audio from second input
		"-shortest",       // End when shortest stream ends
		"-y",
		outputPath,
	}

	cmd, cancel := s.newCmd(args...)
	defer cancel()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg muxing failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// MuxVideoAudioWithOriginal mixes dubbed audio with quieter original audio
func (s *FFmpegService) MuxVideoAudioWithOriginal(videoPath, audioPath, outputPath string, originalVolume float64) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Complex filter to mix audio tracks
	filterComplex := fmt.Sprintf("[0:a]volume=%.2f[a0];[1:a]volume=1.0[a1];[a0][a1]amix=inputs=2:duration=longest[aout]",
		originalVolume)

	args := []string{
		"-i", videoPath,
		"-i", audioPath,
		"-filter_complex", filterComplex,
		"-map", "0:v",
		"-map", "[aout]",
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "192k",
		"-y",
		outputPath,
	}

	cmd, cancel := s.newCmd(args...)
	defer cancel()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg muxing with original failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// GetVideoDuration returns the duration of a video in seconds
func (s *FFmpegService) GetVideoDuration(videoPath string) (float64, error) {
	// Use ffprobe to get duration
	ffprobePath := strings.Replace(s.ffmpegPath, "ffmpeg", "ffprobe", 1)

	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutFFmpeg)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	var duration float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%f", &duration); err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}

// ConcatAudioFiles concatenates multiple audio files into one
func (s *FFmpegService) ConcatAudioFiles(inputPaths []string, outputPath string) error {
	logger.LogDebug("FFmpeg: concatenating %d audio files", len(inputPaths))

	if len(inputPaths) == 0 {
		return fmt.Errorf("no input files provided")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create a temporary file list
	listPath := filepath.Join(filepath.Dir(outputPath), "concat_list.txt")
	var listContent strings.Builder
	for _, p := range inputPaths {
		listContent.WriteString(fmt.Sprintf("file '%s'\n", p))
	}

	if err := os.WriteFile(listPath, []byte(listContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to write concat list: %w", err)
	}
	defer os.Remove(listPath)

	args := []string{
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-acodec", "pcm_s16le", // Re-encode to WAV for consistent format (was -c copy)
		"-ar", "24000",         // Consistent sample rate
		"-ac", "1",             // Mono
		"-y",
		outputPath,
	}

	cmd, cancel := s.newCmd(args...)
	defer cancel()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// GetAudioDuration returns the duration of an audio file in seconds
func (s *FFmpegService) GetAudioDuration(audioPath string) (float64, error) {
	return s.GetVideoDuration(audioPath) // Same logic works for audio
}

// AdjustAudioDuration stretches or compresses audio to match target duration using atempo filter
func (s *FFmpegService) AdjustAudioDuration(inputPath, outputPath string, targetDuration float64) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get actual duration of input audio
	actualDuration, err := s.GetAudioDuration(inputPath)
	if err != nil {
		return fmt.Errorf("failed to get audio duration: %w", err)
	}

	// Skip if durations are very close (within 50ms)
	if abs(actualDuration-targetDuration) < 0.05 {
		// Just copy the file
		input, err := os.ReadFile(inputPath)
		if err != nil {
			return err
		}
		return os.WriteFile(outputPath, input, 0644)
	}

	// CRITICAL FIX: Only speed up audio that's TOO LONG for the subtitle window
	// Never slow down audio that's shorter - this causes the "slow motion" effect
	if actualDuration <= targetDuration {
		// Audio fits within window - just copy without slowing down
		logger.LogDebug("Audio %.2fs fits in %.2fs window - no adjustment needed", actualDuration, targetDuration)
		input, err := os.ReadFile(inputPath)
		if err != nil {
			return err
		}
		return os.WriteFile(outputPath, input, 0644)
	}

	// Audio is too long - calculate tempo factor to speed it up
	// tempo > 1.0 = speed up (shorter duration)
	tempoFactor := actualDuration / targetDuration

	// Cap speed-up at 1.3x - anything faster sounds unnatural
	// Better to have slight audio overlap than chipmunk speech
	const maxSpeedUp = 1.3
	if tempoFactor > maxSpeedUp {
		logger.LogDebug("Audio %.2fs > %.2fs window - capping speed-up from %.2fx to %.2fx",
			actualDuration, targetDuration, tempoFactor, maxSpeedUp)
		tempoFactor = maxSpeedUp
	} else {
		logger.LogDebug("Audio %.2fs > %.2fs window - speeding up by %.2fx", actualDuration, targetDuration, tempoFactor)
	}

	// atempo filter only accepts 0.5-2.0 range
	// For values outside this range, we need to chain multiple atempo filters
	var filterStr string
	if tempoFactor >= 0.5 && tempoFactor <= 2.0 {
		filterStr = fmt.Sprintf("atempo=%.4f", tempoFactor)
	} else if tempoFactor < 0.5 {
		// Chain multiple atempo filters for extreme slow-down
		// e.g., 0.25 = 0.5 * 0.5
		filterStr = "atempo=0.5,atempo=0.5"
		if tempoFactor > 0.25 {
			// Need less slowing, adjust second atempo
			filterStr = fmt.Sprintf("atempo=0.5,atempo=%.4f", tempoFactor/0.5)
		}
	} else {
		// Chain multiple atempo filters for extreme speed-up
		// e.g., 4.0 = 2.0 * 2.0
		filterStr = "atempo=2.0,atempo=2.0"
		if tempoFactor < 4.0 {
			// Need less speeding, adjust second atempo
			filterStr = fmt.Sprintf("atempo=2.0,atempo=%.4f", tempoFactor/2.0)
		}
	}

	// Calculate output duration after tempo adjustment
	outputDuration := actualDuration / tempoFactor

	// Build ffmpeg command
	args := []string{
		"-i", inputPath,
		"-filter:a", filterStr,
	}

	// If output would still exceed target (due to speed cap), trim to fit exactly
	// This prevents audio overflow that causes voice-video sync drift
	if outputDuration > targetDuration+0.05 { // 50ms tolerance
		logger.LogDebug("Trimming audio from %.2fs to %.2fs to maintain sync", outputDuration, targetDuration)
		args = append(args, "-t", fmt.Sprintf("%.3f", targetDuration))
	}

	args = append(args, "-y", outputPath)

	cmd, cancel := s.newCmd(args...)
	defer cancel()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg audio adjustment failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// abs returns the absolute value of a float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// GenerateSilence creates a silent audio file of specified duration
// Outputs WAV format (pcm_s16le) at 24kHz mono to match Edge TTS output
func (s *FFmpegService) GenerateSilence(duration float64, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	args := []string{
		"-f", "lavfi",
		"-i", fmt.Sprintf("anullsrc=r=24000:cl=mono:d=%.3f", duration),
		"-acodec", "pcm_s16le", // WAV format (was libmp3lame which caused beeps)
		"-ar", "24000",         // 24kHz sample rate (match ConvertToWAV)
		"-ac", "1",             // Mono
		"-y",
		outputPath,
	}

	cmd, cancel := s.newCmd(args...)
	defer cancel()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg silence generation failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// ConvertToWAV converts an audio file to WAV format
func (s *FFmpegService) ConvertToWAV(inputPath, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	args := []string{
		"-i", inputPath,
		"-ar", "24000", // 24kHz sample rate
		"-ac", "1",     // Mono
		"-y",
		outputPath,
	}

	cmd, cancel := s.newCmd(args...)
	defer cancel()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg conversion to WAV failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// SplitAudioIntoChunks splits an audio file into chunks for parallel transcription.
// Each chunk has a configurable duration with overlap to prevent word cutoff at boundaries.
// Returns a list of ChunkInfo with paths to the chunk files.
func (s *FFmpegService) SplitAudioIntoChunks(inputPath, outputDir string, chunkDurationSecs, overlapSecs float64) ([]ChunkInfo, error) {
	logger.LogInfo("FFmpeg: splitting audio into chunks (%.0fs each, %.1fs overlap)", chunkDurationSecs, overlapSecs)

	// Get total audio duration
	totalDuration, err := s.GetAudioDuration(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get audio duration: %w", err)
	}

	// Don't chunk if audio is shorter than minimum
	minDuration := config.MinChunkDuration.Seconds()
	if totalDuration <= minDuration {
		// Return single chunk pointing to original file
		logger.LogInfo("Audio (%.1fs) shorter than minimum chunk duration, using single chunk", totalDuration)
		return []ChunkInfo{{
			Index:      0,
			Path:       inputPath,
			StartTime:  0,
			Duration:   totalDuration,
			HasOverlap: false,
		}}, nil
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create chunk output directory: %w", err)
	}

	var chunks []ChunkInfo
	chunkIndex := 0

	// Calculate actual chunk duration (without overlap for stepping)
	stepDuration := chunkDurationSecs

	for startTime := 0.0; startTime < totalDuration; startTime += stepDuration {
		// Calculate this chunk's duration (with overlap, except for last chunk)
		remaining := totalDuration - startTime
		chunkDur := chunkDurationSecs + overlapSecs
		hasOverlap := true

		// Last chunk: don't extend beyond audio length
		if remaining <= chunkDurationSecs+overlapSecs {
			chunkDur = remaining
			hasOverlap = false
		}

		// Generate chunk path
		chunkPath := filepath.Join(outputDir, fmt.Sprintf("chunk_%04d.wav", chunkIndex))

		// Extract chunk using ffmpeg
		args := []string{
			"-i", inputPath,
			"-ss", fmt.Sprintf("%.3f", startTime),
			"-t", fmt.Sprintf("%.3f", chunkDur),
			"-ar", "16000",         // Whisper requirement
			"-ac", "1",             // Mono
			"-acodec", "pcm_s16le", // PCM 16-bit
			"-y",
			chunkPath,
		}

		cmd, cancel := s.newCmd(args...)
		output, err := cmd.CombinedOutput()
		cancel()

		if err != nil {
			return nil, fmt.Errorf("failed to extract chunk %d: %w\nOutput: %s", chunkIndex, err, string(output))
		}

		chunks = append(chunks, ChunkInfo{
			Index:      chunkIndex,
			Path:       chunkPath,
			StartTime:  startTime,
			Duration:   chunkDur,
			HasOverlap: hasOverlap,
		})

		chunkIndex++
	}

	logger.LogInfo("FFmpeg: created %d audio chunks from %.1fs audio", len(chunks), totalDuration)
	return chunks, nil
}

// CleanupChunks removes all chunk files from a directory
func (s *FFmpegService) CleanupChunks(chunks []ChunkInfo) {
	for _, chunk := range chunks {
		os.Remove(chunk.Path)
	}
}
