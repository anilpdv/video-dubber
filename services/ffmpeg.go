package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type FFmpegService struct {
	ffmpegPath string
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
	cmd := exec.Command(s.ffmpegPath, "-version")
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

	cmd := exec.Command(s.ffmpegPath, args...)
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

	cmd := exec.Command(s.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg MP3 extraction failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// MuxVideoAudio combines video (with original audio removed) and new audio
func (s *FFmpegService) MuxVideoAudio(videoPath, audioPath, outputPath string) error {
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

	cmd := exec.Command(s.ffmpegPath, args...)
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

	cmd := exec.Command(s.ffmpegPath, args...)
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

	cmd := exec.Command(ffprobePath, args...)
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
		"-c", "copy",
		"-y",
		outputPath,
	}

	cmd := exec.Command(s.ffmpegPath, args...)
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

	// Calculate tempo factor
	// tempo > 1.0 = speed up (shorter duration)
	// tempo < 1.0 = slow down (longer duration)
	tempoFactor := actualDuration / targetDuration

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

	args := []string{
		"-i", inputPath,
		"-filter:a", filterStr,
		"-y",
		outputPath,
	}

	cmd := exec.Command(s.ffmpegPath, args...)
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
func (s *FFmpegService) GenerateSilence(duration float64, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	args := []string{
		"-f", "lavfi",
		"-i", fmt.Sprintf("anullsrc=r=24000:cl=mono:d=%.3f", duration),
		"-acodec", "libmp3lame",
		"-y",
		outputPath,
	}

	cmd := exec.Command(s.ffmpegPath, args...)
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

	cmd := exec.Command(s.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg conversion to WAV failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}
