package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appTheme "video-translator/ui/theme"
)

// BottomControls is the fixed bottom bar with translation controls
type BottomControls struct {
	widget.BaseWidget

	OnTranslateSelected func()
	OnTranslateAll      func()
	OnSettings          func()
	OnPreviewVoice      func()

	sourceSelector *CompactLanguageSelector
	targetSelector *CompactLanguageSelector
	voiceSelector  *VoiceSelector
	ttsProvider    string

	translateBtn    *PrimaryButton
	translateAllBtn *widget.Button
}

// NewBottomControls creates the bottom control bar
func NewBottomControls() *BottomControls {
	c := &BottomControls{
		ttsProvider: "edge-tts",
	}
	c.ExtendBaseWidget(c)
	return c
}

// SetTTSProvider updates the TTS provider and voice options
func (c *BottomControls) SetTTSProvider(provider string) {
	c.ttsProvider = provider
	if c.voiceSelector != nil {
		c.voiceSelector.SetProvider(provider)
	}
}

// GetSourceLang returns the selected source language code
func (c *BottomControls) GetSourceLang() string {
	if c.sourceSelector != nil {
		return c.sourceSelector.GetSelected()
	}
	return "ru"
}

// GetTargetLang returns the selected target language code
func (c *BottomControls) GetTargetLang() string {
	if c.targetSelector != nil {
		return c.targetSelector.GetSelected()
	}
	return "en"
}

// GetVoice returns the selected voice ID
func (c *BottomControls) GetVoice() string {
	if c.voiceSelector != nil {
		return c.voiceSelector.GetSelected()
	}
	return "en-US-AriaNeural"
}

// GetTTSProvider returns the current TTS provider
func (c *BottomControls) GetTTSProvider() string {
	return c.ttsProvider
}

// SetVoiceOptions updates voice options for the given provider
func (c *BottomControls) SetVoiceOptions(provider string) {
	c.ttsProvider = provider
	if c.voiceSelector != nil {
		c.voiceSelector.SetProvider(provider)
	}
}

// createCard creates a card-like container with subtle background and centered content
func createCard(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(color.NRGBA{R: 40, G: 40, B: 40, A: 255})
	bg.CornerRadius = 8
	return container.NewStack(
		bg,
		container.NewCenter(container.NewPadded(content)),
	)
}

// Build creates the control bar UI
func (c *BottomControls) Build() fyne.CanvasObject {
	// Source language selector
	c.sourceSelector = NewCompactLanguageSelector(SourceLanguages(), nil)
	c.sourceSelector.SetSelected("ru")

	// Arrow icon
	arrow := canvas.NewText("→", color.White)
	arrow.TextSize = 16

	// Target language selector
	c.targetSelector = NewCompactLanguageSelector(TargetLanguages(), nil)
	c.targetSelector.SetSelected("en")

	// Voice selector with preview
	c.voiceSelector = NewVoiceSelector(c.ttsProvider, nil, func() {
		if c.OnPreviewVoice != nil {
			c.OnPreviewVoice()
		}
	})

	// === TRANSLATION SECTION ===
	fromLabel := widget.NewLabel("From:")
	fromLabel.Alignment = fyne.TextAlignTrailing
	toLabel := widget.NewLabel("To:")
	toLabel.Alignment = fyne.TextAlignTrailing

	translationContent := container.NewVBox(
		container.NewHBox(fromLabel, c.sourceSelector.Build()),
		container.NewHBox(toLabel, c.targetSelector.Build()),
	)
	translationCard := createCard(translationContent)

	// === VOICE SECTION ===
	voiceLabel := widget.NewLabel("Voice:")
	voiceContent := container.NewVBox(
		container.NewCenter(voiceLabel),
		c.voiceSelector.Build(),
	)
	voiceCard := createCard(voiceContent)

	// === ACTION SECTION ===
	// Translate button (primary action)
	c.translateBtn = NewPrimaryButton("Translate", theme.MediaPlayIcon(), func() {
		if c.OnTranslateSelected != nil {
			c.OnTranslateSelected()
		}
	})

	// Translate All button
	c.translateAllBtn = widget.NewButton("Translate All", func() {
		if c.OnTranslateAll != nil {
			c.OnTranslateAll()
		}
	})

	actionContent := container.NewVBox(
		c.translateBtn,
		c.translateAllBtn,
	)
	actionCard := createCard(actionContent)

	// === MAIN LAYOUT ===
	// Arrange cards horizontally with spacing
	content := container.NewHBox(
		layout.NewSpacer(),
		translationCard,
		voiceCard,
		layout.NewSpacer(),
		actionCard,
	)

	// Wrap with background and padding
	paddedContent := container.NewPadded(content)

	return container.NewStack(
		NewThemedRectangle(appTheme.ColorNameBottomPanel),
		paddedContent,
	)
}

// CreateRenderer implements fyne.Widget
func (c *BottomControls) CreateRenderer() fyne.WidgetRenderer {
	content := c.Build()
	return widget.NewSimpleRenderer(content)
}

// SetOnPreviewVoice sets the voice preview callback
func (c *BottomControls) SetOnPreviewVoice(callback func()) {
	c.OnPreviewVoice = callback
	if c.voiceSelector != nil && c.voiceSelector.OnPreview == nil {
		c.voiceSelector.OnPreview = callback
	}
}

// Spacer creates a flexible spacer for layouts
func LayoutSpacer() fyne.CanvasObject {
	return layout.NewSpacer()
}
