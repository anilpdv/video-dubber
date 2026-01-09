package ui

import (
	"fmt"
	"video-translator/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type ProgressPanel struct {
	progressBar    *widget.ProgressBar
	statusLabel    *widget.Label
	stageLabel     *widget.Label
	fileLabel      *widget.Label
	outputDirLabel *widget.Label

	currentJob     *models.TranslationJob
	outputDir      string
}

func NewProgressPanel() *ProgressPanel {
	return &ProgressPanel{
		progressBar:    widget.NewProgressBar(),
		statusLabel:    widget.NewLabel("No file selected"),
		stageLabel:     widget.NewLabel(""),
		fileLabel:      widget.NewLabel(""),
		outputDirLabel: widget.NewLabel(""),
	}
}

func (p *ProgressPanel) Build() fyne.CanvasObject {
	p.progressBar.Min = 0
	p.progressBar.Max = 100

	return container.NewVBox(
		p.fileLabel,
		widget.NewSeparator(),
		container.NewVBox(
			widget.NewLabel("Status:"),
			p.statusLabel,
		),
		widget.NewSeparator(),
		container.NewVBox(
			widget.NewLabel("Stage:"),
			p.stageLabel,
		),
		widget.NewSeparator(),
		container.NewVBox(
			widget.NewLabel("Progress:"),
			p.progressBar,
		),
		widget.NewSeparator(),
		container.NewVBox(
			widget.NewLabel("Output folder:"),
			p.outputDirLabel,
		),
	)
}

// SetOutputDirectory sets the output directory to display
func (p *ProgressPanel) SetOutputDirectory(dir string) {
	p.outputDir = dir
	p.outputDirLabel.SetText(dir)
}

func (p *ProgressPanel) SetCurrentJob(job *models.TranslationJob) {
	p.currentJob = job
	p.Update()
}

func (p *ProgressPanel) Update() {
	if p.currentJob == nil {
		p.fileLabel.SetText("No file selected")
		p.statusLabel.SetText("-")
		p.stageLabel.SetText("-")
		p.progressBar.SetValue(0)
		return
	}

	p.fileLabel.SetText(fmt.Sprintf("File: %s", p.currentJob.FileName))
	p.statusLabel.SetText(fmt.Sprintf("%s %s", p.currentJob.StatusIcon(), p.currentJob.StatusText()))

	// Show error details in stage if job failed
	if p.currentJob.Status == models.StatusFailed && p.currentJob.Error != nil {
		p.stageLabel.SetText("Error: " + p.currentJob.Error.Error())
		p.progressBar.SetValue(0)
	} else {
		p.stageLabel.SetText(p.currentJob.CurrentStage)
		p.progressBar.SetValue(float64(p.currentJob.Progress))
	}
}

func (p *ProgressPanel) SetProgress(stage string, percent int) {
	if p.currentJob != nil {
		p.currentJob.CurrentStage = stage
		p.currentJob.Progress = percent
	}
	// Update UI on main Fyne thread
	fyne.Do(func() {
		p.stageLabel.SetText(stage)
		p.progressBar.SetValue(float64(percent))
	})
}

func (p *ProgressPanel) SetStatus(status string) {
	// Update UI on main Fyne thread
	fyne.Do(func() {
		p.statusLabel.SetText(status)
	})
}
