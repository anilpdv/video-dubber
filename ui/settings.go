package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type SettingsPanel struct {
	sourceLangSelect *widget.Select
	targetLangSelect *widget.Select
	voiceSelect      *widget.Select
	previewBtn       *widget.Button
	outputDirEntry   *widget.Entry

	currentTTSProvider string // Track currently selected TTS provider

	onTranslateSelected func()
	onTranslateAll      func()
	onPreviewVoice      func()
}

var supportedSourceLangs = []string{"Russian (ru)", "Spanish (es)", "French (fr)", "German (de)", "Chinese (zh)"}
var supportedTargetLangs = []string{"English (en)", "Spanish (es)", "French (fr)", "German (de)"}

// Piper TTS voices (free, local)
var piperVoices = []string{
	"en_US-amy-medium",
	"en_US-ryan-medium",
	"en_GB-alba-medium",
	"de_DE-thorsten-medium",
	"fr_FR-upmc-medium",
}

// OpenAI TTS voices
var openaiVoices = []string{
	"alloy",
	"echo",
	"fable",
	"onyx",
	"nova",
	"shimmer",
}

// Edge TTS voices (FREE neural TTS from Microsoft)
var edgeTTSVoices = []string{
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

func NewSettingsPanel(onTranslateSelected, onTranslateAll func()) *SettingsPanel {
	sp := &SettingsPanel{
		onTranslateSelected: onTranslateSelected,
		onTranslateAll:      onTranslateAll,
	}

	sp.sourceLangSelect = widget.NewSelect(supportedSourceLangs, nil)
	sp.sourceLangSelect.SetSelected("Russian (ru)")

	sp.targetLangSelect = widget.NewSelect(supportedTargetLangs, nil)
	sp.targetLangSelect.SetSelected("English (en)")

	sp.voiceSelect = widget.NewSelect(piperVoices, nil)
	sp.voiceSelect.SetSelected("en_US-amy-medium")

	// Preview button with play icon
	sp.previewBtn = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
		if sp.onPreviewVoice != nil {
			sp.onPreviewVoice()
		}
	})

	sp.outputDirEntry = widget.NewEntry()
	sp.outputDirEntry.SetPlaceHolder("~/Desktop/Translated")

	return sp
}

func (sp *SettingsPanel) Build() fyne.CanvasObject {
	translateSelectedBtn := widget.NewButton("Translate Selected", func() {
		if sp.onTranslateSelected != nil {
			sp.onTranslateSelected()
		}
	})
	translateSelectedBtn.Importance = widget.HighImportance

	translateAllBtn := widget.NewButton("Translate All", func() {
		if sp.onTranslateAll != nil {
			sp.onTranslateAll()
		}
	})

	settingsRow := container.NewHBox(
		widget.NewLabel("Source:"),
		sp.sourceLangSelect,
		widget.NewLabel("Target:"),
		sp.targetLangSelect,
		widget.NewLabel("Voice:"),
		sp.voiceSelect,
		sp.previewBtn,
		layout.NewSpacer(),
		translateSelectedBtn,
		translateAllBtn,
	)

	return container.NewVBox(
		widget.NewSeparator(),
		container.NewPadded(settingsRow),
	)
}

func (sp *SettingsPanel) GetSourceLang() string {
	selected := sp.sourceLangSelect.Selected
	// Extract language code from "Russian (ru)" -> "ru"
	langMap := map[string]string{
		"Russian (ru)": "ru",
		"Spanish (es)": "es",
		"French (fr)":  "fr",
		"German (de)":  "de",
		"Chinese (zh)": "zh",
	}
	if code, ok := langMap[selected]; ok {
		return code
	}
	return "ru"
}

func (sp *SettingsPanel) GetTargetLang() string {
	selected := sp.targetLangSelect.Selected
	langMap := map[string]string{
		"English (en)": "en",
		"Spanish (es)": "es",
		"French (fr)":  "fr",
		"German (de)":  "de",
	}
	if code, ok := langMap[selected]; ok {
		return code
	}
	return "en"
}

func (sp *SettingsPanel) GetVoice() string {
	return sp.voiceSelect.Selected
}

// SetOnPreviewVoice sets the callback for voice preview button
func (sp *SettingsPanel) SetOnPreviewVoice(callback func()) {
	sp.onPreviewVoice = callback
}

// SetVoiceOptions updates the voice dropdown options based on TTS provider
func (sp *SettingsPanel) SetVoiceOptions(provider string) {
	sp.currentTTSProvider = provider // Track current provider for preview
	switch provider {
	case "openai":
		sp.voiceSelect.Options = openaiVoices
		sp.voiceSelect.SetSelected("nova")
	case "edge-tts":
		sp.voiceSelect.Options = edgeTTSVoices
		sp.voiceSelect.SetSelected("en-US-AriaNeural")
	case "cosyvoice":
		sp.voiceSelect.Options = []string{"(uses voice sample)"}
		sp.voiceSelect.SetSelected("(uses voice sample)")
	default: // piper
		sp.voiceSelect.Options = piperVoices
		sp.voiceSelect.SetSelected("en_US-amy-medium")
	}
	sp.voiceSelect.Refresh()
}

// GetTTSProvider returns the currently selected TTS provider
func (sp *SettingsPanel) GetTTSProvider() string {
	if sp.currentTTSProvider == "" {
		return "piper" // default
	}
	return sp.currentTTSProvider
}
