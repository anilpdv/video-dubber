package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"video-translator/models"
)

type ProgressCallback func(stage string, percent int, message string)

type Pipeline struct {
	ffmpeg *FFmpegService
	config *models.Config

	// Transcription providers
	whisper       *WhisperService
	fasterWhisper *FasterWhisperService

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

// Process runs the full translation pipeline
func (p *Pipeline) Process(job *models.TranslationJob) error {
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

	// Stage 1: Extract Audio (0-15%)
	p.progress("Extracting", 0, "Extracting audio from video...")
	job.SetStatus(models.StatusExtracting, "Extracting audio", 0)

	audioPath := filepath.Join(jobTempDir, "audio.wav")
	if err := p.ffmpeg.ExtractAudio(job.InputPath, audioPath); err != nil {
		job.Fail(err)
		return fmt.Errorf("audio extraction failed: %w", err)
	}
	job.AudioPath = audioPath
	p.progress("Extracting", 15, "Audio extracted")

	// Stage 2: Transcribe (15-40%)
	p.progress("Transcribing", 15, "Starting transcription...")
	job.SetStatus(models.StatusTranscribing, "Transcribing audio", 15)

	var subtitles models.SubtitleList
	var err error

	// Select transcription provider
	provider := p.getTranscriptionProvider()
	switch provider {
	case "faster-whisper":
		p.progress("Transcribing", 16, "Using FasterWhisper (GPU accelerated)...")
		audioDuration, _ := p.ffmpeg.GetVideoDuration(audioPath)
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
				p.progress("Transcribing", percent, msg)
			},
		)

	case "openai":
		p.progress("Transcribing", 16, "Using OpenAI Whisper API...")
		subtitles, err = p.whisper.TranscribeWithOpenAI(
			audioPath,
			p.config.OpenAIKey,
			job.SourceLang,
			func(percent int, message string) {
				p.progress("Transcribing", percent, message)
			},
		)

	default: // "whisper-cpp"
		p.progress("Transcribing", 16, "Using local Whisper...")
		audioDuration, _ := p.ffmpeg.GetVideoDuration(audioPath)
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
				p.progress("Transcribing", percent, msg)
			},
		)
	}

	if err != nil {
		job.Fail(err)
		return fmt.Errorf("transcription failed: %w", err)
	}

	if len(subtitles) == 0 {
		job.Fail(fmt.Errorf("no speech detected in audio"))
		return fmt.Errorf("no speech detected in audio")
	}

	p.progress("Transcribing", 40, fmt.Sprintf("Transcribed %d segments", len(subtitles)))

	// Stage 3: Translate (40-60%)
	p.progress("Translating", 40, "Translating text...")
	job.SetStatus(models.StatusTranslating, "Translating text", 40)

	var translatedSubs models.SubtitleList

	// Select translation provider
	transProvider := p.getTranslationProvider()
	switch transProvider {
	case "deepseek":
		p.progress("Translating", 41, "Using DeepSeek (cost-effective)...")
		translatedSubs, err = p.deepseek.TranslateSubtitles(
			subtitles,
			job.SourceLang,
			job.TargetLang,
			func(current, total int) {
				percent := 40 + (current*20)/total
				msg := fmt.Sprintf("DeepSeek: %d/%d segments", current, total)
				p.progress("Translating", percent, msg)
			},
		)

	case "openai":
		p.progress("Translating", 41, "Using OpenAI GPT-4o-mini...")
		translatedSubs, err = p.translator.TranslateWithOpenAI(
			subtitles,
			job.SourceLang,
			job.TargetLang,
			p.config.OpenAIKey,
			func(current, total int) {
				percent := 40 + (current*20)/total
				msg := fmt.Sprintf("GPT-4o-mini: %d/%d segments", current, total)
				p.progress("Translating", percent, msg)
			},
		)

	default: // "argos"
		p.progress("Translating", 41, "Using local Argos Translate...")
		translatedSubs, err = p.translator.TranslateSubtitlesWithProgress(
			subtitles,
			job.SourceLang,
			job.TargetLang,
			func(current, total int) {
				percent := 40 + (current*20)/total
				msg := fmt.Sprintf("Argos: %d/%d segments", current, total)
				p.progress("Translating", percent, msg)
			},
		)
	}

	if err != nil {
		job.Fail(err)
		return fmt.Errorf("translation failed: %w", err)
	}
	p.progress("Translating", 60, "Translation complete")

	// Stage 4: Text-to-Speech (60-85%)
	p.progress("Synthesizing", 60, "Generating speech...")
	job.SetStatus(models.StatusSynthesizing, "Generating dubbed audio", 60)

	dubbedAudioPath := filepath.Join(jobTempDir, "dubbed.wav")

	// Select TTS provider
	ttsProvider := p.getTTSProvider()
	switch ttsProvider {
	case "openai":
		p.progress("Synthesizing", 61, "Using OpenAI TTS (high quality)...")
		if p.openaiTTS != nil {
			p.openaiTTS.SetVoice(p.config.OpenAITTSVoice)
		}
		err = p.openaiTTS.SynthesizeWithCallback(translatedSubs, dubbedAudioPath, func(current, total int) {
			progress := 60 + (current*25)/total
			p.progress("Synthesizing", progress, fmt.Sprintf("OpenAI TTS: %d/%d", current, total))
		})

	case "cosyvoice":
		p.progress("Synthesizing", 61, "Using CosyVoice (voice cloning)...")
		err = p.cosyvoice.SynthesizeWithCallback(translatedSubs, dubbedAudioPath, func(current, total int) {
			progress := 60 + (current*25)/total
			p.progress("Synthesizing", progress, fmt.Sprintf("CosyVoice: %d/%d", current, total))
		})

	case "edge-tts":
		p.progress("Synthesizing", 61, "Using Edge TTS (FREE neural)...")
		if p.edgeTTS != nil {
			p.edgeTTS.SetVoice(p.config.EdgeTTSVoice)
		}
		err = p.edgeTTS.SynthesizeWithCallback(translatedSubs, dubbedAudioPath, func(current, total int) {
			progress := 60 + (current*25)/total
			p.progress("Synthesizing", progress, fmt.Sprintf("Edge TTS: %d/%d", current, total))
		})

	default: // "piper"
		p.progress("Synthesizing", 61, "Using Piper TTS...")
		p.tts.SetVoice(job.Voice)
		err = p.tts.SynthesizeWithCallback(translatedSubs, dubbedAudioPath, func(current, total int) {
			progress := 60 + (current*25)/total
			p.progress("Synthesizing", progress, fmt.Sprintf("Piper: %d/%d", current, total))
		})
	}

	if err != nil {
		job.Fail(err)
		return fmt.Errorf("speech synthesis failed: %w", err)
	}
	job.DubbedAudioPath = dubbedAudioPath
	p.progress("Synthesizing", 85, "Speech synthesis complete")

	// Stage 5: Mux Video (85-100%)
	p.progress("Muxing", 85, "Creating final video...")
	job.SetStatus(models.StatusMuxing, "Creating final video", 85)

	// Generate output path
	outputPath := p.generateOutputPath(job.InputPath)
	if err := p.ffmpeg.MuxVideoAudio(job.InputPath, dubbedAudioPath, outputPath); err != nil {
		job.Fail(err)
		return fmt.Errorf("video muxing failed: %w", err)
	}

	job.Complete(outputPath)
	p.progress("Complete", 100, "Translation complete!")

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
