package container

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"video-translator/models"
	"video-translator/ui/widgets"
)

// ProgressPanel displays translation progress
type ProgressPanel struct {
	widget.BaseWidget

	currentJob      *models.TranslationJob
	outputDirectory string

	header          *widgets.SectionHeader
	stageProgress   *widgets.StageProgress
	fileLabel       *canvas.Text
	statusLabel     *canvas.Text
	outputLabel     *canvas.Text
}

// NewProgressPanel creates a new progress panel
func NewProgressPanel() *ProgressPanel {
	p := &ProgressPanel{}
	p.ExtendBaseWidget(p)
	return p
}

// SetCurrentJob sets the currently processing job
func (p *ProgressPanel) SetCurrentJob(job *models.TranslationJob) {
	p.currentJob = job
	p.Refresh()
}

// SetOutputDirectory sets the output directory display
func (p *ProgressPanel) SetOutputDirectory(dir string) {
	p.outputDirectory = dir
	p.Refresh()
}

// SetProgress updates the progress display
func (p *ProgressPanel) SetProgress(stage string, percent int) {
	if p.stageProgress != nil {
		p.stageProgress.SetStage(stage, percent)
	}
}

// SetStatus updates the status text
func (p *ProgressPanel) SetStatus(status string) {
	if p.statusLabel != nil {
		p.statusLabel.Text = status
		p.statusLabel.Refresh()
	}
}

// Update refreshes the panel based on current job state
func (p *ProgressPanel) Update() {
	p.Refresh()
}

// Build creates the panel UI
func (p *ProgressPanel) Build() fyne.CanvasObject {
	p.header = widgets.NewSectionHeader("Translation Progress")

	// Current file
	p.fileLabel = canvas.NewText("No file selected", nil)
	p.fileLabel.TextSize = 14
	p.fileLabel.TextStyle = fyne.TextStyle{Bold: true}

	fileRow := container.NewHBox(
		widget.NewLabel("File:"),
		p.fileLabel,
	)

	// Stage progress
	p.stageProgress = widgets.NewStageProgress()

	// Status label
	p.statusLabel = canvas.NewText("Ready", nil)
	p.statusLabel.TextSize = 13

	statusRow := container.NewHBox(
		widget.NewLabel("Status:"),
		p.statusLabel,
	)

	// Output directory
	p.outputLabel = canvas.NewText("", nil)
	p.outputLabel.TextSize = 12

	outputRow := container.NewHBox(
		widget.NewLabel("Output:"),
		p.outputLabel,
	)

	content := container.NewVBox(
		fileRow,
		widget.NewSeparator(),
		p.stageProgress,
		widget.NewSeparator(),
		statusRow,
		outputRow,
	)

	// Initial color setup
	p.refreshColors()

	return container.NewBorder(
		p.header,
		nil,
		nil,
		nil,
		content,
	)
}

func (p *ProgressPanel) refreshColors() {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	if p.fileLabel != nil {
		p.fileLabel.Color = th.Color(theme.ColorNameForeground, variant)
	}
	if p.statusLabel != nil {
		p.statusLabel.Color = th.Color(theme.ColorNameForeground, variant)
	}
	if p.outputLabel != nil {
		p.outputLabel.Color = th.Color(theme.ColorNamePlaceHolder, variant)
	}
}

// Refresh updates the display
func (p *ProgressPanel) Refresh() {
	p.refreshColors()

	if p.currentJob != nil {
		if p.fileLabel != nil {
			p.fileLabel.Text = p.currentJob.FileName
			p.fileLabel.Refresh()
		}

		if p.stageProgress != nil {
			p.stageProgress.SetStatus(p.currentJob.Status)
			p.stageProgress.Progress = p.currentJob.Progress
			p.stageProgress.Refresh()
		}

		if p.statusLabel != nil {
			p.statusLabel.Text = p.currentJob.StatusText()
			p.statusLabel.Refresh()
		}

		if p.outputLabel != nil && p.currentJob.OutputPath != "" {
			p.outputLabel.Text = p.currentJob.OutputPath
			p.outputLabel.Refresh()
		}
	} else {
		if p.fileLabel != nil {
			p.fileLabel.Text = "No file selected"
			p.fileLabel.Refresh()
		}

		if p.stageProgress != nil {
			p.stageProgress.SetStatus(models.StatusPending)
			p.stageProgress.Progress = 0
			p.stageProgress.Refresh()
		}

		if p.statusLabel != nil {
			p.statusLabel.Text = "Ready"
			p.statusLabel.Refresh()
		}
	}

	if p.outputLabel != nil && p.outputDirectory != "" && (p.currentJob == nil || p.currentJob.OutputPath == "") {
		p.outputLabel.Text = fmt.Sprintf("Output to: %s", p.outputDirectory)
		p.outputLabel.Refresh()
	}
}

// CreateRenderer implements fyne.Widget
func (p *ProgressPanel) CreateRenderer() fyne.WidgetRenderer {
	content := p.Build()
	return widget.NewSimpleRenderer(content)
}
