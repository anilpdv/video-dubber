package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"video-translator/models"
	"video-translator/services"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type MainUI struct {
	window   fyne.Window
	fileList *FileList
	progress *ProgressPanel
	settings *SettingsPanel

	jobs     []*models.TranslationJob
	config   *models.Config
	pipeline *services.Pipeline
}

func NewMainUI(w fyne.Window) *MainUI {
	// Load config
	config, err := models.LoadConfig()
	if err != nil {
		config = models.DefaultConfig()
	}

	ui := &MainUI{
		window:   w,
		jobs:     make([]*models.TranslationJob, 0),
		config:   config,
		pipeline: services.NewPipeline(config),
	}

	ui.fileList = NewFileList(ui.onFileAdded, ui.onFileRemoved, ui.onFileSelected)
	ui.fileList.SetWindow(w)
	ui.fileList.SetJobs(ui.jobs) // Connect jobs for status display
	ui.progress = NewProgressPanel()
	ui.progress.SetOutputDirectory(config.OutputDirectory) // Show output folder
	ui.settings = NewSettingsPanel(ui.onTranslateSelected, ui.onTranslateAll)

	// Set voice preview callback
	ui.settings.SetOnPreviewVoice(func() {
		ui.previewSelectedVoice()
	})

	// Initialize voice options based on TTS provider
	ui.settings.SetVoiceOptions(config.TTSProvider)

	// Set progress callback
	ui.pipeline.SetProgressCallback(func(stage string, percent int, message string) {
		// Update UI on main thread
		ui.progress.SetProgress(stage, percent)
		ui.progress.SetStatus(message)
	})

	return ui
}

func (ui *MainUI) Build() fyne.CanvasObject {
	// Left panel: File list
	leftPanel := container.NewBorder(
		widget.NewLabelWithStyle("Video Files", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		ui.fileList.BuildButtons(),
		nil,
		nil,
		ui.fileList.Build(),
	)

	// Right panel: Progress and status
	rightPanel := container.NewVBox(
		widget.NewLabelWithStyle("Translation Progress", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		ui.progress.Build(),
		layout.NewSpacer(),
		ui.buildDependencyStatus(),
	)

	// Main content: split view
	split := container.NewHSplit(leftPanel, rightPanel)
	split.SetOffset(0.35)

	// Bottom: Settings bar
	settingsBar := ui.settings.Build()

	// Full layout
	return container.NewBorder(
		nil,
		settingsBar,
		nil,
		nil,
		split,
	)
}

func (ui *MainUI) buildDependencyStatus() fyne.CanvasObject {
	checkBtn := widget.NewButton("Check Dependencies", func() {
		ui.showDependencyCheck()
	})

	configBtn := widget.NewButton("Settings", func() {
		ui.showSettings()
	})

	// Add info label
	infoLabel := widget.NewLabel("100% Free - No API Keys Required!")
	infoLabel.TextStyle = fyne.TextStyle{Italic: true}

	return container.NewVBox(
		widget.NewSeparator(),
		infoLabel,
		container.NewHBox(checkBtn, configBtn),
	)
}

func (ui *MainUI) showDependencyCheck() {
	results := ui.pipeline.CheckDependencies()

	var status string
	allGood := true

	// Order the results for better display
	order := []string{"ffmpeg", "whisper", "whisper-model", "argos-translate", "argos-ru-en", "piper-tts", "piper-voice"}

	for _, name := range order {
		if err, ok := results[name]; ok {
			if err != nil {
				status += "❌ " + name + ": " + err.Error() + "\n"
				allGood = false
			} else {
				status += "✅ " + name + ": OK\n"
			}
		}
	}

	if allGood {
		status += "\nAll dependencies are installed! Ready to translate."
	} else {
		status += "\nPlease install missing dependencies.\nSee: https://github.com/your-repo for instructions"
	}

	dialog.ShowInformation("Dependency Check", status, ui.window)
}

func (ui *MainUI) showSettings() {
	// Output directory
	outputDirEntry := widget.NewEntry()
	outputDirEntry.SetText(ui.config.OutputDirectory)

	// Provider selection dropdowns
	transcriptionSelect := widget.NewSelect([]string{
		"whisper-cpp",
		"faster-whisper",
		"openai",
	}, nil)
	transcriptionSelect.SetSelected(ui.config.TranscriptionProvider)
	if transcriptionSelect.Selected == "" {
		transcriptionSelect.SetSelected("whisper-cpp")
	}

	translationSelect := widget.NewSelect([]string{
		"argos",
		"openai",
		"deepseek",
	}, nil)
	translationSelect.SetSelected(ui.config.TranslationProvider)
	if translationSelect.Selected == "" {
		translationSelect.SetSelected("argos")
	}

	ttsSelect := widget.NewSelect([]string{
		"edge-tts",
		"piper",
		"openai",
		"cosyvoice",
	}, nil)
	ttsSelect.SetSelected(ui.config.TTSProvider)
	if ttsSelect.Selected == "" {
		ttsSelect.SetSelected("piper")
	}

	// FasterWhisper settings
	fasterWhisperModels := []string{
		"tiny",
		"base",
		"small",
		"medium",
		"large-v2",
		"large-v3",
	}
	fasterWhisperModelSelect := widget.NewSelect(fasterWhisperModels, nil)
	fasterWhisperModelSelect.SetSelected(ui.config.FasterWhisperModel)
	if fasterWhisperModelSelect.Selected == "" {
		fasterWhisperModelSelect.SetSelected("base")
	}

	fasterWhisperDevices := []string{
		"auto",
		"cuda",
		"cpu",
	}
	fasterWhisperDeviceSelect := widget.NewSelect(fasterWhisperDevices, nil)
	fasterWhisperDeviceSelect.SetSelected(ui.config.FasterWhisperDevice)
	if fasterWhisperDeviceSelect.Selected == "" {
		fasterWhisperDeviceSelect.SetSelected("auto")
	}

	// OpenAI TTS model selection
	openaiTTSModels := []string{
		"tts-1",
		"tts-1-hd",
	}
	openaiTTSModelSelect := widget.NewSelect(openaiTTSModels, nil)
	openaiTTSModelSelect.SetSelected(ui.config.OpenAITTSModel)
	if openaiTTSModelSelect.Selected == "" {
		openaiTTSModelSelect.SetSelected("tts-1")
	}

	// CosyVoice settings
	cosyVoiceModes := []string{
		"local",
		"api",
	}
	cosyVoiceModeSelect := widget.NewSelect(cosyVoiceModes, nil)
	cosyVoiceModeSelect.SetSelected(ui.config.CosyVoiceMode)
	if cosyVoiceModeSelect.Selected == "" {
		cosyVoiceModeSelect.SetSelected("local")
	}

	cosyVoiceAPIURLEntry := widget.NewEntry()
	cosyVoiceAPIURLEntry.SetPlaceHolder("http://localhost:8000")
	cosyVoiceAPIURLEntry.SetText(ui.config.CosyVoiceAPIURL)

	voiceSamplePathEntry := widget.NewEntry()
	voiceSamplePathEntry.SetPlaceHolder("Path to voice sample (5-10s audio)")
	voiceSamplePathEntry.SetText(ui.config.VoiceCloneSamplePath)

	voiceSampleBrowseBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			voiceSamplePathEntry.SetText(reader.URI().Path())
			reader.Close()
		}, ui.window)
	})

	voiceSampleRow := container.NewBorder(nil, nil, nil, voiceSampleBrowseBtn, voiceSamplePathEntry)

	// Conditional UI containers
	fasterWhisperSettings := container.NewVBox(
		widget.NewLabel("FasterWhisper Model:"),
		fasterWhisperModelSelect,
		widget.NewLabel("FasterWhisper Device:"),
		fasterWhisperDeviceSelect,
	)
	fasterWhisperSettings.Hide()

	openaiTTSSettings := container.NewVBox(
		widget.NewLabel("OpenAI TTS Model:"),
		openaiTTSModelSelect,
	)
	openaiTTSSettings.Hide()

	cosyVoiceSettings := container.NewVBox(
		widget.NewLabel("CosyVoice Mode:"),
		cosyVoiceModeSelect,
		widget.NewLabel("CosyVoice API URL (for API mode):"),
		cosyVoiceAPIURLEntry,
		widget.NewLabel("Voice Sample (for cloning):"),
		voiceSampleRow,
	)
	cosyVoiceSettings.Hide()

	// Show/hide settings based on current provider
	if transcriptionSelect.Selected == "faster-whisper" {
		fasterWhisperSettings.Show()
	}
	if ttsSelect.Selected == "openai" {
		openaiTTSSettings.Show()
	}
	if ttsSelect.Selected == "cosyvoice" {
		cosyVoiceSettings.Show()
	}

	// Update main bar voice options and show/hide settings when TTS provider changes
	ttsSelect.OnChanged = func(value string) {
		openaiTTSSettings.Hide()
		cosyVoiceSettings.Hide()

		// Update the main bar voice dropdown
		ui.settings.SetVoiceOptions(value)

		if value == "openai" {
			openaiTTSSettings.Show()
		} else if value == "cosyvoice" {
			cosyVoiceSettings.Show()
		}
	}

	// Show/hide FasterWhisper settings based on transcription provider
	transcriptionSelect.OnChanged = func(value string) {
		if value == "faster-whisper" {
			fasterWhisperSettings.Show()
		} else {
			fasterWhisperSettings.Hide()
		}
	}

	// API Keys
	openAIKeyEntry := widget.NewPasswordEntry()
	openAIKeyEntry.SetPlaceHolder("sk-...")
	openAIKeyEntry.SetText(ui.config.OpenAIKey)

	deepSeekKeyEntry := widget.NewPasswordEntry()
	deepSeekKeyEntry.SetPlaceHolder("sk-...")
	deepSeekKeyEntry.SetText(ui.config.DeepSeekKey)

	// Cost estimate
	costInfo := widget.NewLabel("Cost varies by provider selection")
	costInfo.TextStyle = fyne.TextStyle{Italic: true}

	// Build form (voice is in main bar, not here)
	form := widget.NewForm(
		widget.NewFormItem("Output Directory", outputDirEntry),
		widget.NewFormItem("", widget.NewSeparator()),
		widget.NewFormItem("Transcription", transcriptionSelect),
		widget.NewFormItem("Translation", translationSelect),
		widget.NewFormItem("TTS", ttsSelect),
		widget.NewFormItem("", widget.NewSeparator()),
		widget.NewFormItem("OpenAI API Key", openAIKeyEntry),
		widget.NewFormItem("DeepSeek API Key", deepSeekKeyEntry),
		widget.NewFormItem("", costInfo),
	)

	// Provider info
	providerInfo := widget.NewLabel(ui.getProviderCostInfo())
	providerInfo.TextStyle = fyne.TextStyle{Bold: true}

	// Build content with conditional settings
	content := container.NewVBox(
		form,
		fasterWhisperSettings,
		openaiTTSSettings,
		cosyVoiceSettings,
		widget.NewSeparator(),
		providerInfo,
	)

	// Make dialog scrollable for smaller screens
	scrollContent := container.NewVScroll(content)
	scrollContent.SetMinSize(fyne.NewSize(450, 500))

	dialog.ShowCustomConfirm("Settings", "Save", "Cancel", scrollContent, func(save bool) {
		if save {
			// Save basic settings
			ui.config.OutputDirectory = outputDirEntry.Text

			// Save provider selections
			ui.config.TranscriptionProvider = transcriptionSelect.Selected
			ui.config.TranslationProvider = translationSelect.Selected
			ui.config.TTSProvider = ttsSelect.Selected

			// Save FasterWhisper settings
			ui.config.FasterWhisperModel = fasterWhisperModelSelect.Selected
			ui.config.FasterWhisperDevice = fasterWhisperDeviceSelect.Selected

			// Save OpenAI TTS settings
			ui.config.OpenAITTSModel = openaiTTSModelSelect.Selected

			// Save CosyVoice settings
			ui.config.CosyVoiceMode = cosyVoiceModeSelect.Selected
			ui.config.CosyVoiceAPIURL = cosyVoiceAPIURLEntry.Text
			ui.config.VoiceCloneSamplePath = voiceSamplePathEntry.Text

			// Save voice from main bar based on TTS provider
			selectedVoice := ui.settings.GetVoice()
			if ttsSelect.Selected == "openai" {
				ui.config.OpenAITTSVoice = selectedVoice
			} else if ttsSelect.Selected == "edge-tts" {
				ui.config.EdgeTTSVoice = selectedVoice
			} else if ttsSelect.Selected != "cosyvoice" {
				ui.config.DefaultVoice = selectedVoice
			}

			// Save API keys
			ui.config.OpenAIKey = openAIKeyEntry.Text
			ui.config.DeepSeekKey = deepSeekKeyEntry.Text

			// Update legacy flag for backward compatibility
			ui.config.UseOpenAIAPIs = (transcriptionSelect.Selected == "openai" || translationSelect.Selected == "openai")

			if err := ui.config.Save(); err != nil {
				dialog.ShowError(err, ui.window)
				return
			}

			// Recreate pipeline with new config
			ui.pipeline = services.NewPipeline(ui.config)
			ui.pipeline.SetProgressCallback(func(stage string, percent int, message string) {
				ui.progress.SetProgress(stage, percent)
				ui.progress.SetStatus(message)
			})

			// Update the output directory display
			ui.progress.SetOutputDirectory(ui.config.OutputDirectory)

			// Show confirmation
			dialog.ShowInformation("Settings", "Settings saved!\n\n"+ui.getProviderCostInfo(), ui.window)
		}
	}, ui.window)
}

// getProviderCostInfo returns cost estimate based on current provider selection
// Pricing (as of 2024):
// - OpenAI Whisper: $0.006/minute
// - DeepSeek: $0.28/1M input + $0.42/1M output tokens
// - OpenAI gpt-4o-mini: $0.15/1M input + $0.60/1M output tokens
// - OpenAI TTS: $15/1M chars (tts-1), $30/1M chars (tts-1-hd)
// Assumptions for 5hr video: 300 min, ~60K tokens, ~225K characters
func (ui *MainUI) getProviderCostInfo() string {
	transcription := ui.config.TranscriptionProvider
	if transcription == "" {
		transcription = "whisper-cpp"
	}
	translation := ui.config.TranslationProvider
	if translation == "" {
		translation = "argos"
	}
	tts := ui.config.TTSProvider
	if tts == "" {
		tts = "piper"
	}

	var transcriptCostNum, translateCostNum, ttsCostNum float64
	var transcriptCost, translateCost, ttsCost string

	// Transcription costs
	switch transcription {
	case "openai":
		transcriptCostNum = 1.80 // $0.006/min × 300 min
		transcriptCost = "$1.80"
	case "faster-whisper":
		transcriptCostNum = 0
		transcriptCost = "Free (local GPU)"
	default:
		transcriptCostNum = 0
		transcriptCost = "Free (local)"
	}

	// Translation costs (based on ~60K tokens input/output)
	switch translation {
	case "deepseek":
		// $0.28/1M input + $0.42/1M output = ~$0.04 for 60K tokens
		translateCostNum = 0.04
		translateCost = "~$0.04"
	case "openai":
		// $0.15/1M input + $0.60/1M output = ~$0.05 for 60K tokens
		translateCostNum = 0.05
		translateCost = "~$0.05"
	default:
		translateCostNum = 0
		translateCost = "Free (local)"
	}

	// TTS costs (based on ~225K characters)
	switch tts {
	case "openai":
		ttsModel := ui.config.OpenAITTSModel
		if ttsModel == "tts-1-hd" {
			// $30/1M chars × 0.225M = ~$6.75
			ttsCostNum = 6.75
			ttsCost = "~$6.75 (HD)"
		} else {
			// $15/1M chars × 0.225M = ~$3.40
			ttsCostNum = 3.40
			ttsCost = "~$3.40"
		}
	case "edge-tts":
		ttsCostNum = 0
		ttsCost = "Free (neural)"
	case "cosyvoice":
		ttsCostNum = 0
		ttsCost = "Free (local)"
	default:
		ttsCostNum = 0
		ttsCost = "Free (local)"
	}

	total := transcriptCostNum + translateCostNum + ttsCostNum

	return fmt.Sprintf("Est. cost per 5hr video:\n"+
		"Transcription: %s\n"+
		"Translation: %s\n"+
		"TTS: %s\n"+
		"─────────────\n"+
		"Total: $%.2f", transcriptCost, translateCost, ttsCost, total)
}

func (ui *MainUI) onFileAdded(path string) {
	job := models.NewTranslationJob(path)
	job.SourceLang = ui.settings.GetSourceLang()
	job.TargetLang = ui.settings.GetTargetLang()
	job.Voice = ui.settings.GetVoice()
	ui.jobs = append(ui.jobs, job)
	ui.fileList.SetJobs(ui.jobs) // Update reference after append
}

func (ui *MainUI) onFileRemoved(index int) {
	if index >= 0 && index < len(ui.jobs) {
		ui.jobs = append(ui.jobs[:index], ui.jobs[index+1:]...)
		ui.fileList.SetJobs(ui.jobs) // Update reference after remove
	}
}

func (ui *MainUI) onFileSelected(index int) {
	if index >= 0 && index < len(ui.jobs) {
		job := ui.jobs[index]
		ui.progress.SetCurrentJob(job)
	}
}

func (ui *MainUI) onTranslateSelected() {
	selected := ui.fileList.GetSelectedIndex()
	if selected < 0 || selected >= len(ui.jobs) {
		dialog.ShowInformation("No Selection", "Please select a video file to translate.", ui.window)
		return
	}

	job := ui.jobs[selected]
	if job.Status != models.StatusPending {
		dialog.ShowInformation("Already Processing", "This file is already being processed or completed.", ui.window)
		return
	}

	ui.translateJob(job)
}

func (ui *MainUI) onTranslateAll() {
	// Collect pending jobs first
	var pendingJobs []*models.TranslationJob
	for _, job := range ui.jobs {
		if job.Status == models.StatusPending {
			pendingJobs = append(pendingJobs, job)
		}
	}

	if len(pendingJobs) == 0 {
		dialog.ShowInformation("No Files", "No pending files to translate.", ui.window)
		return
	}

	// Process jobs SEQUENTIALLY in background (one at a time to avoid CPU overload)
	go func() {
		for i, job := range pendingJobs {
			// Update UI to show which job is processing
			fyne.Do(func() {
				ui.progress.SetStatus(fmt.Sprintf("Processing %d of %d videos...", i+1, len(pendingJobs)))
			})

			ui.translateJobSync(job)
		}

		// All jobs done
		fyne.Do(func() {
			ui.progress.SetStatus("")
			dialog.ShowInformation("Complete", fmt.Sprintf("All %d videos translated!", len(pendingJobs)), ui.window)
		})
	}()
}

func (ui *MainUI) translateJob(job *models.TranslationJob) {
	// Update job settings from current UI state
	job.SourceLang = ui.settings.GetSourceLang()
	job.TargetLang = ui.settings.GetTargetLang()
	job.Voice = ui.settings.GetVoice()

	// Validate before starting
	if err := ui.pipeline.ValidateJob(job); err != nil {
		dialog.ShowError(err, ui.window)
		return
	}

	job.Status = models.StatusProcessing
	ui.fileList.Refresh()
	ui.progress.SetCurrentJob(job)

	// Run pipeline in background
	go func() {
		err := ui.pipeline.Process(job)

		// Update UI on completion (must use fyne.Do for goroutine)
		fyne.Do(func() {
			ui.fileList.Refresh()
			ui.progress.Update()

			if err != nil {
				dialog.ShowError(err, ui.window)
			} else {
				// Show completion dialog with "Open Folder" button
				dialog.ShowCustomConfirm("Complete", "Open Folder", "Close",
					widget.NewLabel("Translation complete!\n\nOutput saved to:\n"+job.OutputPath),
					func(openFolder bool) {
						if openFolder {
							// Open the output folder in Finder
							exec.Command("open", filepath.Dir(job.OutputPath)).Start()
						}
					}, ui.window)
			}
		})
	}()
}

// translateJobSync processes a job synchronously (for sequential batch processing)
func (ui *MainUI) translateJobSync(job *models.TranslationJob) {
	// Update job settings from current UI state
	fyne.Do(func() {
		job.SourceLang = ui.settings.GetSourceLang()
		job.TargetLang = ui.settings.GetTargetLang()
		job.Voice = ui.settings.GetVoice()
	})

	// Validate before starting
	if err := ui.pipeline.ValidateJob(job); err != nil {
		fyne.Do(func() {
			dialog.ShowError(err, ui.window)
		})
		return
	}

	fyne.Do(func() {
		job.Status = models.StatusProcessing
		ui.fileList.Refresh()
		ui.progress.SetCurrentJob(job)
	})

	// Process synchronously (wait for completion)
	err := ui.pipeline.Process(job)

	// Update UI on completion
	fyne.Do(func() {
		ui.fileList.Refresh()
		ui.progress.Update()

		if err != nil {
			dialog.ShowError(err, ui.window)
		}
	})
}

// GetWindow returns the main window for dialogs
func (ui *MainUI) GetWindow() fyne.Window {
	return ui.window
}

// Spacer helper
func spacer() fyne.CanvasObject {
	return layout.NewSpacer()
}

// previewSelectedVoice generates and plays a sample audio using the selected voice
func (ui *MainUI) previewSelectedVoice() {
	voice := ui.settings.GetVoice()
	provider := ui.settings.GetTTSProvider() // Use current selection, not saved config
	sampleText := "This is a preview of the selected voice."

	// Log what's being used
	services.LogInfo("Preview: provider=%s voice=%s", provider, voice)

	// Show loading status with provider and voice name
	ui.progress.SetStatus(fmt.Sprintf("Generating: %s (%s)", voice, provider))

	go func() {
		// Use app's cache directory for reliable temp file
		homeDir, _ := os.UserHomeDir()
		tempDir := filepath.Join(homeDir, ".cache", "video-translator")
		os.MkdirAll(tempDir, 0755)
		tempPath := filepath.Join(tempDir, "voice_preview.wav")

		var err error

		switch provider {
		case "piper":
			svc := services.NewTTSService(voice)
			err = svc.Synthesize(sampleText, tempPath)
		case "openai":
			svc := services.NewOpenAITTSService(ui.config.OpenAIKey, ui.config.OpenAITTSModel, voice, ui.config.OpenAITTSSpeed)
			err = svc.Synthesize(sampleText, tempPath)
		case "edge-tts":
			svc := services.NewEdgeTTSService(voice)
			err = svc.Synthesize(sampleText, tempPath)
		case "cosyvoice":
			if ui.config.VoiceCloneSamplePath == "" {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("CosyVoice requires a voice sample. Configure it in Settings"), ui.window)
					ui.progress.SetStatus("")
				})
				return
			}
			svc := services.NewCosyVoiceService(
				ui.config.CosyVoicePath,
				ui.config.CosyVoiceMode,
				ui.config.CosyVoiceAPIURL,
				ui.config.VoiceCloneSamplePath,
				ui.config.PythonPath,
			)
			err = svc.Synthesize(sampleText, tempPath)
		default:
			err = fmt.Errorf("unknown TTS provider: %s", provider)
		}

		// Check for TTS errors
		if err != nil {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("TTS failed: %w", err), ui.window)
				ui.progress.SetStatus("")
			})
			return
		}

		// Verify file was created
		if _, statErr := os.Stat(tempPath); os.IsNotExist(statErr) {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("audio file was not created"), ui.window)
				ui.progress.SetStatus("")
			})
			return
		}

		// Update status and play audio
		fyne.Do(func() {
			ui.progress.SetStatus(fmt.Sprintf("Playing: %s (%s)", voice, provider))
		})

		// Play audio synchronously (wait for it to finish)
		playErr := playAudio(tempPath)

		fyne.Do(func() {
			if playErr != nil {
				dialog.ShowError(fmt.Errorf("playback failed: %w", playErr), ui.window)
			}
			ui.progress.SetStatus("")
		})
	}()
}

// playAudio plays an audio file and waits for it to complete
func playAudio(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("afplay", path)
	case "linux":
		cmd = exec.Command("paplay", path)
	case "windows":
		cmd = exec.Command("powershell", "-c", fmt.Sprintf("(New-Object Media.SoundPlayer '%s').PlaySync()", path))
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	// Run and wait for audio to finish playing
	return cmd.Run()
}
