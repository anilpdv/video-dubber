package services

import (
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

// TTSService uses Piper TTS (free, local, no API key)
type TTSService struct {
	piperPath  string
	voiceModel string
	voicesDir  string
	ffmpeg     *FFmpegService
	tempDir    string
}

// Available Piper TTS voices (all free, downloadable from HuggingFace)
var PiperVoices = map[string]string{
	"en_US-amy-medium":      "English (US) - Amy (Female)",
	"en_US-ryan-medium":     "English (US) - Ryan (Male)",
	"en_US-lessac-medium":   "English (US) - Lessac (Female)",
	"en_GB-alba-medium":     "English (UK) - Alba (Female)",
	"en_GB-aru-medium":      "English (UK) - Aru (Male)",
	"en_AU-natasha-medium":  "English (AU) - Natasha (Female)",
	"de_DE-thorsten-medium": "German - Thorsten (Male)",
	"fr_FR-upmc-medium":     "French - UPMC (Female)",
	"es_ES-sharvard-medium": "Spanish - Sharvard (Male)",
	"ru_RU-irina-medium":    "Russian - Irina (Female)",
}

// piperJobData contains data for a Piper TTS job.
type piperJobData struct {
	index int
	text  string
	start time.Duration
	end   time.Duration
}

func NewTTSService(voiceModel string) *TTSService {
	homeDir, _ := os.UserHomeDir()
	voicesDir := filepath.Join(homeDir, ".piper", "voices")
	tempDir := filepath.Join(os.TempDir(), "video-translator-tts")
	os.MkdirAll(tempDir, 0755)
	os.MkdirAll(voicesDir, 0755)

	// Default voice if none specified
	if voiceModel == "" {
		voiceModel = "en_US-amy-medium"
	}

	return &TTSService{
		piperPath:  "piper",
		voiceModel: voiceModel,
		voicesDir:  voicesDir,
		ffmpeg:     NewFFmpegService(),
		tempDir:    tempDir,
	}
}

// CheckInstalled verifies Piper TTS is available
func (s *TTSService) CheckInstalled() error {
	cmd := exec.Command(s.piperPath, "--help")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("piper TTS not found. Install with: pip install piper-tts")
	}
	return nil
}

// CheckVoiceModel verifies the voice model file exists
func (s *TTSService) CheckVoiceModel() error {
	modelPath := s.getModelPath()
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("voice model not found at %s.\nDownload from: https://huggingface.co/rhasspy/piper-voices", modelPath)
	}
	return nil
}

// getModelPath returns the full path to the voice model
func (s *TTSService) getModelPath() string {
	return filepath.Join(s.voicesDir, s.voiceModel+".onnx")
}

// Synthesize generates audio from text using Piper TTS with prosody control
func (s *TTSService) Synthesize(text, outputPath string) error {
	logger.LogInfo("Piper TTS: voice=%s model=%s", s.voiceModel, s.getModelPath())

	if text == "" {
		return fmt.Errorf("empty text provided")
	}

	modelPath := s.getModelPath()

	// Piper command with prosody parameters for better pronunciation
	// --length_scale: Speaking rate (1.0 = normal, 0.9 = slightly faster, 1.1 = slower)
	// --noise_scale: Variability in pronunciation (0.667 = balanced)
	// --noise_w: Phoneme duration variance (0.8 = natural variation)
	cmd := exec.Command(s.piperPath,
		"--model", modelPath,
		"--output_file", outputPath,
		"--length_scale", "1.0",  // Normal speaking rate
		"--noise_scale", "0.667", // Balanced variability
		"--noise_w", "0.8",       // Natural phoneme variation
	)

	// Pass text via stdin
	cmd.Stdin = strings.NewReader(text)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("piper TTS failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// SynthesizeSubtitles generates audio for all subtitles with proper timing
func (s *TTSService) SynthesizeSubtitles(subs models.SubtitleList, outputPath string) error {
	if len(subs) == 0 {
		return fmt.Errorf("no subtitles provided")
	}

	// Create temp directory for segments
	segmentDir := filepath.Join(s.tempDir, fmt.Sprintf("segments_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(segmentDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(segmentDir)

	var segmentPaths []string
	var lastEndTime time.Duration

	for i, sub := range subs {
		// Add silence for gap between subtitles
		if sub.StartTime > lastEndTime {
			gap := sub.StartTime - lastEndTime
			if gap > config.SilenceGapThreshold {
				silencePath := filepath.Join(segmentDir, fmt.Sprintf("silence_%04d.wav", i))
				if err := s.ffmpeg.GenerateSilence(gap.Seconds(), silencePath); err != nil {
					return fmt.Errorf("failed to generate silence: %w", err)
				}
				segmentPaths = append(segmentPaths, silencePath)
			}
		}

		// Skip empty text - just add silence for the duration
		if strings.TrimSpace(sub.Text) == "" {
			duration := sub.EndTime - sub.StartTime
			if duration > 0 {
				silencePath := filepath.Join(segmentDir, fmt.Sprintf("silence_sub_%04d.wav", i))
				if err := s.ffmpeg.GenerateSilence(duration.Seconds(), silencePath); err != nil {
					return fmt.Errorf("failed to generate silence for empty subtitle %d: %w", i+1, err)
				}
				segmentPaths = append(segmentPaths, silencePath)
			}
			lastEndTime = sub.EndTime
			continue
		}

		// Generate speech for this subtitle
		speechPath := filepath.Join(segmentDir, fmt.Sprintf("speech_%04d.wav", i))
		if err := s.Synthesize(sub.Text, speechPath); err != nil {
			return fmt.Errorf("failed to synthesize subtitle %d: %w", i+1, err)
		}

		// Adjust speech duration to match subtitle window for better sync
		targetDuration := (sub.EndTime - sub.StartTime).Seconds()
		if targetDuration > 0.2 { // Only adjust if subtitle has meaningful duration
			adjustedPath := filepath.Join(segmentDir, fmt.Sprintf("adjusted_%04d.wav", i))
			if err := s.ffmpeg.AdjustAudioDuration(speechPath, adjustedPath, targetDuration); err == nil {
				speechPath = adjustedPath // Use adjusted version
			}
			// If adjustment fails, use original speech (better than nothing)
		}
		segmentPaths = append(segmentPaths, speechPath)

		lastEndTime = sub.EndTime
	}

	// Concatenate all segments
	if err := s.ffmpeg.ConcatAudioFiles(segmentPaths, outputPath); err != nil {
		return fmt.Errorf("failed to concatenate audio: %w", err)
	}

	return nil
}

// SynthesizeWithCallback generates audio with progress callback
// Uses internal worker pool for parallel processing (3-4x faster synthesis)
func (s *TTSService) SynthesizeWithCallback(subs models.SubtitleList, outputPath string, onProgress func(current, total int)) error {
	if len(subs) == 0 {
		return fmt.Errorf("no subtitles provided")
	}

	segmentDir := filepath.Join(s.tempDir, fmt.Sprintf("segments_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(segmentDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(segmentDir)

	// Identify which subtitles need TTS (non-empty text)
	var jobs []piperJobData
	for i, sub := range subs {
		if strings.TrimSpace(sub.Text) != "" {
			jobs = append(jobs, piperJobData{
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
		processJob := func(job worker.Job[piperJobData]) (string, error) {
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

		// Run worker pool with dynamic worker count (CPU-intensive local TTS)
		workers := config.DynamicWorkerCount("tts-local")
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


// SetVoice changes the voice model
func (s *TTSService) SetVoice(voice string) {
	s.voiceModel = voice
}

// GetVoice returns the current voice model
func (s *TTSService) GetVoice() string {
	return s.voiceModel
}

// GetVoicesForLanguage returns voices for a specific language code
func GetVoicesForLanguage(langCode string) map[string]string {
	prefix := strings.ToLower(langCode)
	// Convert standard lang codes to Piper format
	if prefix == "en" {
		prefix = "en_"
	} else if len(prefix) == 2 {
		prefix = prefix + "_"
	}

	result := make(map[string]string)
	for voice, desc := range PiperVoices {
		if strings.HasPrefix(strings.ToLower(voice), prefix) {
			result[voice] = desc
		}
	}
	return result
}

// DownloadVoiceModel provides the URL to download a voice model
func DownloadVoiceModelURL(voice string) string {
	// Parse voice name: en_US-amy-medium -> en/en_US/amy/medium/
	parts := strings.Split(voice, "-")
	if len(parts) < 3 {
		return ""
	}

	langCountry := parts[0] // en_US
	langParts := strings.Split(langCountry, "_")
	if len(langParts) < 2 {
		return ""
	}

	lang := langParts[0]     // en
	speaker := parts[1]      // amy
	quality := parts[2]      // medium

	return fmt.Sprintf("https://huggingface.co/rhasspy/piper-voices/resolve/main/%s/%s/%s/%s/%s.onnx",
		lang, langCountry, speaker, quality, voice)
}

// Cleanup removes temporary files
func (s *TTSService) Cleanup() error {
	return os.RemoveAll(s.tempDir)
}
