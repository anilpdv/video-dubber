// Package media provides audio/video processing utilities using FFmpeg.
package media

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"video-translator/internal/logger"
)

// FFmpegService wraps FFmpeg commands for audio/video processing.
type FFmpegService struct {
	ffmpegPath  string
	ffprobePath string
	cache       *DurationCache
}

// NewFFmpegService creates a new FFmpeg service with auto-detected paths.
func NewFFmpegService() *FFmpegService {
	paths := []string{
		"/opt/homebrew/bin/ffmpeg",
		"/usr/local/bin/ffmpeg",
		"/usr/bin/ffmpeg",
		"ffmpeg",
	}

	ffmpegPath := "ffmpeg"
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			ffmpegPath = p
			break
		}
	}

	ffprobePath := strings.Replace(ffmpegPath, "ffmpeg", "ffprobe", 1)

	return &FFmpegService{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
		cache:       NewDurationCache(),
	}
}

// NewFFmpegServiceWithPath creates a new FFmpeg service with a custom path.
func NewFFmpegServiceWithPath(path string) *FFmpegService {
	return &FFmpegService{
		ffmpegPath:  path,
		ffprobePath: strings.Replace(path, "ffmpeg", "ffprobe", 1),
		cache:       NewDurationCache(),
	}
}

// CheckInstalled verifies FFmpeg is available.
func (s *FFmpegService) CheckInstalled() error {
	cmd := exec.Command(s.ffmpegPath, "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg not found at %s: %w", s.ffmpegPath, err)
	}
	return nil
}

// GetPath returns the FFmpeg executable path.
func (s *FFmpegService) GetPath() string {
	return s.ffmpegPath
}

// ExtractAudio extracts audio from video and converts to WAV format (16kHz mono for Whisper).
func (s *FFmpegService) ExtractAudio(videoPath, outputPath string) error {
	logger.Info("FFmpeg: extracting audio → %s", filepath.Base(outputPath))

	if err := ensureDir(outputPath); err != nil {
		return err
	}

	args := []string{
		"-i", videoPath,
		"-vn",
		"-ar", "16000",
		"-ac", "1",
		"-acodec", "pcm_s16le",
		"-y",
		outputPath,
	}

	return s.run(args, "audio extraction")
}

// ExtractAudioMP3 extracts audio as MP3 (smaller file size).
func (s *FFmpegService) ExtractAudioMP3(videoPath, outputPath string) error {
	if err := ensureDir(outputPath); err != nil {
		return err
	}

	args := []string{
		"-i", videoPath,
		"-vn",
		"-acodec", "libmp3lame",
		"-q:a", "2",
		"-y",
		outputPath,
	}

	return s.run(args, "MP3 extraction")
}

// MuxVideoAudio combines video (with original audio removed) and new audio.
func (s *FFmpegService) MuxVideoAudio(videoPath, audioPath, outputPath string) error {
	logger.Info("FFmpeg: muxing video + audio → %s", filepath.Base(outputPath))

	if err := ensureDir(outputPath); err != nil {
		return err
	}

	args := []string{
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",
		"-map", "0:v",
		"-map", "1:a",
		"-shortest",
		"-y",
		outputPath,
	}

	return s.run(args, "muxing")
}

// MuxVideoAudioWithOriginal mixes dubbed audio with quieter original audio.
func (s *FFmpegService) MuxVideoAudioWithOriginal(videoPath, audioPath, outputPath string, originalVolume float64) error {
	if err := ensureDir(outputPath); err != nil {
		return err
	}

	filterComplex := fmt.Sprintf(
		"[0:a]volume=%.2f[a0];[1:a]volume=1.0[a1];[a0][a1]amix=inputs=2:duration=longest[aout]",
		originalVolume,
	)

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

	return s.run(args, "muxing with original audio")
}

// GetDuration returns the duration of a media file in seconds.
// Results are cached to avoid repeated ffprobe calls.
func (s *FFmpegService) GetDuration(mediaPath string) (float64, error) {
	// Check cache first
	if duration, ok := s.cache.Get(mediaPath); ok {
		return duration, nil
	}

	// Get from ffprobe
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		mediaPath,
	}

	cmd := exec.Command(s.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	var duration float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%f", &duration); err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	// Cache the result
	s.cache.Set(mediaPath, duration)

	return duration, nil
}

// GetVideoDuration is an alias for GetDuration (legacy compatibility).
func (s *FFmpegService) GetVideoDuration(videoPath string) (float64, error) {
	return s.GetDuration(videoPath)
}

// GetAudioDuration is an alias for GetDuration (legacy compatibility).
func (s *FFmpegService) GetAudioDuration(audioPath string) (float64, error) {
	return s.GetDuration(audioPath)
}

// ConcatAudioFiles concatenates multiple audio files into one.
func (s *FFmpegService) ConcatAudioFiles(inputPaths []string, outputPath string) error {
	logger.Debug("FFmpeg: concatenating %d audio files", len(inputPaths))

	if len(inputPaths) == 0 {
		return fmt.Errorf("no input files provided")
	}

	if err := ensureDir(outputPath); err != nil {
		return err
	}

	// Create temporary file list
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
		"-acodec", "pcm_s16le",
		"-ar", "24000",
		"-ac", "1",
		"-y",
		outputPath,
	}

	return s.run(args, "concat")
}

// GenerateSilence creates a silent audio file of specified duration.
func (s *FFmpegService) GenerateSilence(duration float64, outputPath string) error {
	if err := ensureDir(outputPath); err != nil {
		return err
	}

	args := []string{
		"-f", "lavfi",
		"-i", fmt.Sprintf("anullsrc=r=24000:cl=mono:d=%.3f", duration),
		"-acodec", "pcm_s16le",
		"-ar", "24000",
		"-ac", "1",
		"-y",
		outputPath,
	}

	return s.run(args, "silence generation")
}

// AdjustAudioDuration stretches or compresses audio to match target duration.
func (s *FFmpegService) AdjustAudioDuration(inputPath, outputPath string, targetDuration float64) error {
	if err := ensureDir(outputPath); err != nil {
		return err
	}

	actualDuration, err := s.GetAudioDuration(inputPath)
	if err != nil {
		return fmt.Errorf("failed to get audio duration: %w", err)
	}

	// Skip if durations are very close (within 50ms)
	if abs(actualDuration-targetDuration) < 0.05 {
		return copyFile(inputPath, outputPath)
	}

	// Only speed up audio that's too long - never slow down
	if actualDuration <= targetDuration {
		logger.Debug("Audio %.2fs fits in %.2fs window - no adjustment needed", actualDuration, targetDuration)
		return copyFile(inputPath, outputPath)
	}

	// Calculate tempo factor to speed it up
	tempoFactor := actualDuration / targetDuration

	// Cap speed-up at 1.3x
	const maxSpeedUp = 1.3
	if tempoFactor > maxSpeedUp {
		logger.Debug("Audio %.2fs > %.2fs window - capping speed-up from %.2fx to %.2fx",
			actualDuration, targetDuration, tempoFactor, maxSpeedUp)
		tempoFactor = maxSpeedUp
	} else {
		logger.Debug("Audio %.2fs > %.2fs window - speeding up by %.2fx", actualDuration, targetDuration, tempoFactor)
	}

	// Build atempo filter
	filterStr := buildTempoFilter(tempoFactor)

	// Calculate output duration after tempo adjustment
	outputDuration := actualDuration / tempoFactor

	args := []string{
		"-i", inputPath,
		"-filter:a", filterStr,
	}

	// Trim to target if still too long
	if outputDuration > targetDuration+0.05 {
		logger.Debug("Trimming audio from %.2fs to %.2fs to maintain sync", outputDuration, targetDuration)
		args = append(args, "-t", fmt.Sprintf("%.3f", targetDuration))
	}

	args = append(args, "-y", outputPath)

	return s.run(args, "audio adjustment")
}

// ConvertToWAV converts an audio file to WAV format.
func (s *FFmpegService) ConvertToWAV(inputPath, outputPath string) error {
	if err := ensureDir(outputPath); err != nil {
		return err
	}

	args := []string{
		"-i", inputPath,
		"-ar", "24000",
		"-ac", "1",
		"-y",
		outputPath,
	}

	return s.run(args, "WAV conversion")
}

// run executes an FFmpeg command and returns any error.
func (s *FFmpegService) run(args []string, operation string) error {
	cmd := exec.Command(s.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg %s failed: %w\nOutput: %s", operation, err, string(output))
	}
	return nil
}

// ensureDir creates the parent directory for a file path.
func ensureDir(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

// abs returns the absolute value of a float64.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// buildTempoFilter creates an atempo filter string for the given tempo factor.
// atempo filter only accepts 0.5-2.0 range, so we chain filters for extreme values.
func buildTempoFilter(tempoFactor float64) string {
	if tempoFactor >= 0.5 && tempoFactor <= 2.0 {
		return fmt.Sprintf("atempo=%.4f", tempoFactor)
	} else if tempoFactor < 0.5 {
		if tempoFactor > 0.25 {
			return fmt.Sprintf("atempo=0.5,atempo=%.4f", tempoFactor/0.5)
		}
		return "atempo=0.5,atempo=0.5"
	} else {
		if tempoFactor < 4.0 {
			return fmt.Sprintf("atempo=2.0,atempo=%.4f", tempoFactor/2.0)
		}
		return "atempo=2.0,atempo=2.0"
	}
}
