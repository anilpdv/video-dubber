package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type SettingsPanel struct {
	sourceLangSelect *widget.Select
	targetLangSelect *widget.Select
	voiceSelect      *widget.Select
	outputDirEntry   *widget.Entry

	onTranslateSelected func()
	onTranslateAll      func()
}

var supportedSourceLangs = []string{"Russian (ru)", "Spanish (es)", "French (fr)", "German (de)", "Chinese (zh)"}
var supportedTargetLangs = []string{"English (en)", "Spanish (es)", "French (fr)", "German (de)"}

// Piper TTS voices (free, local)
var supportedVoices = []string{
	"en_US-amy-medium",
	"en_US-ryan-medium",
	"en_GB-alba-medium",
	"de_DE-thorsten-medium",
	"fr_FR-upmc-medium",
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

	sp.voiceSelect = widget.NewSelect(supportedVoices, nil)
	sp.voiceSelect.SetSelected("en_US-amy-medium")

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
