package widgets

import (
	"image/color"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"video-translator/models"
	appTheme "video-translator/ui/theme"
)

// FileCard displays a video file with status and info
type FileCard struct {
	widget.BaseWidget

	Job        *models.TranslationJob
	OnTapped   func()
	OnRemove   func()
	selected   bool
	hovered    bool
}

// NewFileCard creates a new file card
func NewFileCard(job *models.TranslationJob, onTapped func()) *FileCard {
	c := &FileCard{
		Job:      job,
		OnTapped: onTapped,
	}
	c.ExtendBaseWidget(c)
	return c
}

// SetSelected sets the selection state
func (c *FileCard) SetSelected(selected bool) {
	c.selected = selected
	c.Refresh()
}

// SetJob updates the job
func (c *FileCard) SetJob(job *models.TranslationJob) {
	c.Job = job
	c.Refresh()
}

// Tapped handles tap events
func (c *FileCard) Tapped(_ *fyne.PointEvent) {
	if c.OnTapped != nil {
		c.OnTapped()
	}
}

// TappedSecondary handles secondary tap
func (c *FileCard) TappedSecondary(_ *fyne.PointEvent) {}

// MouseIn handles mouse enter
func (c *FileCard) MouseIn(_ *desktop.MouseEvent) {
	c.hovered = true
	c.Refresh()
}

// MouseOut handles mouse exit
func (c *FileCard) MouseOut() {
	c.hovered = false
	c.Refresh()
}

// MouseMoved handles mouse movement
func (c *FileCard) MouseMoved(_ *desktop.MouseEvent) {}

// CreateRenderer implements fyne.Widget
func (c *FileCard) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 8

	// Selection border
	border := canvas.NewRectangle(color.Transparent)
	border.CornerRadius = 8
	border.StrokeWidth = 2

	// Video icon placeholder
	iconBg := canvas.NewRectangle(color.NRGBA{R: 60, G: 60, B: 60, A: 255})
	iconBg.CornerRadius = 4

	videoIcon := canvas.NewImageFromResource(theme.MediaVideoIcon())
	videoIcon.FillMode = canvas.ImageFillContain

	// File name
	fileName := canvas.NewText("", color.White)
	fileName.TextStyle = fyne.TextStyle{Bold: true}
	fileName.TextSize = 14

	// Status badge
	statusBadge := NewJobStatusBadge(models.StatusPending)

	// Progress bar (shown during processing)
	progressBg := canvas.NewRectangle(color.NRGBA{R: 50, G: 50, B: 50, A: 255})
	progressBg.CornerRadius = 2
	progressFill := canvas.NewRectangle(color.Transparent)
	progressFill.CornerRadius = 2

	// Output path (shown when completed)
	outputLabel := canvas.NewText("", color.Gray{Y: 150})
	outputLabel.TextSize = 11

	return &fileCardRenderer{
		bg:           bg,
		border:       border,
		iconBg:       iconBg,
		videoIcon:    videoIcon,
		fileName:     fileName,
		statusBadge:  statusBadge,
		progressBg:   progressBg,
		progressFill: progressFill,
		outputLabel:  outputLabel,
		widget:       c,
	}
}

type fileCardRenderer struct {
	bg           *canvas.Rectangle
	border       *canvas.Rectangle
	iconBg       *canvas.Rectangle
	videoIcon    *canvas.Image
	fileName     *canvas.Text
	statusBadge  *JobStatusBadge
	progressBg   *canvas.Rectangle
	progressFill *canvas.Rectangle
	outputLabel  *canvas.Text
	widget       *FileCard
}

func (r *fileCardRenderer) Destroy() {}

func (r *fileCardRenderer) Layout(size fyne.Size) {
	padding := float32(10)
	iconSize := float32(48)

	// Background and border
	r.bg.Resize(size)
	r.border.Resize(size)

	// Video icon
	r.iconBg.Resize(fyne.NewSize(iconSize, iconSize))
	r.iconBg.Move(fyne.NewPos(padding, (size.Height-iconSize)/2))

	iconPadding := float32(10)
	r.videoIcon.Resize(fyne.NewSize(iconSize-iconPadding*2, iconSize-iconPadding*2))
	r.videoIcon.Move(fyne.NewPos(padding+iconPadding, (size.Height-iconSize)/2+iconPadding))

	// Text content area
	contentX := padding + iconSize + padding
	contentWidth := size.Width - contentX - padding

	// File name
	r.fileName.Move(fyne.NewPos(contentX, padding))

	// Status badge
	badgeY := padding + r.fileName.MinSize().Height + 4
	r.statusBadge.Resize(r.statusBadge.MinSize())
	r.statusBadge.Move(fyne.NewPos(contentX, badgeY))

	// Progress bar (beside status for processing items)
	if isProcessing(r.widget.Job.Status) {
		progressY := badgeY + 2
		progressWidth := contentWidth - r.statusBadge.MinSize().Width - 20
		if progressWidth < 20 {
			progressWidth = 20 // Minimum progress bar width
		}
		r.progressBg.Resize(fyne.NewSize(progressWidth, 4))
		r.progressBg.Move(fyne.NewPos(contentX+r.statusBadge.MinSize().Width+10, progressY+4))

		fillWidth := progressWidth * float32(r.widget.Job.Progress) / 100
		if fillWidth < 0 {
			fillWidth = 0
		}
		r.progressFill.Resize(fyne.NewSize(fillWidth, 4))
		r.progressFill.Move(fyne.NewPos(contentX+r.statusBadge.MinSize().Width+10, progressY+4))
	}

	// Output path (for completed items)
	if r.widget.Job.Status == models.StatusCompleted && r.widget.Job.OutputPath != "" {
		outputY := badgeY + r.statusBadge.MinSize().Height + 2
		r.outputLabel.Move(fyne.NewPos(contentX, outputY))
	}
}

func (r *fileCardRenderer) MinSize() fyne.Size {
	return fyne.NewSize(200, 72)
}

func (r *fileCardRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{
		r.bg,
		r.border,
		r.iconBg,
		r.videoIcon,
		r.fileName,
		r.statusBadge,
	}

	if isProcessing(r.widget.Job.Status) {
		objs = append(objs, r.progressBg, r.progressFill)
	}

	if r.widget.Job.Status == models.StatusCompleted && r.widget.Job.OutputPath != "" {
		objs = append(objs, r.outputLabel)
	}

	return objs
}

func (r *fileCardRenderer) Refresh() {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	// Background color based on state
	if r.widget.selected {
		r.bg.FillColor = th.Color(appTheme.ColorNameSurfaceVariant, variant)
		r.border.StrokeColor = th.Color(theme.ColorNamePrimary, variant)
	} else if r.widget.hovered {
		r.bg.FillColor = th.Color(appTheme.ColorNameSurface, variant)
		r.border.StrokeColor = color.Transparent
	} else {
		r.bg.FillColor = color.Transparent
		r.border.StrokeColor = color.Transparent
	}

	// Update file name
	if r.widget.Job != nil {
		r.fileName.Text = r.widget.Job.FileName
		r.statusBadge.SetStatus(r.widget.Job.Status)

		// Progress fill
		if isProcessing(r.widget.Job.Status) {
			r.progressFill.FillColor = th.Color(theme.ColorNamePrimary, variant)
		}

		// Output path
		if r.widget.Job.OutputPath != "" {
			r.outputLabel.Text = truncatePath(r.widget.Job.OutputPath, 40)
		}
	}

	r.bg.Refresh()
	r.border.Refresh()
	r.fileName.Refresh()
	r.statusBadge.Refresh()
	r.progressBg.Refresh()
	r.progressFill.Refresh()
	r.outputLabel.Refresh()
}

func isProcessing(status models.JobStatus) bool {
	switch status {
	case models.StatusProcessing, models.StatusExtracting, models.StatusTranscribing,
		models.StatusTranslating, models.StatusSynthesizing, models.StatusMuxing:
		return true
	}
	return false
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	if len(base) >= maxLen-3 {
		return "..." + base[len(base)-(maxLen-3):]
	}

	availableForDir := maxLen - len(base) - 4 // 4 for "/..."
	if availableForDir > 0 && len(dir) > availableForDir {
		return "..." + dir[len(dir)-availableForDir:] + "/" + base
	}

	return "..." + path[len(path)-(maxLen-3):]
}
