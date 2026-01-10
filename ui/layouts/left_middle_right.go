package layouts

import (
	"fyne.io/fyne/v2"
)

// LeftMiddleRightLayout creates a three-column layout
// Left and right have fixed widths, middle is flexible
type LeftMiddleRightLayout struct {
	LeftWidth  float32
	RightWidth float32
	Padding    float32
}

// NewLeftMiddleRightLayout creates a new three-column layout
func NewLeftMiddleRightLayout(leftWidth, rightWidth, padding float32) *LeftMiddleRightLayout {
	return &LeftMiddleRightLayout{
		LeftWidth:  leftWidth,
		RightWidth: rightWidth,
		Padding:    padding,
	}
}

// Layout arranges the objects: [0] = left, [1] = middle, [2] = right
func (l *LeftMiddleRightLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 3 {
		return
	}

	left := objects[0]
	middle := objects[1]
	right := objects[2]

	// Calculate middle width
	middleWidth := size.Width - l.LeftWidth - l.RightWidth - (l.Padding * 4)
	if middleWidth < 0 {
		middleWidth = 0
	}

	// Position left section
	left.Resize(fyne.NewSize(l.LeftWidth, size.Height))
	left.Move(fyne.NewPos(l.Padding, 0))

	// Position middle section
	middle.Resize(fyne.NewSize(middleWidth, size.Height))
	middle.Move(fyne.NewPos(l.LeftWidth+l.Padding*2, 0))

	// Position right section
	right.Resize(fyne.NewSize(l.RightWidth, size.Height))
	right.Move(fyne.NewPos(size.Width-l.RightWidth-l.Padding, 0))
}

// MinSize returns the minimum size needed for the layout
func (l *LeftMiddleRightLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 3 {
		return fyne.NewSize(0, 0)
	}

	left := objects[0]
	middle := objects[1]
	right := objects[2]

	leftMin := left.MinSize()
	middleMin := middle.MinSize()
	rightMin := right.MinSize()

	minHeight := fyne.Max(leftMin.Height, fyne.Max(middleMin.Height, rightMin.Height))
	minWidth := l.LeftWidth + middleMin.Width + l.RightWidth + (l.Padding * 4)

	return fyne.NewSize(minWidth, minHeight)
}

// ContentWithBottomBar creates a layout with main content and a fixed-height bottom bar
type ContentWithBottomBar struct {
	BottomHeight float32
}

// NewContentWithBottomBar creates a layout with content area and fixed bottom bar
func NewContentWithBottomBar(bottomHeight float32) *ContentWithBottomBar {
	return &ContentWithBottomBar{
		BottomHeight: bottomHeight,
	}
}

// Layout arranges: [0] = content, [1] = bottom bar
func (l *ContentWithBottomBar) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}

	content := objects[0]
	bottom := objects[1]

	contentHeight := size.Height - l.BottomHeight
	if contentHeight < 0 {
		contentHeight = 0
	}

	// Content fills top area
	content.Resize(fyne.NewSize(size.Width, contentHeight))
	content.Move(fyne.NewPos(0, 0))

	// Bottom bar at the bottom
	bottom.Resize(fyne.NewSize(size.Width, l.BottomHeight))
	bottom.Move(fyne.NewPos(0, contentHeight))
}

// MinSize returns the minimum size
func (l *ContentWithBottomBar) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 2 {
		return fyne.NewSize(0, l.BottomHeight)
	}

	content := objects[0]
	bottom := objects[1]

	contentMin := content.MinSize()
	bottomMin := bottom.MinSize()

	return fyne.NewSize(
		fyne.Max(contentMin.Width, bottomMin.Width),
		contentMin.Height+l.BottomHeight,
	)
}
