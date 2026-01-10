package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"video-translator/internal/config"
	"video-translator/internal/logger"
	"video-translator/internal/media"
	"video-translator/internal/worker"
	"video-translator/models"
)

// EdgeTTSService handles text-to-speech using Microsoft Edge TTS (FREE)
type EdgeTTSService struct {
	voice   string
	ffmpeg  *FFmpegService
	tempDir string
}

// Edge TTS voices - common high-quality neural voices
var EdgeTTSVoices = map[string]string{
	"en-US-AriaNeural":     "Aria (US Female, Natural)",
	"en-US-GuyNeural":      "Guy (US Male, Friendly)",
	"en-US-JennyNeural":    "Jenny (US Female, Assistant)",
	"en-GB-SoniaNeural":    "Sonia (British Female)",
	"en-GB-RyanNeural":     "Ryan (British Male)",
	"en-AU-NatashaNeural":  "Natasha (Australian Female)",
	"de-DE-KatjaNeural":    "Katja (German Female)",
	"de-DE-ConradNeural":   "Conrad (German Male)",
	"fr-FR-DeniseNeural":   "Denise (French Female)",
	"fr-FR-HenriNeural":    "Henri (French Male)",
	"es-ES-ElviraNeural":   "Elvira (Spanish Female)",
	"es-ES-AlvaroNeural":   "Alvaro (Spanish Male)",
	"it-IT-ElsaNeural":     "Elsa (Italian Female)",
	"it-IT-DiegoNeural":    "Diego (Italian Male)",
	"pt-BR-FranciscaNeural": "Francisca (Brazilian Portuguese Female)",
	"ja-JP-NanamiNeural":   "Nanami (Japanese Female)",
	"ko-KR-SunHiNeural":    "SunHi (Korean Female)",
	"zh-CN-XiaoxiaoNeural": "Xiaoxiao (Chinese Female)",
	"ru-RU-SvetlanaNeural": "Svetlana (Russian Female)",
	"ru-RU-DmitryNeural":   "Dmitry (Russian Male)",
}

// NewEdgeTTSService creates a new Edge TTS service
func NewEdgeTTSService(voice string) *EdgeTTSService {
	if voice == "" {
		voice = "en-US-AriaNeural"
	}

	tempDir := filepath.Join(os.TempDir(), "video-translator-edge-tts")
	os.MkdirAll(tempDir, 0755)

	return &EdgeTTSService{
		voice:   voice,
		ffmpeg:  NewFFmpegService(),
		tempDir: tempDir,
	}
}

// CheckInstalled verifies edge-tts is installed
func (s *EdgeTTSService) CheckInstalled() error {
	cmd := exec.Command("edge-tts", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("edge-tts not installed. Install with: pip install edge-tts")
	}
	return nil
}

// SetVoice changes the voice for synthesis
func (s *EdgeTTSService) SetVoice(voice string) {
	if voice != "" {
		s.voice = voice
	}
}

// Synthesize generates audio from text using Edge TTS
func (s *EdgeTTSService) Synthesize(text, outputPath string) error {
	logger.LogInfo("Edge TTS: voice=%s", s.voice)

	if text == "" {
		return fmt.Errorf("empty text provided")
	}

	// Clean the voice name
	voice := strings.TrimSpace(s.voice)

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get absolute path for output
	absOutputPath, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Create temp file for text input (avoids shell escaping issues)
	tempFile, err := os.CreateTemp(filepath.Dir(absOutputPath), "edge_tts_text_*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempFileName := tempFile.Name()

	// Cleanup temp file when done
	defer func() {
		tempFile.Close()
		os.Remove(tempFileName)
	}()

	// Write text to temp file
	if _, err := tempFile.WriteString(text); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tempFile.Close()

	// Retry logic (3 attempts like KrillinAI)
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := s.attemptTTS(tempFileName, voice, absOutputPath, attempt)
		if err == nil {
			// Verify output file exists
			if _, statErr := os.Stat(absOutputPath); os.IsNotExist(statErr) {
				return fmt.Errorf("edge-tts output file not found: %s", absOutputPath)
			}
			return nil
		}

		// Wait before retry (exponential backoff)
		if attempt < maxRetries {
			waitTime := time.Duration(attempt) * 2 * time.Second
			time.Sleep(waitTime)
		}
	}

	return fmt.Errorf("edge-tts failed after %d attempts", maxRetries)
}

// attemptTTS makes a single TTS attempt
func (s *EdgeTTSService) attemptTTS(tempFileName, voice, outputPath string, _ int) error {
	// Determine output format based on extension
	ext := strings.ToLower(filepath.Ext(outputPath))

	// Edge-tts outputs mp3 by default, we'll convert if needed
	mp3Path := outputPath
	needsConversion := false
	if ext == ".wav" {
		mp3Path = strings.TrimSuffix(outputPath, ext) + ".mp3"
		needsConversion = true
	}

	// Build edge-tts command
	cmdArgs := []string{
		"--file", tempFileName,
		"--voice", voice,
		"--write-media", mp3Path,
	}

	// Create context with timeout (60 seconds)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "edge-tts", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("edge-tts timeout")
		}
		return fmt.Errorf("edge-tts failed: %s, output: %s", err, string(output))
	}

	// Convert MP3 to WAV if needed
	if needsConversion {
		if err := s.ffmpeg.ConvertToWAV(mp3Path, outputPath); err != nil {
			os.Remove(mp3Path)
			return fmt.Errorf("failed to convert to WAV: %w", err)
		}
		os.Remove(mp3Path)
	}

	return nil
}

// SynthesizeSubtitles generates audio for all subtitles with proper timing
func (s *EdgeTTSService) SynthesizeSubtitles(subs models.SubtitleList, outputPath string) error {
	return s.SynthesizeWithCallback(subs, outputPath, nil)
}

// edgeJobData contains data for an Edge TTS job.
type edgeJobData struct {
	index int
	text  string
	start time.Duration
	end   time.Duration
}

// SynthesizeWithCallback generates audio for subtitles with progress callback
// Uses internal worker pool for parallel processing (Edge TTS is FREE with generous rate limits)
func (s *EdgeTTSService) SynthesizeWithCallback(
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
	var jobs []edgeJobData
	for i, sub := range subs {
		if strings.TrimSpace(sub.Text) != "" {
			jobs = append(jobs, edgeJobData{
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
		processJob := func(job worker.Job[edgeJobData]) (string, error) {
			data := job.Data
			speechPath := filepath.Join(segmentDir, fmt.Sprintf("speech_%04d.wav", data.index))

			// Synthesize the text
			if err := s.synthesizeSingle(data.text, speechPath); err != nil {
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

		// Run worker pool with error tolerance (Edge TTS generates silence for failed segments)
		// Use dynamic worker count for optimal parallelism based on system resources
		workers := config.DynamicWorkerCount("tts-api")
		results, errors := worker.ProcessWithErrors(jobs, workers, processJob, progressCallback)

		// Build speech paths map, using silence for failed jobs
		ffmpegMedia := media.NewFFmpegServiceWithPath(s.ffmpeg.GetPath())
		for i, path := range results {
			jobData := jobs[i]
			if errors != nil && i < len(errors) && errors[i] != nil {
				// Generate silence for failed segment (like KrillinAI behavior)
				duration := (jobData.end - jobData.start).Seconds()
				if duration > 0 {
					silencePath := filepath.Join(segmentDir, fmt.Sprintf("silence_fail_%04d.wav", jobData.index))
					if err := ffmpegMedia.GenerateSilence(duration, silencePath); err == nil {
						speechPaths[jobData.index] = silencePath
					}
				}
			} else if path != "" {
				speechPaths[jobData.index] = path
			}
		}
	}

	// Build final audio using AudioAssembler with parallel gap processing
	internalSubs := models.ToInternalSubtitles(subs)
	ffmpegMediaFinal := media.NewFFmpegServiceWithPath(s.ffmpeg.GetPath())
	assembler := media.NewAudioAssembler(ffmpegMediaFinal, segmentDir)

	if err := assembler.AssembleFromSpeechPathsParallel(internalSubs, speechPaths, outputPath); err != nil {
		return fmt.Errorf("failed to assemble audio: %w", err)
	}

	return nil
}


// synthesizeSingle synthesizes a single text segment with retry
func (s *EdgeTTSService) synthesizeSingle(text, outputPath string) error {
	if text == "" {
		return fmt.Errorf("empty text")
	}

	// Clean the voice name
	voice := strings.TrimSpace(s.voice)

	// Create temp file for text input
	tempFile, err := os.CreateTemp(filepath.Dir(outputPath), "edge_tts_*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempFileName := tempFile.Name()
	defer func() {
		tempFile.Close()
		os.Remove(tempFileName)
	}()

	if _, err := tempFile.WriteString(text); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tempFile.Close()

	// Retry logic
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := s.attemptTTS(tempFileName, voice, outputPath, attempt)
		if err == nil {
			return nil
		}
		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return fmt.Errorf("edge-tts failed after %d retries", maxRetries)
}

// GetEdgeTTSVoices returns the list of available Edge TTS voices
func GetEdgeTTSVoices() map[string]string {
	return EdgeTTSVoices
}

// GetEdgeTTSVoiceList returns voice IDs as a slice (most common ones)
func GetEdgeTTSVoiceList() []string {
	return []string{
		"en-US-AriaNeural",
		"en-US-GuyNeural",
		"en-US-JennyNeural",
		"en-GB-SoniaNeural",
		"en-GB-RyanNeural",
		"de-DE-KatjaNeural",
		"fr-FR-DeniseNeural",
		"es-ES-ElviraNeural",
		"ru-RU-SvetlanaNeural",
	}
}

// EstimateCost returns 0 since Edge TTS is FREE
func (s *EdgeTTSService) EstimateCost(charCount int) float64 {
	return 0.0 // FREE!
}
