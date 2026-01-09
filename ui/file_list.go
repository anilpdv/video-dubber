package ui

import (
	"path/filepath"
	"video-translator/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

type FileList struct {
	files         []string
	jobs          []*models.TranslationJob // Reference to jobs for status display
	list          *widget.List
	selectedIndex int
	window        fyne.Window

	onFileAdded    func(path string)
	onFileRemoved  func(index int)
	onFileSelected func(index int)
}

func NewFileList(onAdded func(string), onRemoved func(int), onSelected func(int)) *FileList {
	fl := &FileList{
		files:         make([]string, 0),
		selectedIndex: -1,
		onFileAdded:   onAdded,
		onFileRemoved: onRemoved,
		onFileSelected: onSelected,
	}

	fl.list = widget.NewList(
		func() int { return len(fl.files) },
		func() fyne.CanvasObject {
			return container.NewVBox(
				container.NewHBox(
					widget.NewLabel("⏳"), // Status icon placeholder
					widget.NewLabel("filename.mp4"),
				),
				widget.NewLabel(""), // Output path placeholder (hidden when empty)
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			box := obj.(*fyne.Container)
			topRow := box.Objects[0].(*fyne.Container)
			outputLabel := box.Objects[1].(*widget.Label)
			iconLabel := topRow.Objects[0].(*widget.Label)
			nameLabel := topRow.Objects[1].(*widget.Label)

			// Get status icon from job if available
			statusIcon := "⏳"
			outputPath := ""
			if fl.jobs != nil && int(id) < len(fl.jobs) {
				job := fl.jobs[id]
				statusIcon = job.StatusIcon()
				// Show output path for completed jobs
				if job.Status == models.StatusCompleted && job.OutputPath != "" {
					outputPath = "→ " + job.OutputPath
				}
			}

			iconLabel.SetText(statusIcon)
			nameLabel.SetText(filepath.Base(fl.files[id]))
			outputLabel.SetText(outputPath)
		},
	)

	fl.list.OnSelected = func(id widget.ListItemID) {
		fl.selectedIndex = int(id)
		if fl.onFileSelected != nil {
			fl.onFileSelected(fl.selectedIndex)
		}
	}

	return fl
}

func (fl *FileList) SetWindow(w fyne.Window) {
	fl.window = w
}

func (fl *FileList) Build() fyne.CanvasObject {
	return fl.list
}

func (fl *FileList) BuildButtons() fyne.CanvasObject {
	addBtn := widget.NewButton("Add Files", fl.showFileDialog)
	addFolderBtn := widget.NewButton("Add Folder", fl.showFolderDialog)
	removeBtn := widget.NewButton("Remove", fl.removeSelected)

	return container.NewHBox(addBtn, addFolderBtn, removeBtn)
}

func (fl *FileList) showFileDialog() {
	if fl.window == nil {
		return
	}

	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()

		path := reader.URI().Path()
		fl.addFile(path)
	}, fl.window)

	// Filter for video files
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".mp4", ".mkv", ".avi", ".mov", ".webm"}))
	fd.Show()
}

func (fl *FileList) showFolderDialog() {
	if fl.window == nil {
		return
	}

	fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}

		// List all video files in folder
		items, err := uri.List()
		if err != nil {
			return
		}

		videoExts := map[string]bool{".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".webm": true}
		for _, item := range items {
			ext := filepath.Ext(item.Path())
			if videoExts[ext] {
				fl.addFile(item.Path())
			}
		}
	}, fl.window)

	fd.Show()
}

func (fl *FileList) addFile(path string) {
	// Check if already added
	for _, f := range fl.files {
		if f == path {
			return
		}
	}

	fl.files = append(fl.files, path)
	fl.list.Refresh()

	if fl.onFileAdded != nil {
		fl.onFileAdded(path)
	}
}

func (fl *FileList) removeSelected() {
	if fl.selectedIndex < 0 || fl.selectedIndex >= len(fl.files) {
		return
	}

	if fl.onFileRemoved != nil {
		fl.onFileRemoved(fl.selectedIndex)
	}

	fl.files = append(fl.files[:fl.selectedIndex], fl.files[fl.selectedIndex+1:]...)
	fl.selectedIndex = -1
	fl.list.Refresh()
}

func (fl *FileList) GetSelectedIndex() int {
	return fl.selectedIndex
}

func (fl *FileList) GetFiles() []string {
	return fl.files
}

func (fl *FileList) Refresh() {
	fl.list.Refresh()
}

// SetJobs sets the reference to jobs for status display
func (fl *FileList) SetJobs(jobs []*models.TranslationJob) {
	fl.jobs = jobs
}
