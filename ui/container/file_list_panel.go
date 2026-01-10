package container

import (
	"video-translator/models"
	"video-translator/ui/widgets"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// FileListPanel manages the video file list
type FileListPanel struct {
	widget.BaseWidget

	jobs        []*models.TranslationJob
	cards       []*widgets.FileCard
	selectedIdx int
	window      fyne.Window

	OnFileAdded    func(path string)
	OnFileRemoved  func(index int)
	OnFileSelected func(index int)

	content    *fyne.Container
	scrollable *container.Scroll
	header     *widgets.SectionHeader
	addBtn     *widget.Button
	addFolderBtn *widget.Button
	removeBtn  *widget.Button
}

// NewFileListPanel creates a new file list panel
func NewFileListPanel(window fyne.Window) *FileListPanel {
	p := &FileListPanel{
		window:      window,
		selectedIdx: -1,
		jobs:        make([]*models.TranslationJob, 0),
		cards:       make([]*widgets.FileCard, 0),
	}
	p.ExtendBaseWidget(p)
	return p
}

// SetJobs updates the job list
func (p *FileListPanel) SetJobs(jobs []*models.TranslationJob) {
	p.jobs = jobs
	p.rebuildCards()
	p.Refresh()
}

// GetSelectedIndex returns the selected job index
func (p *FileListPanel) GetSelectedIndex() int {
	return p.selectedIdx
}

// Refresh refreshes all cards
func (p *FileListPanel) Refresh() {
	for i, card := range p.cards {
		if i < len(p.jobs) {
			card.SetJob(p.jobs[i])
			card.SetSelected(i == p.selectedIdx)
		}
	}
	if p.content != nil {
		p.content.Refresh()
	}
}

func (p *FileListPanel) rebuildCards() {
	p.cards = make([]*widgets.FileCard, len(p.jobs))
	for i, job := range p.jobs {
		idx := i
		card := widgets.NewFileCard(job, func() {
			p.selectItem(idx)
		})
		card.SetSelected(i == p.selectedIdx)
		p.cards[i] = card
	}
	p.updateContent()
}

func (p *FileListPanel) selectItem(index int) {
	// Deselect previous
	if p.selectedIdx >= 0 && p.selectedIdx < len(p.cards) {
		p.cards[p.selectedIdx].SetSelected(false)
	}

	p.selectedIdx = index

	// Select new
	if p.selectedIdx >= 0 && p.selectedIdx < len(p.cards) {
		p.cards[p.selectedIdx].SetSelected(true)
	}

	if p.OnFileSelected != nil {
		p.OnFileSelected(index)
	}
}

func (p *FileListPanel) updateContent() {
	if p.content == nil {
		return
	}

	// Clear and rebuild
	p.content.RemoveAll()

	for _, card := range p.cards {
		p.content.Add(card)
	}

	if len(p.cards) == 0 {
		emptyLabel := widget.NewLabel("No videos added yet.\nClick 'Add Files' to get started.")
		emptyLabel.Alignment = fyne.TextAlignCenter
		p.content.Add(emptyLabel)
	}

	p.content.Refresh()
}

// Build creates the panel UI
func (p *FileListPanel) Build() fyne.CanvasObject {
	p.header = widgets.NewSectionHeader("Video Files")

	// File list content
	p.content = container.NewVBox()
	p.scrollable = container.NewVScroll(p.content)
	p.scrollable.SetMinSize(fyne.NewSize(250, 200))

	// Action buttons - toolbar style below header
	p.addBtn = widget.NewButtonWithIcon("Add Files", theme.ContentAddIcon(), func() {
		p.showFileDialog()
	})

	p.addFolderBtn = widget.NewButtonWithIcon("Add Folder", theme.FolderOpenIcon(), func() {
		p.showFolderDialog()
	})

	p.removeBtn = widget.NewButtonWithIcon("Remove", theme.ContentRemoveIcon(), func() {
		if p.selectedIdx >= 0 && p.OnFileRemoved != nil {
			p.OnFileRemoved(p.selectedIdx)
			p.selectedIdx = -1
		}
	})

	// Toolbar with buttons (padded for left margin)
	toolbar := container.NewPadded(container.NewHBox(
		p.addBtn,
		p.addFolderBtn,
		p.removeBtn,
	))

	// Header + toolbar at top
	topSection := container.NewVBox(
		p.header,
		toolbar,
	)

	return container.NewBorder(
		topSection,
		nil, // No bottom content
		nil,
		nil,
		p.scrollable,
	)
}

func (p *FileListPanel) showFileDialog() {
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()

		if p.OnFileAdded != nil {
			p.OnFileAdded(reader.URI().Path())
		}
	}, p.window)

	fd.SetFilter(storage.NewExtensionFileFilter([]string{".mp4", ".mkv", ".avi", ".mov", ".webm"}))
	fd.Show()
}

func (p *FileListPanel) showFolderDialog() {
	fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}

		items, err := uri.List()
		if err != nil {
			return
		}

		videoExts := map[string]bool{
			".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".webm": true,
		}

		for _, item := range items {
			ext := ""
			path := item.Path()
			if len(path) > 4 {
				ext = path[len(path)-4:]
			}
			if len(path) > 5 && path[len(path)-5:] == ".webm" {
				ext = ".webm"
			}

			if videoExts[ext] && p.OnFileAdded != nil {
				p.OnFileAdded(path)
			}
		}
	}, p.window)

	fd.Show()
}

// CreateRenderer implements fyne.Widget
func (p *FileListPanel) CreateRenderer() fyne.WidgetRenderer {
	content := p.Build()
	return widget.NewSimpleRenderer(content)
}
