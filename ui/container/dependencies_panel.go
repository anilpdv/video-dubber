package container

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"video-translator/ui/widgets"
)

// DependenciesPanel displays dependency check results as a page
type DependenciesPanel struct {
	widget.BaseWidget

	checkFunc   func() map[string]error
	resultsList *fyne.Container
}

// NewDependenciesPanel creates a new dependencies panel
func NewDependenciesPanel(checkFunc func() map[string]error) *DependenciesPanel {
	p := &DependenciesPanel{
		checkFunc: checkFunc,
	}
	p.ExtendBaseWidget(p)
	return p
}

// Build creates the panel UI
func (p *DependenciesPanel) Build() fyne.CanvasObject {
	header := widgets.NewSectionHeader("Dependencies")

	// Results list container
	p.resultsList = container.NewVBox()

	// Check button
	checkBtn := widget.NewButtonWithIcon("Check Dependencies", theme.ViewRefreshIcon(), func() {
		p.runCheck()
	})
	checkBtn.Importance = widget.HighImportance

	// Description
	desc := widget.NewLabel("Check if all required tools are installed for video translation.")
	desc.Wrapping = fyne.TextWrapWord

	// Initial check
	p.runCheck()

	content := container.NewVBox(
		desc,
		widget.NewSeparator(),
		p.resultsList,
		widget.NewSeparator(),
		container.NewHBox(checkBtn),
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

func (p *DependenciesPanel) runCheck() {
	if p.checkFunc == nil || p.resultsList == nil {
		return
	}

	results := p.checkFunc()
	p.resultsList.RemoveAll()

	order := []string{
		"ffmpeg",
		"whisperkit",
		"whisper-cpp",
		"whisper-model",
		"argos-translate",
		"argos-ru-en",
		"piper-tts",
		"piper-voice",
		"edge-tts",
	}

	// Friendly names for display
	names := map[string]string{
		"ffmpeg":          "FFmpeg",
		"whisperkit":      "WhisperKit (Apple Silicon)",
		"whisper-cpp":     "Whisper.cpp",
		"whisper-model":   "Whisper Model",
		"argos-translate": "Argos Translate",
		"argos-ru-en":     "Russian-English Model",
		"piper-tts":       "Piper TTS",
		"piper-voice":     "Piper Voice",
		"edge-tts":        "Edge TTS",
	}

	allGood := true
	for _, key := range order {
		err, ok := results[key]
		if !ok {
			continue
		}

		name := names[key]
		if name == "" {
			name = key
		}

		var statusIcon fyne.CanvasObject
		var statusText string
		var statusColor color.Color

		if err != nil {
			statusIcon = canvas.NewImageFromResource(theme.CancelIcon())
			statusText = err.Error()
			statusColor = color.NRGBA{R: 255, G: 100, B: 100, A: 255}
			allGood = false
		} else {
			statusIcon = canvas.NewImageFromResource(theme.ConfirmIcon())
			statusText = "OK"
			statusColor = color.NRGBA{R: 100, G: 255, B: 100, A: 255}
		}

		iconImage := statusIcon.(*canvas.Image)
		iconImage.FillMode = canvas.ImageFillContain
		iconImage.SetMinSize(fyne.NewSize(20, 20))

		nameLabel := widget.NewLabel(name)
		nameLabel.TextStyle = fyne.TextStyle{Bold: true}

		statusLabel := canvas.NewText(statusText, statusColor)
		statusLabel.TextSize = 12

		row := container.NewHBox(
			iconImage,
			nameLabel,
			widget.NewLabel("-"),
			container.NewStack(statusLabel),
		)

		p.resultsList.Add(row)
	}

	// Summary
	p.resultsList.Add(widget.NewSeparator())
	var summaryText string
	var summaryColor color.Color
	if allGood {
		summaryText = "All dependencies are installed!"
		summaryColor = color.NRGBA{R: 100, G: 255, B: 100, A: 255}
	} else {
		summaryText = "Some dependencies are missing. Please install them."
		summaryColor = color.NRGBA{R: 255, G: 200, B: 100, A: 255}
	}
	summary := canvas.NewText(summaryText, summaryColor)
	summary.TextStyle = fyne.TextStyle{Bold: true}
	p.resultsList.Add(container.NewCenter(summary))

	p.resultsList.Refresh()
}

// CreateRenderer implements fyne.Widget
func (p *DependenciesPanel) CreateRenderer() fyne.WidgetRenderer {
	content := p.Build()
	return widget.NewSimpleRenderer(content)
}
