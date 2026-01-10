package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// ThemedRectangle is a rectangle that uses theme colors
type ThemedRectangle struct {
	widget.BaseWidget

	ColorName    fyne.ThemeColorName
	CornerRadius float32
	StrokeColor  color.Color
	StrokeWidth  float32
	MinWidth     float32
	MinHeight    float32
}

// NewThemedRectangle creates a new themed rectangle
func NewThemedRectangle(colorName fyne.ThemeColorName) *ThemedRectangle {
	r := &ThemedRectangle{
		ColorName:    colorName,
		CornerRadius: 0,
	}
	r.ExtendBaseWidget(r)
	return r
}

// NewThemedRectangleWithRadius creates a themed rectangle with rounded corners
func NewThemedRectangleWithRadius(colorName fyne.ThemeColorName, radius float32) *ThemedRectangle {
	r := &ThemedRectangle{
		ColorName:    colorName,
		CornerRadius: radius,
	}
	r.ExtendBaseWidget(r)
	return r
}

// SetMinSize sets the minimum size for the rectangle
func (r *ThemedRectangle) SetMinSize(size fyne.Size) {
	r.MinWidth = size.Width
	r.MinHeight = size.Height
	r.Refresh()
}

// CreateRenderer implements fyne.Widget
func (r *ThemedRectangle) CreateRenderer() fyne.WidgetRenderer {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	rect := canvas.NewRectangle(th.Color(r.ColorName, variant))
	rect.CornerRadius = r.CornerRadius

	return &themedRectangleRenderer{
		rect:   rect,
		widget: r,
	}
}

type themedRectangleRenderer struct {
	rect   *canvas.Rectangle
	widget *ThemedRectangle
}

func (r *themedRectangleRenderer) Destroy() {}

func (r *themedRectangleRenderer) Layout(size fyne.Size) {
	r.rect.Resize(size)
}

func (r *themedRectangleRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.widget.MinWidth, r.widget.MinHeight)
}

func (r *themedRectangleRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.rect}
}

func (r *themedRectangleRenderer) Refresh() {
	fillColor := fyne.CurrentApp().Settings().Theme().Color(
		r.widget.ColorName,
		fyne.CurrentApp().Settings().ThemeVariant(),
	)
	r.rect.FillColor = fillColor
	r.rect.CornerRadius = r.widget.CornerRadius

	if r.widget.StrokeColor != nil {
		r.rect.StrokeColor = r.widget.StrokeColor
		r.rect.StrokeWidth = r.widget.StrokeWidth
	}

	r.rect.Refresh()
}

// Panel is a container with a themed background
type Panel struct {
	widget.BaseWidget

	Content      fyne.CanvasObject
	ColorName    fyne.ThemeColorName
	CornerRadius float32
	Padding      float32
}

// NewPanel creates a new panel with themed background
func NewPanel(colorName fyne.ThemeColorName, content fyne.CanvasObject) *Panel {
	p := &Panel{
		Content:      content,
		ColorName:    colorName,
		CornerRadius: 8,
		Padding:      10,
	}
	p.ExtendBaseWidget(p)
	return p
}

// CreateRenderer implements fyne.Widget
func (p *Panel) CreateRenderer() fyne.WidgetRenderer {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	bg := canvas.NewRectangle(th.Color(p.ColorName, variant))
	bg.CornerRadius = p.CornerRadius

	return &panelRenderer{
		bg:      bg,
		content: p.Content,
		widget:  p,
	}
}

type panelRenderer struct {
	bg      *canvas.Rectangle
	content fyne.CanvasObject
	widget  *Panel
}

func (r *panelRenderer) Destroy() {}

func (r *panelRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)

	padding := r.widget.Padding
	contentSize := fyne.NewSize(size.Width-padding*2, size.Height-padding*2)
	r.content.Resize(contentSize)
	r.content.Move(fyne.NewPos(padding, padding))
}

func (r *panelRenderer) MinSize() fyne.Size {
	contentMin := r.content.MinSize()
	padding := r.widget.Padding * 2
	return fyne.NewSize(contentMin.Width+padding, contentMin.Height+padding)
}

func (r *panelRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.content}
}

func (r *panelRenderer) Refresh() {
	fillColor := fyne.CurrentApp().Settings().Theme().Color(
		r.widget.ColorName,
		fyne.CurrentApp().Settings().ThemeVariant(),
	)
	r.bg.FillColor = fillColor
	r.bg.CornerRadius = r.widget.CornerRadius
	r.bg.Refresh()
	r.content.Refresh()
}
