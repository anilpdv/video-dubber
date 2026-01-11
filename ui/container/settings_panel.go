package container

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"video-translator/models"
	"video-translator/services"
	"video-translator/ui/widgets"
)

// SettingsPanel displays settings as an inline page
type SettingsPanel struct {
	widget.BaseWidget

	window fyne.Window
	config *models.Config

	// UI elements
	outputDirEntry       *widget.Entry
	transcriptionSelect  *widget.Select
	translationSelect    *widget.Select
	ttsSelect            *widget.Select
	openaiTTSModelSelect *widget.Select
	cosyVoiceModeSelect       *widget.Select
	cosyVoiceAPIURLEntry      *widget.Entry
	voiceSamplePathEntry      *widget.Entry
	openAIKeyEntry            *widget.Entry
	deepSeekKeyEntry          *widget.Entry
	groqAPIKeyEntry           *widget.Entry

	// Audio mixing controls
	keepBackgroundAudioCheck *widget.Check
	backgroundVolumeSlider   *widget.Slider
	backgroundVolumeLabel    *widget.Label

	// Conditional containers
	whisperKitSettings    *fyne.Container
	whisperKitModelSelect *widget.Select
	whisperKitModelStatus *widget.Label
	whisperKitDownloadBtn *widget.Button
	openaiTTSSettings     *fyne.Container
	cosyVoiceSettings     *fyne.Container

	OnSave       func(config *models.Config)
	OnTTSChanged func(provider string)
}

// NewSettingsPanel creates a new settings panel
func NewSettingsPanel(window fyne.Window, config *models.Config) *SettingsPanel {
	p := &SettingsPanel{
		window: window,
		config: config,
	}
	p.ExtendBaseWidget(p)
	return p
}

// SetConfig updates the config and refreshes the panel
func (p *SettingsPanel) SetConfig(config *models.Config) {
	p.config = config
	p.Refresh()
}

// Build creates the panel UI
func (p *SettingsPanel) Build() fyne.CanvasObject {
	header := widgets.NewSectionHeader("Settings")

	// Output directory
	p.outputDirEntry = widget.NewEntry()
	p.outputDirEntry.SetText(p.config.OutputDirectory)

	browseBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			p.outputDirEntry.SetText(uri.Path())
		}, p.window)
	})

	outputRow := container.NewBorder(nil, nil, nil, browseBtn, p.outputDirEntry)

	// Provider selections with min height
	p.transcriptionSelect = widget.NewSelect([]string{
		"whisperkit",  // Native macOS (recommended for Apple Silicon)
		"whisper-cpp", // Cross-platform fallback
		"openai",
		"groq",
	}, func(value string) {
		p.updateConditionalUI()
	})
	p.transcriptionSelect.SetSelected(getOrDefault(p.config.TranscriptionProvider, "whisperkit"))

	p.translationSelect = widget.NewSelect([]string{
		"argos",
		"openai",
		"deepseek",
	}, nil)
	p.translationSelect.SetSelected(getOrDefault(p.config.TranslationProvider, "argos"))

	p.ttsSelect = widget.NewSelect([]string{
		"edge-tts",
		"piper",
		"openai",
		"cosyvoice",
	}, func(value string) {
		p.updateConditionalUI()
		if p.OnTTSChanged != nil {
			p.OnTTSChanged(value)
		}
	})
	p.ttsSelect.SetSelected(getOrDefault(p.config.TTSProvider, "edge-tts"))

	// Helper to wrap widget with minimum height
	withMinHeight := func(w fyne.CanvasObject, height float32) *fyne.Container {
		spacer := canvas.NewRectangle(color.Transparent)
		spacer.SetMinSize(fyne.NewSize(0, height))
		return container.NewStack(spacer, w)
	}

	// WhisperKit settings (shown when whisperkit is selected)
	// Model sizes for info display
	modelSizes := map[string]string{
		"tiny":     "~75MB - Fastest",
		"base":     "~150MB - Balanced",
		"small":    "~500MB - Better quality",
		"medium":   "~1.5GB - High quality",
		"large-v2": "~3GB - Best quality",
		"large-v3": "~3GB - Latest",
	}

	modelInfoLabel := widget.NewLabel("")
	modelInfoLabel.TextStyle = fyne.TextStyle{Italic: true}

	// Create status label and download button BEFORE select widget
	// (select's SetSelected triggers callback that needs these)
	p.whisperKitModelStatus = widget.NewLabel("Checking...")
	p.whisperKitDownloadBtn = widget.NewButtonWithIcon("Download Model", theme.DownloadIcon(), func() {
		p.downloadWhisperKitModel()
	})

	p.whisperKitModelSelect = widget.NewSelect([]string{
		"tiny",
		"base",
		"small",
		"medium",
		"large-v2",
		"large-v3",
	}, func(value string) {
		// Update model info when selection changes
		if size, ok := modelSizes[value]; ok {
			modelInfoLabel.SetText(size)
		}
		// Recheck if selected model is downloaded
		p.checkWhisperKitModel()
	})
	selectedModel := getOrDefault(p.config.WhisperKitModel, "base")
	p.whisperKitModelSelect.SetSelected(selectedModel)
	if size, ok := modelSizes[selectedModel]; ok {
		modelInfoLabel.SetText(size)
	}

	whisperKitForm := widget.NewForm(
		widget.NewFormItem("Model", p.whisperKitModelSelect),
	)

	p.whisperKitSettings = container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabel("WhisperKit Settings"),
		container.NewPadded(whisperKitForm),
		container.NewPadded(modelInfoLabel),
		container.NewHBox(
			p.whisperKitModelStatus,
			p.whisperKitDownloadBtn,
		),
	)

	// OpenAI TTS settings
	p.openaiTTSModelSelect = widget.NewSelect([]string{
		"tts-1", "tts-1-hd",
	}, nil)
	p.openaiTTSModelSelect.SetSelected(getOrDefault(p.config.OpenAITTSModel, "tts-1"))

	openaiTTSForm := widget.NewForm(
		widget.NewFormItem("Model", withMinHeight(p.openaiTTSModelSelect, 40)),
	)
	p.openaiTTSSettings = container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabel("OpenAI TTS Settings"),
		container.NewPadded(openaiTTSForm),
	)

	// CosyVoice settings
	p.cosyVoiceModeSelect = widget.NewSelect([]string{
		"local", "api",
	}, nil)
	p.cosyVoiceModeSelect.SetSelected(getOrDefault(p.config.CosyVoiceMode, "local"))

	p.cosyVoiceAPIURLEntry = widget.NewEntry()
	p.cosyVoiceAPIURLEntry.SetPlaceHolder("http://localhost:8000")
	p.cosyVoiceAPIURLEntry.SetText(p.config.CosyVoiceAPIURL)

	p.voiceSamplePathEntry = widget.NewEntry()
	p.voiceSamplePathEntry.SetPlaceHolder("Path to voice sample (5-10s audio)")
	p.voiceSamplePathEntry.SetText(p.config.VoiceCloneSamplePath)

	voiceSampleBrowseBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			p.voiceSamplePathEntry.SetText(reader.URI().Path())
			reader.Close()
		}, p.window)
	})

	voiceSampleRow := container.NewBorder(nil, nil, nil, voiceSampleBrowseBtn, p.voiceSamplePathEntry)

	cosyVoiceForm := widget.NewForm(
		widget.NewFormItem("Mode", withMinHeight(p.cosyVoiceModeSelect, 40)),
		widget.NewFormItem("API URL", p.cosyVoiceAPIURLEntry),
		widget.NewFormItem("Voice Sample", voiceSampleRow),
	)
	p.cosyVoiceSettings = container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabel("CosyVoice Settings"),
		container.NewPadded(cosyVoiceForm),
	)

	// API Keys
	p.openAIKeyEntry = widget.NewPasswordEntry()
	p.openAIKeyEntry.SetPlaceHolder("sk-...")
	p.openAIKeyEntry.SetText(p.config.OpenAIKey)

	p.deepSeekKeyEntry = widget.NewPasswordEntry()
	p.deepSeekKeyEntry.SetPlaceHolder("sk-...")
	p.deepSeekKeyEntry.SetText(p.config.DeepSeekKey)

	p.groqAPIKeyEntry = widget.NewPasswordEntry()
	p.groqAPIKeyEntry.SetPlaceHolder("gsk_...")
	p.groqAPIKeyEntry.SetText(p.config.GroqAPIKey)

	// Audio mixing controls
	p.keepBackgroundAudioCheck = widget.NewCheck("Keep background audio/music", nil)
	p.keepBackgroundAudioCheck.SetChecked(p.config.KeepBackgroundAudio)

	p.backgroundVolumeLabel = widget.NewLabel(fmt.Sprintf("Background volume: %.0f%%", p.config.BackgroundAudioVolume*100))
	p.backgroundVolumeSlider = widget.NewSlider(0, 100)
	p.backgroundVolumeSlider.Value = p.config.BackgroundAudioVolume * 100
	p.backgroundVolumeSlider.OnChanged = func(value float64) {
		p.backgroundVolumeLabel.SetText(fmt.Sprintf("Background volume: %.0f%%", value))
	}

	// Cost info
	costInfo := widget.NewLabel(p.getCostEstimate())
	costInfo.TextStyle = fyne.TextStyle{Italic: true}
	costInfo.Wrapping = fyne.TextWrapWord

	// Forms - using widget.NewForm for proper label-input alignment
	mainForm := widget.NewForm(
		widget.NewFormItem("Output Directory", outputRow),
	)

	selectHeight := float32(40)
	providersForm := widget.NewForm(
		widget.NewFormItem("Transcription", withMinHeight(p.transcriptionSelect, selectHeight)),
		widget.NewFormItem("Translation", withMinHeight(p.translationSelect, selectHeight)),
		widget.NewFormItem("TTS", withMinHeight(p.ttsSelect, selectHeight)),
	)

	apiKeysForm := widget.NewForm(
		widget.NewFormItem("OpenAI API Key", p.openAIKeyEntry),
		widget.NewFormItem("DeepSeek API Key", p.deepSeekKeyEntry),
		widget.NewFormItem("Groq API Key", p.groqAPIKeyEntry),
	)

	audioMixingForm := container.NewVBox(
		p.keepBackgroundAudioCheck,
		p.backgroundVolumeLabel,
		p.backgroundVolumeSlider,
	)

	// Save button
	saveBtn := widget.NewButtonWithIcon("Save Settings", theme.DocumentSaveIcon(), func() {
		p.saveSettings()
	})
	saveBtn.Importance = widget.HighImportance

	// Initialize conditional visibility
	p.updateConditionalUI()

	// Content with indented sections
	content := container.NewVBox(
		widget.NewLabel("General"),
		container.NewPadded(mainForm),
		widget.NewSeparator(),
		widget.NewLabel("Providers"),
		container.NewPadded(providersForm),
		p.whisperKitSettings,
		p.openaiTTSSettings,
		p.cosyVoiceSettings,
		widget.NewSeparator(),
		widget.NewLabel("API Keys"),
		container.NewPadded(apiKeysForm),
		widget.NewSeparator(),
		widget.NewLabel("Audio Mixing"),
		container.NewPadded(audioMixingForm),
		widget.NewSeparator(),
		widget.NewLabel("Cost Estimate (per 5hr video)"),
		container.NewPadded(costInfo),
		widget.NewSeparator(),
		container.NewHBox(saveBtn),
	)

	scrollable := container.NewVScroll(content)

	return container.NewBorder(
		container.NewPadded(header),
		nil,
		nil,
		nil,
		container.NewPadded(scrollable),
	)
}

func (p *SettingsPanel) updateConditionalUI() {
	if p.openaiTTSSettings == nil || p.cosyVoiceSettings == nil || p.whisperKitSettings == nil {
		return
	}

	// WhisperKit settings
	if p.transcriptionSelect.Selected == "whisperkit" {
		p.whisperKitSettings.Show()
		p.checkWhisperKitModel()
	} else {
		p.whisperKitSettings.Hide()
	}

	// OpenAI TTS settings
	if p.ttsSelect.Selected == "openai" {
		p.openaiTTSSettings.Show()
	} else {
		p.openaiTTSSettings.Hide()
	}

	// CosyVoice settings
	if p.ttsSelect.Selected == "cosyvoice" {
		p.cosyVoiceSettings.Show()
	} else {
		p.cosyVoiceSettings.Hide()
	}
}

// checkWhisperKitModel checks if the WhisperKit model is downloaded
func (p *SettingsPanel) checkWhisperKitModel() {
	if p.whisperKitModelSelect == nil {
		return
	}

	model := p.whisperKitModelSelect.Selected
	if model == "" {
		model = "base"
	}
	service := services.NewWhisperKitService(model)

	if service.IsModelDownloaded() {
		p.whisperKitModelStatus.SetText(fmt.Sprintf("%s model ready", model))
		p.whisperKitDownloadBtn.Hide()
	} else {
		p.whisperKitModelStatus.SetText(fmt.Sprintf("%s model not downloaded", model))
		p.whisperKitDownloadBtn.Show()
	}
}

// downloadWhisperKitModel downloads the model with progress dialog
func (p *SettingsPanel) downloadWhisperKitModel() {
	model := p.whisperKitModelSelect.Selected
	if model == "" {
		model = "base"
	}

	progress := widget.NewProgressBarInfinite()
	statusLabel := widget.NewLabel(fmt.Sprintf("Downloading WhisperKit %s model...", model))
	statusLabel.Wrapping = fyne.TextWrapWord

	content := container.NewVBox(
		widget.NewLabel("This may take a few minutes depending on your internet connection."),
		widget.NewSeparator(),
		statusLabel,
		progress,
	)

	d := dialog.NewCustomWithoutButtons(fmt.Sprintf("Downloading WhisperKit %s Model", model), content, p.window)
	d.Resize(fyne.NewSize(400, 150))
	d.Show()

	go func() {
		service := services.NewWhisperKitService(model)
		err := service.DownloadModel(func(line string) {
			// Update status label on main thread
			fyne.Do(func() {
				statusLabel.SetText(line)
			})
		})

		// All UI updates must be on main thread
		fyne.Do(func() {
			d.Hide()

			if err != nil {
				dialog.ShowError(err, p.window)
			} else {
				p.checkWhisperKitModel()
				dialog.ShowInformation("Download Complete", fmt.Sprintf("WhisperKit %s model is ready to use!", model), p.window)
			}
		})
	}()
}

func (p *SettingsPanel) saveSettings() {
	p.config.OutputDirectory = p.outputDirEntry.Text
	p.config.TranscriptionProvider = p.transcriptionSelect.Selected
	p.config.TranslationProvider = p.translationSelect.Selected
	p.config.TTSProvider = p.ttsSelect.Selected

	// WhisperKit model
	p.config.WhisperKitModel = p.whisperKitModelSelect.Selected

	p.config.OpenAITTSModel = p.openaiTTSModelSelect.Selected

	p.config.CosyVoiceMode = p.cosyVoiceModeSelect.Selected
	p.config.CosyVoiceAPIURL = p.cosyVoiceAPIURLEntry.Text
	p.config.VoiceCloneSamplePath = p.voiceSamplePathEntry.Text

	p.config.OpenAIKey = p.openAIKeyEntry.Text
	p.config.DeepSeekKey = p.deepSeekKeyEntry.Text
	p.config.GroqAPIKey = p.groqAPIKeyEntry.Text

	p.config.KeepBackgroundAudio = p.keepBackgroundAudioCheck.Checked
	p.config.BackgroundAudioVolume = p.backgroundVolumeSlider.Value / 100.0

	p.config.UseOpenAIAPIs = (p.config.TranscriptionProvider == "openai" || p.config.TranslationProvider == "openai")

	if err := p.config.Save(); err != nil {
		dialog.ShowError(err, p.window)
		return
	}

	dialog.ShowInformation("Settings", "Settings saved successfully!", p.window)

	if p.OnSave != nil {
		p.OnSave(p.config)
	}
}

func (p *SettingsPanel) getCostEstimate() string {
	transcription := getOrDefault(p.config.TranscriptionProvider, "whisper-cpp")
	translation := getOrDefault(p.config.TranslationProvider, "argos")
	tts := getOrDefault(p.config.TTSProvider, "edge-tts")

	var transcriptCostNum, translateCostNum, ttsCostNum float64
	var transcriptCost, translateCost, ttsCost string

	switch transcription {
	case "openai":
		transcriptCostNum = 1.80
		transcriptCost = "$1.80"
	case "groq":
		transcriptCostNum = 0.15
		transcriptCost = "~$0.15 (ultra-fast)"
	case "whisperkit":
		transcriptCostNum = 0
		transcriptCost = "Free (Apple Silicon)"
	default:
		transcriptCostNum = 0
		transcriptCost = "Free (local)"
	}

	switch translation {
	case "deepseek":
		translateCostNum = 0.04
		translateCost = "~$0.04"
	case "openai":
		translateCostNum = 0.05
		translateCost = "~$0.05"
	default:
		translateCostNum = 0
		translateCost = "Free (local)"
	}

	switch tts {
	case "openai":
		ttsModel := getOrDefault(p.config.OpenAITTSModel, "tts-1")
		if ttsModel == "tts-1-hd" {
			ttsCostNum = 6.75
			ttsCost = "~$6.75 (HD)"
		} else {
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

	return fmt.Sprintf("Transcription: %s\nTranslation: %s\nTTS: %s\nTotal: $%.2f",
		transcriptCost, translateCost, ttsCost, total)
}

func getOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// CreateRenderer implements fyne.Widget
func (p *SettingsPanel) CreateRenderer() fyne.WidgetRenderer {
	content := p.Build()
	return widget.NewSimpleRenderer(content)
}
