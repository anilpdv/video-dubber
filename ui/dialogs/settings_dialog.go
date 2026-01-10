package dialogs

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"video-translator/models"
)

// SettingsDialog displays and manages application settings
type SettingsDialog struct {
	window fyne.Window
	config *models.Config

	// UI elements
	outputDirEntry          *widget.Entry
	transcriptionSelect     *widget.Select
	translationSelect       *widget.Select
	ttsSelect               *widget.Select
	fasterWhisperModelSelect *widget.Select
	fasterWhisperDeviceSelect *widget.Select
	openaiTTSModelSelect    *widget.Select
	cosyVoiceModeSelect     *widget.Select
	cosyVoiceAPIURLEntry    *widget.Entry
	voiceSamplePathEntry    *widget.Entry
	openAIKeyEntry          *widget.Entry
	deepSeekKeyEntry        *widget.Entry
	groqAPIKeyEntry         *widget.Entry

	// Audio mixing controls
	keepBackgroundAudioCheck *widget.Check
	backgroundVolumeSlider   *widget.Slider
	backgroundVolumeLabel    *widget.Label

	// Conditional containers
	fasterWhisperSettings *fyne.Container
	openaiTTSSettings     *fyne.Container
	cosyVoiceSettings     *fyne.Container

	OnSave    func(config *models.Config)
	OnTTSChanged func(provider string)
}

// NewSettingsDialog creates a new settings dialog
func NewSettingsDialog(window fyne.Window, config *models.Config) *SettingsDialog {
	return &SettingsDialog{
		window: window,
		config: config,
	}
}

// Show displays the settings dialog
func (d *SettingsDialog) Show() {
	content := d.build()

	// Make dialog scrollable
	scrollContent := container.NewVScroll(content)
	scrollContent.SetMinSize(fyne.NewSize(480, 520))

	dialog.ShowCustomConfirm("Settings", "Save", "Cancel", scrollContent, func(save bool) {
		if save {
			d.saveSettings()
			if d.OnSave != nil {
				d.OnSave(d.config)
			}
		}
	}, d.window)
}

func (d *SettingsDialog) build() fyne.CanvasObject {
	// Output directory
	d.outputDirEntry = widget.NewEntry()
	d.outputDirEntry.SetText(d.config.OutputDirectory)

	browseBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			d.outputDirEntry.SetText(uri.Path())
		}, d.window)
	})

	outputRow := container.NewBorder(nil, nil, nil, browseBtn, d.outputDirEntry)

	// Provider selections
	d.transcriptionSelect = widget.NewSelect([]string{
		"whisper-cpp",
		"faster-whisper",
		"openai",
		"groq",
	}, func(value string) {
		d.updateConditionalUI()
	})
	d.transcriptionSelect.SetSelected(getOrDefault(d.config.TranscriptionProvider, "whisper-cpp"))

	d.translationSelect = widget.NewSelect([]string{
		"argos",
		"openai",
		"deepseek",
	}, nil)
	d.translationSelect.SetSelected(getOrDefault(d.config.TranslationProvider, "argos"))

	d.ttsSelect = widget.NewSelect([]string{
		"edge-tts",
		"piper",
		"openai",
		"cosyvoice",
	}, func(value string) {
		d.updateConditionalUI()
		if d.OnTTSChanged != nil {
			d.OnTTSChanged(value)
		}
	})
	d.ttsSelect.SetSelected(getOrDefault(d.config.TTSProvider, "edge-tts"))

	// FasterWhisper settings
	d.fasterWhisperModelSelect = widget.NewSelect([]string{
		"tiny", "base", "small", "medium", "large-v2", "large-v3",
	}, nil)
	d.fasterWhisperModelSelect.SetSelected(getOrDefault(d.config.FasterWhisperModel, "base"))

	d.fasterWhisperDeviceSelect = widget.NewSelect([]string{
		"auto", "cuda", "cpu",
	}, nil)
	d.fasterWhisperDeviceSelect.SetSelected(getOrDefault(d.config.FasterWhisperDevice, "auto"))

	d.fasterWhisperSettings = container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabel("FasterWhisper Settings"),
		widget.NewForm(
			widget.NewFormItem("Model", d.fasterWhisperModelSelect),
			widget.NewFormItem("Device", d.fasterWhisperDeviceSelect),
		),
	)

	// OpenAI TTS settings
	d.openaiTTSModelSelect = widget.NewSelect([]string{
		"tts-1", "tts-1-hd",
	}, nil)
	d.openaiTTSModelSelect.SetSelected(getOrDefault(d.config.OpenAITTSModel, "tts-1"))

	d.openaiTTSSettings = container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabel("OpenAI TTS Settings"),
		widget.NewForm(
			widget.NewFormItem("Model", d.openaiTTSModelSelect),
		),
	)

	// CosyVoice settings
	d.cosyVoiceModeSelect = widget.NewSelect([]string{
		"local", "api",
	}, nil)
	d.cosyVoiceModeSelect.SetSelected(getOrDefault(d.config.CosyVoiceMode, "local"))

	d.cosyVoiceAPIURLEntry = widget.NewEntry()
	d.cosyVoiceAPIURLEntry.SetPlaceHolder("http://localhost:8000")
	d.cosyVoiceAPIURLEntry.SetText(d.config.CosyVoiceAPIURL)

	d.voiceSamplePathEntry = widget.NewEntry()
	d.voiceSamplePathEntry.SetPlaceHolder("Path to voice sample (5-10s audio)")
	d.voiceSamplePathEntry.SetText(d.config.VoiceCloneSamplePath)

	voiceSampleBrowseBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			d.voiceSamplePathEntry.SetText(reader.URI().Path())
			reader.Close()
		}, d.window)
	})

	voiceSampleRow := container.NewBorder(nil, nil, nil, voiceSampleBrowseBtn, d.voiceSamplePathEntry)

	d.cosyVoiceSettings = container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabel("CosyVoice Settings"),
		widget.NewForm(
			widget.NewFormItem("Mode", d.cosyVoiceModeSelect),
			widget.NewFormItem("API URL", d.cosyVoiceAPIURLEntry),
			widget.NewFormItem("Voice Sample", voiceSampleRow),
		),
	)

	// API Keys
	d.openAIKeyEntry = widget.NewPasswordEntry()
	d.openAIKeyEntry.SetPlaceHolder("sk-...")
	d.openAIKeyEntry.SetText(d.config.OpenAIKey)

	d.deepSeekKeyEntry = widget.NewPasswordEntry()
	d.deepSeekKeyEntry.SetPlaceHolder("sk-...")
	d.deepSeekKeyEntry.SetText(d.config.DeepSeekKey)

	d.groqAPIKeyEntry = widget.NewPasswordEntry()
	d.groqAPIKeyEntry.SetPlaceHolder("gsk_...")
	d.groqAPIKeyEntry.SetText(d.config.GroqAPIKey)

	// Audio mixing controls
	d.keepBackgroundAudioCheck = widget.NewCheck("Keep background audio/music", nil)
	d.keepBackgroundAudioCheck.SetChecked(d.config.KeepBackgroundAudio)

	d.backgroundVolumeLabel = widget.NewLabel(fmt.Sprintf("Background volume: %.0f%%", d.config.BackgroundAudioVolume*100))
	d.backgroundVolumeSlider = widget.NewSlider(0, 100)
	d.backgroundVolumeSlider.Value = d.config.BackgroundAudioVolume * 100
	d.backgroundVolumeSlider.OnChanged = func(value float64) {
		d.backgroundVolumeLabel.SetText(fmt.Sprintf("Background volume: %.0f%%", value))
	}

	// Cost info
	costInfo := widget.NewLabel(d.getCostEstimate())
	costInfo.TextStyle = fyne.TextStyle{Italic: true}
	costInfo.Wrapping = fyne.TextWrapWord

	// Main form
	mainForm := widget.NewForm(
		widget.NewFormItem("Output Directory", outputRow),
	)

	providersForm := widget.NewForm(
		widget.NewFormItem("Transcription", d.transcriptionSelect),
		widget.NewFormItem("Translation", d.translationSelect),
		widget.NewFormItem("TTS", d.ttsSelect),
	)

	apiKeysForm := widget.NewForm(
		widget.NewFormItem("OpenAI API Key", d.openAIKeyEntry),
		widget.NewFormItem("DeepSeek API Key", d.deepSeekKeyEntry),
		widget.NewFormItem("Groq API Key", d.groqAPIKeyEntry),
	)

	// Audio mixing form
	audioMixingForm := container.NewVBox(
		d.keepBackgroundAudioCheck,
		d.backgroundVolumeLabel,
		d.backgroundVolumeSlider,
	)

	// Initialize conditional visibility
	d.updateConditionalUI()

	return container.NewVBox(
		widget.NewLabel("General"),
		mainForm,
		widget.NewSeparator(),
		widget.NewLabel("Providers"),
		providersForm,
		d.fasterWhisperSettings,
		d.openaiTTSSettings,
		d.cosyVoiceSettings,
		widget.NewSeparator(),
		widget.NewLabel("API Keys"),
		apiKeysForm,
		widget.NewSeparator(),
		widget.NewLabel("Audio Mixing"),
		audioMixingForm,
		widget.NewSeparator(),
		widget.NewLabel("Cost Estimate (per 5hr video)"),
		costInfo,
	)
}

func (d *SettingsDialog) updateConditionalUI() {
	// Guard against nil containers (called during build before containers are created)
	if d.fasterWhisperSettings == nil || d.openaiTTSSettings == nil || d.cosyVoiceSettings == nil {
		return
	}

	// FasterWhisper
	if d.transcriptionSelect.Selected == "faster-whisper" {
		d.fasterWhisperSettings.Show()
	} else {
		d.fasterWhisperSettings.Hide()
	}

	// OpenAI TTS
	if d.ttsSelect.Selected == "openai" {
		d.openaiTTSSettings.Show()
	} else {
		d.openaiTTSSettings.Hide()
	}

	// CosyVoice
	if d.ttsSelect.Selected == "cosyvoice" {
		d.cosyVoiceSettings.Show()
	} else {
		d.cosyVoiceSettings.Hide()
	}
}

func (d *SettingsDialog) saveSettings() {
	d.config.OutputDirectory = d.outputDirEntry.Text
	d.config.TranscriptionProvider = d.transcriptionSelect.Selected
	d.config.TranslationProvider = d.translationSelect.Selected
	d.config.TTSProvider = d.ttsSelect.Selected

	d.config.FasterWhisperModel = d.fasterWhisperModelSelect.Selected
	d.config.FasterWhisperDevice = d.fasterWhisperDeviceSelect.Selected

	d.config.OpenAITTSModel = d.openaiTTSModelSelect.Selected

	d.config.CosyVoiceMode = d.cosyVoiceModeSelect.Selected
	d.config.CosyVoiceAPIURL = d.cosyVoiceAPIURLEntry.Text
	d.config.VoiceCloneSamplePath = d.voiceSamplePathEntry.Text

	d.config.OpenAIKey = d.openAIKeyEntry.Text
	d.config.DeepSeekKey = d.deepSeekKeyEntry.Text
	d.config.GroqAPIKey = d.groqAPIKeyEntry.Text

	d.config.KeepBackgroundAudio = d.keepBackgroundAudioCheck.Checked
	d.config.BackgroundAudioVolume = d.backgroundVolumeSlider.Value / 100.0

	d.config.UseOpenAIAPIs = (d.config.TranscriptionProvider == "openai" || d.config.TranslationProvider == "openai")

	if err := d.config.Save(); err != nil {
		dialog.ShowError(err, d.window)
	}
}

func (d *SettingsDialog) getCostEstimate() string {
	transcription := getOrDefault(d.config.TranscriptionProvider, "whisper-cpp")
	translation := getOrDefault(d.config.TranslationProvider, "argos")
	tts := getOrDefault(d.config.TTSProvider, "edge-tts")

	var transcriptCostNum, translateCostNum, ttsCostNum float64
	var transcriptCost, translateCost, ttsCost string

	switch transcription {
	case "openai":
		transcriptCostNum = 1.80
		transcriptCost = "$1.80"
	case "groq":
		transcriptCostNum = 0.15
		transcriptCost = "~$0.15 (ultra-fast)"
	case "faster-whisper":
		transcriptCostNum = 0
		transcriptCost = "Free (local GPU)"
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
		ttsModel := getOrDefault(d.config.OpenAITTSModel, "tts-1")
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

// ShowDependencyCheck shows the dependency check dialog
func ShowDependencyCheck(window fyne.Window, results map[string]error) {
	var status string
	allGood := true

	order := []string{"ffmpeg", "whisper", "whisper-model", "argos-translate", "argos-ru-en", "piper-tts", "piper-voice"}

	for _, name := range order {
		if err, ok := results[name]; ok {
			if err != nil {
				status += "  " + name + ": " + err.Error() + "\n"
				allGood = false
			} else {
				status += "  " + name + ": OK\n"
			}
		}
	}

	if allGood {
		status += "\nAll dependencies are installed!"
	} else {
		status += "\nPlease install missing dependencies."
	}

	dialog.ShowInformation("Dependency Check", status, window)
}
