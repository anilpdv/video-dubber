package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// SectionHeader is a styled section title
type SectionHeader struct {
	widget.BaseWidget

	Title       string
	Subtitle    string
	ShowDivider bool
}

// NewSectionHeader creates a new section header
func NewSectionHeader(title string) *SectionHeader {
	h := &SectionHeader{
		Title:       title,
		ShowDivider: true,
	}
	h.ExtendBaseWidget(h)
	return h
}

// NewSectionHeaderWithSubtitle creates a section header with subtitle
func NewSectionHeaderWithSubtitle(title, subtitle string) *SectionHeader {
	h := &SectionHeader{
		Title:       title,
		Subtitle:    subtitle,
		ShowDivider: true,
	}
	h.ExtendBaseWidget(h)
	return h
}

// SetTitle updates the title
func (h *SectionHeader) SetTitle(title string) {
	h.Title = title
	h.Refresh()
}

// SetSubtitle updates the subtitle
func (h *SectionHeader) SetSubtitle(subtitle string) {
	h.Subtitle = subtitle
	h.Refresh()
}

// CreateRenderer implements fyne.Widget
func (h *SectionHeader) CreateRenderer() fyne.WidgetRenderer {
	title := canvas.NewText(h.Title, color.White)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 16

	subtitle := canvas.NewText(h.Subtitle, color.Gray{Y: 150})
	subtitle.TextSize = 12

	divider := canvas.NewRectangle(color.Transparent)
	divider.SetMinSize(fyne.NewSize(0, 1))

	return &sectionHeaderRenderer{
		title:    title,
		subtitle: subtitle,
		divider:  divider,
		widget:   h,
	}
}

type sectionHeaderRenderer struct {
	title    *canvas.Text
	subtitle *canvas.Text
	divider  *canvas.Rectangle
	widget   *SectionHeader
}

func (r *sectionHeaderRenderer) Destroy() {}

func (r *sectionHeaderRenderer) Layout(size fyne.Size) {
	padding := float32(10)
	y := padding

	// Title
	titleSize := r.title.MinSize()
	r.title.Move(fyne.NewPos(padding, y))
	y += titleSize.Height

	// Subtitle
	if r.widget.Subtitle != "" {
		y += 2 // small gap
		r.subtitle.Move(fyne.NewPos(padding, y))
		y += r.subtitle.MinSize().Height
	}

	// Divider
	if r.widget.ShowDivider {
		y += padding
		r.divider.Resize(fyne.NewSize(size.Width-padding*2, 1))
		r.divider.Move(fyne.NewPos(padding, y))
	}
}

func (r *sectionHeaderRenderer) MinSize() fyne.Size {
	padding := float32(10)
	height := padding + r.title.MinSize().Height

	if r.widget.Subtitle != "" {
		height += 2 + r.subtitle.MinSize().Height
	}

	if r.widget.ShowDivider {
		height += padding + 1
	}

	height += padding

	// Calculate width based on title width
	titleWidth := r.title.MinSize().Width + padding*2
	minWidth := fyne.Max(150, titleWidth)

	return fyne.NewSize(minWidth, height)
}

func (r *sectionHeaderRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.title}
	if r.widget.Subtitle != "" {
		objs = append(objs, r.subtitle)
	}
	if r.widget.ShowDivider {
		objs = append(objs, r.divider)
	}
	return objs
}

func (r *sectionHeaderRenderer) Refresh() {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	r.title.Text = r.widget.Title
	r.title.Color = th.Color(theme.ColorNameForeground, variant)
	r.title.Refresh()

	r.subtitle.Text = r.widget.Subtitle
	r.subtitle.Color = th.Color(theme.ColorNamePlaceHolder, variant)
	r.subtitle.Refresh()

	r.divider.FillColor = th.Color(theme.ColorNameSeparator, variant)
	r.divider.Refresh()
}

// Spacer creates a flexible spacer
func Spacer() fyne.CanvasObject {
	return &spacerWidget{}
}

type spacerWidget struct {
	widget.BaseWidget
}

func (s *spacerWidget) CreateRenderer() fyne.WidgetRenderer {
	return &spacerRenderer{}
}

func (s *spacerWidget) MinSize() fyne.Size {
	return fyne.NewSize(0, 0)
}

type spacerRenderer struct{}

func (r *spacerRenderer) Destroy()                          {}
func (r *spacerRenderer) Layout(_ fyne.Size)                {}
func (r *spacerRenderer) MinSize() fyne.Size                { return fyne.NewSize(0, 0) }
func (r *spacerRenderer) Objects() []fyne.CanvasObject      { return nil }
func (r *spacerRenderer) Refresh()                          {}
