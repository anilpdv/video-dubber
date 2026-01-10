package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// IconButton is a button that displays only an icon
type IconButton struct {
	widget.BaseWidget

	Icon      fyne.Resource
	OnTapped  func()
	IconSize  float32
	Padding   float32
	hovered   bool
	pressed   bool
	disabled  bool
}

// NewIconButton creates a new icon button
func NewIconButton(icon fyne.Resource, onTapped func()) *IconButton {
	b := &IconButton{
		Icon:     icon,
		OnTapped: onTapped,
		IconSize: 20,
		Padding:  8,
	}
	b.ExtendBaseWidget(b)
	return b
}

// SetDisabled sets the disabled state
func (b *IconButton) SetDisabled(disabled bool) {
	b.disabled = disabled
	b.Refresh()
}

// Tapped handles tap events
func (b *IconButton) Tapped(_ *fyne.PointEvent) {
	if b.disabled || b.OnTapped == nil {
		return
	}
	b.OnTapped()
}

// TappedSecondary handles secondary tap events
func (b *IconButton) TappedSecondary(_ *fyne.PointEvent) {}

// MouseIn handles mouse enter
func (b *IconButton) MouseIn(_ *desktop.MouseEvent) {
	b.hovered = true
	b.Refresh()
}

// MouseOut handles mouse exit
func (b *IconButton) MouseOut() {
	b.hovered = false
	b.pressed = false
	b.Refresh()
}

// MouseMoved handles mouse movement
func (b *IconButton) MouseMoved(_ *desktop.MouseEvent) {}

// MouseDown handles mouse down
func (b *IconButton) MouseDown(_ *desktop.MouseEvent) {
	b.pressed = true
	b.Refresh()
}

// MouseUp handles mouse up
func (b *IconButton) MouseUp(_ *desktop.MouseEvent) {
	b.pressed = false
	b.Refresh()
}

// CreateRenderer implements fyne.Widget
func (b *IconButton) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 6

	icon := canvas.NewImageFromResource(b.Icon)
	icon.FillMode = canvas.ImageFillContain

	return &iconButtonRenderer{
		bg:     bg,
		icon:   icon,
		widget: b,
	}
}

type iconButtonRenderer struct {
	bg     *canvas.Rectangle
	icon   *canvas.Image
	widget *IconButton
}

func (r *iconButtonRenderer) Destroy() {}

func (r *iconButtonRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)

	// Center the icon
	padding := r.widget.Padding
	iconSize := r.widget.IconSize
	iconX := (size.Width - iconSize) / 2
	iconY := (size.Height - iconSize) / 2
	r.icon.Resize(fyne.NewSize(iconSize, iconSize))
	r.icon.Move(fyne.NewPos(iconX, iconY))

	_ = padding // reserved for future use
}

func (r *iconButtonRenderer) MinSize() fyne.Size {
	size := r.widget.IconSize + r.widget.Padding*2
	return fyne.NewSize(size, size)
}

func (r *iconButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.icon}
}

func (r *iconButtonRenderer) Refresh() {
	// Update background based on state
	if r.widget.pressed {
		r.bg.FillColor = fyne.CurrentApp().Settings().Theme().Color(
			theme.ColorNamePressed, fyne.CurrentApp().Settings().ThemeVariant())
	} else if r.widget.hovered {
		r.bg.FillColor = fyne.CurrentApp().Settings().Theme().Color(
			theme.ColorNameHover, fyne.CurrentApp().Settings().ThemeVariant())
	} else {
		r.bg.FillColor = color.Transparent
	}

	// Update icon
	if r.widget.Icon != nil {
		r.icon.Resource = r.widget.Icon
	}

	r.bg.Refresh()
	r.icon.Refresh()
}

// PrimaryButton is a prominent action button with primary color
type PrimaryButton struct {
	widget.BaseWidget

	Text     string
	Icon     fyne.Resource
	OnTapped func()
	hovered  bool
	pressed  bool
	disabled bool
}

// NewPrimaryButton creates a new primary action button
func NewPrimaryButton(text string, icon fyne.Resource, onTapped func()) *PrimaryButton {
	b := &PrimaryButton{
		Text:     text,
		Icon:     icon,
		OnTapped: onTapped,
	}
	b.ExtendBaseWidget(b)
	return b
}

// SetDisabled sets the disabled state
func (b *PrimaryButton) SetDisabled(disabled bool) {
	b.disabled = disabled
	b.Refresh()
}

// Tapped handles tap events
func (b *PrimaryButton) Tapped(_ *fyne.PointEvent) {
	if b.disabled || b.OnTapped == nil {
		return
	}
	b.OnTapped()
}

// TappedSecondary handles secondary tap events
func (b *PrimaryButton) TappedSecondary(_ *fyne.PointEvent) {}

// MouseIn handles mouse enter
func (b *PrimaryButton) MouseIn(_ *desktop.MouseEvent) {
	b.hovered = true
	b.Refresh()
}

// MouseOut handles mouse exit
func (b *PrimaryButton) MouseOut() {
	b.hovered = false
	b.pressed = false
	b.Refresh()
}

// MouseMoved handles mouse movement
func (b *PrimaryButton) MouseMoved(_ *desktop.MouseEvent) {}

// MouseDown handles mouse down
func (b *PrimaryButton) MouseDown(_ *desktop.MouseEvent) {
	b.pressed = true
	b.Refresh()
}

// MouseUp handles mouse up
func (b *PrimaryButton) MouseUp(_ *desktop.MouseEvent) {
	b.pressed = false
	b.Refresh()
}

// CreateRenderer implements fyne.Widget
func (b *PrimaryButton) CreateRenderer() fyne.WidgetRenderer {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	bg := canvas.NewRectangle(th.Color(theme.ColorNamePrimary, variant))
	bg.CornerRadius = 8

	label := canvas.NewText(b.Text, color.White)
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.Alignment = fyne.TextAlignCenter

	var icon *canvas.Image
	if b.Icon != nil {
		icon = canvas.NewImageFromResource(b.Icon)
		icon.FillMode = canvas.ImageFillContain
	}

	return &primaryButtonRenderer{
		bg:     bg,
		label:  label,
		icon:   icon,
		widget: b,
	}
}

type primaryButtonRenderer struct {
	bg     *canvas.Rectangle
	label  *canvas.Text
	icon   *canvas.Image
	widget *PrimaryButton
}

func (r *primaryButtonRenderer) Destroy() {}

func (r *primaryButtonRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)

	padding := float32(16)
	iconSize := float32(18)

	if r.icon != nil {
		r.icon.Resize(fyne.NewSize(iconSize, iconSize))

		// Calculate positions with bounds checking
		totalWidth := iconSize + 8 + r.label.MinSize().Width
		startX := (size.Width - totalWidth) / 2

		// Ensure startX is not negative
		if startX < padding {
			startX = padding
		}

		r.icon.Move(fyne.NewPos(startX, (size.Height-iconSize)/2))

		labelX := startX + iconSize + 8
		availableLabelWidth := size.Width - labelX - padding
		if availableLabelWidth < r.label.MinSize().Width {
			availableLabelWidth = r.label.MinSize().Width
		}
		r.label.Resize(fyne.NewSize(availableLabelWidth, r.label.MinSize().Height))
		r.label.Move(fyne.NewPos(labelX, (size.Height-r.label.MinSize().Height)/2))
	} else {
		// Center text only
		r.label.Resize(r.label.MinSize())
		r.label.Move(fyne.NewPos(
			(size.Width-r.label.MinSize().Width)/2,
			(size.Height-r.label.MinSize().Height)/2,
		))
	}
}

func (r *primaryButtonRenderer) MinSize() fyne.Size {
	textMin := r.label.MinSize()
	width := textMin.Width + 48 // increased padding
	height := textMin.Height + 16

	if r.icon != nil {
		width += 30 // icon + spacing
	}

	return fyne.NewSize(width, fyne.Max(height, 40))
}

func (r *primaryButtonRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.bg, r.label}
	if r.icon != nil {
		objs = append(objs, r.icon)
	}
	return objs
}

func (r *primaryButtonRenderer) Refresh() {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	// Background color based on state
	if r.widget.disabled {
		r.bg.FillColor = th.Color(theme.ColorNameDisabledButton, variant)
		r.label.Color = th.Color(theme.ColorNameDisabled, variant)
	} else if r.widget.pressed {
		r.bg.FillColor = th.Color(theme.ColorNamePressed, variant)
		r.label.Color = th.Color(theme.ColorNameForeground, variant)
	} else if r.widget.hovered {
		// Slightly brighter primary
		primary := th.Color(theme.ColorNamePrimary, variant)
		if nrgba, ok := primary.(color.NRGBA); ok {
			nrgba.R = min(255, nrgba.R+20)
			nrgba.G = min(255, nrgba.G+20)
			nrgba.B = min(255, nrgba.B+20)
			r.bg.FillColor = nrgba
		} else {
			r.bg.FillColor = primary
		}
		r.label.Color = th.Color(theme.ColorNameForeground, variant)
	} else {
		r.bg.FillColor = th.Color(theme.ColorNamePrimary, variant)
		r.label.Color = th.Color(theme.ColorNameForeground, variant)
	}

	r.label.Text = r.widget.Text
	r.bg.Refresh()
	r.label.Refresh()
	if r.icon != nil {
		r.icon.Refresh()
	}
}
