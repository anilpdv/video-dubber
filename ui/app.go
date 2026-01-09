package ui

import (
	"fmt"
	"os/exec"
	"path/filepath"
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

	// Piper voices
	piperVoices := []string{
		"en_US-amy-medium",
		"en_US-ryan-medium",
		"en_GB-alba-medium",
		"de_DE-thorsten-medium",
		"fr_FR-upmc-medium",
	}

	// OpenAI TTS voices
	openaiVoices := []string{
		"alloy",
		"echo",
		"fable",
		"onyx",
		"nova",
		"shimmer",
	}

	// Edge TTS voices (FREE neural TTS from Microsoft)
	edgeTTSVoices := []string{
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

	// Voice selection (changes based on TTS provider)
	voiceSelect := widget.NewSelect(piperVoices, nil)
	if ui.config.TTSProvider == "openai" {
		voiceSelect.Options = openaiVoices
		voiceSelect.SetSelected(ui.config.OpenAITTSVoice)
	} else if ui.config.TTSProvider == "edge-tts" {
		voiceSelect.Options = edgeTTSVoices
		voiceSelect.SetSelected(ui.config.EdgeTTSVoice)
	} else {
		voiceSelect.SetSelected(ui.config.DefaultVoice)
	}

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

	// Update voice options and show/hide settings when TTS provider changes
	ttsSelect.OnChanged = func(value string) {
		openaiTTSSettings.Hide()
		cosyVoiceSettings.Hide()

		if value == "openai" {
			voiceSelect.Options = openaiVoices
			voiceSelect.SetSelected("nova")
			openaiTTSSettings.Show()
		} else if value == "edge-tts" {
			voiceSelect.Options = edgeTTSVoices
			voiceSelect.SetSelected("en-US-AriaNeural")
		} else if value == "cosyvoice" {
			voiceSelect.Options = []string{"(uses voice sample)"}
			voiceSelect.SetSelected("(uses voice sample)")
			cosyVoiceSettings.Show()
		} else {
			voiceSelect.Options = piperVoices
			voiceSelect.SetSelected("en_US-amy-medium")
		}
		voiceSelect.Refresh()
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

	// Build form
	form := widget.NewForm(
		widget.NewFormItem("Output Directory", outputDirEntry),
		widget.NewFormItem("", widget.NewSeparator()),
		widget.NewFormItem("Transcription", transcriptionSelect),
		widget.NewFormItem("Translation", translationSelect),
		widget.NewFormItem("TTS", ttsSelect),
		widget.NewFormItem("Voice", voiceSelect),
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

			// Save voice based on TTS provider
			if ttsSelect.Selected == "openai" {
				ui.config.OpenAITTSVoice = voiceSelect.Selected
			} else if ttsSelect.Selected == "edge-tts" {
				ui.config.EdgeTTSVoice = voiceSelect.Selected
			} else if ttsSelect.Selected != "cosyvoice" {
				ui.config.DefaultVoice = voiceSelect.Selected
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
	pendingJobs := 0
	for _, job := range ui.jobs {
		if job.Status == models.StatusPending {
			pendingJobs++
		}
	}

	if pendingJobs == 0 {
		dialog.ShowInformation("No Files", "No pending files to translate.", ui.window)
		return
	}

	for _, job := range ui.jobs {
		if job.Status == models.StatusPending {
			ui.translateJob(job)
		}
	}
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

// GetWindow returns the main window for dialogs
func (ui *MainUI) GetWindow() fyne.Window {
	return ui.window
}

// Spacer helper
func spacer() fyne.CanvasObject {
	return layout.NewSpacer()
}
