package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"video-translator/internal/config"
	"video-translator/internal/logger"
	"video-translator/models"
)

type ProgressCallback func(stage string, percent int, message string)

type Pipeline struct {
	ffmpeg *FFmpegService
	config *models.Config

	// Transcription providers
	whisper       *WhisperService
	fasterWhisper *FasterWhisperService
	groq          *GroqTranscriptionService

	// Translation providers
	translator *TranslatorService
	deepseek   *DeepSeekService

	// TTS providers
	tts       *TTSService
	openaiTTS *OpenAITTSService
	cosyvoice *CosyVoiceService
	edgeTTS   *EdgeTTSService

	onProgress ProgressCallback
	tempDir    string
}

func NewPipeline(config *models.Config) *Pipeline {
	tempDir := filepath.Join(os.TempDir(), "video-translator")
	os.MkdirAll(tempDir, 0755)

	p := &Pipeline{
		ffmpeg:     NewFFmpegService(),
		config:     config,
		tempDir:    tempDir,
		whisper:    NewWhisperService(),
		translator: NewTranslatorService(),
		tts:        NewTTSService(config.DefaultVoice),
	}

	// Initialize FasterWhisper if selected
	if config.TranscriptionProvider == "faster-whisper" {
		p.fasterWhisper = NewFasterWhisperService(
			config.PythonPath,
			config.FasterWhisperModel,
			config.FasterWhisperDevice,
		)
	}

	// Initialize Groq if selected and API key available
	if config.TranscriptionProvider == "groq" && config.GroqAPIKey != "" {
		p.groq = NewGroqTranscriptionService(config.GroqAPIKey)
	}

	// Initialize DeepSeek if API key is provided
	if config.DeepSeekKey != "" {
		p.deepseek = NewDeepSeekService(config.DeepSeekKey)
	}

	// Initialize OpenAI TTS if selected and API key available
	if config.TTSProvider == "openai" && config.OpenAIKey != "" {
		p.openaiTTS = NewOpenAITTSService(
			config.OpenAIKey,
			config.OpenAITTSModel,
			config.OpenAITTSVoice,
			config.OpenAITTSSpeed,
		)
	}

	// Initialize CosyVoice if selected
	if config.TTSProvider == "cosyvoice" {
		p.cosyvoice = NewCosyVoiceService(
			config.CosyVoicePath,
			config.CosyVoiceMode,
			config.CosyVoiceAPIURL,
			config.VoiceCloneSamplePath,
			config.PythonPath,
		)
	}

	// Initialize Edge TTS if selected (FREE neural TTS)
	if config.TTSProvider == "edge-tts" {
		p.edgeTTS = NewEdgeTTSService(config.EdgeTTSVoice)
	}

	return p
}

func (p *Pipeline) SetProgressCallback(cb ProgressCallback) {
	p.onProgress = cb
}

func (p *Pipeline) progress(stage string, percent int, message string) {
	if p.onProgress != nil {
		p.onProgress(stage, percent, message)
	}
}

// Process runs the full translation pipeline using the default progress callback
func (p *Pipeline) Process(job *models.TranslationJob) error {
	return p.ProcessWithCallback(job, p.onProgress)
}

// ProcessWithCallback runs the full translation pipeline with a custom progress callback
// This allows parallel processing of multiple videos with per-job progress tracking
func (p *Pipeline) ProcessWithCallback(job *models.TranslationJob, onProgress ProgressCallback) error {
	// Helper to safely call progress callback
	reportProgress := func(stage string, percent int, message string) {
		if onProgress != nil {
			onProgress(stage, percent, message)
		}
	}
	// Generate unique ID for temp files
	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	jobTempDir := filepath.Join(p.tempDir, jobID)
	if err := os.MkdirAll(jobTempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(jobTempDir) // Cleanup on completion

	// Update job settings if not set
	if job.SourceLang == "" {
		job.SourceLang = p.config.DefaultSourceLang
	}
	if job.TargetLang == "" {
		job.TargetLang = p.config.DefaultTargetLang
	}
	if job.Voice == "" {
		job.Voice = p.config.DefaultVoice
	}

	// Stage 1: Extract Audio
	logger.LogInfo("Pipeline: Stage 1/5 - Extracting audio from %s", filepath.Base(job.InputPath))
	reportProgress("Extracting", config.ProgressExtractStart, "Extracting audio from video...")
	job.SetStatus(models.StatusExtracting, "Extracting audio", config.ProgressExtractStart)

	audioPath := filepath.Join(jobTempDir, "audio.wav")
	if err := p.ffmpeg.ExtractAudio(job.InputPath, audioPath); err != nil {
		job.Fail(err)
		return fmt.Errorf("audio extraction failed: %w", err)
	}
	job.AudioPath = audioPath
	reportProgress("Extracting", config.ProgressExtractEnd, "Audio extracted")

	// Stage 2: Transcribe (with parallel chunking for long audio)
	provider := p.getTranscriptionProvider()
	logger.LogInfo("Pipeline: Stage 2/5 - Transcribing with %s (lang=%s)", provider, job.SourceLang)
	reportProgress("Transcribing", config.ProgressTranscribeStart, "Starting transcription...")
	job.SetStatus(models.StatusTranscribing, "Transcribing audio", config.ProgressTranscribeStart)

	// Get audio duration to determine if chunking is beneficial
	audioDuration, _ := p.ffmpeg.GetVideoDuration(audioPath)

	var subtitles models.SubtitleList
	var err error

	// Use parallel chunking for audio longer than 30 seconds (for local providers)
	// Skip chunking for API providers (openai, groq) - they handle it internally
	useChunking := audioDuration > config.MinChunkDuration.Seconds() && provider != "openai" && provider != "groq"

	if useChunking && (provider == "whisper-cpp" || provider == "faster-whisper") {
		// Split audio into chunks for parallel processing
		chunkDir := filepath.Join(jobTempDir, "chunks")
		chunks, chunkErr := p.ffmpeg.SplitAudioIntoChunks(
			audioPath,
			chunkDir,
			config.AudioChunkDuration.Seconds(),
			config.AudioChunkOverlap.Seconds(),
		)
		if chunkErr != nil {
			logger.LogError("Failed to split audio: %v, falling back to sequential", chunkErr)
			useChunking = false
		} else {
			defer p.ffmpeg.CleanupChunks(chunks)

			// Calculate progress range for transcription
			transcribeRange := config.ProgressTranscribeEnd - config.ProgressTranscribeStart

			switch provider {
			case "faster-whisper":
				reportProgress("Transcribing", config.ProgressTranscribeStart+1,
					fmt.Sprintf("FasterWhisper: processing %d chunks in parallel...", len(chunks)))
				subtitles, err = p.fasterWhisper.TranscribeChunksParallel(
					chunks,
					job.SourceLang,
					func(completed, total int) {
						percent := config.ProgressTranscribeStart + (completed*transcribeRange)/total
						reportProgress("Transcribing", percent,
							fmt.Sprintf("FasterWhisper: %d/%d chunks", completed, total))
					},
				)
			default: // whisper-cpp
				reportProgress("Transcribing", config.ProgressTranscribeStart+1,
					fmt.Sprintf("Whisper: processing %d chunks in parallel...", len(chunks)))
				subtitles, err = p.whisper.TranscribeChunksParallel(
					chunks,
					job.SourceLang,
					func(completed, total int) {
						percent := config.ProgressTranscribeStart + (completed*transcribeRange)/total
						reportProgress("Transcribing", percent,
							fmt.Sprintf("Whisper: %d/%d chunks", completed, total))
					},
				)
			}
		}
	}

	// Fallback to sequential transcription (or if chunking wasn't used)
	if !useChunking || subtitles == nil {
		switch provider {
		case "faster-whisper":
			reportProgress("Transcribing", config.ProgressTranscribeStart+1, "Using FasterWhisper (GPU accelerated)...")
			subtitles, err = p.fasterWhisper.TranscribeWithProgress(
				audioPath,
				job.SourceLang,
				audioDuration,
				func(currentSec float64, percent int) {
					remaining := audioDuration - currentSec
					var msg string
					if remaining > 60 {
						msg = fmt.Sprintf("FasterWhisper: %.0f min remaining", remaining/60)
					} else {
						msg = fmt.Sprintf("FasterWhisper: %.0f sec remaining", remaining)
					}
					reportProgress("Transcribing", percent, msg)
				},
			)

		case "openai":
			reportProgress("Transcribing", config.ProgressTranscribeStart+1, "Using OpenAI Whisper API...")
			subtitles, err = p.whisper.TranscribeWithOpenAI(
				audioPath,
				p.config.OpenAIKey,
				job.SourceLang,
				func(percent int, message string) {
					reportProgress("Transcribing", percent, message)
				},
			)

		case "groq":
			reportProgress("Transcribing", config.ProgressTranscribeStart+1, "Using Groq Whisper (ultra-fast)...")
			subtitles, err = p.groq.TranscribeWithProgress(
				audioPath,
				job.SourceLang,
				func(percent int, message string) {
					reportProgress("Transcribing", percent, message)
				},
			)

		default: // "whisper-cpp"
			reportProgress("Transcribing", config.ProgressTranscribeStart+1, "Using local Whisper...")
			subtitles, err = p.whisper.TranscribeWithProgress(
				audioPath,
				job.SourceLang,
				audioDuration,
				func(currentSec float64, percent int) {
					remaining := audioDuration - currentSec
					var msg string
					if remaining > 60 {
						msg = fmt.Sprintf("Transcribing... %.0f min remaining", remaining/60)
					} else {
						msg = fmt.Sprintf("Transcribing... %.0f sec remaining", remaining)
					}
					reportProgress("Transcribing", percent, msg)
				},
			)
		}
	}

	if err != nil {
		job.Fail(err)
		return fmt.Errorf("transcription failed: %w", err)
	}

	if len(subtitles) == 0 {
		job.Fail(fmt.Errorf("no speech detected in audio"))
		return fmt.Errorf("no speech detected in audio")
	}

	reportProgress("Transcribing", config.ProgressTranscribeEnd, fmt.Sprintf("Transcribed %d segments", len(subtitles)))

	// Stage 3: Translate
	transProvider := p.getTranslationProvider()
	logger.LogInfo("Pipeline: Stage 3/5 - Translating with %s (%s → %s)", transProvider, job.SourceLang, job.TargetLang)
	reportProgress("Translating", config.ProgressTranslateStart, "Translating text...")
	job.SetStatus(models.StatusTranslating, "Translating text", config.ProgressTranslateStart)

	translateRange := config.ProgressTranslateEnd - config.ProgressTranslateStart
	var translatedSubs models.SubtitleList
	switch transProvider {
	case "deepseek":
		reportProgress("Translating", config.ProgressTranslateStart+1, "Using DeepSeek (cost-effective)...")
		translatedSubs, err = p.deepseek.TranslateSubtitles(
			subtitles,
			job.SourceLang,
			job.TargetLang,
			func(current, total int) {
				percent := config.ProgressTranslateStart + (current*translateRange)/total
				msg := fmt.Sprintf("DeepSeek: %d/%d segments", current, total)
				reportProgress("Translating", percent, msg)
			},
		)

	case "openai":
		reportProgress("Translating", config.ProgressTranslateStart+1, "Using OpenAI GPT-4o-mini...")
		translatedSubs, err = p.translator.TranslateWithOpenAI(
			subtitles,
			job.SourceLang,
			job.TargetLang,
			p.config.OpenAIKey,
			func(current, total int) {
				percent := config.ProgressTranslateStart + (current*translateRange)/total
				msg := fmt.Sprintf("GPT-4o-mini: %d/%d segments", current, total)
				reportProgress("Translating", percent, msg)
			},
		)

	default: // "argos"
		reportProgress("Translating", config.ProgressTranslateStart+1, "Using local Argos Translate...")
		translatedSubs, err = p.translator.TranslateSubtitlesWithProgress(
			subtitles,
			job.SourceLang,
			job.TargetLang,
			func(current, total int) {
				percent := config.ProgressTranslateStart + (current*translateRange)/total
				msg := fmt.Sprintf("Argos: %d/%d segments", current, total)
				reportProgress("Translating", percent, msg)
			},
		)
	}

	if err != nil {
		job.Fail(err)
		return fmt.Errorf("translation failed: %w", err)
	}
	reportProgress("Translating", config.ProgressTranslateEnd, "Translation complete")

	// Stage 4: Text-to-Speech
	ttsProvider := p.getTTSProvider()
	logger.LogInfo("Pipeline: Stage 4/5 - Synthesizing with %s (voice=%s)", ttsProvider, job.Voice)
	reportProgress("Synthesizing", config.ProgressSynthesizeStart, "Generating speech...")
	job.SetStatus(models.StatusSynthesizing, "Generating dubbed audio", config.ProgressSynthesizeStart)

	synthesizeRange := config.ProgressSynthesizeEnd - config.ProgressSynthesizeStart
	dubbedAudioPath := filepath.Join(jobTempDir, "dubbed.wav")
	switch ttsProvider {
	case "openai":
		reportProgress("Synthesizing", config.ProgressSynthesizeStart+1, "Using OpenAI TTS (high quality)...")
		if p.openaiTTS != nil {
			p.openaiTTS.SetVoice(job.Voice)
		}
		err = p.openaiTTS.SynthesizeWithCallback(translatedSubs, dubbedAudioPath, func(current, total int) {
			progress := config.ProgressSynthesizeStart + (current*synthesizeRange)/total
			reportProgress("Synthesizing", progress, fmt.Sprintf("OpenAI TTS: %d/%d", current, total))
		})

	case "cosyvoice":
		reportProgress("Synthesizing", config.ProgressSynthesizeStart+1, "Using CosyVoice (voice cloning)...")
		err = p.cosyvoice.SynthesizeWithCallback(translatedSubs, dubbedAudioPath, func(current, total int) {
			progress := config.ProgressSynthesizeStart + (current*synthesizeRange)/total
			reportProgress("Synthesizing", progress, fmt.Sprintf("CosyVoice: %d/%d", current, total))
		})

	case "edge-tts":
		reportProgress("Synthesizing", config.ProgressSynthesizeStart+1, "Using Edge TTS (FREE neural)...")
		if p.edgeTTS != nil {
			p.edgeTTS.SetVoice(job.Voice)
		}
		err = p.edgeTTS.SynthesizeWithCallback(translatedSubs, dubbedAudioPath, func(current, total int) {
			progress := config.ProgressSynthesizeStart + (current*synthesizeRange)/total
			reportProgress("Synthesizing", progress, fmt.Sprintf("Edge TTS: %d/%d", current, total))
		})

	default: // "piper"
		reportProgress("Synthesizing", config.ProgressSynthesizeStart+1, "Using Piper TTS...")
		p.tts.SetVoice(job.Voice)
		err = p.tts.SynthesizeWithCallback(translatedSubs, dubbedAudioPath, func(current, total int) {
			progress := config.ProgressSynthesizeStart + (current*synthesizeRange)/total
			reportProgress("Synthesizing", progress, fmt.Sprintf("Piper: %d/%d", current, total))
		})
	}

	if err != nil {
		job.Fail(err)
		return fmt.Errorf("speech synthesis failed: %w", err)
	}
	job.DubbedAudioPath = dubbedAudioPath
	reportProgress("Synthesizing", config.ProgressSynthesizeEnd, "Speech synthesis complete")

	// Stage 5: Mux Video
	logger.LogInfo("Pipeline: Stage 5/5 - Muxing final video")
	reportProgress("Muxing", config.ProgressMuxStart, "Creating final video...")
	job.SetStatus(models.StatusMuxing, "Creating final video", config.ProgressMuxStart)

	// Generate output path
	outputPath := p.generateOutputPath(job.InputPath)
	if err := p.ffmpeg.MuxVideoAudio(job.InputPath, dubbedAudioPath, outputPath); err != nil {
		job.Fail(err)
		return fmt.Errorf("video muxing failed: %w", err)
	}

	job.Complete(outputPath)
	logger.LogInfo("Pipeline: Complete! Output: %s", outputPath)
	reportProgress("Complete", config.ProgressMuxEnd, "Translation complete!")

	return nil
}

// getTranscriptionProvider returns the effective transcription provider
func (p *Pipeline) getTranscriptionProvider() string {
	// Check explicit provider selection
	if p.config.TranscriptionProvider != "" {
		return p.config.TranscriptionProvider
	}
	// Legacy: use OpenAI if UseOpenAIAPIs is enabled
	if p.config.UseOpenAIAPIs && p.config.OpenAIKey != "" {
		return "openai"
	}
	return "whisper-cpp"
}

// getTranslationProvider returns the effective translation provider
func (p *Pipeline) getTranslationProvider() string {
	// Check explicit provider selection
	if p.config.TranslationProvider != "" {
		return p.config.TranslationProvider
	}
	// Legacy: use OpenAI if UseOpenAIAPIs is enabled
	if p.config.UseOpenAIAPIs && p.config.OpenAIKey != "" {
		return "openai"
	}
	return "argos"
}

// getTTSProvider returns the effective TTS provider
func (p *Pipeline) getTTSProvider() string {
	if p.config.TTSProvider != "" {
		return p.config.TTSProvider
	}
	return "piper"
}

// ProcessWithOriginalAudio mixes dubbed audio with quieter original
func (p *Pipeline) ProcessWithOriginalAudio(job *models.TranslationJob, originalVolume float64) error {
	// Similar to Process but uses MuxVideoAudioWithOriginal
	// ... (implementation would be similar with the additional parameter)
	return p.Process(job) // Simplified for now
}

// generateOutputPath creates the output file path
func (p *Pipeline) generateOutputPath(inputPath string) string {
	dir := p.config.OutputDirectory
	if dir == "" {
		dir = filepath.Dir(inputPath)
	}

	// Expand ~ to home directory
	if strings.HasPrefix(dir, "~") {
		homeDir, _ := os.UserHomeDir()
		dir = filepath.Join(homeDir, dir[1:])
	}

	// Create output directory if it doesn't exist
	os.MkdirAll(dir, 0755)

	// Generate output filename
	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputName := fmt.Sprintf("%s_translated%s", baseName, filepath.Ext(inputPath))

	return filepath.Join(dir, outputName)
}

// ValidateJob checks if a job can be processed
func (p *Pipeline) ValidateJob(job *models.TranslationJob) error {
	// Check input file exists
	if _, err := os.Stat(job.InputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", job.InputPath)
	}

	// Check FFmpeg (always required)
	if err := p.ffmpeg.CheckInstalled(); err != nil {
		return err
	}

	// Validate transcription provider
	switch p.getTranscriptionProvider() {
	case "faster-whisper":
		if p.fasterWhisper == nil {
			return fmt.Errorf("FasterWhisper not initialized")
		}
		if err := p.fasterWhisper.CheckInstalled(); err != nil {
			return err
		}
	case "openai":
		if p.config.OpenAIKey == "" {
			return fmt.Errorf("OpenAI API key required for OpenAI Whisper")
		}
	case "groq":
		if p.groq == nil || p.config.GroqAPIKey == "" {
			return fmt.Errorf("Groq API key required. Get one at https://console.groq.com")
		}
		if err := p.groq.CheckInstalled(); err != nil {
			return err
		}
	default: // whisper-cpp
		if err := p.whisper.CheckInstalled(); err != nil {
			return err
		}
		if err := p.whisper.CheckModel(); err != nil {
			return err
		}
	}

	// Validate translation provider
	switch p.getTranslationProvider() {
	case "deepseek":
		if p.deepseek == nil || p.config.DeepSeekKey == "" {
			return fmt.Errorf("DeepSeek API key required")
		}
	case "openai":
		if p.config.OpenAIKey == "" {
			return fmt.Errorf("OpenAI API key required for translation")
		}
	default: // argos
		if err := p.translator.CheckInstalled(); err != nil {
			return err
		}
		if err := p.translator.CheckLanguagePackage(job.SourceLang, job.TargetLang); err != nil {
			return err
		}
	}

	// Validate TTS provider
	switch p.getTTSProvider() {
	case "openai":
		if p.openaiTTS == nil || p.config.OpenAIKey == "" {
			return fmt.Errorf("OpenAI API key required for TTS")
		}
	case "cosyvoice":
		if p.cosyvoice == nil {
			return fmt.Errorf("CosyVoice not initialized")
		}
		if err := p.cosyvoice.CheckInstalled(); err != nil {
			return err
		}
		if !p.cosyvoice.HasVoiceSample() {
			return fmt.Errorf("voice sample required for CosyVoice")
		}
	case "edge-tts":
		if p.edgeTTS == nil {
			return fmt.Errorf("Edge TTS not initialized")
		}
		if err := p.edgeTTS.CheckInstalled(); err != nil {
			return err
		}
	default: // piper
		if err := p.tts.CheckInstalled(); err != nil {
			return err
		}
		if err := p.tts.CheckVoiceModel(); err != nil {
			return err
		}
	}

	return nil
}

// CheckDependencies verifies all required tools are installed
func (p *Pipeline) CheckDependencies() map[string]error {
	results := make(map[string]error)

	// Always check FFmpeg
	results["ffmpeg"] = p.ffmpeg.CheckInstalled()

	// Check transcription providers
	results["whisper-cpp"] = p.whisper.CheckInstalled()
	results["whisper-model"] = p.whisper.CheckModel()
	if p.fasterWhisper != nil {
		results["faster-whisper"] = p.fasterWhisper.CheckInstalled()
	}
	if p.groq != nil {
		results["groq"] = p.groq.CheckInstalled()
	}

	// Check translation providers
	results["argos-translate"] = p.translator.CheckInstalled()
	results["argos-ru-en"] = p.translator.CheckLanguagePackage("ru", "en")
	if p.deepseek != nil {
		results["deepseek"] = p.deepseek.CheckAPIKey()
	}

	// Check TTS providers
	results["piper-tts"] = p.tts.CheckInstalled()
	results["piper-voice"] = p.tts.CheckVoiceModel()
	if p.openaiTTS != nil {
		results["openai-tts"] = p.openaiTTS.CheckInstalled()
	}
	if p.cosyvoice != nil {
		results["cosyvoice"] = p.cosyvoice.CheckInstalled()
	}
	if p.edgeTTS != nil {
		results["edge-tts"] = p.edgeTTS.CheckInstalled()
	}

	return results
}

// Cleanup removes all temporary files
func (p *Pipeline) Cleanup() error {
	return os.RemoveAll(p.tempDir)
}

// ProcessAsync runs the pipeline in a goroutine
func (p *Pipeline) ProcessAsync(job *models.TranslationJob, done chan<- error) {
	go func() {
		done <- p.Process(job)
	}()
}

// GetEstimatedTime estimates processing time based on video duration
func (p *Pipeline) GetEstimatedTime(videoDuration float64) time.Duration {
	// Estimate based on provider selection
	multiplier := 2.0 // Default for local processing

	switch p.getTranscriptionProvider() {
	case "openai":
		multiplier = 0.5 // API is fast
	case "faster-whisper":
		multiplier = 0.3 // GPU is fast
	}

	return time.Duration(videoDuration*multiplier) * time.Second
}

// GetProviderInfo returns information about currently selected providers
func (p *Pipeline) GetProviderInfo() map[string]string {
	return map[string]string{
		"transcription": p.getTranscriptionProvider(),
		"translation":   p.getTranslationProvider(),
		"tts":           p.getTTSProvider(),
	}
}
