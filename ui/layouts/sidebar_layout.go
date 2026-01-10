package layouts

import (
	"fyne.io/fyne/v2"
)

// SidebarLayout creates a layout with a fixed-width sidebar and flexible content area
type SidebarLayout struct {
	SidebarWidth float32
}

// NewSidebarLayout creates a new sidebar layout with the specified sidebar width
func NewSidebarLayout(sidebarWidth float32) *SidebarLayout {
	return &SidebarLayout{
		SidebarWidth: sidebarWidth,
	}
}

// Layout arranges the objects: [0] = sidebar, [1] = content
func (l *SidebarLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}

	sidebar := objects[0]
	content := objects[1]

	// Sidebar takes fixed width on the left
	sidebar.Resize(fyne.NewSize(l.SidebarWidth, size.Height))
	sidebar.Move(fyne.NewPos(0, 0))

	// Content fills the remaining space
	contentWidth := size.Width - l.SidebarWidth
	content.Resize(fyne.NewSize(contentWidth, size.Height))
	content.Move(fyne.NewPos(l.SidebarWidth, 0))
}

// MinSize returns the minimum size needed for the layout
func (l *SidebarLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 2 {
		return fyne.NewSize(l.SidebarWidth, 0)
	}

	sidebar := objects[0]
	content := objects[1]

	sidebarMin := sidebar.MinSize()
	contentMin := content.MinSize()

	return fyne.NewSize(
		l.SidebarWidth+contentMin.Width,
		fyne.Max(sidebarMin.Height, contentMin.Height),
	)
}
