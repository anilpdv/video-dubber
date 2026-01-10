package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"video-translator/models"
	appTheme "video-translator/ui/theme"
)

// JobStatusBadge displays a job status with colored indicator
type JobStatusBadge struct {
	widget.BaseWidget

	Status    models.JobStatus
	ShowLabel bool
}

// NewJobStatusBadge creates a new status badge
func NewJobStatusBadge(status models.JobStatus) *JobStatusBadge {
	b := &JobStatusBadge{
		Status:    status,
		ShowLabel: true,
	}
	b.ExtendBaseWidget(b)
	return b
}

// NewJobStatusDot creates a status badge without label (dot only)
func NewJobStatusDot(status models.JobStatus) *JobStatusBadge {
	b := &JobStatusBadge{
		Status:    status,
		ShowLabel: false,
	}
	b.ExtendBaseWidget(b)
	return b
}

// SetStatus updates the status
func (b *JobStatusBadge) SetStatus(status models.JobStatus) {
	b.Status = status
	b.Refresh()
}

// CreateRenderer implements fyne.Widget
func (b *JobStatusBadge) CreateRenderer() fyne.WidgetRenderer {
	dot := canvas.NewCircle(color.Transparent)
	label := canvas.NewText("", color.White)
	label.TextSize = 12

	return &jobStatusBadgeRenderer{
		dot:    dot,
		label:  label,
		widget: b,
	}
}

type jobStatusBadgeRenderer struct {
	dot    *canvas.Circle
	label  *canvas.Text
	widget *JobStatusBadge
}

func (r *jobStatusBadgeRenderer) Destroy() {}

func (r *jobStatusBadgeRenderer) Layout(size fyne.Size) {
	dotSize := float32(8)
	dotY := (size.Height - dotSize) / 2

	r.dot.Resize(fyne.NewSize(dotSize, dotSize))
	r.dot.Move(fyne.NewPos(4, dotY))

	if r.widget.ShowLabel {
		labelY := (size.Height - r.label.MinSize().Height) / 2
		r.label.Move(fyne.NewPos(dotSize+10, labelY))
	}
}

func (r *jobStatusBadgeRenderer) MinSize() fyne.Size {
	if r.widget.ShowLabel {
		labelSize := r.label.MinSize()
		return fyne.NewSize(8+10+labelSize.Width+4, fyne.Max(16, labelSize.Height))
	}
	return fyne.NewSize(16, 16)
}

func (r *jobStatusBadgeRenderer) Objects() []fyne.CanvasObject {
	if r.widget.ShowLabel {
		return []fyne.CanvasObject{r.dot, r.label}
	}
	return []fyne.CanvasObject{r.dot}
}

func (r *jobStatusBadgeRenderer) Refresh() {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	// Get color based on status
	var colorName fyne.ThemeColorName
	var statusText string

	switch r.widget.Status {
	case models.StatusPending:
		colorName = appTheme.ColorNameJobPending
		statusText = "Pending"
	case models.StatusProcessing, models.StatusExtracting, models.StatusTranscribing,
		models.StatusTranslating, models.StatusSynthesizing, models.StatusMuxing:
		colorName = appTheme.ColorNameJobProcessing
		statusText = getProcessingText(r.widget.Status)
	case models.StatusCompleted:
		colorName = appTheme.ColorNameJobCompleted
		statusText = "Completed"
	case models.StatusFailed:
		colorName = appTheme.ColorNameJobFailed
		statusText = "Failed"
	default:
		colorName = appTheme.ColorNameJobPending
		statusText = string(r.widget.Status)
	}

	r.dot.FillColor = th.Color(colorName, variant)
	r.dot.Refresh()

	r.label.Text = statusText
	r.label.Color = th.Color(theme.ColorNameForeground, variant)
	r.label.Refresh()
}

func getProcessingText(status models.JobStatus) string {
	switch status {
	case models.StatusProcessing:
		return "Starting..."
	case models.StatusExtracting:
		return "Extracting"
	case models.StatusTranscribing:
		return "Transcribing"
	case models.StatusTranslating:
		return "Translating"
	case models.StatusSynthesizing:
		return "Synthesizing"
	case models.StatusMuxing:
		return "Muxing"
	default:
		return "Processing"
	}
}

// StatusColor returns the appropriate color for a job status
func StatusColor(status models.JobStatus) fyne.ThemeColorName {
	switch status {
	case models.StatusPending:
		return appTheme.ColorNameJobPending
	case models.StatusProcessing, models.StatusExtracting, models.StatusTranscribing,
		models.StatusTranslating, models.StatusSynthesizing, models.StatusMuxing:
		return appTheme.ColorNameJobProcessing
	case models.StatusCompleted:
		return appTheme.ColorNameJobCompleted
	case models.StatusFailed:
		return appTheme.ColorNameJobFailed
	default:
		return appTheme.ColorNameJobPending
	}
}
