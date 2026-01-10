package widgets

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"video-translator/models"
	appTheme "video-translator/ui/theme"
)

// Stage represents a pipeline stage
type Stage struct {
	ID    string
	Label string
}

// DefaultStages returns the translation pipeline stages
func DefaultStages() []Stage {
	return []Stage{
		{ID: "extract", Label: "Extract"},
		{ID: "transcribe", Label: "Transcribe"},
		{ID: "translate", Label: "Translate"},
		{ID: "synthesize", Label: "Synthesize"},
		{ID: "mux", Label: "Mux"},
	}
}

// StageProgress shows pipeline progress visually
type StageProgress struct {
	widget.BaseWidget

	Stages       []Stage
	CurrentStage string
	Progress     int // 0-100 progress within current stage
	Status       models.JobStatus
}

// NewStageProgress creates a new stage progress widget
func NewStageProgress() *StageProgress {
	p := &StageProgress{
		Stages:       DefaultStages(),
		CurrentStage: "",
		Progress:     0,
		Status:       models.StatusPending,
	}
	p.ExtendBaseWidget(p)
	return p
}

// SetStage updates the current stage and progress
func (p *StageProgress) SetStage(stage string, progress int) {
	// Map pipeline stage names to UI stage IDs
	stageMap := map[string]string{
		"Extracting":   "extract",
		"Transcribing": "transcribe",
		"Translating":  "translate",
		"Synthesizing": "synthesize",
		"Muxing":       "mux",
		"Complete":     "complete",
	}

	if mappedStage, ok := stageMap[stage]; ok {
		p.CurrentStage = mappedStage
	} else {
		p.CurrentStage = strings.ToLower(stage)
	}
	p.Progress = progress
	p.Refresh()
}

// SetStatus updates the job status
func (p *StageProgress) SetStatus(status models.JobStatus) {
	p.Status = status

	// Map status to stage
	switch status {
	case models.StatusExtracting:
		p.CurrentStage = "extract"
	case models.StatusTranscribing:
		p.CurrentStage = "transcribe"
	case models.StatusTranslating:
		p.CurrentStage = "translate"
	case models.StatusSynthesizing:
		p.CurrentStage = "synthesize"
	case models.StatusMuxing:
		p.CurrentStage = "mux"
	case models.StatusCompleted:
		p.CurrentStage = "complete"
	case models.StatusPending:
		p.CurrentStage = ""
	}

	p.Refresh()
}

// CreateRenderer implements fyne.Widget
func (p *StageProgress) CreateRenderer() fyne.WidgetRenderer {
	stageDots := make([]*canvas.Circle, len(p.Stages))
	stageLabels := make([]*canvas.Text, len(p.Stages))
	connectors := make([]*canvas.Rectangle, len(p.Stages)-1)

	for i := range p.Stages {
		stageDots[i] = canvas.NewCircle(color.Transparent)
		stageLabels[i] = canvas.NewText(p.Stages[i].Label, color.White)
		stageLabels[i].TextSize = 11
		stageLabels[i].Alignment = fyne.TextAlignCenter

		if i < len(p.Stages)-1 {
			connectors[i] = canvas.NewRectangle(color.Transparent)
		}
	}

	// Overall progress bar
	progressBg := canvas.NewRectangle(color.NRGBA{R: 50, G: 50, B: 50, A: 255})
	progressBg.CornerRadius = 3
	progressFill := canvas.NewRectangle(color.Transparent)
	progressFill.CornerRadius = 3

	progressLabel := canvas.NewText("0%", color.White)
	progressLabel.TextSize = 12

	return &stageProgressRenderer{
		stageDots:     stageDots,
		stageLabels:   stageLabels,
		connectors:    connectors,
		progressBg:    progressBg,
		progressFill:  progressFill,
		progressLabel: progressLabel,
		widget:        p,
	}
}

type stageProgressRenderer struct {
	stageDots     []*canvas.Circle
	stageLabels   []*canvas.Text
	connectors    []*canvas.Rectangle
	progressBg    *canvas.Rectangle
	progressFill  *canvas.Rectangle
	progressLabel *canvas.Text
	widget        *StageProgress
}

func (r *stageProgressRenderer) Destroy() {}

func (r *stageProgressRenderer) Layout(size fyne.Size) {
	padding := float32(16)
	dotSize := float32(10)
	connectorHeight := float32(2)

	// Calculate spacing for stages
	numStages := len(r.widget.Stages)
	if numStages == 0 {
		return
	}

	// Calculate total width needed for labels to determine spacing
	availableWidth := size.Width - padding*2
	spacing := availableWidth / float32(numStages)

	y := padding

	// Layout stage dots and connectors
	for i := range r.widget.Stages {
		// Center each stage in its allocated space
		centerX := padding + float32(i)*spacing + spacing/2

		// Connector (before dot, except first)
		if i > 0 {
			prevCenterX := padding + float32(i-1)*spacing + spacing/2
			connX := prevCenterX + dotSize/2
			connWidth := centerX - prevCenterX - dotSize
			if connWidth > 0 {
				r.connectors[i-1].Resize(fyne.NewSize(connWidth, connectorHeight))
				r.connectors[i-1].Move(fyne.NewPos(connX, y+(dotSize-connectorHeight)/2))
			}
		}

		// Dot - centered in the stage area
		r.stageDots[i].Resize(fyne.NewSize(dotSize, dotSize))
		r.stageDots[i].Move(fyne.NewPos(centerX-dotSize/2, y))

		// Label below dot - centered
		labelSize := r.stageLabels[i].MinSize()
		labelX := centerX - labelSize.Width/2
		// Clamp label X to stay within bounds
		if labelX < padding {
			labelX = padding
		}
		if labelX+labelSize.Width > size.Width-padding {
			labelX = size.Width - padding - labelSize.Width
		}
		r.stageLabels[i].Move(fyne.NewPos(labelX, y+dotSize+6))
	}

	// Progress bar below stages
	labelHeight := r.stageLabels[0].MinSize().Height
	progressY := y + dotSize + 8 + labelHeight + 12
	progressHeight := float32(4)

	r.progressBg.Resize(fyne.NewSize(size.Width-padding*2, progressHeight))
	r.progressBg.Move(fyne.NewPos(padding, progressY))

	fillWidth := (size.Width - padding*2) * float32(r.widget.Progress) / 100
	if fillWidth < 0 {
		fillWidth = 0
	}
	r.progressFill.Resize(fyne.NewSize(fillWidth, progressHeight))
	r.progressFill.Move(fyne.NewPos(padding, progressY))

	// Progress label - right aligned
	r.progressLabel.Move(fyne.NewPos(size.Width-padding-r.progressLabel.MinSize().Width, progressY+progressHeight+6))
}

func (r *stageProgressRenderer) MinSize() fyne.Size {
	// Calculate proper height: padding + dot + gap + label + gap + progress + gap + label
	padding := float32(16)
	dotSize := float32(10)
	labelHeight := float32(14) // Approximate label height
	progressHeight := float32(4)

	height := padding + dotSize + 6 + labelHeight + 12 + progressHeight + 6 + labelHeight + padding
	return fyne.NewSize(350, height)
}

func (r *stageProgressRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0)

	// Add connectors first (behind dots)
	for _, conn := range r.connectors {
		objs = append(objs, conn)
	}

	// Add dots and labels
	for i := range r.widget.Stages {
		objs = append(objs, r.stageDots[i])
		objs = append(objs, r.stageLabels[i])
	}

	// Add progress bar
	objs = append(objs, r.progressBg, r.progressFill, r.progressLabel)

	return objs
}

func (r *stageProgressRenderer) Refresh() {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	currentIdx := -1
	for i, stage := range r.widget.Stages {
		if stage.ID == r.widget.CurrentStage {
			currentIdx = i
			break
		}
	}

	// Handle completed state
	if r.widget.Status == models.StatusCompleted {
		currentIdx = len(r.widget.Stages)
	}

	// Update stage visuals
	for i := range r.widget.Stages {
		var dotColor color.Color
		var labelColor color.Color

		if i < currentIdx {
			// Completed stage
			dotColor = th.Color(appTheme.ColorNameJobCompleted, variant)
			labelColor = th.Color(theme.ColorNameForeground, variant)
		} else if i == currentIdx {
			// Current stage
			dotColor = th.Color(appTheme.ColorNameJobProcessing, variant)
			labelColor = th.Color(theme.ColorNamePrimary, variant)
		} else {
			// Pending stage
			dotColor = th.Color(appTheme.ColorNameJobPending, variant)
			labelColor = th.Color(appTheme.ColorNameTextSecondary, variant)
		}

		r.stageDots[i].FillColor = dotColor
		r.stageDots[i].Refresh()
		r.stageLabels[i].Color = labelColor
		r.stageLabels[i].Refresh()

		// Update connector
		if i > 0 {
			if i <= currentIdx {
				r.connectors[i-1].FillColor = th.Color(appTheme.ColorNameJobCompleted, variant)
			} else {
				r.connectors[i-1].FillColor = th.Color(appTheme.ColorNameJobPending, variant)
			}
			r.connectors[i-1].Refresh()
		}
	}

	// Update progress bar
	if r.widget.Status == models.StatusCompleted {
		r.progressFill.FillColor = th.Color(appTheme.ColorNameJobCompleted, variant)
	} else if currentIdx >= 0 {
		r.progressFill.FillColor = th.Color(theme.ColorNamePrimary, variant)
	} else {
		r.progressFill.FillColor = color.Transparent
	}
	r.progressFill.Refresh()

	// Update progress label
	r.progressLabel.Text = formatProgress(r.widget.Progress)
	r.progressLabel.Color = th.Color(theme.ColorNameForeground, variant)
	r.progressLabel.Refresh()
}

func formatProgress(progress int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	return fmt.Sprintf("%d%%", progress)
}
